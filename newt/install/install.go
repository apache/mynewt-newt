// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//  http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// ----------------------------------------------------------------------------
// install: Handles project installs, upgrades, and syncs.
// ----------------------------------------------------------------------------
//
// This file implements three newt operations:
// * Install - Downloads repos that aren't installed yet.  The downloaded
//             version matches what `project.yml` specifies.
//
// * Upgrade - Ensures the installed version of each repo matches what
//             `project.yml` specifies.  This is similar to Install, but it
//             also operates on already-installed repos.
//
// * Sync    - Fetches and pulls the latest for each repo, but does not change
//             the branch (version).
//
// All three operations operate on the repos specified in the `project.yml`
// file and the set of each repo's dependencies.
//
// Within the `project.yml` file, repo requirements are expressed with one of
// the following forms:
//     * [Normalized version]: #.#.#
//           (e.g., "1.3.0")
//     * [Floating version]:   #[.#]-<stability
//           (e.g., "0-dev")
//     * [Git commit]:         <git-commit-ish>-commit
//           (e.g., "0aae710654b48d9a84d54de771cc18427709df7d-commit")
//
// The first two types (normalized version and floating version) are called
// "version specifiers".  Version specifiers map to "official releases", while
// git commits typically map to "custom versions".
//
// Before newt can do anything with a repo requirement, it needs to extrapolate
// two pieces of information:
// 1. The normalized version number.
// 2. The git commit.
//
// Newt needs the normalized version to determine the repo's dependencies, and
// to ensure the version of the repo is compatible with the version of newt
// being used.  Newt needs the git commit so that it knows how to checkout the
// desired repo version.
//
// ### VERSION SPECIFIERS
//
// A repo's `repository.yml` file maps version specifiers to git commits in its
// `repo.versions` field.  For example:
//    repo.versions:
//        "0.0.0": "master"
//        "1.0.0": "mynewt_1_0_0_tag"
//        "1.1.0": "mynewt_1_1_0_tag"
//        "0-dev": "0.0.0"
//
// By performing a series of recursive lookups, newt converts a version
// specifier to a normalized-version,git-commit pair.
//
// ### GIT COMMITS
//
// When newt encounters a git commit in the `project.yml` file, it already has
// one piece of information that it needs: the git commit.  Newt uses the
// following procedure to extrapolate its corresponding repo version:
//
// 1. If the repo at the commit contains a `version.yml` file, read the version
//    from this file.
// 2. Else, if the repo's `repository.yml` file maps the commit to a version
//    number, use that version number.
// 3. Else, warn the user and assume 0.0.0.
//
// The `version.yml` file is expected to be present in every commit in a repo.
// It has the following form:
//    repo.version: <normalized-version-number>
//
// For example, if commit 10 of repo X contains the following `version.yml`
// file:
//    repo.version: 1.10.0
//
// and commit 20 of repo X changes `version.yml` to:
//    repo.version: 2.0.0
//
// then newt extrapolates 1.10.0 from commits 10 through 19 (inclusive).
// Commit 20 and beyond correspond to 2.0.0.
//
// ### VERSION STRINGS
//
// Newt uses the following procedure when displaying a repo version to the
// user:
//
// Official releases are expressed as a normalized version.
//     e.g., 1.10.0
//
// Custom versions are expressed with the following form:
//     <extrapolated-version>/<git-commit>
//
// E.g.,:
//     0.0.0/0aae710654b48d9a84d54de771cc18427709df7d
// ----------------------------------------------------------------------------

package install

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/deprepo"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/newt/compat"
)

type installOp int

const (
	INSTALL_OP_INSTALL installOp = iota
	INSTALL_OP_UPGRADE
	INSTALL_OP_SYNC
)

