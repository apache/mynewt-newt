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
	"sort"

	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/newt/resolve"
)

type DepEntry struct {
	PkgName string
	// Expressions that enable the dependency.
	DepExprs parse.ExprSet
	// Required APIs and their enabling expressions.
	ReqApiExprs parse.ExprMap
	// Satisfied APIs and their enabling expressions.
	ApiExprs parse.ExprMap
}

// Key=parent, Value=slice of children
// For normal dependency graph:  parent=depender, children=dependees.
// For reverse dependency graph: parent=dependee, children=dependers.
type DepGraph map[string][]DepEntry

type depEntrySorter struct {
	entries []DepEntry
}

func (s depEntrySorter) Len() int {
	return len(s.entries)
}
func (s depEntrySorter) Swap(i, j int) {
	s.entries[i], s.entries[j] = s.entries[j], s.entries[i]
}
func (s depEntrySorter) Less(i, j int) bool {
	return s.entries[i].PkgName < s.entries[j].PkgName
}
func SortDepEntries(entries []DepEntry) {
	sorter := depEntrySorter{entries}
	sort.Sort(sorter)
}

func depGraph(rs *resolve.ResolveSet) (DepGraph, error) {
	graph := DepGraph{}

	for _, parent := range rs.Rpkgs {
		pname := parent.Lpkg.FullName()
		graph[pname] = make([]DepEntry, 0, len(parent.Deps))

		for _, dep := range parent.Deps {
			child := dep.Rpkg

			graph[pname] = append(graph[pname], DepEntry{
				PkgName:     child.Lpkg.FullName(),
				DepExprs:    dep.Exprs,
				ReqApiExprs: dep.ApiExprMap,
				ApiExprs:    child.Apis,
			})
		}

		SortDepEntries(graph[pname])
	}

	return graph, nil
}

func revdepGraph(rs *resolve.ResolveSet) (DepGraph, error) {
	graph, err := depGraph(rs)
	if err != nil {
		return nil, err
	}

	rgraph := DepGraph{}
	for parent, entries := range graph {
		// Ensure parent is present in graph even if no one depends on it.
		if rgraph[parent] == nil {
			rgraph[parent] = []DepEntry{}
		}

		// Reverse the dependency relationship for each child and add to the
		// graph.
		for _, entry := range entries {
			rgraph[entry.PkgName] = append(rgraph[entry.PkgName], DepEntry{
				PkgName:     parent,
				DepExprs:    entry.DepExprs,
				ReqApiExprs: entry.ReqApiExprs,
				ApiExprs:    entry.ApiExprs,
			})
		}
	}

	for _, entries := range rgraph {
		SortDepEntries(entries)
	}

	return rgraph, nil
}

func depString(entry DepEntry) string {
	s := fmt.Sprintf("%s", entry.PkgName)

	type ApiPair struct {
		api   string
		exprs []*parse.Node
	}

	if len(entry.ReqApiExprs) > 0 {
		apis := make([]string, 0, len(entry.ReqApiExprs))
		for api, _ := range entry.ReqApiExprs {
			apis = append(apis, api)
		}
		sort.Strings(apis)

		for _, api := range apis {
			reqes := entry.ReqApiExprs[api]
			reqdis := reqes.Disjunction().String()

			apies := entry.ApiExprs[api]
			apidis := apies.Disjunction().String()

			s += "(api:" + api
			if reqdis != "" || apidis != "" {
				s += ",syscfg:" + reqdis
				if apidis != "" {
					s += ";" + apidis
				}
			}
			s += ")"
		}
	} else {
		dis := entry.DepExprs.Disjunction().String()
		if dis != "" {
			s += "(syscfg:" + dis + ")"
		}
	}

	return s
}

func DepGraphText(graph DepGraph) string {
	parents := make([]string, 0, len(graph))
	for pname, _ := range graph {
		parents = append(parents, pname)
	}
	sort.Strings(parents)

	buffer := bytes.NewBufferString("")

	fmt.Fprintf(buffer, "Dependency graph (depender --> [dependees]):")
	for _, pname := range parents {
		fmt.Fprintf(buffer, "\n    * %s --> [", pname)
		for i, child := range graph[pname] {
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
	parents := make([]string, 0, len(graph))
	for pname, _ := range graph {
		parents = append(parents, pname)
	}
	sort.Strings(parents)

	buffer := bytes.NewBufferString("")

	fmt.Fprintf(buffer, "Reverse dependency graph (dependee <-- [dependers]):")
	for _, pname := range parents {
		fmt.Fprintf(buffer, "\n    * %s <-- [", pname)
		for i, child := range graph[pname] {
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
		pname := p.Lpkg.FullName()
		if dg[pname] == nil {
			missing = append(missing, p)
		} else {
			newDg[pname] = dg[pname]
		}
	}

	return newDg, missing
}
