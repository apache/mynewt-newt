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

package repo

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/compat"
	"mynewt.apache.org/newt/newt/config"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

const REPO_NAME_LOCAL = "local"
const REPO_DEFAULT_PERMS = 0755

const REPO_FILE_NAME = "repository.yml"
const REPOS_DIR = "repos"

type Repo struct {
	name       string
	downloader downloader.Downloader
	localPath  string
	ignDirs    []string
	updated    bool
	local      bool
	ncMap      compat.NewtCompatMap

	// If repo was added from package (not from project.yml), this variable stores name of this package
	pkgName string

	// True if this repo was cloned during this invocation of newt.
	newlyCloned bool

	// commit => [dependencies]
	deps map[string][]*RepoDependency

	// version => commit
	vers map[newtutil.RepoVersion]string

	hasSubmodules bool
	submodules    []string
}

type RepoDependency struct {
	Name    string
	VerReqs newtutil.RepoVersion
	Fields  map[string]string
}

func (r *Repo) SetPkgName(pName string) {
	r.pkgName = pName
}

func (r *Repo) CommitDepMap() map[string][]*RepoDependency {
	return r.deps
}

func (r *Repo) AllDeps() []*RepoDependency {
	commits := make([]string, 0, len(r.deps))
	for commit, _ := range r.deps {
		commits = append(commits, commit)
	}
	sort.Strings(commits)

	deps := []*RepoDependency{}
	for _, b := range commits {
		deps = append(deps, r.deps[b]...)
	}

	return deps
}

func (r *Repo) AddIgnoreDir(ignDir string) {
	r.ignDirs = append(r.ignDirs, ignDir)
}

func (r *Repo) ignoreDir(dir string) bool {
	for _, idir := range r.ignDirs {
		if idir == dir {
			return true
		}
	}
	return false
}

func (r *Repo) Downloader() downloader.Downloader {
	return r.downloader
}

func (repo *Repo) FilteredSearchList(
	curPath string, searchedMap map[string]struct{}) ([]string, error) {

	list := []string{}

	path := filepath.Join(repo.Path(), curPath)
	dirList, err := ioutil.ReadDir(path)
	if err != nil {
		// The repo could not be found in the `repos` directory.  Display a
		// warning if the `project.state` file indicates that the repo has been
		// installed.
		var warning error
		if interfaces.GetProject().RepoIsInstalled(repo.Name()) {
			warning = util.FmtNewtError("failed to read repo \"%s\": %s",
				repo.Name(), err.Error())
		}
		return list, warning
	}

	for _, dirEnt := range dirList {
		// Resolve symbolic links.
		entPath := filepath.Join(path, dirEnt.Name())
		entry, err := os.Stat(entPath)
		if err != nil {
			return nil, util.ChildNewtError(err)
		}

		name := entry.Name()

		if !entry.IsDir() {
			continue
		}

		// Don't search the same directory twice.  This check is necessary in
		// case of symlink cycles.
		absPath, err := filepath.EvalSymlinks(entPath)
		if err != nil {
			return nil, util.ChildNewtError(err)
		}
		if _, ok := searchedMap[absPath]; ok {
			continue
		}
		searchedMap[absPath] = struct{}{}

		if strings.HasPrefix(name, ".") {
			continue
		}
		if repo.ignoreDir(filepath.Join(curPath, name)) {
			continue
		}
		list = append(list, name)
	}
	return list, nil
}

func (r *Repo) Name() string {
	return r.name
}

func (r *Repo) Path() string {
	return r.localPath
}

func (r *Repo) IsLocal() bool {
	return r.local
}

func (r *Repo) IsNewlyCloned() bool {
	return r.newlyCloned
}

func RepoFilePath(repoName string) string {
	return interfaces.GetProject().Path() + "/" + REPOS_DIR + "/" +
		".configs/" + repoName
}

func (r *Repo) repoFilePath() string {
	return RepoFilePath(r.name)
}

func (r *Repo) patchesFilePath() string {
	return interfaces.GetProject().Path() + "/" + REPOS_DIR +
		"/.patches/"
}