// Determines the currently installed version of the specified repo.  If the
// repo doesn't have a valid `version.yml` file, and it isn't using a commit
// that maps to a version, 0.0.0 is returned.
func detectVersion(r *repo.Repo) (newtutil.RepoVersion, error) {
	ver, err := r.InstalledVersion()
	if err != nil {
		return newtutil.RepoVersion{}, err
	}

	// Fallback to 0.0.0 if version detection failed.
	if ver == nil {
		commit, err := r.CurrentHash()
		if err != nil {
			return newtutil.RepoVersion{}, err
		}

		// Create a 0.0.0 version specifier with the indicated commit string.
		ver = &newtutil.RepoVersion{
			Commit: commit,
		}

		util.StatusMessage(util.VERBOSITY_QUIET,
			"WARNING: Could not detect version of installed repo \"%s\"; "+
				"assuming %s\n", r.Name(), ver.String())
	}

	log.Debugf("currently installed version of repo \"%s\": %s",
		r.Name(), ver.String())

	return *ver, nil
}

type Installer struct {
	// Map of all repos in the project.
	repos deprepo.RepoMap

	// Version of each installed repo.
	vers deprepo.VersionMap

	// Required versions of installed repos, as read from `project.yml`.
	reqs deprepo.RequirementMap
}

func NewInstaller(repos deprepo.RepoMap,
	reqs deprepo.RequirementMap) (Installer, error) {

	inst := Installer{
		repos: repos,
		vers:  deprepo.VersionMap{},
		reqs:  reqs,
	}

	// Detect the installed versions of all repos.
	var firstErr error
	for n, r := range inst.repos {
		if !r.IsLocal() && !r.IsNewlyCloned() {
			ver, err := detectVersion(r)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
			} else {
				inst.vers[n] = ver
			}
		}
	}

	return inst, firstErr
}

// Retrieves the installed version of the specified repo.  Versions get
// detected and cached when the installer is constructed.  This function just
// retrieves the corresponding entry from the cache.
func (inst *Installer) installedVer(repoName string) *newtutil.RepoVersion {
	ver, ok := inst.vers[repoName]
	if !ok {
		return nil
	} else {
		return &ver
	}
}

// Given a slice of repos, recursively appends all depended-on repos, ensuring
// each element is unique.
//
// @param repos                 The list of dependent repos to process.
// @param vm                    Indicates the version of each repo to consider.
//                                  Pass nil to consider all versions of all
//                                  repos.
//
// @return []*repo.Repo         The original list, augmented with all
//                                  depended-on repos.
func (inst *Installer) ensureDepsInList(repos []*repo.Repo,
	vm deprepo.VersionMap) []*repo.Repo {

	seen := map[string]struct{}{}

	var recurse func(r *repo.Repo) []*repo.Repo
	recurse = func(r *repo.Repo) []*repo.Repo {
		// Don't process this repo a second time.
		if _, ok := seen[r.Name()]; ok {
			return nil
		}
		seen[r.Name()] = struct{}{}

		result := []*repo.Repo{r}

		var deps []*repo.RepoDependency
		if vm == nil {
			deps = r.AllDeps()
		} else {
			deps = r.DepsForVersion(vm[r.Name()])
		}
		for _, d := range deps {
			depRepo := inst.repos[d.Name]
			result = append(result, recurse(depRepo)...)
		}

		return result
	}

	deps := []*repo.Repo{}
	for _, r := range repos {
		deps = append(deps, recurse(r)...)
	}

	return deps
}

