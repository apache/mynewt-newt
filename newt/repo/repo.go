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
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/compat"
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

	// [branch] =>
	deps map[string][]*RepoDependency

	// version => branch
	vers map[newtutil.RepoVersion]string
}

type RepoDependency struct {
	Name    string
	VerReqs []newtutil.RepoVersionReq
	Fields  map[string]string
}

func (r *Repo) BranchDepMap() map[string][]*RepoDependency {
	return r.deps
}

func (r *Repo) AllDeps() []*RepoDependency {
	branches := make([]string, 0, len(r.deps))
	for branch, _ := range r.deps {
		branches = append(branches, branch)
	}
	sort.Strings(branches)

	deps := []*RepoDependency{}
	for _, b := range branches {
		deps = append(deps, r.deps[b]...)
	}

	return deps
}

func (r *Repo) DepsForVersion(ver newtutil.RepoVersion) []*RepoDependency {
	branch, err := r.BranchFromVer(ver)
	if err != nil {
		return nil
	}

	return r.deps[branch]
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
		if err != nil {
			continue
		}

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

func (r *Repo) repoFilePath() string {
	return interfaces.GetProject().Path() + "/" + REPOS_DIR + "/" +
		".configs/" + r.name + "/"
}

func (r *Repo) patchesFilePath() string {
	return interfaces.GetProject().Path() + "/" + REPOS_DIR +
		"/.patches/"
}

func (r *Repo) downloadRepo(branchName string) error {
	dl := r.downloader

	tmpdir, err := newtutil.MakeTempRepoDir()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	// Download the git repo, returns the git repo, checked out to that branch
	if err := dl.DownloadRepo(branchName, tmpdir); err != nil {
		return util.FmtNewtError("Error downloading repository %s: %s",
			r.Name(), err.Error())
	}

	// Copy the Git repo into the the desired local path of the repo
	if err := util.CopyDir(tmpdir, r.Path()); err != nil {
		// Cleanup any directory that might have been created if we error out
		// here.
		os.RemoveAll(r.Path())
		return err
	}

	return nil
}

func (r *Repo) checkExists() bool {
	return util.NodeExist(r.Path())
}

func (r *Repo) updateRepo(branchName string) error {
	err := r.downloader.UpdateRepo(r.Path(), branchName)
	if err != nil {
		// If the update failed because the repo directory has been deleted,
		// clone the repo again.
		if util.IsNotExist(err) {
			err = r.downloadRepo(branchName)
		}
		if err != nil {
			return util.FmtNewtError(
				"Error updating \"%s\": %s", r.Name(), err.Error())
		}
	}

	return nil
}

func (r *Repo) cleanupRepo(branchName string) error {
	dl := r.downloader
	err := dl.CleanupRepo(r.Path(), branchName)
	if err != nil {
		return util.FmtNewtError("Error cleaning and updating: %s", err.Error())
	}
	return nil
}

func (r *Repo) saveLocalDiff() (string, error) {
	dl := r.downloader
	diff, err := dl.LocalDiff(r.Path())
	if err != nil {
		return "", util.FmtNewtError("Error creating diff for \"%s\" : %s",
			r.Name(), err.Error())
	}

	// NOTE: date was not a typo: https://golang.org/pkg/time/#Time.Format
	timenow := time.Now().Format("20060102_150405")
	filename := r.patchesFilePath() + r.Name() + "_" + timenow + ".diff"

	f, err := os.Create(filename)
	if err != nil {
		return "",
			util.FmtNewtError("Error creating repo diff file \"%s\": %s", filename, err.Error())
	}
	defer f.Close()

	_, err = f.Write(diff)
	if err != nil {
		return "",
			util.FmtNewtError("Error writing repo diff file \"%s\": %s", filename, err.Error())
	}

	return filename, nil
}

func (r *Repo) currentBranch() (string, error) {
	dl := r.downloader
	branch, err := dl.CurrentBranch(r.Path())
	if err != nil {
		return "",
			util.FmtNewtError("Error finding current branch for \"%s\" : %s",
				r.Name(), err.Error())
	}
	return filepath.Base(branch), nil
}

func (r *Repo) BranchFromVer(ver newtutil.RepoVersion) (string, error) {
	nver, err := r.NormalizeVersion(ver)
	if err != nil {
		return "", err
	}

	branch := r.vers[nver]
	if branch == "" {
		return "",
			util.FmtNewtError("repo \"%s\" version %s does not map to a branch",
				r.Name(), nver.String())
	}

	return branch, nil
}

func (r *Repo) CurrentVersion() (*newtutil.RepoVersion, error) {
	branch, err := r.currentBranch()
	if err != nil {
		return nil, err
	}

	for _, v := range r.AllVersions() {
		if r.vers[v] == branch {
			return &v, nil
		}
	}

	// No matching version.
	return nil, nil
}

func (r *Repo) CurrentNormalizedVersion() (*newtutil.RepoVersion, error) {
	ver, err := r.CurrentVersion()
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, nil
	}

	*ver, err = r.NormalizeVersion(*ver)
	if err != nil {
		return nil, err
	}

	return ver, nil
}

