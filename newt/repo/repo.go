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
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/compat"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
)

const REPO_NAME_LOCAL = "local"
const REPO_DEFAULT_PERMS = 0755

const REPO_FILE_NAME = "repository.yml"
const REPOS_DIR = "repos"

type Repo struct {
	name       string
	downloader downloader.Downloader
	localPath  string
	versreq    []interfaces.VersionReqInterface
	rdesc      *RepoDesc
	deps       []*RepoDependency
	ignDirs    []string
	updated    bool
	local      bool
	ncMap      compat.NewtCompatMap
}

type RepoDesc struct {
	name string
	vers map[*Version]string
}

type RepoDependency struct {
	versreq   []interfaces.VersionReqInterface
	name      string
	Storerepo *Repo
}

func (rd *RepoDependency) String() string {
	rstr := "<"

	for idx, vr := range rd.versreq {
		if idx != 0 {
			rstr = rstr + " " + vr.Version().String()
		} else {
			rstr = rstr + vr.Version().String()
		}
	}
	rstr = rstr + ">"
	return rstr
}

func (r *Repo) Deps() []*RepoDependency {
	return r.deps
}

func (r *Repo) AddDependency(rd *RepoDependency) {
	r.deps = append(r.deps, rd)
}

func (rd *RepoDependency) Name() string {
	return rd.name
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
		return list, util.FmtNewtError("failed to read repo \"%s\": %s",
			repo.Name(), err.Error())
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

func NewRepoDependency(rname string, verstr string) (*RepoDependency, error) {
	var err error

	rd := &RepoDependency{}
	rd.versreq, err = LoadVersionMatches(verstr)
	if err != nil {
		return nil, err
	}
	rd.name = rname

	return rd, nil
}

func pickVersion(repo *Repo, versions []*Version) ([]*Version, error) {
	fmt.Printf("Dependency list for %s contains a specific commit tag, "+
		"so normal version number/stability comparison cannot be done.\n",
		repo.Name())
	fmt.Printf("If the following list does not contain the requirement to use, " +
		"then modify your project.yml so that it does.\n")
	for {
		for i, vers := range versions {
			fmt.Printf(" %d) %s\n", i, vers.String())
		}
		fmt.Printf("Pick the index of a version to use from above list: ")
		line, _, err := bufio.NewReader(os.Stdin).ReadLine()
		if err != nil {
			return nil, util.NewNewtError(fmt.Sprintf("Couldn't read "+
				"response: %s", err.Error()))
		}
		idx, err := strconv.ParseUint(string(line), 10, 8)
		if err != nil {
			fmt.Printf("Error: could not parse the response.\n")
		} else {
			repo.versreq, err = LoadVersionMatches(versions[idx].String())
			return []*Version{versions[idx]}, nil
		}
	}
}

func CheckDeps(upgrade bool, checkRepos map[string]*Repo) error {
	// For each dependency, get it's version
	depArray := map[string][]*Version{}

	for _, checkRepo := range checkRepos {
		for _, rd := range checkRepo.Deps() {
			lookupRepo := checkRepos[rd.Name()]

			_, vers, ok := lookupRepo.rdesc.Match(rd.Storerepo)
			if !ok {
				return util.NewNewtError(fmt.Sprintf("No "+
					"matching version for dependent repository %s", rd.name))
			}
			log.Debugf("Dependency for %s: %s (%s)", checkRepo.Name(), rd.Name(), vers.String())

			_, ok = depArray[rd.Name()]
			if !ok {
				depArray[rd.Name()] = []*Version{}
			}
			depArray[rd.Name()] = append(depArray[rd.Name()], vers)
		}
	}

	for repoName, depVersList := range depArray {
		if len(depVersList) <= 1 {
			continue
		}

		pickVer := false
		for _, depVers := range depVersList {
			if depVers.Tag() != "" {
				pickVer = true
				break
			}
		}
		if pickVer {
			newArray, err := pickVersion(checkRepos[repoName],
				depArray[repoName])
			depArray[repoName] = newArray
			if err != nil {
				return err
			}
		}
	}
	for repoName, depVersList := range depArray {
		for _, depVers := range depVersList {
			for _, curVers := range depVersList {
				if depVers.CompareVersions(depVers, curVers) != 0 ||
					depVers.Stability() != curVers.Stability() {
					return util.NewNewtError(fmt.Sprintf(
						"Conflict detected.  Repository %s has multiple dependency versions on %s. "+
							"Notion of repository version is %s, whereas required is %s ",
						repoName, curVers, depVers))
				}
			}
		}
	}

	return nil
}

func (rd *RepoDesc) MatchVersion(searchVers *Version) (string, *Version, bool) {
	for vers, curBranch := range rd.vers {
		if vers.CompareVersions(vers, searchVers) == 0 &&
			searchVers.Stability() == vers.Stability() {
			return curBranch, vers, true
		}
	}
	return "", nil, false
}

func (rd *RepoDesc) Match(r *Repo) (string, *Version, bool) {
	log.Debugf("Requires repository version %s for %s\n", r.VersionRequirements(),
		r.Name())
	for vers, branch := range rd.vers {
		if vers.SatisfiesVersion(r.VersionRequirements()) {
			log.Debugf("Found matching version %s for repo %s",
				vers.String(), r.Name())
			if vers.Stability() != VERSION_STABILITY_NONE {
				// Load the branch as a version, and search for it
				searchVers, err := LoadVersion(branch)
				if err != nil {
					return "", nil, false
				}
				// Need to match the NONE stability in order to find the right
				// branch.
				searchVers.stability = VERSION_STABILITY_NONE

				var ok bool
				branch, vers, ok = rd.MatchVersion(searchVers)
				if !ok {
					return "", nil, false
				}
				log.Debugf("Founding matching version %s for search version %s, related branch is %s\n",
					vers, searchVers, branch)

			}

			return branch, vers, true
		} else {
			log.Debugf("Rejected version %s for repo %s",
				vers.String(), r.Name())
		}
	}

	/*
	 * No match so far. See if requirements have a repository tag directly.
	 * If so, then return that as the branch.
	 */
	for _, versreq := range r.VersionRequirements() {
		tag := versreq.Version().Tag()
		if tag != "" {
			log.Debugf("Requirements for %s have a tag option %s\n",
				r.Name(), tag)
			return tag, NewTag(tag), true
		}
	}
	return "", nil, false
}

func (rd *RepoDesc) SatisfiesVersion(vers *Version, versReqs []interfaces.VersionReqInterface) bool {
	var err error
	versMatches := []interfaces.VersionReqInterface{}
	for _, versReq := range versReqs {
		versMatch := &VersionMatch{}
		versMatch.compareType = versReq.CompareType()

		if versReq.Version().Stability() != VERSION_STABILITY_NONE {
			// Look up this item in the RepoDescription, and get a version
			searchVers := versReq.Version().(*Version)
			branch, _, ok := rd.MatchVersion(searchVers)
			if !ok {
				return false
			}
			versMatch.Vers, err = LoadVersion(branch)
			if err != nil {
				return false
			}
		} else {
			versMatch.Vers = versReq.Version().(*Version)
		}

		versMatches = append(versMatches, versMatch)
	}

	return vers.SatisfiesVersion(versMatches)
}

func (rd *RepoDesc) Init(name string, versBranchMap map[string]string) error {
	rd.name = name
	rd.vers = map[*Version]string{}

	for versStr, branch := range versBranchMap {
		log.Debugf("Printing version %s for remote repo %s", versStr, name)
		vers, err := LoadVersion(versStr)
		if err != nil {
			return err
		}

		// store branch->version mapping
		rd.vers[vers] = branch
	}

	return nil
}

func (rd *RepoDesc) String() string {
	name := rd.name + "@"
	for k, v := range rd.vers {
		name += fmt.Sprintf("%s=%s", k.String(), v)
		name += "#"
	}

	return name
}

func NewRepoDesc(name string, versBranchMap map[string]string) (*RepoDesc, error) {
	rd := &RepoDesc{}

	if err := rd.Init(name, versBranchMap); err != nil {
		return nil, err
	}

	return rd, nil
}

func (r *Repo) GetRepoDesc() (*RepoDesc, error) {
	if r.rdesc == nil {
		return nil, util.NewNewtError(fmt.Sprintf(
			"Repository description for %s not yet initialized.  Must "+
				"download it first. ", r.Name()))
	} else {
		return r.rdesc, nil
	}
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

func (r *Repo) VersionRequirements() []interfaces.VersionReqInterface {
	return r.versreq
}

func (r *Repo) VersionRequirementsString() string {
	str := ""
	for _, vreq := range r.versreq {
		str += vreq.String()
	}

	return str
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

	// Download the git repo, returns the git repo, checked out to that branch
	tmpdir, err := dl.DownloadRepo(branchName)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Error download repository %s, : %s",
			r.Name(), err.Error()))
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
	dl := r.downloader
	err := dl.UpdateRepo(r.Path(), branchName)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Error updating\n"))
	}
	return nil
}