// Normalizes the installer's set of repo requirements.  Only the repos in the
// specified slice are considered.
//
// A repo requirement takes one of two forms:
//  * Version specifier (e.g., 1.3.0. or 0-dev).
//  * Git commit (e.g., 1f48a3c or master).
//
// This function converts requirements from the second form to the first.  A
// git commit is converted to a version number with this procedure:
//
// 1. If the specified commit contains a `version.yml` file, read the version
//    from this file.
// 2. Else, if the repo's `repository.yml` file maps the commit to a version
//    number, use that version number.
// 3. Else, assume 0.0.0.
func (inst *Installer) inferReqVers(repos []*repo.Repo) error {
	for _, r := range repos {
		reqs, ok := inst.reqs[r.Name()]
		if ok {
			for i, req := range reqs {
				if req.Ver.Commit != "" {
					ver, err := r.NonInstalledVersion(req.Ver.Commit)
					if err != nil {
						return err
					}

					if ver == nil {
						util.StatusMessage(util.VERBOSITY_QUIET,
							"WARNING: Could not detect version of "+
								"requested repo %s:%s; assuming 0.0.0\n",
							r.Name(), req.Ver.Commit)

						ver = &req.Ver
					}
					reqs[i].Ver = *ver
					reqs[i].Ver.Commit, err = r.HashFromVer(reqs[i].Ver)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// Determines if the `project.yml` file specifies a nonexistent repo version.
// Only the repos in the specified slice are considered.
//
// @param repos                 The list of repos to consider during the check.
// @param m                     A matrix containing all versions of the
//                                  specified repos.
//
// @return error                Error if any repo requirement is invalid.
func (inst *Installer) detectIllegalRepoReqs(
	repos []*repo.Repo, m deprepo.Matrix) error {

	var lines []string
	for _, r := range repos {
		reqs, ok := inst.reqs[r.Name()]
		if ok {
			row := m.FindRow(r.Name())
			if row == nil {
				return util.FmtNewtError(
					"internal error; repo \"%s\" missing from matrix", r.Name())
			}

			r := inst.repos[r.Name()]
			nreqs, err := r.NormalizeVerReqs(reqs)
			if err != nil {
				return err
			}

			anySatisfied := false
			for _, ver := range row.Vers {
				if ver.SatisfiesAll(nreqs) {
					anySatisfied = true
					break
				}
			}
			if !anySatisfied {
				line := fmt.Sprintf("    %s,%s", r.Name(),
					newtutil.RepoVerReqsString(nreqs))
				lines = append(lines, line)
			}
		}
	}

	if len(lines) > 0 {
		sort.Strings(lines)
		return util.NewNewtError(
			"project.yml file specifies nonexistent repo versions:\n" +
				strings.Join(lines, "\n"))
	}

	return nil
}

// Removes repos that shouldn't be installed from the specified list.  A repo
// should not be installed if it is already installed (any version).
//
// @param repos                 The list of repos to filter.
//
// @return []*Repo              The filtered list of repos.
func (inst *Installer) filterInstallList(
	vm deprepo.VersionMap) (deprepo.VersionMap, error) {

	filtered := deprepo.VersionMap{}

	for name, ver := range vm {
		curVer := inst.installedVer(name)
		if curVer == nil {
			filtered[name] = ver
		} else {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"Skipping \"%s\": already installed (%s)\n",
				name, curVer.String())
		}
	}

	return filtered, nil
}

// Indicates whether a repo should be upgraded to the specified version.  A
// repo should be upgraded if it is not currently installed, or if a version
// other than the desired one is installed.
func (inst *Installer) shouldUpgradeRepo(
	repoName string, destVer newtutil.RepoVersion) (bool, error) {

	curVer := inst.installedVer(repoName)

	// If the repo isn't installed, it needs to be upgraded.
	if curVer == nil {
		return true, nil
	}

	r := inst.repos[repoName]
	if r == nil {
		return false, util.FmtNewtError(
			"internal error: nonexistent repo has version: %s", repoName)
	}

	if !r.VersionsEqual(*curVer, destVer) {
		return true, nil
	}

	equiv, err := r.CommitsEquivalent(curVer.Commit, destVer.Commit)
	if err != nil {
		return false, err
	}

	return !equiv, nil
}

// Removes repos that shouldn't be upgraded from the specified list.  A repo
// should not be upgraded if the desired version is already installed.
//
// @param repos                 The list of repos to filter.
// @param vm                    Specifies the desired version of each repo.
//
// @return []*Repo              The filtered list of repos.
func (inst *Installer) filterUpgradeList(
	vm deprepo.VersionMap) (deprepo.VersionMap, error) {

	filtered := deprepo.VersionMap{}

	for name, ver := range vm {
		doUpgrade, err := inst.shouldUpgradeRepo(name, ver)
		if err != nil {
			return nil, err
		}
		if doUpgrade {
			filtered[name] = ver
		} else {
			curVer := inst.installedVer(name)
			if curVer == nil {
				return nil, util.FmtNewtError(
					"internal error: should upgrade repo %s, "+
						"but no version installed",
					name)
			}
			curVer.Commit = ver.Commit
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"Skipping \"%s\": already upgraded (%s)\n",
				name, curVer.String())
		}
	}

	return filtered, nil
}

// Describes an imminent install or upgrade operation to the user.  The
// displayed message applies to the specified repo.
func (inst *Installer) installMessageOneRepo(
	repoName string, op installOp, force bool, curVer *newtutil.RepoVersion,
	destVer newtutil.RepoVersion) (string, error) {

	// If the repo isn't installed yet, this is an install, not an upgrade.
	if op == INSTALL_OP_UPGRADE && curVer == nil {
		op = INSTALL_OP_INSTALL
	}

	var verb string
	switch op {
	case INSTALL_OP_INSTALL:
		if !force {
			verb = "install"
		} else {
			verb = "reinstall"
		}

	case INSTALL_OP_UPGRADE:
		verb = "upgrade"

	case INSTALL_OP_SYNC:
		verb = "sync"

	default:
		return "", util.FmtNewtError(
			"internal error: invalid install op: %v", op)
	}

	msg := fmt.Sprintf("    %s %s ", verb, repoName)
	if op == INSTALL_OP_UPGRADE {
		msg += fmt.Sprintf("(%s --> %s)", curVer.String(), destVer.String())
	} else if op != INSTALL_OP_SYNC {
		msg += fmt.Sprintf("(%s)", destVer.String())
	} else {
		// Sync operation.  Don't print the project version.  Instead, print
		// the actual branch name later during the sync.
	}

	return msg, nil
}

// Describes an imminent repo operation to the user.  In addition, prompts the
// user for confirmation if the `-a` (ask) option was specified.
func (inst *Installer) installPrompt(vm deprepo.VersionMap, op installOp,
	force bool, ask bool) (bool, error) {

	if len(vm) == 0 {
		return true, nil
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Making the following changes to the project:\n")

	names := vm.SortedNames()
	for _, name := range names {
		r := inst.repos[name]
		curVer := inst.installedVer(name)
		if curVer != nil && curVer.Commit != "" {
			c, err := r.CurrentHash()
			if err == nil {
				curVer.Commit = c
			}
		}
		destVer := vm[name]

		msg, err := inst.installMessageOneRepo(
			name, op, force, curVer, destVer)
		if err != nil {
			return false, err
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT, "%s\n", msg)
	}

	if !ask {
		return true, nil
	}

	for {
		fmt.Printf("Proceed? [Y/n] ")
		line, more, err := bufio.NewReader(os.Stdin).ReadLine()
		if more || err != nil {
			return false, util.ChildNewtError(err)
		}

		trimmed := strings.ToLower(strings.TrimSpace(string(line)))
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, "y") {
			// User wants to proceed.
			return true, nil
		}

		if strings.HasPrefix(trimmed, "n") {
			// User wants to cancel.
			return false, nil
		}

		// Invalid response.
		fmt.Printf("Invalid response.\n")
	}
}

// Determines whether a repo version's `Commit` field should be maintained.  If
// the commit corresponds exactly to a repo version in `repository.yml` (as
// opposed to simply indicating its version in a `version.yml` file), then the
// commit string should be discarded.  If the commit string is kept, newt
// interprets the version as being different from the official release version,
// triggering an upgrade.
func (inst *Installer) shouldKeepCommit(
	repoName string, commit string) (bool, error) {

	if commit == "" {
		return false, nil
	}

	r := inst.repos[repoName]
	if r == nil {
		return false, nil
	}

	vers, err := r.VersFromEquivCommit(commit)
	if err != nil {
		return false, err
	}
	if len(vers) > 0 {
		return false, nil
	}

	return true, nil
}

// Filters out repos from a version map, keeping only those which are present
// in the supplied slice.
func filterVersionMap(
	vm deprepo.VersionMap, keep []*repo.Repo) deprepo.VersionMap {

	filtered := deprepo.VersionMap{}
	for _, r := range keep {
		name := r.Name()
		if ver, ok := vm[name]; ok {
			filtered[name] = ver
		}
	}

	return filtered
}

// Creates a slice of repos, each corresponding to an element in the provided
// version map.  The returned slice is sorted by repo name.
func (inst *Installer) versionMapRepos(
	vm deprepo.VersionMap) ([]*repo.Repo, error) {

	repos := make([]*repo.Repo, 0, len(vm))

	names := vm.SortedNames()
	for _, name := range names {
		r := inst.repos[name]
		if r == nil {
			return nil, util.FmtNewtError(
				"internal error: repo \"%s\" missing from Installer#repos",
				name)
		}

		repos = append(repos, r)
	}

	return repos, nil
}

// Calculates a map of repos and version numbers that should be included in an
// install or upgrade operation.
func (inst *Installer) calcVersionMap(candidates []*repo.Repo) (
	deprepo.VersionMap, error) {

	// Repos that depend on any specified repos must also be considered during
	// the install / upgrade operation.
	repoList := inst.ensureDepsInList(candidates, nil)

	m, err := deprepo.BuildMatrix(repoList, inst.vers)
	if err != nil {
		return nil, err
	}

	if err := inst.inferReqVers(repoList); err != nil {
		return nil, err
	}

	// If the `project.yml` file specifies an invalid repo version, abort now.
	if err := inst.detectIllegalRepoReqs(repoList, m); err != nil {
		return nil, err
	}

	// Remove blocked repo versions from the table.
	if err := deprepo.PruneMatrix(
		&m, inst.repos, inst.reqs); err != nil {

		return nil, err
	}

	// Construct a repo dependency graph from the `project.yml` version
	// requirements and from each repo's dependency list.
	dg, err := deprepo.BuildDepGraph(inst.repos, inst.reqs)
	if err != nil {
		return nil, err
	}

	log.Debugf("Repo dependency graph:\n%s\n", dg.String())
	log.Debugf("Repo reverse dependency graph:\n%s\n", dg.Reverse().String())

	// Try to find a version set that satisfies the dependency graph.  If no
	// such set exists, report the conflicts and abort.
	vm, conflicts := deprepo.FindAcceptableVersions(m, dg)
	if vm == nil {
		return nil, deprepo.ConflictError(conflicts)
	}

	log.Debugf("Repo version map:\n%s\n", vm.String())

	// If project.yml specified any specific git commits, ensure we get them.
	for name, ver := range vm {
		reqs := inst.reqs[name]
		if len(reqs) > 0 {
			keep, err := inst.shouldKeepCommit(name, reqs[0].Ver.Commit)
			if err != nil {
				return nil, err
			}
			if keep {
				ver.Commit = reqs[0].Ver.Commit
			}
			vm[name] = ver
		}
	}

	// Now that we know which repo versions we want, we can eliminate some
	// false-positives from the repo list.
	repoList = inst.ensureDepsInList(candidates, vm)
	vm = filterVersionMap(vm, repoList)

	return vm, nil
}

// Checks if any repos in the specified slice are in a dirty state.  If any
// repos are dirty and `force` is *not* enabled, an error is returned.  If any
// repos are dirty and `force` is enabled, a warning is displayed.
func verifyRepoDirtyState(repos []*repo.Repo, force bool) error {
	// [repo] => dirty-state.
	var m map[*repo.Repo]string

	// Collect all dirty repos and insert them into m.
	for _, r := range repos {
		dirtyState, err := r.DirtyState()
		if err != nil {
			return err
		}

		if dirtyState != "" {
			if m == nil {
				m = make(map[*repo.Repo]string)
				m[r] = dirtyState
			}
		}
	}

	if len(m) > 0 {
		s := "some repos are in a dirty state:\n"
		for r, d := range m {
			s += fmt.Sprintf("    %s: contains %s\n", r.Name(), d)
		}

		if !force {
			s += "Specify the `-f` (force) switch to attempt anyway"
			return util.NewNewtError(s)
		} else {
			util.StatusMessage(util.VERBOSITY_QUIET, "WARNING: %s\n", s)
		}
	}

	return nil

}

func verifyNewtCompat(repos []*repo.Repo, vm deprepo.VersionMap) error {
	var errors []string

	for _, r := range repos {
		destVer := vm[r.Name()]
		code, msg := r.CheckNewtCompatibility(destVer, newtutil.NewtVersion)

		switch code {
		case compat.NEWT_COMPAT_WARN:
			util.StatusMessage(util.VERBOSITY_QUIET, "WARNING: %s\n", msg)
		case compat.NEWT_COMPAT_ERROR:
			errors = append(errors, msg)
		}
	}

	if errors != nil {
		return util.NewNewtError(strings.Join(errors, "\n"))
	}

	return nil
}

// Installs the specified set of repos.
func (inst *Installer) Install(
	candidates []*repo.Repo, force bool, ask bool) error {

	vm, err := inst.calcVersionMap(candidates)
	if err != nil {
		return err
	}

	// Perform some additional filtering on the list of repos to process.
	if !force {
		// Don't install a repo if it is already installed (any version).  We
		// skip this filter for forced reinstalls.
		vm, err = inst.filterInstallList(vm)
		if err != nil {
			return err
		}
	}

	// Notify the user of what install operations are about to happen, and
	// prompt if the `-a` (ask) option was specified.
	proceed, err := inst.installPrompt(vm, INSTALL_OP_INSTALL, force, ask)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	repos, err := inst.versionMapRepos(vm)
	if err != nil {
		return err
	}

	if err := verifyNewtCompat(repos, vm); err != nil {
		return err
	}

	// For a forced install, delete all existing repos.
	if force {
		for _, r := range repos {
			// Don't delete the local project directory!  And don't delete a
			// repo that was just cloned during this invocation of newt.
			if !r.IsLocal() && !r.IsNewlyCloned() {
				util.StatusMessage(util.VERBOSITY_DEFAULT,
					"Removing old copy of \"%s\" (%s)\n", r.Name(), r.Path())
				os.RemoveAll(r.Path())
				delete(inst.vers, r.Name())
			}
		}
	}

	// Install each repo in the version map.
	for _, r := range repos {
		destVer := vm[r.Name()]
		if err := r.Install(destVer); err != nil {
			return err
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"%s successfully installed version %s\n",
			r.Name(), destVer.String())
	}

	return nil
}

// Installs or upgrades the specified set of repos.
func (inst *Installer) Upgrade(candidates []*repo.Repo, force bool,
	ask bool) error {

	if err := verifyRepoDirtyState(candidates, force); err != nil {
		return err
	}

	vm, err := inst.calcVersionMap(candidates)
	if err != nil {
		return err
	}

	// Don't upgrade a repo if we already have the desired version.
	vm, err = inst.filterUpgradeList(vm)
	if err != nil {
		return err
	}

	// Notify the user of what install operations are about to happen, and
	// prompt if the `-a` (ask) option was specified.
	proceed, err := inst.installPrompt(vm, INSTALL_OP_UPGRADE, false, ask)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	repos, err := inst.versionMapRepos(vm)
	if err != nil {
		return err
	}

	if err := verifyNewtCompat(repos, vm); err != nil {
		return err
	}

	// Upgrade each repo in the version map.
	for _, r := range repos {
		destVer := vm[r.Name()]
		if err := r.Upgrade(destVer); err != nil {
			return err
		}
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"%s successfully upgraded to version %s\n",
			r.Name(), destVer.String())
	}

	return nil
}

// Syncs the specified set of repos.
func (inst *Installer) Sync(candidates []*repo.Repo,
	force bool, ask bool) error {

	if err := verifyRepoDirtyState(candidates, force); err != nil {
		return err
	}

	vm, err := inst.calcVersionMap(candidates)
	if err != nil {
		return err
	}

	// Notify the user of what install operations are about to happen, and
	// prompt if the `-a` (ask) option was specified.
	proceed, err := inst.installPrompt(vm, INSTALL_OP_SYNC, false, ask)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	repos, err := inst.versionMapRepos(vm)
	if err != nil {
		return err
	}

	// Sync each repo in the list.
	var anyFails bool
	for _, r := range repos {
		ver := inst.installedVer(r.Name())
		if ver == nil {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"No installed version of %s found, skipping\n",
				r.Name())
		} else {
			if _, err := r.Sync(*ver); err != nil {
				util.StatusMessage(util.VERBOSITY_QUIET,
					"Failed to sync repo \"%s\": %s\n",
					r.Name(), err.Error())
				anyFails = true
			}
		}
	}

	if anyFails {
		return util.FmtNewtError("Failed to sync")
	}

	return nil
}

type repoInfo struct {
	installedVer *newtutil.RepoVersion
	errorText    string
	dirtyState   string
	needsUpgrade bool
}

// Collects information about the specified repo.  If a version map is provided
// (i.e., vm is not nil), this function also queries the repo's remote to
// determine if the repo can be upgraded.
func (inst *Installer) gatherInfo(r *repo.Repo,
	vm *deprepo.VersionMap) repoInfo {

	ri := repoInfo{}

	if !r.CheckExists() {
		return ri
	}

	ver, err := r.InstalledVersion()
	if err != nil {
		ri.errorText = strings.TrimSpace(err.Error())
		return ri
	}
	ri.installedVer = ver

	dirty, err := r.DirtyState()
	if err != nil {
		ri.errorText = strings.TrimSpace(err.Error())
		return ri
	}
	ri.dirtyState = dirty

	if vm != nil {
		if ver == nil || *ver != (*vm)[r.Name()] {
			ri.needsUpgrade = true
		}
	}

	return ri
}

// Prints out information about the specified repos:
//     * Currently installed version.
//     * Whether upgrade is possible.
//     * Whether repo is in a dirty state.
//
// @param repos                 The set of repositories to inspect.
// @param remote                Whether to perform any remote queries to
//                                  determine if upgrades are needed.
func (inst *Installer) Info(repos []*repo.Repo, remote bool) error {
	var vmp *deprepo.VersionMap

	if remote {
		vm, err := inst.calcVersionMap(repos)
		if err != nil {
			return err
		}

		vmp = &vm
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Repository info:\n")
	for _, r := range repos {
		ri := inst.gatherInfo(r, vmp)
		s := fmt.Sprintf("    * %s:", r.Name())

		if ri.installedVer == nil {
			s += " (not installed)"
		} else if ri.errorText != "" {
			s += fmt.Sprintf(" (unknown: %s)", ri.errorText)
		} else {
			s += fmt.Sprintf(" %s", ri.installedVer.String())
			if ri.dirtyState != "" {
				s += fmt.Sprintf(" (dirty: %s)", ri.dirtyState)
			}
			if ri.needsUpgrade {
				s += " (needs upgrade)"
			}
		}
		util.StatusMessage(util.VERBOSITY_DEFAULT, "%s\n", s)
	}

	return nil
}
