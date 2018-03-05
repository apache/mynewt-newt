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

package project

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/compat"
	"mynewt.apache.org/newt/newt/deprepo"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

var globalProject *Project = nil

const PROJECT_FILE_NAME = "project.yml"

var ignoreSearchDirs []string = []string{
	"bin",
	"repos",
}

type Project struct {
	// Name of this project
	name string

	// Base path of the project
	BasePath string

	packages interfaces.PackageList

	projState *ProjectState

	// Contains all the repos that form this project.  Each repo is in one of
	// two states:
	//    * description: Only the repo's basic description fields have been
	//                   read from `project.yml` or from a dependent repo's
	//                   `repository.yml` file.  This repo's `repository.yml`
	//                   file still needs to be read.
	//    * complete: The repo's `repository.yml` file exists and has been
	//                read.
	repos deprepo.RepoMap

	// The local repository at the top-level of the project.  This repo is
	// excluded from most repo operations.
	localRepo *repo.Repo

	// Required versions of installed repos, as read from `project.yml`.
	rootRepoReqs deprepo.RequirementMap

	warnings []string

	yc ycfg.YCfg
}

type installOp int

const (
	INSTALL_OP_INSTALL installOp = iota
	INSTALL_OP_UPGRADE
	INSTALL_OP_SYNC
)

func initProject(dir string) error {
	var err error

	globalProject, err = LoadProject(dir)
	if err != nil {
		return err
	}
	if err := globalProject.loadPackageList(); err != nil {
		return err
	}

	return nil
}

func initialize() error {
	if globalProject == nil {
		wd, err := os.Getwd()
		wd = filepath.ToSlash(wd)
		if err != nil {
			return util.NewNewtError(err.Error())
		}
		if err := initProject(wd); err != nil {
			return err
		}
	}
	return nil
}

func TryGetProject() (*Project, error) {
	if err := initialize(); err != nil {
		return nil, err
	}
	return globalProject, nil
}

func GetProject() *Project {
	if _, err := TryGetProject(); err != nil {
		panic(err.Error())
	}

	return globalProject
}

func ResetProject() {
	globalProject = nil
}

func ResetDeps(newList interfaces.PackageList) interfaces.PackageList {
	return nil
	if globalProject == nil {
		return nil
	}
	oldList := globalProject.packages
	globalProject.packages = newList

	if newList == nil {
		globalProject.loadPackageList()
	}
	return oldList
}

func NewProject(dir string) (*Project, error) {
	proj := &Project{}

	if err := proj.Init(dir); err != nil {
		return nil, err
	}

	return proj, nil
}

func (proj *Project) Path() string {
	return proj.BasePath
}

func (proj *Project) Name() string {
	return proj.name
}

func (proj *Project) Repos() map[string]*repo.Repo {
	return proj.repos
}

func (proj *Project) FindRepo(rname string) *repo.Repo {
	if rname == repo.REPO_NAME_LOCAL {
		return proj.LocalRepo()
	} else {
		r, _ := proj.repos[rname]
		return r
	}
}

func (proj *Project) FindRepoPath(rname string) string {
	r := proj.FindRepo(rname)
	if r == nil {
		return ""
	}

	return r.Path()
}

// Indicates whether the specified repo is present in the `project.state` file.
func (proj *Project) RepoIsInstalled(rname string) bool {
	return proj.projState.GetInstalledVersion(rname) != nil
}

func (proj *Project) RepoIsRoot(rname string) bool {
	return proj.rootRepoReqs[rname] != nil
}

func (proj *Project) LocalRepo() *repo.Repo {
	return proj.localRepo
}

func (proj *Project) Warnings() []string {
	return proj.warnings
}

// Selects repositories from the global state that satisfy the specified
// predicate.
func (proj *Project) SelectRepos(pred func(r *repo.Repo) bool) []*repo.Repo {
	all := proj.repos.Sorted()
	var filtered []*repo.Repo

	for _, r := range all {
		if pred(r) {
			filtered = append(filtered, r)
		}
	}

	return filtered
}

// Indicates whether a repo should be installed.  A repo should be installed if
// it is not currently installed.
func (proj *Project) shouldInstallRepo(repoName string) bool {
	return proj.projState.GetInstalledVersion(repoName) == nil
}