func (r *Repo) cleanupRepo(branchName string) error {
	dl := r.downloader
	err := dl.CleanupRepo(r.Path(), branchName)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Error cleaning and updating\n"))
	}
	return nil
}

func (r *Repo) saveLocalDiff() (string, error) {
	dl := r.downloader
	diff, err := dl.LocalDiff(r.Path())
	if err != nil {
		return "", util.NewNewtError(fmt.Sprintf(
			"Error creating diff for \"%s\" : %s", r.Name(), err.Error()))
	}

	// NOTE: date was not a typo: https://golang.org/pkg/time/#Time.Format
	timenow := time.Now().Format("20060102_150405")
	filename := r.patchesFilePath() + r.Name() + "_" + timenow + ".diff"

	f, err := os.Create(filename)
	if err != nil {
		return "", util.NewNewtError(fmt.Sprintf(
			"Error creating repo diff file \"%s\"", filename))
	}
	defer f.Close()

	_, err = f.Write(diff)
	if err != nil {
		return "", util.NewNewtError(fmt.Sprintf(
			"Error writing repo diff file \"%s\"", filename))
	}

	return filename, nil
}

func (r *Repo) currentBranch() (string, error) {
	dl := r.downloader
	branch, err := dl.CurrentBranch(r.Path())
	if err != nil {
		return "", util.NewNewtError(fmt.Sprintf("Error finding current branch for \"%s\" : %s",
			r.Name(), err.Error()))
	}
	return filepath.Base(branch), nil
}

