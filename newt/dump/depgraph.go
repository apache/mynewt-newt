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

package dump

import (
	"sort"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/parse"
)

type DepGraphEntry struct {
	PkgName     string              `json:"name"`
	DepExprs    []string            `json:"dep_exprs,omitempty"`
	ApiExprs    map[string][]string `json:"api_exprs,omitempty"`
	ReqApiExprs map[string][]string `json:"req_api_exprs,omitempty"`
}

type DepGraph map[string][]DepGraphEntry

func exprSetStrings(es parse.ExprSet) []string {
	ss := make([]string, 0, len(es))
	for s, _ := range es {
		ss = append(ss, s)
	}
	sort.Strings(ss)

	return ss
}

func exprMapStrings(em parse.ExprMap) map[string][]string {
	m := make(map[string][]string, len(em))
	for k, es := range em {
		m[k] = exprSetStrings(es)
	}

	return m
}

func newDepGraph(bdg builder.DepGraph) DepGraph {
	dg := make(DepGraph, len(bdg))

	for parent, children := range bdg {
		for _, child := range children {
			dg[parent] = append(dg[parent], DepGraphEntry{
				PkgName:     child.PkgName,
				DepExprs:    exprSetStrings(child.DepExprs),
				ApiExprs:    exprMapStrings(child.ApiExprs),
				ReqApiExprs: exprMapStrings(child.ReqApiExprs),
			})
		}
	}

	return dg
}