// Checks for repository.yml file presence in specified repo folder.
// If there is no such file, the repo in specified folder is external.
func (r *Repo) IsExternal(dir string) bool {
	path := dir + "/repository.yml"
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true
	} else {
		return false
	}
}

// Check that there is a list of submodules to clone; if the key is
// not present, assume that all submodules should be cloned, otherwise
// only those configured will be cloned (or none if "")
func (r *Repo) loadSubmoduleConfig(yc ycfg.YCfg) error {
	r.hasSubmodules = yc.HasKey("repo.submodules")
	if r.hasSubmodules {
		submodules, err := yc.GetValStringSliceNonempty("repo.submodules", nil)
		if err != nil {
			return util.PreNewtError(err,
				"failure parsing submodules for repo \"%s\"", r.Name())
		}
		r.submodules = submodules
	}
	return nil
}

func (r *Repo) downloadRepo(commit string) error {
	dl := r.downloader

	tmpdir, err := newtutil.MakeTempRepoDir()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	// Download the git repo, returns the git repo, checked out to that commit
	if err := dl.Clone(commit, tmpdir); err != nil {
		return util.FmtNewtError("Error downloading repository %s: %s",
			r.Name(), err.Error())
	}

	if !r.IsExternal(tmpdir) {
		yc, err := config.ReadFile(tmpdir + "/" + REPO_FILE_NAME)
		if err != nil {
			return err
		}

		if err := r.loadSubmoduleConfig(yc); err != nil {
			return err
		}
	}

	if r.hasSubmodules {
		if len(r.submodules) == 0 {
			util.StatusMessage(util.VERBOSITY_VERBOSE, "Skipping submodule updates\n")
		} else {
			for _, submodule := range r.submodules {
				if err := dl.UpdateSubmodule(tmpdir, submodule); err != nil {
					return util.FmtNewtError("Error updating submodule \"%s\" for repository %s: %s",
						submodule, r.Name(), err.Error())
				}
			}
		}
	} else {
		if err := dl.UpdateSubmodules(tmpdir); err != nil {
			return util.FmtNewtError("Error updating submodules for repository %s: %s",
				r.Name(), err.Error())
		}
	}

	// Copy the Git repo into the the desired local path of the repo
	if err := util.CopyDir(tmpdir, r.Path()); err != nil {
		// Cleanup any directory that might have been created if we error out
		// here.
		os.RemoveAll(r.Path())
		return err
	}

	r.newlyCloned = true
	return nil
}

func (r *Repo) CheckExists() bool {
	return util.NodeExist(r.Path())
}

func (r *Repo) updateRepo(commit string) error {
	// Clone the repo if it doesn't exist.
	if err := r.EnsureExists(); err != nil {
		return err
	}

	// Fetch and checkout the specified commit.
	if err := r.downloader.Fetch(r.Path()); err != nil {
		return util.FmtNewtError(
			"Error updating \"%s\": %s", r.Name(), err.Error())
	}

	// If the specified commit doesn't exist, try inserting "_rc#" into the
	// string.  This is useful when a release candidate is being tested.  In
	// this case, the "rc" tags exist, but the official release tag has not
	// been created yet.
	if _, err := r.downloader.CommitType(r.Path(), commit); err != nil {
		newCommit, err := r.downloader.LatestRc(r.Path(), commit)
		if err != nil {
			return util.FmtNewtError(
				"Error updating \"%s\": %s", r.Name(), err.Error())
		}

		if newCommit != commit {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"in repo \"%s\": commit \"%s\" does not exist; "+
					"using \"%s\" instead\n",
				r.Name(), commit, newCommit)
			commit = newCommit
		}
	}

	if err := r.downloader.Checkout(r.Path(), commit); err != nil {
		return util.FmtNewtError(
			"Error updating \"%s\": %s", r.Name(), err.Error())
	}

	return nil
}

// Indicates whether the specified repo is in a clean or dirty state.
//
// @return string               Text describing repo's dirty state, or "" if
//                                  clean.
// @return error                Error.
func (r *Repo) DirtyState() (string, error) {
	return r.downloader.DirtyState(r.Path())
}

