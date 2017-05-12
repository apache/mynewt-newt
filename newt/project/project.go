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
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/compat"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
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

	// Repositories configured on this project
	repos    map[string]*repo.Repo
	warnings []string

	localRepo *repo.Repo

	v *viper.Viper
}

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

func (proj *Project) LocalRepo() *repo.Repo {
	return proj.localRepo
}

func (proj *Project) Warnings() []string {
	return proj.warnings
}

func (proj *Project) upgradeCheck(r *repo.Repo, vers *repo.Version,
	force bool) (bool, error) {
	rdesc, err := r.GetRepoDesc()
	if err != nil {
		return false, err
	}

	branch, newVers, _ := rdesc.Match(r)
	if newVers == nil {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"No matching version to upgrade to "+
				"found for %s.  Please check your project requirements.",
			r.Name())
		return false, util.NewNewtError(fmt.Sprintf("Cannot find a "+
			"version of repository %s that matches project requirements.",
			r.Name()))
	}

	// If the change between the old repository and the new repository would cause
	// and upgrade.  Then prompt for an upgrade response, unless the force option
	// is present.
	if vers.CompareVersions(newVers, vers) != 0 ||
		vers.Stability() != newVers.Stability() {
		if !force {
			str := ""
			if newVers.Stability() != repo.VERSION_STABILITY_NONE {
				str += "(" + branch + ")"
			}

			fmt.Printf("Would you like to upgrade repository %s from %s to %s %s? [Yn] ",
				r.Name(), vers.String(), newVers.String(), str)
			line, more, err := bufio.NewReader(os.Stdin).ReadLine()
			if more || err != nil {
				return false, util.NewNewtError(fmt.Sprintf(
					"Couldn't read upgrade response: %s\n", err.Error()))
			}

			// Anything but no means yes.
			answer := strings.ToUpper(strings.Trim(string(line), " "))
			if answer == "N" || answer == "NO" {
				fmt.Printf("User says don't upgrade, skipping upgrade of %s\n",
					r.Name())
				return true, nil
			}
		}
	} else {
		util.StatusMessage(util.VERBOSITY_VERBOSE,
			"Repository %s doesn't need to be upgraded, latest "+
				"version installed.\n", r.Name())
		return true, nil
	}

	return false, nil
}

func (proj *Project) checkVersionRequirements(r *repo.Repo, upgrade bool, force bool) (bool, error) {
	rdesc, err := r.GetRepoDesc()
	if err != nil {
		return false, err
	}

	rname := r.Name()

	vers := proj.projState.GetInstalledVersion(rname)
	if vers != nil {
		ok := rdesc.SatisfiesVersion(vers, r.VersionRequirements())
		if !ok && !upgrade {
			util.StatusMessage(util.VERBOSITY_QUIET, "WARNING: Installed "+
				"version %s of repository %s does not match desired "+
				"version %s in project file.  You can fix this by either upgrading"+
				" your repository, or modifying the project.yml file.\n",
				vers, rname, r.VersionRequirementsString())
			return true, err
		} else {
			if !upgrade {
				util.StatusMessage(util.VERBOSITY_VERBOSE, "%s correct version already installed\n", r.Name())
				return true, nil
			} else {
				skip, err := proj.upgradeCheck(r, vers, force)
				return skip, err
			}
		}
	} else {
		// Fallthrough and perform the installation.
		// Check to make sure that this repository contains a version
		// that can satisfy.
		_, _, ok := rdesc.Match(r)
		if !ok {
			fmt.Printf("WARNING: No matching repository version found for repository "+
				"%s specified in project.\n", r.Name())
			return true, err
		}
	}

	return false, nil
}

func (proj *Project) checkDeps(r *repo.Repo) error {
	repos, updated, err := r.UpdateDesc()
	if err != nil {
		return err
	}

	if !updated {
		return nil
	}

	for _, newRepo := range repos {
		curRepo, ok := proj.repos[newRepo.Name()]
		if !ok {
			proj.repos[newRepo.Name()] = newRepo
			return proj.UpdateRepos()
		} else {
			// Add any dependencies we might have found here.
			for _, dep := range newRepo.Deps() {
				newRepo.DownloadDesc()
				newRepo.ReadDesc()
				curRepo.AddDependency(dep)
			}
		}
	}

	return nil
}

func (proj *Project) UpdateRepos() error {
	repoList := proj.Repos()
	for _, r := range repoList {
		if r.IsLocal() {
			continue
		}

		err := proj.checkDeps(r)
		if err != nil {
			return err
		}
	}
	return nil
}