// Indicates whether a repo should be upgraded to the specified version.  A
// repo should be upgraded if it is not currently installed, or if a version
// other than the desired one is installed.
func (proj *Project) shouldUpgradeRepo(repoName string,
	destVer newtutil.RepoVersion) bool {

	r := proj.repos[repoName]
	if r == nil {
		return false
	}

	stateVer := proj.projState.GetInstalledVersion(repoName)
	if stateVer == nil {
		return true
	}

	return !r.VersionsEqual(*stateVer, destVer)
}

// Removes repos that shouldn't be installed from the specified list.  A repo
// should not be installed if it is already installed (any version).
//
// @param repos                 The list of repos to filter.
//
// @return []*Repo              The filtered list of repos.
func (proj *Project) filterInstallList(repos []*repo.Repo) []*repo.Repo {
	keep := []*repo.Repo{}

	for _, r := range repos {
		curVer := proj.projState.GetInstalledVersion(r.Name())
		if curVer == nil {
			keep = append(keep, r)
		} else {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"Skipping \"%s\": already installed (%s)\n",
				r.Name(), curVer.String())
		}
	}

	return keep
}

// Removes repos that shouldn't be upgraded from the specified list.  A repo
// should not be upgraded if the desired version is already installed.
//
// @param repos                 The list of repos to filter.
// @param vm                    Specifies the desired version of each repo.
//
// @return []*Repo              The filtered list of repos.
func (proj *Project) filterUpgradeList(
	repos []*repo.Repo, vm deprepo.VersionMap) []*repo.Repo {

	keep := []*repo.Repo{}

	for _, r := range repos {
		destVer := vm[r.Name()]
		if proj.shouldUpgradeRepo(r.Name(), destVer) {
			keep = append(keep, r)
		} else {
			curVer := proj.projState.GetInstalledVersion(r.Name())
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"Skipping \"%s\": already upgraded (%s)\n",
				r.Name(), curVer.String())
		}
	}

	return keep
}