func (r *Repo) AllVersions() []newtutil.RepoVersion {
	var vers []newtutil.RepoVersion
	for ver, _ := range r.vers {
		vers = append(vers, ver)
	}

	return newtutil.SortedVersions(vers)
}

func (r *Repo) NormalizedVersions() ([]newtutil.RepoVersion, error) {
	verMap := map[newtutil.RepoVersion]struct{}{}

	for ver, _ := range r.vers {
		nver, err := r.NormalizeVersion(ver)
		if err != nil {
			return nil, err
		}
		verMap[nver] = struct{}{}
	}

	vers := make([]newtutil.RepoVersion, 0, len(verMap))
	for ver, _ := range verMap {
		vers = append(vers, ver)
	}

	return vers, nil
}

// Converts the specified version to its equivalent x.x.x form for this repo.
// For example, this might convert "0-dev" to "0.0.0" (depending on the
// `repository.yml` file contents).
func (r *Repo) NormalizeVersion(
	ver newtutil.RepoVersion) (newtutil.RepoVersion, error) {

	origVer := ver
	for {
		if ver.Stability == "" ||
			ver.Stability == newtutil.VERSION_STABILITY_NONE {
			return ver, nil
		}
		verStr := r.vers[ver]
		if verStr == "" {
			return ver, util.FmtNewtError(
				"cannot normalize version \"%s\" for repo \"%s\"; "+
					"no mapping to numeric version",
				origVer.String(), r.Name())
		}

		nextVer, err := newtutil.ParseRepoVersion(verStr)
		if err != nil {
			return ver, err
		}
		ver = nextVer
	}
}

// Normalizes the version component of a version requirement.
func (r *Repo) NormalizeVerReq(verReq newtutil.RepoVersionReq) (
	newtutil.RepoVersionReq, error) {

	ver, err := r.NormalizeVersion(verReq.Ver)
	if err != nil {
		return verReq, err
	}

	verReq.Ver = ver
	return verReq, nil
}

// Normalizes the version component of each specified version requirement.
func (r *Repo) NormalizeVerReqs(verReqs []newtutil.RepoVersionReq) (
	[]newtutil.RepoVersionReq, error) {

	result := make([]newtutil.RepoVersionReq, len(verReqs))
	for i, verReq := range verReqs {
		n, err := r.NormalizeVerReq(verReq)
		if err != nil {
			return nil, err
		}
		result[i] = n
	}

	return result, nil
}

// Compares the two specified versions for equality.  Two versions are equal if
// they ultimately map to the same branch.
func (r *Repo) VersionsEqual(v1 newtutil.RepoVersion,
	v2 newtutil.RepoVersion) bool {

	if newtutil.CompareRepoVersions(v1, v2) == 0 {
		return true
	}

	b1, err := r.BranchFromVer(v1)
	if err != nil {
		return false
	}

	b2, err := r.BranchFromVer(v2)
	if err != nil {
		return false
	}

	return b1 == b2
}

func (r *Repo) Install(ver newtutil.RepoVersion) error {
	branch, err := r.BranchFromVer(ver)
	if err != nil {
		return err
	}

	if err := r.updateRepo(branch); err != nil {
		return err
	}

	return nil
}

func (r *Repo) Upgrade(ver newtutil.RepoVersion, force bool) error {
	branch, err := r.BranchFromVer(ver)
	if err != nil {
		return err
	}

	changes, err := r.downloader.AreChanges(r.Path())
	if err != nil {
		return err
	}

	if changes && !force {
		return util.FmtNewtError(
			"Repository \"%s\" contains local changes.  Provide the "+
				"-f option to attempt a merge.", r.Name())
	}

	if err := r.updateRepo(branch); err != nil {
		return err
	}

	return nil
}

