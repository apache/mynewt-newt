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

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
)

// [repo-name] => repo
type RepoMap map[string]*repo.Repo

// [repo-name] => repo-version
type VersionMap map[string]newtutil.RepoVersion

// [repo-name] => requirements-for-key-repo
type RequirementMap map[string][]newtutil.RepoVersionReq

// Indicates an inability to find an acceptable version of a particular repo.
type Conflict struct {
	RepoName string
	Filters  []Filter
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

	for repoName, ver := range vm {
		if len(s) > 0 {
			s += "\n"
		}
		s += fmt.Sprintf("%s:%s", repoName, ver.String())
	}
	return s
}

// Constructs a version matrix from the specified repos.  Each row in the
// resulting matrix corresponds to a repo in the supplied slice.  Each row node
// represents a single version of the repo.
func BuildMatrix(repos []*repo.Repo, vm VersionMap) (Matrix, error) {
	m := Matrix{}

	for _, r := range repos {
		if !r.IsLocal() {
			vers, err := r.NormalizedVersions()
			if err != nil {
				return m, err
			}
			if err := m.AddRow(r.Name(), vers); err != nil {
				return m, err
			}
		}
	}

	return m, nil
}

// Builds a repo dependency graph from the repo requirements expressed in the
// `project.yml` file.
func BuildDepGraph(repos RepoMap, rootReqs RequirementMap) (DepGraph, error) {
	dg := DepGraph{}

	// First, add the hard dependencies expressed in `project.yml`.
	for repoName, verReqs := range rootReqs {
		repo := repos[repoName]
		normalizedReqs, err := repo.NormalizeVerReqs(verReqs)
		if err != nil {
			return nil, err
		}

		if err := dg.AddRootDep(repoName, normalizedReqs); err != nil {
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
				verReqs, err := depRepo.NormalizeVerReqs(d.VerReqs)
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

// Prunes unusable repo versions from the specified matrix.
func PruneMatrix(m *Matrix, repos RepoMap, rootReqs RequirementMap) error {
	pruned := map[*repo.RepoDependency]struct{}{}

	// Removes versions of the depended-on package that fail to satisfy the
	// dependent's requirements.  Each of the depended-on package's
	// dependencies are then recursively visited.
	var recurse func(dependentName string, dep *repo.RepoDependency) error
	recurse = func(dependentName string, dep *repo.RepoDependency) error {
		// Don't prune the same dependency twice; prevents infinite
		// recursion.
		if _, ok := pruned[dep]; ok {
			return nil
		}
		pruned[dep] = struct{}{}

		// Remove versions of this depended-on package that don't satisfy the
		// dependency's version requirements.
		r := repos[dep.Name]
		normalizedReqs, err := r.NormalizeVerReqs(dep.VerReqs)
		if err != nil {
			return err
		}
		filter := Filter{
			Name: dependentName,
			Reqs: normalizedReqs,
		}
		m.ApplyFilter(dep.Name, filter)

		// If there is only one version of the depended-on package left in the
		// matrix, we can recursively call this function on all its
		// dependencies.
		//
		// We don't do it, but it is possible to prune when there is more than
		// one version remaining.  To accomplish this, we would collect the
		// requirements from all versions, find their union, and remove
		// depended-on packages that satisfy none of the requirements in the
		// union.  The actual implementation (only prune when one version
		// remains) is a simplified implementation of this general procedure.
		// In exchange for simplicity, some unusable versions remain in the
		// matrix that could have been pruned.  These unsuable versions must be
		// evaluated unnecessarily when the matrix is being searched for an
		// acceptable version set.
		row := m.FindRow(r.Name())
		if row != nil && len(row.Vers) == 1 {
			ver := row.Vers[0]
			commit, err := r.CommitFromVer(ver)
			if err != nil {
				return err
			}
			depRepo := repos[dep.Name]
			for _, ddep := range depRepo.CommitDepMap()[commit] {
				name := fmt.Sprintf("%s,%s", depRepo.Name(), ver.String())
				if err := recurse(name, ddep); err != nil {
					return err
				}
			}
		}

		return nil
	}

	// Prune versions that are guaranteed to be unusable.  Any repo version
	// which doesn't satisfy a requirement in `project.yml` is a
	// known-bad-version and can be removed.  These repos' dependencies can
	// then be pruned in turn.
	for repoName, reqs := range rootReqs {
		if len(reqs) > 0 {
			dep := &repo.RepoDependency{
				Name:    repoName,
				VerReqs: reqs,
			}

			if err := recurse("project.yml", dep); err != nil {
				return err
			}
		}
	}

	return nil
}

// Produces an error describing the specified set of repo conflicts.
func ConflictError(conflicts []Conflict) error {
	s := ""

	for _, c := range conflicts {
		if s != "" {
			s += "\n\n"
		}

		s += fmt.Sprintf("    Installation of repo \"%s\" is blocked:",
			c.RepoName)

		lines := []string{}
		for _, f := range c.Filters {
			lines = append(lines, fmt.Sprintf("\n    %30s requires %s %s",
				f.Name, c.RepoName, newtutil.RepoVerReqsString(f.Reqs)))
		}
		sort.Strings(lines)
		s += strings.Join(lines, "")
	}

	return util.NewNewtError("Repository conflicts:\n" + s)
}

// Searches a version matrix for a set of acceptable repo versions.  If there
// isn't an acceptable set of versions, the set with the fewest conflicts is
// returned.
//
// @param m                     Matrix containing all unpruned repo versions.
// @param dg                    The repo dependency graph.
//
// @return VersionMap           The first perfect set of repo versions, or the
//                                  closest match if there is no perfect set.
// @return []string             nil if a perfect set was found, else the names
//                                  of the repos that lack a suitable version
//                                  in the returned version map.
func findClosestMatch(m Matrix, dg DepGraph) (VersionMap, []string) {
	// Tracks the best match seen so far.
	type Best struct {
		vm       VersionMap
		failures []string
	}
	var best Best

	for {
		vm := m.CurVersions()
		badRepos := dg.conflictingRepos(vm)
		if len(badRepos) == 0 {
			// Found a perfect match.  Return it.
			return vm, nil
		}

		if best.failures == nil || len(badRepos) < len(best.failures) {
			best.vm = vm
			best.failures = badRepos
		}

		// Evaluate the next set of versions on the following iteration.
		if !m.Increment() {
			// All version sets evaluated.  Return the best match.
			return best.vm, best.failures
		}
	}
}

// Finds the first set of repo versions which satisfies the dependency graph.
// If there is no acceptable set, a slice of conflicts is returned instead.
//
// @param m                     Matrix containing all unpruned repo versions.
// @param dg                    The repo dependency graph.
// @return VersionMap           The first perfect set of repo versions, or nil
//                                  if there is no perfect set.
// @return []Conflict           nil if a perfect set was found, else the set of
//                                  conflicts preventing a perfect match from
//                                  being returned.
func FindAcceptableVersions(m Matrix, dg DepGraph) (VersionMap, []Conflict) {
	vm, failures := findClosestMatch(m, dg)
	if len(failures) == 0 {
		// No failures implies a perfect match was found.  Return it.
		return vm, nil
	}

	// A perfect version set doesn't exist.  Generate the set of relevant
	// conflicts and return it.
	conflicts := make([]Conflict, len(failures))

	// Build a reverse dependency graph.  This will make it easy to determine
	// which version requirements are relevant to the failure.
	rg := dg.Reverse()
	for i, f := range failures {
		conflict := Conflict{
			RepoName: f,
		}
		for _, node := range rg[f] {
			// Determine if this filter is responsible for any conflicts.
			// Record the name of the filter if it applies.
			var filterName string
			if node.Name == rootDependencyName {
				filterName = "project.yml"
			} else {
				// If the version of the repo in the closest-match version map
				// is the same one that imposes this version requirement,
				// include it in the conflict object.
				if newtutil.CompareRepoVersions(vm[node.Name], node.Ver) == 0 {
					filterName = fmt.Sprintf(
						"%s,%s", node.Name, node.Ver.String())
				}
			}

			if filterName != "" {
				conflict.Filters = append(conflict.Filters, Filter{
					Name: filterName,
					Reqs: node.VerReqs,
				})
			}
		}
		conflicts[i] = conflict
	}

	return nil, conflicts
}
