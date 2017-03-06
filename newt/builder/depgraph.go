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

	"mynewt.apache.org/newt/newt/resolve"
)

type DepGraph map[*resolve.ResolvePackage][]*resolve.ResolveDep
type graphMap map[*resolve.ResolvePackage]map[*resolve.ResolveDep]struct{}

func graphMapAdd(gm graphMap, p *resolve.ResolvePackage, c *resolve.ResolveDep) {
	dstGraph := gm[p]
	if dstGraph == nil {
		dstGraph = map[*resolve.ResolveDep]struct{}{}
	}
	dstGraph[c] = struct{}{}

	gm[p] = dstGraph
}

func graphMapToDepGraph(gm graphMap) DepGraph {
	dg := DepGraph{}

	for parent, childMap := range gm {
		dg[parent] = []*resolve.ResolveDep{}
		for child, _ := range childMap {
			dg[parent] = append(dg[parent], child)
		}
		resolve.SortResolveDeps(dg[parent])
	}

	return dg
}

func depGraph(rs *resolve.ResolveSet) (DepGraph, error) {
	graph := DepGraph{}

	for _, parent := range rs.Rpkgs {
		graph[parent] = []*resolve.ResolveDep{}

		for _, dep := range parent.Deps {
			graph[parent] = append(graph[parent], dep)
		}

		resolve.SortResolveDeps(graph[parent])
	}

	return graph, nil
}

func revdepGraph(rs *resolve.ResolveSet) (DepGraph, error) {
	graph, err := depGraph(rs)
	if err != nil {
		return nil, err
	}

	revGm := graphMap{}
	for parent, children := range graph {
		for _, child := range children {
			rParent := child.Rpkg
			rChild := *child
			rChild.Rpkg = parent

			graphMapAdd(revGm, rParent, &rChild)
		}
	}

	return graphMapToDepGraph(revGm), nil
}

func depString(dep *resolve.ResolveDep) string {
	s := fmt.Sprintf("%s", dep.Rpkg.Lpkg.FullName())
	if dep.Api != "" {
		s += fmt.Sprintf("(api:%s)", dep.Api)
	}

	return s
}

func DepGraphText(graph DepGraph) string {
	parents := make([]*resolve.ResolvePackage, 0, len(graph))
	for lpkg, _ := range graph {
		parents = append(parents, lpkg)
	}
	parents = resolve.SortResolvePkgs(parents)

	buffer := bytes.NewBufferString("")

	fmt.Fprintf(buffer, "Dependency graph (depender --> [dependees]):")
	for _, parent := range parents {
		children := resolve.SortResolveDeps(graph[parent])
		fmt.Fprintf(buffer, "\n    * %s --> [", parent.Lpkg.FullName())
		for i, child := range children {
			if i != 0 {
				fmt.Fprintf(buffer, " ")
			}
			fmt.Fprintf(buffer, "%s", depString(child))
		}
		fmt.Fprintf(buffer, "]")
	}

	return buffer.String()
}

func RevdepGraphText(graph DepGraph) string {
	parents := make([]*resolve.ResolvePackage, 0, len(graph))
	for lpkg, _ := range graph {
		parents = append(parents, lpkg)
	}
	parents = resolve.SortResolvePkgs(parents)

	buffer := bytes.NewBufferString("")

	fmt.Fprintf(buffer, "Reverse dependency graph (dependee <-- [dependers]):")
	for _, parent := range parents {
		children := resolve.SortResolveDeps(graph[parent])
		fmt.Fprintf(buffer, "\n    * %s <-- [", parent.Lpkg.FullName())
		for i, child := range children {
			if i != 0 {
				fmt.Fprintf(buffer, " ")
			}
			fmt.Fprintf(buffer, "%s", depString(child))
		}
		fmt.Fprintf(buffer, "]")
	}

	return buffer.String()
}

// Extracts a new dependency graph containing only the specified parents.
//
// @param dg                    The source graph to filter.
// @param parents               The parent nodes to keep.
//
// @return DepGraph             Filtered dependency graph.
//         []*ResolvePackage    Specified packages that were not parents in
//                                  original graph.
func FilterDepGraph(dg DepGraph, parents []*resolve.ResolvePackage) (
	DepGraph, []*resolve.ResolvePackage) {

	newDg := DepGraph{}

	var missing []*resolve.ResolvePackage
	for _, p := range parents {
		if dg[p] == nil {
			missing = append(missing, p)
		} else {
			newDg[p] = dg[p]
		}
	}

	return newDg, missing
}
