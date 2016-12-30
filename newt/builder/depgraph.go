/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package builder

import (
	"bytes"
	"fmt"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

type DepGraph map[*pkg.LocalPackage][]*pkg.LocalPackage
type graphMap map[*pkg.LocalPackage]map[*pkg.LocalPackage]struct{}

func graphMapAdd(gm graphMap, p *pkg.LocalPackage, c *pkg.LocalPackage) {
	dstGraph := gm[p]
	if dstGraph == nil {
		dstGraph = map[*pkg.LocalPackage]struct{}{}
	}
	dstGraph[c] = struct{}{}

	gm[p] = dstGraph
}

func graphMapToDepGraph(gm graphMap) DepGraph {
	dg := DepGraph{}

	for parent, childMap := range gm {
		dg[parent] = []*pkg.LocalPackage{}
		for child, _ := range childMap {
			dg[parent] = append(dg[parent], child)
		}
		pkg.SortLclPkgs(dg[parent])
	}

	return dg
}

func (b *Builder) depGraph() (DepGraph, error) {
	graph := DepGraph{}

	proj := project.GetProject()
	for parent, _ := range b.PkgMap {
		graph[parent] = []*pkg.LocalPackage{}

		for _, dep := range parent.Deps() {
			child := proj.ResolveDependency(dep).(*pkg.LocalPackage)
			if child == nil {
				return nil, util.FmtNewtError(
					"cannot resolve package \"%s\"; depender=\"%s\"",
					dep.String(), parent.FullName())
			}

			graph[parent] = append(graph[parent], child)
		}

		pkg.SortLclPkgs(graph[parent])
	}

	return graph, nil
}

func (b *Builder) revdepGraph() (DepGraph, error) {
	graph, err := b.depGraph()
	if err != nil {
		return nil, err
	}

	revGm := graphMap{}
	for parent, children := range graph {
		for _, child := range children {
			graphMapAdd(revGm, child, parent)
		}
	}

	return graphMapToDepGraph(revGm), nil
}

func mergeDepGraphs(graphs ...DepGraph) DepGraph {
	gm := graphMap{}

	for _, graph := range graphs {
		for parent, children := range graph {
			if gm[parent] == nil {
				gm[parent] = map[*pkg.LocalPackage]struct{}{}
			}

			for _, child := range children {
				graphMapAdd(gm, parent, child)
			}
		}
	}

	dg := graphMapToDepGraph(gm)

	return dg
}

func joinedDepGraph(builders []*Builder) (DepGraph, error) {
	finalGraph := DepGraph{}

	for _, b := range builders {
		graph, err := b.depGraph()
		if err != nil {
			return nil, err
		}
		finalGraph = mergeDepGraphs(finalGraph, graph)
	}

	return finalGraph, nil
}

func joinedRevdepGraph(builders []*Builder) (DepGraph, error) {
	finalGraph := DepGraph{}

	for _, b := range builders {
		graph, err := b.revdepGraph()
		if err != nil {
			return nil, err
		}
		finalGraph = mergeDepGraphs(finalGraph, graph)
	}

	return finalGraph, nil
}

func DepGraphText(graph DepGraph) string {
	parents := make([]*pkg.LocalPackage, 0, len(graph))
	for lpkg, _ := range graph {
		parents = append(parents, lpkg)
	}
	parents = pkg.SortLclPkgs(parents)

	buffer := bytes.NewBufferString("")

	fmt.Fprintf(buffer, "Dependency graph (depender --> [dependees]):")
	for _, parent := range parents {
		children := graph[parent]
		fmt.Fprintf(buffer, "\n    * %s --> [", parent.FullName())
		for i, child := range children {
			if i != 0 {
				fmt.Fprintf(buffer, " ")
			}
			fmt.Fprintf(buffer, "%s", child.FullName())
		}
		fmt.Fprintf(buffer, "]")
	}

	return buffer.String()
}

func RevdepGraphText(graph DepGraph) string {
	parents := make([]*pkg.LocalPackage, 0, len(graph))
	for lpkg, _ := range graph {
		parents = append(parents, lpkg)
	}
	parents = pkg.SortLclPkgs(parents)

	buffer := bytes.NewBufferString("")

	fmt.Fprintf(buffer, "Reverse dependency graph (dependee <-- [dependers]):")
	for _, parent := range parents {
		children := graph[parent]
		fmt.Fprintf(buffer, "\n    * %s <-- [", parent.FullName())
		for i, child := range children {
			if i != 0 {
				fmt.Fprintf(buffer, " ")
			}
			fmt.Fprintf(buffer, "%s", child.FullName())
		}
		fmt.Fprintf(buffer, "]")
	}

	return buffer.String()
}

// Extracts a new dependency graph containing only the specified parents.
//
// @return DepGraph             Filtered dependency graph
//         []*pkg.LocalPackage  Specified packages that were not parents in
//                                  original graph.
func FilterDepGraph(dg DepGraph, parents []*pkg.LocalPackage) (
	DepGraph, []*pkg.LocalPackage) {

	newDg := DepGraph{}

	var missing []*pkg.LocalPackage
	for _, p := range parents {
		if dg[p] == nil {
			missing = append(missing, p)
		} else {
			newDg[p] = dg[p]
		}
	}

	return newDg, missing
}