func (proj *Project) Install(upgrade bool, force bool) error {
	repoList := proj.Repos()

	for rname, _ := range repoList {
		// Ignore the local repo on install
		if rname == repo.REPO_NAME_LOCAL {
			continue
		}

		// First thing we do is update repository description.  This
		// will get us available branches and versions in the repository.
		if err := proj.UpdateRepos(); err != nil {
			return err
		}
	}

	// Get repository list, and print every repo and it's dependencies.
	if err := repo.CheckDeps(upgrade, proj.Repos()); err != nil {
		return err
	}

	for rname, r := range proj.Repos() {
		if r.IsLocal() {
			continue
		}
		// Check the version requirements on this repository, and see
		// whether or not we need to install/upgrade it.
		skip, err := proj.checkVersionRequirements(r, upgrade, force)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		// Do the hard work of actually copying and installing the repository.
		rvers, err := r.Install(upgrade || force)
		if err != nil {
			return err
		}

		if upgrade {
			util.StatusMessage(util.VERBOSITY_VERBOSE, "%s successfully upgraded to version %s\n",
				r.Name(), rvers.String())
		} else {
			util.StatusMessage(util.VERBOSITY_VERBOSE, "%s successfully installed version %s\n",
				r.Name(), rvers.String())
		}

		// Update the project state with the new repository version information.
		proj.projState.Replace(rname, rvers)
	}

	// Save the project state, including any updates or changes to the project
	// information that either install or upgrade caused.
	if err := proj.projState.Save(); err != nil {
		return err
	}

	return nil
}

func (proj *Project) Upgrade(force bool) error {
	return proj.Install(true, force)
}

func (proj *Project) loadRepo(rname string, v *viper.Viper) error {
	varName := fmt.Sprintf("repository.%s", rname)

	repoVars := v.GetStringMapString(varName)
	if len(repoVars) == 0 {
		return util.NewNewtError(fmt.Sprintf("Missing configuration for "+
			"repository %s.", rname))
	}
	if repoVars["type"] == "" {
		return util.NewNewtError(fmt.Sprintf("Missing type for repository " +
			rname))
	}

	dl, err := downloader.LoadDownloader(rname, repoVars)
	if err != nil {
		return err
	}

	rversreq := repoVars["vers"]
	r, err := repo.NewRepo(rname, rversreq, dl)
	if err != nil {
		return err
	}

	for _, ignDir := range ignoreSearchDirs {
		r.AddIgnoreDir(ignDir)
	}

	rd, err := repo.NewRepoDependency(rname, rversreq)
	if err != nil {
		return err
	}
	rd.Storerepo = r

	proj.localRepo.AddDependency(rd)

	// Read the repo's descriptor file so that we have its newt version
	// compatibility map.
	r.ReadDesc()

	rvers := proj.projState.GetInstalledVersion(rname)
	code, msg := r.CheckNewtCompatibility(rvers, newtutil.NewtVersion)
	switch code {
	case compat.NEWT_COMPAT_GOOD:
	case compat.NEWT_COMPAT_WARN:
		util.StatusMessage(util.VERBOSITY_QUIET, "WARNING: %s.\n", msg)
	case compat.NEWT_COMPAT_ERROR:
		return util.NewNewtError(msg)
	}

	log.Debugf("Loaded repository %s (type: %s, user: %s, repo: %s)", rname,
		repoVars["type"], repoVars["user"], repoVars["repo"])

	proj.repos[r.Name()] = r
	return nil
}

func (proj *Project) checkNewtVer() error {
	compatSms := proj.v.GetStringMapString("project.newt_compatibility")
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

func (proj *Project) loadConfig() error {
	v, err := util.ReadConfig(proj.BasePath,
		strings.TrimSuffix(PROJECT_FILE_NAME, ".yml"))
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	// Store configuration object for access to future values,
	// this avoids keeping every string around as a project variable when
	// we need to process it later.
	proj.v = v

	proj.projState, err = LoadProjectState()
	if err != nil {
		return err
	}

	proj.name = v.GetString("project.name")

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

	rstrs := v.GetStringSlice("project.repositories")
	for _, repoName := range rstrs {
		if err := proj.loadRepo(repoName, v); err != nil {
			return err
		}
	}

	ignoreDirs := v.GetStringSlice("project.ignore_dirs")
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

	// Load Project configuration
	if err := proj.loadConfig(); err != nil {
		return err
	}

	return nil
}

func matchNamePath(name, path string) bool {
	// assure that name and path use the same path separator...
	names := filepath.SplitList(name)
	name = filepath.Join(names...)

	if strings.HasSuffix(path, name) {
		return true
	}
	return false
}

func (proj *Project) ResolveDependency(dep interfaces.DependencyInterface) interfaces.PackageInterface {
	type NamePath struct {
		name string
		path string
	}

	var errorPkgs []NamePath
	for _, pkgList := range proj.packages {
		for _, pkg := range *pkgList {
			name := pkg.Name()
			path := pkg.BasePath()
			if !matchNamePath(name, path) {
				errorPkgs = append(errorPkgs, NamePath{name: name, path: path})
			}
			if dep.SatisfiesDependency(pkg) {
				return pkg
			}
		}
	}

	for _, namepath := range errorPkgs {
		util.StatusMessage(util.VERBOSITY_VERBOSE,
			"Package name \"%s\" doesn't match path \"%s\"\n", namepath.name, namepath.path)
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
	} else {
		repo = proj.repos[repoName]
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
			if proj.projState.installedRepos[name] != nil {
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