// Determines if the `project.yml` file specifies a nonexistent repo version.
// Only the repos in the specified slice are considered.
//
// @param repos                 The list of repos to consider during the check.
// @param m                     A matrix containing all versions of the
//                                  specified repos.
//
// @return error                Error if any repo requirement is invalid.
func (proj *Project) detectIllegalRepoReqs(
	repos []*repo.Repo, m deprepo.Matrix) error {

	var lines []string
	for _, r := range repos {
		reqs, ok := proj.rootRepoReqs[r.Name()]
		if ok {
			row := m.FindRow(r.Name())
			if row == nil {
				return util.FmtNewtError(
					"internal error; repo \"%s\" missing from matrix", r.Name())
			}

			r := proj.repos[r.Name()]
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

// Installs or upgrades a single repo to the specified version.
func (proj *Project) installRepo(r *repo.Repo, ver newtutil.RepoVersion,
	upgrade bool, force bool) error {

	// Install the acceptable version.
	if upgrade {
		if err := r.Upgrade(ver, force); err != nil {
			return err
		}
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"%s successfully upgraded to version %s\n",
			r.Name(), ver.String())
	} else {
		if err := r.Install(ver); err != nil {
			return err
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"%s successfully installed version %s\n",
			r.Name(), ver.String())
	}

	// Update the project state with the new repository version
	// information.
	proj.projState.Replace(r.Name(), ver)

	return nil
}

func (proj *Project) installMessageOneRepo(
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
	if curVer != nil {
		msg += fmt.Sprintf("(%s --> %s)", curVer.String(), destVer.String())
	} else {
		msg += fmt.Sprintf("(%s)", destVer.String())
	}

	return msg, nil
}

// Describes an imminent repo operation to the user.  In addition, prompts the
// user for confirmation if the `-a` (ask) option was specified.
func (proj *Project) installPrompt(repoList []*repo.Repo,
	vm deprepo.VersionMap, op installOp, force bool, ask bool) (bool, error) {

	if len(repoList) == 0 {
		return true, nil
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Making the following changes to the project:\n")

	for _, r := range repoList {
		curVer := proj.projState.GetInstalledVersion(r.Name())
		destVer := vm[r.Name()]

		msg, err := proj.installMessageOneRepo(
			r.Name(), op, force, curVer, destVer)
		if err != nil {
			return false, err
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT, "%s\n", msg)
	}
	util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")

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

// Installs or upgrades repos matching the specified predicate.
func (proj *Project) InstallIf(
	upgrade bool, force bool, ask bool,
	predicate func(r *repo.Repo) bool) error {

	// Make sure we have an up to date copy of all `repository.yml` files.
	if err := proj.downloadRepositoryYmlFiles(); err != nil {
		return err
	}

	// Now that all repos have been successfully fetched, we can finish the
	// install procedure locally.

	// Determine which repos the user wants to install or upgrade.
	specifiedRepoList := proj.SelectRepos(predicate)

	// Repos that depend on any specified repos must also be considered during
	// the install / upgrade operation.
	repoList := proj.ensureDepsInList(specifiedRepoList, nil)

	// Construct a table of all published repo versions.
	m, err := deprepo.BuildMatrix(
		repoList, proj.projState.AllInstalledVersions())
	if err != nil {
		return err
	}

	// If the `project.yml` file specifies an invalid repo version, abort now.
	if err := proj.detectIllegalRepoReqs(repoList, m); err != nil {
		return err
	}

	// Remove blocked repo versions from the table.
	if err := deprepo.PruneMatrix(
		&m, proj.repos, proj.rootRepoReqs); err != nil {

		return err
	}

	// Construct a repo dependency graph from the `project.yml` version
	// requirements and from each repo's dependency list.
	dg, err := deprepo.BuildDepGraph(proj.repos, proj.rootRepoReqs)
	if err != nil {
		return err
	}

	// Try to find a version set that satisfies the dependency graph.  If no
	// such set exists, report the conflicts and abort.
	vm, conflicts := deprepo.FindAcceptableVersions(m, dg)
	if vm == nil {
		return deprepo.ConflictError(conflicts)
	}

	// Now that we know which repo versions we want, we can eliminate some
	// false-positives from the repo list.
	repoList = proj.ensureDepsInList(specifiedRepoList, vm)

	// Perform some additional filtering on the list of repos to process.
	if upgrade {
		// Don't upgrade a repo if we already have the desired version.
		repoList = proj.filterUpgradeList(repoList, vm)
	} else if !force {
		// Don't install a repo if it is already installed (any version).  We
		// skip this filter for forced reinstalls.
		repoList = proj.filterInstallList(repoList)
	}

	// Notify the user of what install operations are about to happen, and
	// prompt if the `-a` (ask) option was specified.
	var op installOp
	if upgrade {
		op = INSTALL_OP_UPGRADE
	} else {
		op = INSTALL_OP_INSTALL
	}
	proceed, err := proj.installPrompt(repoList, vm, op, force, ask)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	// For a forced install, delete all existing repos.
	if !upgrade && force {
		for _, r := range repoList {
			// Don't delete the local project directory!
			if !r.IsLocal() {
				util.StatusMessage(util.VERBOSITY_DEFAULT,
					"Removing old copy of \"%s\" (%s)\n", r.Name(), r.Path())
				os.RemoveAll(r.Path())
				proj.projState.Delete(r.Name())
			}
		}
	}

	// Install or upgrade each repo in the list.
	for _, r := range repoList {
		destVer := vm[r.Name()]
		if err := proj.installRepo(r, destVer, upgrade, force); err != nil {
			return err
		}
	}

	// Save the project state, including any updates or changes to the project
	// information that either install or upgrade caused.
	if err := proj.projState.Save(); err != nil {
		return err
	}

	return nil
}

// Syncs (i.e., git pulls) repos matching the specified predicate.
func (proj *Project) SyncIf(
	force bool, ask bool, predicate func(r *repo.Repo) bool) error {

	// Make sure we have an up to date copy of all `repository.yml` files.
	if err := proj.downloadRepositoryYmlFiles(); err != nil {
		return err
	}

	// A sync operation never changes repo versions in the state file, so we
	// can proceed with the currently-installed versions.
	vm := proj.projState.AllInstalledVersions()

	// Determine which repos the user wants to sync.
	repoList := proj.SelectRepos(predicate)

	// Repos that depend on any specified repos must also be considered during
	// the sync operation.
	repoList = proj.ensureDepsInList(repoList, vm)

	// Notify the user of what install operations are about to happen, and
	// prompt if the `-a` (ask) option was specified.
	proceed, err := proj.installPrompt(
		repoList, vm, INSTALL_OP_SYNC, force, ask)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	// Sync each repo in the list.
	var anyFails bool
	for _, r := range repoList {
		ver, ok := vm[r.Name()]
		if !ok {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"No installed version of %s found, skipping\n",
				r.Name())
		} else {
			if _, err := r.Sync(ver, force); err != nil {
				util.StatusMessage(util.VERBOSITY_QUIET,
					"Failed to sync repo \"%s\": %s\n",
					r.Name(), err.Error())
				anyFails = true
			}
		}
	}

	if anyFails {
		var forceMsg string
		if !force {
			forceMsg = ".  To force resync, add the -f (force) option."
		}
		return util.FmtNewtError("Failed to sync%s", forceMsg)
	}

	return nil
}

// Loads a complete repo definition from the appropriate `repository.yml` file.
// The supplied fields form a basic repo description as read from `project.yml`
// or from another repo's dependency list.
//
// @param name                  The name of the repo to read.
// @param fields                Fields containing the basic repo description.
//
// @return *Repo                The fully-read repo on success; nil on failure.
// @return error                Error on failure.
func (proj *Project) loadRepo(name string, fields map[string]string) (
	*repo.Repo, error) {

	// First, read the repo description from the supplied fields.
	if fields["type"] == "" {
		return nil,
			util.FmtNewtError("Missing type for repository %s", name)
	}

	dl, err := downloader.LoadDownloader(name, fields)
	if err != nil {
		return nil, err
	}

	// Construct a new repo object from the basic description information.
	r, err := repo.NewRepo(name, dl)
	if err != nil {
		return nil, err
	}

	for _, ignDir := range ignoreSearchDirs {
		r.AddIgnoreDir(ignDir)
	}

	// Read the full repo definition from its `repository.yml` file.
	if err := r.Read(); err != nil {
		return r, err
	}

	// Warn the user about incompatibilities with this version of newt.
	ver := proj.projState.GetInstalledVersion(name)
	if ver != nil {
		code, msg := r.CheckNewtCompatibility(*ver, newtutil.NewtVersion)
		switch code {
		case compat.NEWT_COMPAT_GOOD:
		case compat.NEWT_COMPAT_WARN:
			util.StatusMessage(util.VERBOSITY_QUIET, "WARNING: %s.\n", msg)
		case compat.NEWT_COMPAT_ERROR:
			return nil, util.NewNewtError(msg)
		}
	}

	// XXX: This log message assumes a "github" type repo.
	log.Debugf("Loaded repository %s (type: %s, user: %s, repo: %s)", name,
		fields["type"], fields["user"], fields["repo"])

	return r, nil
}

func (proj *Project) checkNewtVer() error {
	compatSms := proj.yc.GetValStringMapString(
		"project.newt_compatibility", nil)

	// If this project doesn't have a newt compatibility map, just assume there
	// is no incompatibility.
	if len(compatSms) == 0 {
		return nil
	}

	tbl, err := compat.ParseNcTable(compatSms)
	if err != nil {
		return util.FmtNewtError("Error reading project.yml: %s", err.Error())
	}

	code, msg := tbl.CheckNewtVer(newtutil.NewtVersion)
	msg = fmt.Sprintf("This version of newt (%s) is incompatible with "+
		"your project; %s", newtutil.NewtVersion.String(), msg)

	switch code {
	case compat.NEWT_COMPAT_GOOD:
		return nil
	case compat.NEWT_COMPAT_WARN:
		util.StatusMessage(util.VERBOSITY_QUIET, "WARNING: %s.\n", msg)
		return nil
	case compat.NEWT_COMPAT_ERROR:
		return util.NewNewtError(msg)
	default:
		return nil
	}
}

// Loads the `repository.yml` file for each depended-on repo.  This
func (proj *Project) loadRepoDeps(download bool) error {
	seen := map[string]struct{}{}

	loadDeps := func(r *repo.Repo) ([]*repo.Repo, error) {
		var newRepos []*repo.Repo

		depMap := r.BranchDepMap()
		for _, depSlice := range depMap {
			for _, dep := range depSlice {
				if _, ok := seen[dep.Name]; !ok {
					seen[r.Name()] = struct{}{}

					depRepo := proj.repos[dep.Name]
					if depRepo == nil {
						depRepo, _ = proj.loadRepo(dep.Name, dep.Fields)
						proj.repos[dep.Name] = depRepo
					}
					newRepos = append(newRepos, depRepo)

					if download {
						if _, err := depRepo.UpdateDesc(); err != nil {
							return nil, err
						}
					}
				}
			}
		}

		return newRepos, nil
	}

	curRepos := proj.repos.Sorted()
	for len(curRepos) > 0 {
		var nextRepos []*repo.Repo

		for _, r := range curRepos {
			depRepos, err := loadDeps(r)
			if err != nil {
				return err
			}

			nextRepos = append(nextRepos, depRepos...)
		}

		curRepos = nextRepos
	}

	return nil
}

func (proj *Project) downloadRepositoryYmlFiles() error {
	// Download the `repository.yml` file for each root-level repo (those
	// specified in the `project.yml` file).
	for _, r := range proj.repos.Sorted() {
		if !r.IsLocal() {
			if _, err := r.UpdateDesc(); err != nil {
				return err
			}
		}
	}

	// Download the `repository.yml` file for each depended-on repo.
	if err := proj.loadRepoDeps(true); err != nil {
		return err
	}

	return nil
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
func (proj *Project) ensureDepsInList(repos []*repo.Repo,
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
			depRepo := proj.repos[d.Name]
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

func (proj *Project) loadConfig() error {
	yc, err := newtutil.ReadConfig(proj.BasePath,
		strings.TrimSuffix(PROJECT_FILE_NAME, ".yml"))
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	// Store configuration object for access to future values,
	// this avoids keeping every string around as a project variable when
	// we need to process it later.
	proj.yc = yc

	proj.projState, err = LoadProjectState()
	if err != nil {
		return err
	}

	proj.name = yc.GetValString("project.name", nil)

	// Local repository always included in initialization
	r, err := repo.NewLocalRepo(proj.name)
	if err != nil {
		return err
	}

	proj.repos[proj.name] = r
	proj.localRepo = r
	for _, ignDir := range ignoreSearchDirs {
		r.AddIgnoreDir(ignDir)
	}

	// Assume every item starting with "repository." is a repository descriptor
	// and try to load it.
	for k, _ := range yc.AllSettings() {
		repoName := strings.TrimPrefix(k, "repository.")
		if repoName != k {
			fields := yc.GetValStringMapString(k, nil)
			r, _ := proj.loadRepo(repoName, fields)

			verReqs, err := newtutil.ParseRepoVersionReqs(fields["vers"])
			if err != nil {
				return util.FmtNewtError(
					"Repo \"%s\" contains invalid version requirement: %s (%s)",
					repoName, fields["vers"], err.Error())
			}

			proj.repos[repoName] = r
			proj.rootRepoReqs[repoName] = verReqs
		}
	}

	// Read `repository.yml` files belonging to dependee repos from disk.
	// These repos might not be specified in the `project.yml` file, but they
	// are still part of the project.
	if err := proj.loadRepoDeps(false); err != nil {
		return err
	}

	ignoreDirs := yc.GetValStringSlice("project.ignore_dirs", nil)
	for _, ignDir := range ignoreDirs {
		repoName, dirName, err := newtutil.ParsePackageString(ignDir)
		if err != nil {
			return err
		}
		if repoName == "" {
			r = proj.LocalRepo()
		} else {
			r = proj.FindRepo(repoName)
		}
		if r == nil {
			return util.NewNewtError(
				fmt.Sprintf("ignore_dirs: unknown repo %s", repoName))
		}
		r.AddIgnoreDir(dirName)
	}

	if err := proj.checkNewtVer(); err != nil {
		return err
	}

	return nil
}

func (proj *Project) Init(dir string) error {
	proj.BasePath = filepath.ToSlash(filepath.Clean(dir))

	// Only one project per system, when created, set it as the global project
	interfaces.SetProject(proj)

	proj.repos = map[string]*repo.Repo{}
	proj.rootRepoReqs = map[string][]newtutil.RepoVersionReq{}

	// Load Project configuration
	if err := proj.loadConfig(); err != nil {
		return err
	}

	return nil
}

func (proj *Project) ResolveDependency(dep interfaces.DependencyInterface) interfaces.PackageInterface {
	type NamePath struct {
		name string
		path string
	}

	for _, pkgList := range proj.packages {
		for _, pkg := range *pkgList {
			if dep.SatisfiesDependency(pkg) {
				return pkg
			}
		}
	}

	return nil
}

func (proj *Project) ResolvePackage(
	dfltRepo interfaces.RepoInterface, name string) (*pkg.LocalPackage, error) {
	// Trim trailing slash from name.  This is necessary when tab
	// completion is used to specify the name.
	name = strings.TrimSuffix(name, "/")

	repoName, pkgName, err := newtutil.ParsePackageString(name)
	if err != nil {
		return nil, util.FmtNewtError("invalid package name: %s (%s)", name,
			err.Error())
	}

	var repo interfaces.RepoInterface
	if repoName == "" {
		repo = dfltRepo
	} else if proj.repos[repoName] != nil {
		repo = proj.repos[repoName]
	} else {
		return nil, util.FmtNewtError("invalid package name: %s (unkwn repo %s)",
			name, repoName)
	}
	dep, err := pkg.NewDependency(repo, pkgName)
	if err != nil {
		return nil, util.FmtNewtError("invalid package name: %s (%s)", name,
			err.Error())
	}
	if dep == nil {
		return nil, util.NewNewtError("invalid package name: " + name)
	}
	pack := proj.ResolveDependency(dep)
	if pack == nil {
		return nil, util.NewNewtError("unknown package: " + name)
	}

	return pack.(*pkg.LocalPackage), nil
}

// Resolves a path with an optional repo prefix (e.g., "@apache-mynewt-core").
func (proj *Project) ResolvePath(
	basePath string, name string) (string, error) {

	repoName, subPath, err := newtutil.ParsePackageString(name)
	if err != nil {
		return "", util.FmtNewtError("invalid path: %s (%s)", name,
			err.Error())
	}

	if repoName == "" {
		return basePath + "/" + subPath, nil
	} else {
		repo := proj.repos[repoName]
		if repo == nil {
			return "", util.FmtNewtError("Unknown repository: %s", repoName)
		}

		return repo.Path() + "/" + subPath, nil
	}
}

func findProjectDir(dir string) (string, error) {
	for {
		projFile := path.Clean(dir) + "/" + PROJECT_FILE_NAME

		log.Debugf("Searching for project file %s", projFile)
		if util.NodeExist(projFile) {
			break
		}

		// Move back one directory and continue searching
		dir = path.Clean(dir + "../../")
		// path.Clean returns . if processing results in empty string.
		// Need to check for . on Windows.
		if dir == "/" || dir == "." {
			return "", util.NewNewtError("No project file found!")
		}
	}

	return dir, nil
}

func (proj *Project) loadPackageList() error {
	proj.packages = interfaces.PackageList{}

	// Go through a list of repositories, starting with local, and search for
	// packages / store them in the project package list.
	repos := proj.Repos()
	for name, repo := range repos {
		list, warnings, err := pkg.ReadLocalPackages(repo, repo.Path())
		if err != nil {
			/* Failed to read the repo's package list.  Report the failure as a
			 * warning if the project state indicates that this repo should be
			 * installed.
			 */
			if _, ok := proj.projState.installedRepos[name]; ok {
				util.StatusMessage(util.VERBOSITY_QUIET, err.Error()+"\n")
			}
		} else {
			proj.packages[name] = list
		}

		proj.warnings = append(proj.warnings, warnings...)
	}

	return nil
}

func (proj *Project) PackageList() interfaces.PackageList {
	return proj.packages
}

func (proj *Project) PackagesOfType(pkgType interfaces.PackageType) []interfaces.PackageInterface {
	matches := []interfaces.PackageInterface{}

	packs := proj.PackageList()
	for _, packHash := range packs {
		for _, pack := range *packHash {
			if pkgType == -1 || pack.Type() == pkgType {
				matches = append(matches, pack)
			}
		}
	}

	return matches
}

func LoadProject(dir string) (*Project, error) {
	projDir, err := findProjectDir(dir)
	if err != nil {
		return nil, err
	}

	proj, err := NewProject(projDir)

	return proj, err
}