func (r *Repo) Upgrade(ver newtutil.RepoVersion) error {
	commit, err := r.CommitFromVer(ver)
	if err != nil {
		return err
	}

	if err := r.updateRepo(commit); err != nil {
		return err
	}

	return nil
}

// Fetches all remotes and downloads an up to date copy of `repository.yml`
// from master.  The repo object is then populated with the contents of the
// downladed file.  If this repo has already had its descriptor updated, this
// function is a no-op.
func (r *Repo) UpdateDesc() (bool, error) {
	if r.updated {
		return false, nil
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE, "[%s]:\n", r.Name())

	// Make sure the repo's "origin" remote points to the correct URL.  This is
	// necessary in case the user changed his `project.yml` file to point to a
	// different fork.
	if err := r.downloader.FixupOrigin(r.localPath); err != nil {
		return false, err
	}

	// Download `repository.yml`.
	if err := r.DownloadDesc(); err != nil {
		return false, err
	}

	// Read `repository.yml` and populate this repo object.
	if err := r.Read(); err != nil {
		return false, err
	}

	r.updated = true

	return true, nil
}

func (r *Repo) EnsureExists() error {
	// Clone the repo if it doesn't exist.
	if !r.CheckExists() {
		branch := r.downloader.MainBranch()
		if err := r.downloadRepo(branch); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repo) downloadFile(commit string, srcPath string) (string, error) {
	dl := r.downloader

	// Clone the repo if it doesn't exist.
	if err := r.EnsureExists(); err != nil {
		return "", err
	}

	cpath := r.repoFilePath()
	if err := os.MkdirAll(cpath, REPO_DEFAULT_PERMS); err != nil {
		return "", util.ChildNewtError(err)
	}

	if err := dl.FetchFile(commit, r.localPath, srcPath, cpath); err != nil {
		return "", util.FmtNewtError(
			"Download of \"%s\" from repo:%s commit:%s failed: %s",
			srcPath, r.Name(), commit, err.Error())
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Download of \"%s\" from repo:%s commit:%s successful\n",
		srcPath, r.Name(), commit)

	return cpath + "/" + srcPath, nil
}

func (r *Repo) downloadRepositoryYml() error {
	if _, err := r.downloadFile(r.downloader.MainBranch(), REPO_FILE_NAME); err != nil {
		return err
	}

	return nil
}

// Downloads the repository description, i.e., `repository.yml`.
func (r *Repo) DownloadDesc() error {
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Downloading "+
		"repository description\n")

	// Remember if the directory already exists.  If it doesn't, we'll create
	// it.  If downloading fails, only remove the directory if we just created
	// it.
	createdDir := false

	// Configuration path
	cpath := r.repoFilePath()
	if util.NodeNotExist(cpath) {
		if err := os.MkdirAll(cpath, REPO_DEFAULT_PERMS); err != nil {
			return util.NewNewtError(err.Error())
		}
		createdDir = true
	}

	if err := r.downloadRepositoryYml(); err != nil {
		if createdDir {
			os.RemoveAll(cpath)
		}
		return err
	}

	return nil
}

// IsDetached indicates whether a repo is in a "detached head" state.
func (r *Repo) IsDetached() (bool, error) {
	branch, err := r.downloader.CurrentBranch(r.Path())
	if err != nil {
		return false, err
	}

	return branch == "", nil
}

func parseRepoDepMap(depName string,
	repoMapYml interface{}) (map[string]*RepoDependency, error) {

	result := map[string]*RepoDependency{}

	tlMap, err := cast.ToStringMapE(repoMapYml)
	if err != nil {
		return nil, util.FmtNewtError("missing \"repository.yml\" file")
	}

	versYml, ok := tlMap["vers"]
	if !ok {
		return nil, util.FmtNewtError("missing \"vers\" map")
	}

	versMap, err := cast.ToStringMapStringE(versYml)
	if err != nil {
		return nil, util.FmtNewtError("invalid \"vers\" map: %v", err)
	}

	fields := map[string]string{}
	for k, v := range tlMap {
		if s, ok := v.(string); ok {
			fields[k] = s
		}
	}

	for commit, verReqsStr := range versMap {
		verReq, err := newtutil.ParseRepoVersion(verReqsStr)
		if err != nil {
			return nil, util.FmtNewtError("invalid version string: %s: %s",
				verReqsStr, err.Error())
		}

		result[commit] = &RepoDependency{
			Name:    depName,
			VerReqs: verReq,
			Fields:  fields,
		}
	}

	return result, nil
}

