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
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/newt/compat"
	"mynewt.apache.org/newt/newt/config"
	"mynewt.apache.org/newt/newt/deprepo"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/install"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

var globalProject *Project = nil

const PROJECT_FILE_NAME = "project.yml"
const PATCHES_DIR = "patches"

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

	// Contains all the repos that form this project.  Each repo is in one of
	// two states:
	//    * description: Only the repo's basic description fields have been
	//                   read from `project.yml` or from a dependent repo's
	//                   `repository.yml` file.  This repo's `repository.yml`
	//                   file still needs to be read.
	//    * complete: The repo's `repository.yml` file exists and has been
	//                read.
	repos deprepo.RepoMap

	// Contains names of repositories that will be upgraded.
	// If it's empty all repos are allowed.
	reposAllowedRe []*regexp.Regexp

	// Contains names of repositories that will be excluded from upgrade.
	// Can override repositories from reposAllowed.
	reposIgnoredRe []*regexp.Regexp

	// The local repository at the top-level of the project.  This repo is
	// excluded from most repo operations.
	localRepo *repo.Repo

	// Required versions of installed repos, as read from `project.yml`.
	rootRepoReqs deprepo.RequirementMap

	warnings []string

	// Indicates the repos whose version we couldn't detect.  Prevents
	// duplicate warnings.
	unknownRepoVers map[string]struct{}

	yc ycfg.YCfg
}

func initProject(dir string, download bool) error {
	var err error

	globalProject, err = LoadProject(dir, download)
	if err != nil {
		return err
	}

	if download {
		err = globalProject.UpgradeIf(newtutil.NewtForce, newtutil.NewtAsk,
			func(r *repo.Repo) bool { return !r.IsExternal(r.Path()) })
		if err != nil {
			return err
		}
	}

	if err := globalProject.loadPackageList(); err != nil {
		return err
	}

	return nil
}

func initialize(download bool) error {
	if globalProject == nil {
		wd, err := os.Getwd()
		wd = filepath.ToSlash(wd)
		if err != nil {
			return util.NewNewtError(err.Error())
		}
		if err := initProject(wd, download); err != nil {
			return err
		}
	}
	return nil
}

func TryGetProject() (*Project, error) {
	if err := initialize(false); err != nil {
		return nil, err
	}
	return globalProject, nil
}