func (r *Repo) Install(force bool) (*Version, error) {
	exists := util.NodeExist(r.Path())
	if exists && !force {
		return nil, util.NewNewtError(fmt.Sprintf(
			"Repository %s already exists, provide the -f option "+
				"to overwrite", r.Name()))
	}

	branchName, vers, found := r.rdesc.Match(r)
	if !found {
		return nil, util.NewNewtError(fmt.Sprintf("No repository "+
			"matching description %s found", r.rdesc.String()))
	}

	// if the repo is already cloned, try to cleanup and checkout the requested branch
	if exists {
		err := r.cleanupRepo(branchName)
		if err == nil {
			return vers, nil
		}

		// cleanup failed, so remove current copy and let download clone again...
		if err := os.RemoveAll(r.Path()); err != nil {
			return nil, util.NewNewtError(err.Error())
		}
	}

	// repo was not already cloned or cleanup failed...
	if err := r.downloadRepo(branchName); err != nil {
		return nil, err
	}

	return vers, nil
}

func (r *Repo) Sync(vers *Version, force bool) (bool, bool, error) {
	var exists bool
	var err error
	var currBranch string

	exists = r.checkExists()

	// Update the repo description
	if _, updated, err := r.UpdateDesc(); updated != true || err != nil {
		return exists, false, util.NewNewtError("Cannot update repository description.")
	}

	branchName, _, found := r.rdesc.MatchVersion(vers)
	if found == false {
		return exists, false, util.NewNewtError(fmt.Sprintf(
			"Branch description for %s not found", r.Name()))
	}

	if exists {
		// Here assuming that if the branch was changed by the user,
		// the user must know what he's doing...
		// but, if -f is passed let's just save the work and re-clone
		currBranch, err = r.currentBranch()

		// currBranch == HEAD means we're dettached from HEAD, so
		// ignore and move to "new" tag
		if err != nil {
			return exists, false, err
		} else if currBranch != "HEAD" && currBranch != branchName {
			msg := "Unexpected local branch for %s: \"%s\" != \"%s\"\n"
			if force {
				util.StatusMessage(util.VERBOSITY_VERBOSE,
					msg, r.rdesc.name, currBranch, branchName)
			} else {
				err = util.NewNewtError(
					fmt.Sprintf(msg, r.rdesc.name, currBranch, branchName))
				return exists, false, err
			}
		}

		// Don't try updating if on an invalid branch...
		if currBranch == "HEAD" || currBranch == branchName {
			util.StatusMessage(util.VERBOSITY_VERBOSE, "Updating repository...\n")
			err = r.updateRepo(branchName)
			if err == nil {
				util.StatusMessage(util.VERBOSITY_VERBOSE, "Update successful!\n")
				return exists, true, err
			} else {
				util.StatusMessage(util.VERBOSITY_VERBOSE, "Update failed!\n")
				if !force {
					return exists, false, err
				}
			}
		}

		filename, err := r.saveLocalDiff()
		if err != nil {
			return exists, false, err
		}
		wd, _ := os.Getwd()
		filename, _ = filepath.Rel(wd, filename)

		util.StatusMessage(util.VERBOSITY_DEFAULT, "Saved local diff: "+
			"\"%s\"\n", filename)

		err = r.cleanupRepo(branchName)
		if err != nil {
			return exists, false, err
		}

	} else {
		// fresh or updating was unsuccessfull and force was given...
		err = r.downloadRepo(branchName)
		if err != nil {
			return exists, false, err
		}
	}

	return exists, true, nil
}