func (r *Repo) readDepRepos(yc ycfg.YCfg) error {
	depMap, err := yc.GetValStringMap("repo.deps", nil)
	util.OneTimeWarningError(err)

	for depName, repoMapYml := range depMap {
		rdm, err := parseRepoDepMap(depName, repoMapYml)
		if err != nil {
			return util.FmtNewtError(
				"Error while parsing 'repo.deps' for repo \"%s\", "+
					"dependency \"%s\": %s", r.Name(), depName, err.Error())
		}

		for commit, dep := range rdm {
			r.deps[commit] = append(r.deps[commit], dep)
		}
	}

	return nil
}

// Reads a `repository.yml` file and populates the receiver repo with its
// contents.
func (r *Repo) Read() error {
	r.Init(r.Name(), r.downloader)

	yc, err := config.ReadFile(r.repoFilePath() + "/" + REPO_FILE_NAME)
	if err != nil {
		return err
	}

	if err := r.loadSubmoduleConfig(yc); err != nil {
		return err
	}

	versMap, err := yc.GetValStringMapString("repo.versions", nil)
	util.OneTimeWarningError(err)

	for versStr, commit := range versMap {
		log.Debugf("Printing version %s for remote repo %s", versStr, r.name)
		vers, err := newtutil.ParseRepoVersion(versStr)
		if err != nil {
			return util.PreNewtError(err,
				"failure parsing version for repo \"%s\"", r.Name())
		}

		// store commit->version mapping
		r.vers[vers] = commit
	}

	if err := r.readDepRepos(yc); err != nil {
		return err
	}

	// Read the newt version compatibility map.
	r.ncMap, err = compat.ReadNcMap(yc)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repo) Init(repoName string, d downloader.Downloader) error {
	r.name = repoName
	r.downloader = d
	r.deps = map[string][]*RepoDependency{}
	r.vers = map[newtutil.RepoVersion]string{}

	path := interfaces.GetProject().Path()

	if r.local {
		r.localPath = filepath.ToSlash(filepath.Clean(path))
	} else {
		r.localPath = filepath.ToSlash(filepath.Clean(path + "/" + REPOS_DIR + "/" + r.name))
	}

	return nil
}

func (r *Repo) CheckNewtCompatibility(
	rvers newtutil.RepoVersion, nvers newtutil.Version) (
	compat.NewtCompatCode, string) {

	// If this repo doesn't have a newt compatibility map, just assume there is
	// no incompatibility.
	if len(r.ncMap) == 0 {
		return compat.NEWT_COMPAT_GOOD, ""
	}

	rnuver := rvers.ToNuVersion()
	tbl, ok := r.ncMap[rnuver]
	if !ok {
		// Unknown repo version.
		return compat.NEWT_COMPAT_WARN,
			r.Name() + ": Repo version missing from compatibility map"
	}

	code, text := tbl.CheckNewtVer(nvers)
	if code == compat.NEWT_COMPAT_GOOD {
		return code, text
	}

	return code, fmt.Sprintf("This version of newt (%s) is incompatible with "+
		"your version of the %s repo (%s); %s",
		nvers.String(), r.name, rnuver.String(), text)
}

func NewRepo(repoName string, d downloader.Downloader) (*Repo, error) {
	r := &Repo{
		local: false,
	}

	if err := r.Init(repoName, d); err != nil {
		return nil, err
	}

	return r, nil
}

func NewLocalRepo(repoName string) (*Repo, error) {
	r := &Repo{
		local: true,
	}

	// Local repos always get a git downloader.  The downloader is needed to
	// determine the git state of the project.
	if err := r.Init(repoName, downloader.NewGitDownloader()); err != nil {
		return nil, err
	}

	return r, nil
}