func TryGetOrDownloadProject() (*Project, error) {
	if err := initialize(true); err != nil {
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

func NewProject(dir string, download bool) (*Project, error) {
	proj := &Project{}

	if err := proj.Init(dir, download); err != nil {
		return nil, err
	}

	return proj, nil
}

func (proj *Project) patternsMatch(patterns *[]*regexp.Regexp, repoName string) bool {
	for _, re := range *patterns {
		if re.MatchString(repoName) {
			return true
		}
	}

	return false
}

func (proj *Project) isRepoIgnored(repoName string) bool {
	return proj.patternsMatch(&proj.reposIgnoredRe, repoName)
}

func (proj *Project) isRepoAllowed(repoName string) bool {
	if (len(proj.reposAllowedRe) == 0) || proj.patternsMatch(&proj.reposAllowedRe, repoName) {
		return !proj.patternsMatch(&proj.reposIgnoredRe, repoName)
	}

	return false
}

func (proj *Project) isRepoAdded(repoName string) bool {
	for _, pr := range proj.repos {
		if pr.Name() == repoName {
			return true
		}
	}
	return false
}

func (proj *Project) GetPkgRepos() error {
	for _, pkgList := range proj.packages {
		for _, pkg := range *pkgList {
			if pkg.PkgConfig().HasKey("repository") {
				for k, _ := range pkg.PkgConfig().AllSettings() {
					repoName := strings.TrimPrefix(k, "repository.")
					if repoName != k {
						fields, err := pkg.PkgConfig().GetValStringMapString(k, nil)
						util.OneTimeWarningError(err)

						if !proj.isRepoAllowed(repoName) {
							continue
						}

						r, err := proj.loadRepo(repoName, fields)
						if err != nil {
							// if `repository.yml` does not exist, it is not an error; we
							// will just download a new copy.
							if !util.IsNotExist(err) {
								return err
							}
						}
						if r == nil {
							continue
						}

						verReq, err := newtutil.ParseRepoVersion(fields["vers"])
						if err != nil {
							return util.FmtNewtError(
								"Repo \"%s\" contains invalid version requirement: "+
									"%s (%s)",
								repoName, fields["vers"], err.Error())
						}
						r.SetPkgName(pkg.Name())

						if !proj.isRepoAdded(r.Name()) {
							if err := proj.addRepo(r, true); err != nil {
								return err
							}
							proj.rootRepoReqs[repoName] = verReq
						}

						if _, err := os.Stat(pkg.BasePath() + "/" + PATCHES_DIR + "/" + r.Name()); os.IsNotExist(err) {
							continue
						} else {
							dirEntries, err := os.ReadDir(pkg.BasePath() + "/" + PATCHES_DIR + "/" + r.Name())
							if err != nil {
								return err
							}

							for _, e := range dirEntries {
								if strings.HasSuffix(e.Name(), ".patch") {
									r.AddPatch(pkg.BasePath() + "/" + PATCHES_DIR + "/" + r.Name() + "/" + e.Name())
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func (proj *Project) SetGitEnvVariables() error {
	err := os.Setenv("GIT_COMMITTER_NAME", "newt")
	if err != nil {
		return err
	}

	err = os.Setenv("GIT_COMMITTER_EMAIL", "dev@mynewt.apache.org")
	if err != nil {
		return err
	}
	return nil
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

func (proj *Project) GetRepoVersion(
	rname string) (*newtutil.RepoVersion, error) {

	r := proj.repos[rname]
	if r == nil {
		return nil, nil
	}

	ver, err := r.InstalledVersion()
	if err != nil {
		return nil, err
	}

	if ver == nil {
		commit, err := r.CurrentHash()
		if err != nil {
			return nil, err
		}
		if proj.unknownRepoVers == nil {
			proj.unknownRepoVers = map[string]struct{}{}
		}

		if _, ok := proj.unknownRepoVers[rname]; !ok {
			proj.unknownRepoVers[rname] = struct{}{}
		}
		ver = &newtutil.RepoVersion{
			Commit: commit,
		}
	}

	return ver, nil
}

// XXX: Incorrect comment.
// Indicates whether the specified repo is present in the `project.state` file.
func (proj *Project) RepoIsInstalled(rname string) bool {
	ver, err := proj.GetRepoVersion(rname)
	return err == nil && ver != nil
}

func (proj *Project) RepoIsRoot(rname string) bool {
	_, ok := proj.rootRepoReqs[rname]
	return ok == true
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

// Installs or upgrades repos matching the specified predicate.
func (proj *Project) UpgradeIf(
	force bool, ask bool, predicate func(r *repo.Repo) bool) error {

	// Make sure we have an up to date copy of all `repository.yml` files.
	if err := proj.downloadRepositoryYmlFiles(); err != nil {
		return err
	}

	// Now that all repos have been successfully fetched, we can finish the
	// install procedure locally.

	// Determine which repos the user wants to install or upgrade.
	specifiedRepoList := proj.SelectRepos(predicate)

	inst, err := install.NewInstaller(proj.repos, proj.rootRepoReqs)
	if err != nil {
		return err
	}

	return inst.Upgrade(specifiedRepoList, force, ask)
}

func (proj *Project) InfoIf(predicate func(r *repo.Repo) bool,
	remote bool) error {

	if remote {
		// Make sure we have an up to date copy of all `repository.yml` files.
		if err := proj.downloadRepositoryYmlFiles(); err != nil {
			return err
		}
	}

	// Determine which repos the user wants info about.
	repoList := proj.SelectRepos(predicate)

	// Ignore errors.  We will deal with bad repos individually when we display
	// info about them.
	inst, _ := install.NewInstaller(proj.repos, proj.rootRepoReqs)
	if err := inst.Info(repoList, remote); err != nil {
		return err
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
// @return *Repo                The fully-read repo on success; nil on failure or when repo is not allowed
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

	// XXX: This log message assumes a "github" type repo.
	log.Debugf("Loaded repository %s (type: %s, user: %s, repo: %s)", name,
		fields["type"], fields["user"], fields["repo"])

	return r, nil
}

func (proj *Project) checkNewtVer() error {
	compatSms, err := proj.yc.GetValStringMapString(
		"project.newt_compatibility", nil)
	util.OneTimeWarningError(err)

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
		util.OneTimeWarning("%s", msg)
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

		depMap := r.CommitDepMap()
		for _, depSlice := range depMap {
			for _, dep := range depSlice {
				if _, ok := seen[dep.Name]; !ok {
					seen[r.Name()] = struct{}{}

					depRepo := proj.repos[dep.Name]
					if depRepo == nil {
						var err error

						if !proj.isRepoAllowed(dep.Name) {
							continue
						}

						depRepo, err = proj.loadRepo(dep.Name, dep.Fields)
						if err != nil {
							// if `repository.yml` does not exist, it is not an
							// error; we will just download a new copy.
							if !util.IsNotExist(err) {
								return nil, err
							}
						}
						if depRepo == nil {
							continue
						}
						if err := proj.addRepo(depRepo, download); err != nil {
							return nil, err
						}
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
		if r.IsUpdated() {
			continue
		}

		if r.IsLocal() {
			continue
		}

		if r.IsExternal(r.Path()) {
			ver := proj.rootRepoReqs[r.Name()]

			// External repositories can only use commit stability since they do
			// not have repository.yml
			if len(ver.Commit) == 0 {
				return util.FmtNewtError(
					"External repository \"%s\" does not specify valid commit version (%s)",
					r.Name(), ver.String())
			}

			// No need to fetch if requested commit is already checked out
			if r.IsHeadCommit(ver.Commit) {
				continue
			}
		}

		if _, err := r.UpdateDesc(); err != nil {
			return err
		}
	}

	// Download the `repository.yml` file for each depended-on repo.
	if err := proj.loadRepoDeps(true); err != nil {
		return err
	}

	return nil
}

func (proj *Project) verifyNewtCompat() error {
	var errors []string

	for name, r := range proj.repos {
		// If a repo doesn't have a downloader then it is
		// a project root that is not a repository
		if r.Downloader() == nil {
			continue
		}

		// Cannot verify version if project is not installed
		if !proj.RepoIsInstalled(name) {
			continue
		}

		ver, err := proj.GetRepoVersion(name)
		if err != nil {
			return err
		}

		if ver != nil {
			code, msg := r.CheckNewtCompatibility(*ver, newtutil.NewtVersion)
			switch code {
			case compat.NEWT_COMPAT_GOOD:
			case compat.NEWT_COMPAT_WARN:
				util.OneTimeWarning("%s", msg)
			case compat.NEWT_COMPAT_ERROR:
				errors = append(errors, msg)
			}
		}
	}

	if errors != nil {
		return util.NewNewtError(strings.Join(errors, "\n"))
	}

	return nil
}

// addRepo Adds an entry to the project's repo map.  It clones the repo if it
// does not exist locally.
func (proj *Project) addRepo(r *repo.Repo, download bool) error {
	if download {
		if err := r.EnsureExists(); err != nil {
			return err
		}
	} else {
		if !r.CheckExists() {
			return util.NewNewtError(
				fmt.Sprintf(
					"Repo \"%s\" is not installed, please run `newt upgrade`!",
					r.Name()))
		}
	}

	proj.repos[r.Name()] = r
	return nil
}

func (proj *Project) createRegexpPatterns(patterns []string) ([]*regexp.Regexp, error) {
	var ret []*regexp.Regexp
	var errLines []string

	for _, pattern := range patterns {
		var s string

		if strings.HasPrefix(pattern, "~") {
			s = "^" + pattern[1:]
		} else if strings.HasSuffix(pattern, "*") {
			s = "^" + pattern[:len(pattern)-1] + ".*$"
		} else {
			s = "^" + pattern + "$"
		}

		re, err := regexp.Compile(s)
		if err != nil {
			errLines = append(errLines, fmt.Sprintf("Invalid pattern: %s", pattern))
		} else {
			ret = append(ret, re)
		}
	}

	if len(errLines) > 0 {
		return ret, util.NewNewtError(strings.Join(errLines, "\n"))
	} else {
		return ret, nil
	}
}

func (proj *Project) loadConfig(download bool) error {
	yc, err := config.ReadFile(proj.BasePath + "/" + PROJECT_FILE_NAME)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	// Store configuration object for access to future values,
	// this avoids keeping every string around as a project variable when
	// we need to process it later.
	proj.yc = yc

	proj.name, err = yc.GetValString("project.name", nil)
	util.OneTimeWarningError(err)

	var reposAllowed []string
	var reposIgnored []string

	reposAllowed, err = yc.GetValStringSlice("project.repositories.allowed", nil)
	util.OneTimeWarningError(err)
	proj.reposAllowedRe, err = proj.createRegexpPatterns(util.UniqueStrings(reposAllowed))
	util.OneTimeWarningError(err)

	reposIgnored, err = yc.GetValStringSlice("project.repositories.ignored", nil)
	util.OneTimeWarningError(err)
	reposIgnored = append(reposIgnored, newtutil.NewtIgnore...)
	proj.reposIgnoredRe, err = proj.createRegexpPatterns(util.UniqueStrings(reposIgnored))
	util.OneTimeWarningError(err)

	if proj.isRepoIgnored("apache-mynewt-core") {
		return util.NewNewtError("apache-mynewt-core repository cannot be on ignored list.")
	}

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
			fields, err := yc.GetValStringMapString(k, nil)
			util.OneTimeWarningError(err)

			if proj.isRepoIgnored(repoName) {
				continue
			}

			r, err := proj.loadRepo(repoName, fields)
			if err != nil {
				// if `repository.yml` does not exist, it is not an error; we
				// will just download a new copy.
				if !util.IsNotExist(err) {
					return err
				}
			}
			if r == nil {
				continue
			}

			verReq, err := newtutil.ParseRepoVersion(fields["vers"])
			if err != nil {
				return util.FmtNewtError(
					"Repo \"%s\" contains invalid version requirement: "+
						"%s (%s)",
					repoName, fields["vers"], err.Error())
			}

			if err := proj.addRepo(r, download); err != nil {
				return err
			}
			proj.rootRepoReqs[repoName] = verReq
		}
	}

	// Read `repository.yml` files belonging to dependee repos from disk.
	// These repos might not be specified in the `project.yml` file, but they
	// are still part of the project.
	if err := proj.loadRepoDeps(download); err != nil {
		return err
	}

	if !util.SkipNewtCompat {
		// Warn the user about incompatibilities with this version of newt.
		if err := proj.verifyNewtCompat(); err != nil {
			return err
		}
	}

	ignoreDirs, err := yc.GetValStringSlice("project.ignore_dirs", nil)
	util.OneTimeWarningError(err)
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

func (proj *Project) Init(dir string, download bool) error {
	proj.BasePath = filepath.ToSlash(filepath.Clean(dir))

	// Only one project per system, when created, set it as the global project
	interfaces.SetProject(proj)

	proj.repos = map[string]*repo.Repo{}
	proj.rootRepoReqs = map[string]newtutil.RepoVersion{}

	// Load Project configuration
	if err := proj.loadConfig(download); err != nil {
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
		if err == nil {
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

func LoadProject(dir string, download bool) (*Project, error) {
	projDir, err := findProjectDir(dir)
	if err != nil {
		return nil, err
	}

	proj, err := NewProject(projDir, download)

	return proj, err
}
