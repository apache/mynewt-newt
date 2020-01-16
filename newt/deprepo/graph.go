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

// Describes a repo that depends on other repos.
type Dependent struct {
	Name string
	Ver  newtutil.RepoVersion
}

const rootDependencyName = ""
const rootRepoName = "project.yml"

// Represents a top-level repo dependency (i.e., a repo specified in
// `project.yml`).
var rootDependent = Dependent{Name: rootDependencyName}

// A single node in a repo dependency graph.
type DepGraphNode struct {
	// Name of depended-on repo.
	Name string
	// Expresses the versions of the repo that satisfy this dependency.
	VerReqs []newtutil.RepoVersionReq
}

// A repo dependency graph.
// Key: A repo with dependencies.
// Value: The corresponding list of dependencies.
type DepGraph map[Dependent][]DepGraphNode

// A single node in a repo reverse dependency graph.
type RevdepGraphNode struct {
	// The name of the dependent repo.
	Name string
	// The version of the dependent repo.
	Ver newtutil.RepoVersion
	// The dependent's version requirements that apply to the graph key.
	VerReqs []newtutil.RepoVersionReq
}

// A repo reverse dependency graph.
// Key: A depended-on repo.
// Value: The corresponding list of dependencies.
type RevdepGraph map[string][]RevdepGraphNode

func repoNameVerString(repoName string, ver newtutil.RepoVersion) string {
	if repoName == rootDependencyName {
		return rootRepoName
	} else {
		return fmt.Sprintf("%s-%s", repoName, ver.String())
	}
}

func (dep *Dependent) String() string {
	return repoNameVerString(dep.Name, dep.Ver)
}

func (dgn *DepGraphNode) String() string {
	return fmt.Sprintf("%s,%s", dgn.Name,
		newtutil.RepoVerReqsString(dgn.VerReqs))
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
	return fmt.Sprintf("%s,%s", repoNameVerString(rgn.Name, rgn.Ver),
		newtutil.RepoVerReqsString(rgn.VerReqs))
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

	dep := Dependent{
		Name: repoName,
		Ver:  repoVer,
	}

	if _, ok := dg[dep]; ok {
		return util.FmtNewtError(
			"Duplicate repo-version-pair in repo dependency graph: %s,%s",
			repoName, repoVer.String())
	}

	for depName, depReqs := range reqMap {
		dg[dep] = append(dg[dep], DepGraphNode{
			Name:    depName,
			VerReqs: depReqs,
		})
	}

	return nil
}

// Adds a root dependency (i.e., required repo specified in `project.yml`).
func (dg DepGraph) AddRootDep(repoName string,
	verReqs []newtutil.RepoVersionReq) error {

	rootDeps := dg[rootDependent]
	for _, d := range rootDeps {
		if d.Name == repoName {
			return util.FmtNewtError(
				"Duplicate root dependency repo dependency graph: %s",
				repoName)
		}
	}

	dg[rootDependent] = append(dg[rootDependent], DepGraphNode{
		Name:    repoName,
		VerReqs: verReqs,
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
			// Nothing depends on project.yml (""), so exclude it from the result.
			if node.Name != "" {
				rg[node.Name] = append(rg[node.Name], RevdepGraphNode{
					Name:    dependent.Name,
					Ver:     dependent.Ver,
					VerReqs: node.VerReqs,
				})
			}
		}
	}

	return rg
}

// Identifies repos which cannot satisfy all their dependents.  For example, if
// `project.yml` requires X1 and Y2, but Y2 requires X2, then X is a
// conflicting repo (no overlap in requirement sets).
//
// Returns the names of all repos that cannot be included in the build.  For
// example, if:
//     * X depends on Z 1.0.0
//     * Y depends on Z 2.0.0
// then Z is included in the returned slice because it can't satisfy all its
// dependents.  X and Y are *not* included (unless they also have depedents
// that cannot be satisfied).
func (dg DepGraph) conflictingRepos(vm VersionMap) []string {
	badRepoMap := map[string]struct{}{}

	// Create a reverse dependency graph.
	rg := dg.Reverse()

	for dependeeName, nodes := range rg {
		dependeeVer, ok := vm[dependeeName]
		if !ok {
			// This version was pruned from the matrix.  It is unusable.
			badRepoMap[dependeeName] = struct{}{}
			continue
		}

		// Dependee is the repo being depended on.  We want to determine if it
		// can be included in the project without violating any dependency
		// requirements.
		//
		// For each repo that depends on dependee (i.e., for each dependent):
		// 1. Determine which of the dependent's requirements apply to the
		//    dependee (e.g., if we are evaluating v2 of the dependent, then
		//    v1's requirements do not apply).
		// 2. Check if the dependee satisfies all applicable requirements.
		for _, node := range nodes {
			var nodeApplies bool
			if node.Name == rootDependencyName {
				// project.yml requirements are always applicable.
				nodeApplies = true
			} else {
				// Repo dependency requirements only apply if they are
				// associated with the version of the dependent that we are
				// evaluating.
				dependentVer, ok := vm[node.Name]
				if ok {
					nodeApplies = newtutil.CompareRepoVersions(
						dependentVer, node.Ver) == 0
				}
			}
			if nodeApplies {
				if !dependeeVer.SatisfiesAll(node.VerReqs) {
					badRepoMap[dependeeName] = struct{}{}
					break
				}
			}
		}
	}

	badRepoSlice := make([]string, 0, len(badRepoMap))
	for repoName, _ := range badRepoMap {
		badRepoSlice = append(badRepoSlice, repoName)
	}
	sort.Strings(badRepoSlice)

	return badRepoSlice
}
