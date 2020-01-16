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
package deprepo

import (
	"fmt"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
)

// [repo-name] => repo
type RepoMap map[string]*repo.Repo

// [repo-name] => repo-version
type VersionMap map[string]newtutil.RepoVersion

// [repo-name] => requirements-for-key-repo
type RequirementMap map[string]newtutil.RepoVersion

type ConflictEntry struct {
	Dependent   RVPair
	DependeeVer newtutil.RepoVersion
}

// Indicates dependencies on different versions of the same repo.
type Conflict struct {
	DependeeName string
	Entries      []ConflictEntry
}

func (c *Conflict) SortEntries() {
	sort.Slice(c.Entries, func(i int, j int) bool {
		ci := c.Entries[i]
		cj := c.Entries[j]

		x := CompareRVPairs(ci.Dependent, cj.Dependent)
		if x != 0 {
			return x < 0
		}

		return newtutil.CompareRepoVersions(ci.DependeeVer, cj.DependeeVer) < 0
	})
}

// Returns a sorted slice of all constituent repo names.
func (vm VersionMap) SortedNames() []string {
	names := make([]string, 0, len(vm))
	for name, _ := range vm {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// Returns a slice of all constituent repos, sorted by name.
func (rm RepoMap) Sorted() []*repo.Repo {
	names := make([]string, 0, len(rm))
	for n, _ := range rm {
		names = append(names, n)
	}
	sort.Strings(names)

	repos := make([]*repo.Repo, len(names))
	for i, n := range names {
		repos[i] = rm[n]
	}

	return repos
}

func (vm VersionMap) String() string {
	s := ""

	names := vm.SortedNames()
	for _, name := range names {
		ver := vm[name]
		if len(s) > 0 {
			s += "\n"
		}
		s += fmt.Sprintf("%s:%s", name, ver.String())
	}
	return s
}

// Builds a repo dependency graph from the repo requirements expressed in the
// `project.yml` file.
func BuildDepGraph(repos RepoMap, rootReqs RequirementMap) (DepGraph, error) {
	dg := DepGraph{}

	// First, add the hard dependencies expressed in `project.yml`.
	for repoName, verReq := range rootReqs {
		repo := repos[repoName]
		normalizedReq, err := repo.NormalizeVerReq(verReq)
		if err != nil {
			return nil, err
		}

		if err := dg.AddRootDep(repoName, normalizedReq); err != nil {
			return nil, err
		}
	}

	// Add inter-repo dependencies to the graph.
	for _, r := range repos.Sorted() {
		nvers, err := r.NormalizedVersions()
		if err != nil {
			return nil, err
		}
		for _, v := range nvers {
			deps := r.DepsForVersion(v)
			reqMap := RequirementMap{}
			for _, d := range deps {
				depRepo := repos[d.Name]
				verReqs, err := depRepo.NormalizeVerReq(d.VerReqs)
				if err != nil {
					return nil, err
				}
				reqMap[d.Name] = verReqs
			}
			if err := dg.AddRepoVer(r.Name(), v, reqMap); err != nil {
				return nil, err
			}
		}
	}

	return dg, nil
}

// PruneDepGraph removes all entries from a depgraph that aren't in the given
// repo slice.  This is necessary when the user wants to upgrade only a subset
// of repos in the project.
func PruneDepGraph(dg DepGraph, keep []*repo.Repo) {
	for k, _ := range dg {
		// The empty string indicates a `project.yml` requirement.  Always
		// keep these.
		if k.Name != "" {
			found := false

			for _, r := range keep {
				if k.Name == r.Name() {
					found = true
					break
				}
			}
			if !found {
				delete(dg, k)
			}
		}
	}
}

// Produces an error describing the specified set of repo conflicts.
func ConflictError(conflicts []Conflict) error {
	s := ""

	for _, c := range conflicts {
		if s != "" {
			s += "\n\n"
		}

		s += fmt.Sprintf("    Installation of repo \"%s\" is blocked:",
			c.DependeeName)

		lines := []string{}
		for _, e := range c.Entries {
			dependee := RVPair{
				Name: c.DependeeName,
				Ver:  e.DependeeVer,
			}
			lines = append(lines, fmt.Sprintf("\n    %30s requires %s",
				e.Dependent.String(), dependee.String()))
		}
		sort.Strings(lines)
		s += strings.Join(lines, "")
	}

	return util.NewNewtError("Repository conflicts:\n" + s)
}

// ResolveRepoDeps calculates the set of repo versions a project should be
// upgraded to.
//
// dg: The project's repo dependency graph.  This includes root dependencies
// 	   (i.e., dependencies expressed in `project.yml`).
//
// Returns a version map on success; a set of conflicts on failure.
func ResolveRepoDeps(dg DepGraph) (VersionMap, []Conflict) {
	// Represents an entry in the working set.
	type WSNode struct {
		highPrio  bool   // If true, always "wins" without conflict.
		dependent RVPair // Repo that expresses this dependency.
		dependee  RVPair // Repo that is depended on.
	}

	ws := map[string]WSNode{}        // Working set (key=dependent).
	cm := map[string]*Conflict{}     // Conflict map (key=dependee).
	visited := map[string]struct{}{} // Tracks which nodes have been visited.

	// Updates (or inserts a new) conflict object into the conflict map (cm).
	addConflict := func(dependent RVPair, dependee RVPair) {
		c := cm[dependee.Name]
		if c == nil {
			wsn := ws[dependee.Name]

			c = &Conflict{
				DependeeName: repoNameString(dependee.Name),
				Entries: []ConflictEntry{
					ConflictEntry{
						Dependent: RVPair{
							Name: repoNameString(wsn.dependent.Name),
							Ver:  wsn.dependent.Ver,
						},
						DependeeVer: wsn.dependee.Ver,
					},
				},
			}
			cm[dependee.Name] = c
		}

		c.Entries = append(c.Entries, ConflictEntry{
			Dependent:   dependent,
			DependeeVer: dependee.Ver,
		})
	}

	// Attempts to add a single node to the working set.  In case of a
	// conflict, the conflict map is updated instead.
	addWsNode := func(dependent RVPair, dependee RVPair) {
		old, ok := ws[dependee.Name]
		if !ok {
			// This is the first time we've seen this repo.
			ws[dependee.Name] = WSNode{
				highPrio:  false,
				dependent: dependent,
				dependee:  dependee,
			}
			return
		}

		if newtutil.CompareRepoVersions(old.dependee.Ver, dependee.Ver) == 0 {
			// We have already seen this exact dependency.  Ignore the
			// duplicate.
			return
		}

		// There is already a dependency for a different version of this repo.
		// Handle the conflict.
		if old.highPrio {
			// Root commit dependencies take priority over all others.
			log.Debugf("discarding repo dependency in favor "+
				"of root override: dep=%s->%s root=%s->%s",
				dependent.String(), dependee.String(),
				old.dependent.String(), old.dependee.String())
		} else {
			addConflict(dependent, dependee)
		}
	}

	// Adds one entry to the working set (ws) for each of the given repo's
	// dependencies.
	visit := func(rvp RVPair) {
		nodes := dg[rvp]
		for _, node := range nodes {
			addWsNode(rvp, node)
		}

		visited[rvp.Name] = struct{}{}
	}

	// Insert all the root dependencies (dependencies in `project.yml`) into
	// the working set.  A root dependency is "high priority" if it points to a
	// git commit rather than a version number.
	rootDeps := dg[RVPair{}]
	for _, dep := range rootDeps {
		ws[dep.Name] = WSNode{
			highPrio:  dep.Ver.Commit != "",
			dependent: RVPair{Name: rootDependencyName},
			dependee:  dep,
		}
	}

	// Repeatedly iterate through the working set, visiting each unvisited
	// node.  Stop looping when an iteration produces no new nodes.
	for len(visited) < len(ws) {
		// Iterate the working set in a consistent order.  The order doesn't
		// matter for well-defined projects.  For invalid projects, different
		// iteration orders can result in different conflicts being reported.
		names := make([]string, 0, len(ws))
		for name, _ := range ws {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			wsn := ws[name]
			if _, ok := visited[name]; !ok {
				visit(wsn.dependee)
			}
		}
	}

	// It is an error if any conflicts were detected.
	if len(cm) > 0 {
		var cs []Conflict
		for _, c := range cm {
			c.SortEntries()
			cs = append(cs, *c)
		}

		sort.Slice(cs, func(i int, j int) bool {
			return strings.Compare(cs[i].DependeeName, cs[j].DependeeName) < 0
		})

		return nil, cs
	}

	// The working set now contains the target version of each repo in the
	// project.
	vm := VersionMap{}
	for name, wsn := range ws {
		vm[name] = wsn.dependee.Ver
	}

	return vm, nil
}
