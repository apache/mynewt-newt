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

// deprepo: Package for resolving repo dependencies.
//
// Vocabulary:
//     * Dependee:  A repo that is depended on.
//     * Dependent: A repo that depends on others.

package deprepo

import (
	"fmt"
	"sort"
	"strings"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

const rootDependencyName = ""
const rootRepoName = "project.yml"

// Represents a top-level repo dependency (i.e., a repo specified in
// `project.yml`).
var rootDependent = RVPair{Name: rootDependencyName}

// Represents a repo-name,version pair.
type RVPair struct {
	Name string
	Ver  newtutil.RepoVersion
}

// A repo dependency graph.
// Key: A repo with dependencies.
// Value: The corresponding list of dependencies.
type DepGraph map[RVPair][]RVPair

// A single node in a repo reverse dependency graph.
type RevdepGraphNode struct {
	// The name of the dependent repo.
	Name string
	// The version of the dependent repo.
	DependentVer newtutil.RepoVersion
	// The version of the dependee repo that is required.
	DependeeVer newtutil.RepoVersion
}

// A repo reverse dependency graph.
// Key: A depended-on repo.
// Value: The corresponding list of dependencies.
type RevdepGraph map[string][]RevdepGraphNode

func repoNameString(repoName string) string {
	if repoName == rootDependencyName {
		return rootRepoName
	} else {
		return repoName
	}
}

func repoNameVerString(repoName string, ver newtutil.RepoVersion) string {
	if repoName == rootDependencyName || repoName == rootRepoName {
		return rootRepoName
	} else {
		return fmt.Sprintf("%s/%s", repoName, ver.String())
	}
}

func (rvp *RVPair) String() string {
	return repoNameVerString(rvp.Name, rvp.Ver)
}

func CompareRVPairs(a RVPair, b RVPair) int {
	x := strings.Compare(a.Name, b.Name)
	if x != 0 {
		return x
	}

	x = newtutil.CompareRepoVersions(a.Ver, b.Ver)
	if x != 0 {
		return x
	}

	return 0
}

func (dg DepGraph) String() string {
	lines := make([]string, 0, len(dg))

	for dependent, nodes := range dg {
		line := fmt.Sprintf("%s:", dependent.String())
		for _, node := range nodes {
			line += fmt.Sprintf(" (%s)", node.String())
		}

		lines = append(lines, line)
	}

	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func (rgn *RevdepGraphNode) String() string {
	return fmt.Sprintf("%s,%s", repoNameVerString(rgn.Name, rgn.DependentVer),
		rgn.DependeeVer.String())
}

func (rg RevdepGraph) String() string {
	lines := make([]string, 0, len(rg))

	for repoName, nodes := range rg {
		line := fmt.Sprintf("%s:", repoName)
		for _, node := range nodes {
			line += fmt.Sprintf(" (%s)", node.String())
		}

		lines = append(lines, line)
	}

	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// Adds all dependencies expressed by a single version of a repo.
//
// @param repoName              The name of the dependent repo.
// @param repoVer               The version of the dependent repo.
// @param reqMap                The dependency requirements of the specified
//                                  repo version.
func (dg DepGraph) AddRepoVer(repoName string, repoVer newtutil.RepoVersion,
	reqMap RequirementMap) error {

	dep := RVPair{
		Name: repoName,
		Ver:  repoVer,
	}

	if _, ok := dg[dep]; ok {
		return util.FmtNewtError(
			"Duplicate repo-version-pair in repo dependency graph: %s,%s",
			repoName, repoVer.String())
	}

	for depName, depReq := range reqMap {
		dg[dep] = append(dg[dep], RVPair{
			Name: depName,
			Ver:  depReq,
		})
	}

	return nil
}

// Adds a root dependency (i.e., required repo specified in `project.yml`).
func (dg DepGraph) AddRootDep(repoName string, ver newtutil.RepoVersion) error {
	rootDeps := dg[rootDependent]
	for _, d := range rootDeps {
		if d.Name == repoName {
			return util.FmtNewtError(
				"Duplicate root dependency repo dependency graph: %s",
				repoName)
		}
	}

	dg[rootDependent] = append(dg[rootDependent], RVPair{
		Name: repoName,
		Ver:  ver,
	})

	return nil
}

// Reverses a dependency graph, forming a reverse dependency graph.
//
// A normal dependency graph expresses the following relationship:
//     [dependent] => depended-on
//
// A reverse dependency graph expresses the following relationship:
//     [depended-on] => dependent
func (dg DepGraph) Reverse() RevdepGraph {
	rg := RevdepGraph{}

	for dependent, nodes := range dg {
		for _, node := range nodes {
			// Nothing depends on project.yml (""), so exclude it from the
			// result.
			if node.Name != "" {
				rg[node.Name] = append(rg[node.Name], RevdepGraphNode{
					Name:         dependent.Name,
					DependentVer: dependent.Ver,
					DependeeVer:  node.Ver,
				})
			}
		}
	}

	return rg
}