func (r *Repo) Sync(ver newtutil.RepoVersion, force bool) (bool, error) {
	var currBranch string

	// Update the repo description
	if _, err := r.UpdateDesc(); err != nil {
		return false, util.NewNewtError("Cannot update repository description.")
	}

	branchName, err := r.BranchFromVer(ver)
	if err != nil {
		return false, err
	}
	if branchName == "" {
		return false, util.FmtNewtError(
			"No branch mapping for %s,%s", r.Name(), ver.String())
	}

	// Here assuming that if the branch was changed by the user,
	// the user must know what he's doing...
	// but, if -f is passed let's just save the work and re-clone
	currBranch, err = r.currentBranch()

	// currBranch == HEAD means we're dettached from HEAD, so
	// ignore and move to "new" tag
	if err != nil {
		return false, err
	} else if currBranch != "HEAD" && currBranch != branchName {
		msg := "Unexpected local branch for %s: \"%s\" != \"%s\""
		if force {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				msg+"\n", r.Name(), currBranch, branchName)
		} else {
			return false, util.FmtNewtError(
				msg, r.Name(), currBranch, branchName)
		}
	}

	// Don't try updating if on an invalid branch...
	if currBranch == "HEAD" || currBranch == branchName {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Syncing repository \"%s\"... ", r.Name())
		err = r.updateRepo(branchName)
		if err == nil {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "success\n")
			return true, err
		} else {
			util.StatusMessage(util.VERBOSITY_QUIET, "failed: %s\n",
				err.Error())
			if !force {
				return false, err
			}
		}
	}

	filename, err := r.saveLocalDiff()
	if err != nil {
		return false, err
	}
	wd, _ := os.Getwd()
	filename, _ = filepath.Rel(wd, filename)

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Saved local diff: "+
		"\"%s\"\n", filename)

	err = r.cleanupRepo(branchName)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r *Repo) UpdateDesc() (bool, error) {
	var err error

	if r.updated {
		return false, nil
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE, "[%s]:\n", r.Name())

	if err = r.DownloadDesc(); err != nil {
		return false, err
	}

	if err := r.Read(); err != nil {
		return false, err
	}

	r.updated = true

	return true, nil
}

func (r *Repo) downloadRepositoryYml() error {
	dl := r.downloader
	dl.SetBranch("master")

	// Clone the repo if it doesn't exist.
	if util.NodeNotExist(r.localPath) {
		if err := r.downloadRepo("master"); err != nil {
			return err
		}
	}

	cpath := r.repoFilePath()
	if err := dl.FetchFile(r.localPath, REPO_FILE_NAME, cpath); err != nil {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Download failed\n")

		return err
	}

	// also create a directory to save diffs for sync
	cpath = r.repoFilePath()
	if util.NodeNotExist(cpath) {
		if err := os.MkdirAll(cpath, REPO_DEFAULT_PERMS); err != nil {
			return util.NewNewtError(err.Error())
		}
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Download successful!\n")
	return nil
}

// Download the repository description.
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
	if !ok {
		return nil, util.FmtNewtError("invalid \"vers\" map")
	}

	fields := map[string]string{}
	for k, v := range tlMap {
		if s, ok := v.(string); ok {
			fields[k] = s
		}
	}

	for branch, verReqsStr := range versMap {
		verReqs, err := newtutil.ParseRepoVersionReqs(verReqsStr)
		if err != nil {
			return nil, util.FmtNewtError("invalid version string: %s",
				verReqsStr)
		}

		result[branch] = &RepoDependency{
			Name:    depName,
			VerReqs: verReqs,
			Fields:  fields,
		}
	}

	return result, nil
}

func (r *Repo) readDepRepos(yc ycfg.YCfg) error {
	depMap := yc.GetValStringMap("repo.deps", nil)
	for depName, repoMapYml := range depMap {
		rdm, err := parseRepoDepMap(depName, repoMapYml)
		if err != nil {
			return util.FmtNewtError(
				"Error while parsing 'repo.deps' for repo \"%s\", "+
					"dependency \"%s\": %s", r.Name(), depName, err.Error())
		}

		for branch, dep := range rdm {
			r.deps[branch] = append(r.deps[branch], dep)
		}
	}

	return nil
}

// Reads a `repository.yml` file and populates the receiver repo with its
// contents.
func (r *Repo) Read() error {
	r.Init(r.Name(), r.downloader)

	yc, err := newtutil.ReadConfig(r.repoFilePath(),
		strings.TrimSuffix(REPO_FILE_NAME, ".yml"))
	if err != nil {
		return err
	}

	versMap := yc.GetValStringMapString("repo.versions", nil)
	for versStr, branch := range versMap {
		log.Debugf("Printing version %s for remote repo %s", versStr, r.name)
		vers, err := newtutil.ParseRepoVersion(versStr)
		if err != nil {
			return err
		}

		// store branch->version mapping
		r.vers[vers] = branch
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
			"Repo version missing from compatibility map"
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

	if err := r.Init(repoName, nil); err != nil {
		return nil, err
	}

	return r, nil
}