func (r *Repo) UpdateDesc() ([]*Repo, bool, error) {
	var err error

	if r.updated {
		return nil, false, nil
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE, "[%s]:\n", r.Name())

	if err = r.DownloadDesc(); err != nil {
		return nil, false, err
	}

	_, repos, err := r.ReadDesc()
	if err != nil {
		fmt.Printf("ReadDesc: %v\n", err)
		return nil, false, err
	}

	r.updated = true

	return repos, true, nil
}

// Download the repository description.
func (r *Repo) DownloadDesc() error {
	dl := r.downloader

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Downloading "+
		"repository description\n")

	// Configuration path
	cpath := r.repoFilePath()
	if util.NodeNotExist(cpath) {
		if err := os.MkdirAll(cpath, REPO_DEFAULT_PERMS); err != nil {
			return util.NewNewtError(err.Error())
		}
	}

	dl.SetBranch("master")
	if err := dl.FetchFile(REPO_FILE_NAME,
		cpath+"/"+REPO_FILE_NAME); err != nil {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Download failed\n")
		return err
	}

	// also create a directory to save diffs for sync
	cpath = r.patchesFilePath()
	if util.NodeNotExist(cpath) {
		if err := os.MkdirAll(cpath, REPO_DEFAULT_PERMS); err != nil {
			return util.NewNewtError(err.Error())
		}
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Download successful!\n")

	return nil
}

func (r *Repo) readDepRepos(v *viper.Viper) ([]*Repo, error) {
	rdesc := r.rdesc
	repos := []*Repo{}

	branch, _, ok := rdesc.Match(r)
	if !ok {
		// No matching branch, barf!
		return nil, util.NewNewtError(fmt.Sprintf("No "+
			"matching branch for %s repo", r.Name()))
	}

	repoTag := fmt.Sprintf("%s.repositories", branch)

	repoList := v.GetStringMap(repoTag)
	for repoName, repoItf := range repoList {
		repoVars := cast.ToStringMapString(repoItf)

		dl, err := downloader.LoadDownloader(repoName, repoVars)
		if err != nil {
			return nil, err
		}

		rversreq := repoVars["vers"]
		newRepo, err := NewRepo(repoName, rversreq, dl)
		if err != nil {
			return nil, err
		}

		rd, err := NewRepoDependency(repoName, rversreq)
		if err != nil {
			return nil, err
		}
		rd.Storerepo = newRepo

		r.AddDependency(rd)

		repos = append(repos, newRepo)
	}
	return repos, nil
}

func (r *Repo) ReadDesc() (*RepoDesc, []*Repo, error) {
	if util.NodeNotExist(r.repoFilePath() + REPO_FILE_NAME) {
		return nil, nil,
			util.NewNewtError("No configuration exists for repository " + r.name)
	}

	v, err := util.ReadConfig(r.repoFilePath(),
		strings.TrimSuffix(REPO_FILE_NAME, ".yml"))
	if err != nil {
		return nil, nil, err
	}

	name := v.GetString("repo.name")
	versMap := v.GetStringMapString("repo.versions")

	rdesc, err := NewRepoDesc(name, versMap)
	if err != nil {
		return nil, nil, err
	}
	r.rdesc = rdesc

	repos, err := r.readDepRepos(v)
	if err != nil {
		return nil, nil, err
	}

	// Read the newt version compatibility map.
	r.ncMap, err = compat.ReadNcMap(v)
	if err != nil {
		return nil, nil, err
	}

	return rdesc, repos, nil
}

func (r *Repo) Init(repoName string, rversreq string, d downloader.Downloader) error {
	var err error

	r.name = repoName
	r.downloader = d
	r.deps = []*RepoDependency{}
	r.versreq, err = LoadVersionMatches(rversreq)
	if err != nil {
		return err
	}

	path := interfaces.GetProject().Path()

	if r.local {
		r.localPath = filepath.ToSlash(filepath.Clean(path))
	} else {
		r.localPath = filepath.ToSlash(filepath.Clean(path + "/" + REPOS_DIR + "/" + r.name))
	}

	return nil
}

func (r *Repo) CheckNewtCompatibility(rvers *Version, nvers newtutil.Version) (
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

func NewRepo(repoName string, rversreq string, d downloader.Downloader) (*Repo, error) {
	r := &Repo{
		local: false,
	}

	if err := r.Init(repoName, rversreq, d); err != nil {
		return nil, err
	}

	return r, nil
}

func NewLocalRepo(repoName string) (*Repo, error) {
	r := &Repo{
		local: true,
	}

	if err := r.Init(repoName, "", nil); err != nil {
		return nil, err
	}

	return r, nil
}
