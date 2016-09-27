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
	"os"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/interfaces"
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
	updated    bool
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

func (r *Repo) Deps() []*RepoDependency {
	return r.deps
}

func (r *Repo) AddDependency(rd *RepoDependency) {
	r.deps = append(r.deps, rd)
}

func (rd *RepoDependency) Name() string {
	return rd.name
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
	for vers, branch := range rd.vers {
		log.Debugf("Repository version requires for %s are %s\n", r.Name(), r.VersionRequirements())
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
			"Repository description for %s not yet initailized.  Must "+
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
	return r.name == REPO_NAME_LOCAL
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

func (r *Repo) Install(force bool) (*Version, error) {
	// Copy the git repo into /repos/, error'ing out if the repo already exists
	if util.NodeExist(r.Path()) {
		if force {
			if err := os.RemoveAll(r.Path()); err != nil {
				return nil, util.NewNewtError(err.Error())
			}
		} else {
			return nil, util.NewNewtError(fmt.Sprintf("Repository %s already "+
				"exists in local tree, cannot install.  Provide -f to override.", r.Path()))
		}
	}

	branchName, vers, found := r.rdesc.Match(r)
	if !found {
		return nil, util.NewNewtError(fmt.Sprintf("No repository matching description %s found",
			r.rdesc.String()))
	}

	dl := r.downloader

	// Download the git repo, returns the git repo, checked out to that branch
	tmpdir, err := dl.DownloadRepo(branchName)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Error download repository %s, : %s",
			r.Name(), err.Error()))
	}

	// Copy the Git repo into the the desired local path of the repo
	if err := util.CopyDir(tmpdir, r.Path()); err != nil {
		// Cleanup any directory that might have been created if we error out
		// here.
		os.RemoveAll(r.Path())
		return nil, err
	}

	return vers, nil
}

func (r *Repo) UpdateDesc() ([]*Repo, bool, error) {
	var err error

	if r.updated {
		return nil, false, nil
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "%s\n", r.Name())

	if err = r.DownloadDesc(); err != nil {
		return nil, false, err
	}

	_, repos, err := r.ReadDesc()
	if err != nil {
		return nil, false, err
	}

	r.updated = true

	return repos, true, nil
}

// Download the repository description.
func (r *Repo) DownloadDesc() error {
	dl := r.downloader

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Downloading "+
		"repository description for %s...\n", r.Name())

	// Configuration path
	cpath := r.repoFilePath()
	if util.NodeNotExist(cpath) {
		if err := os.MkdirAll(cpath, REPO_DEFAULT_PERMS); err != nil {
			return util.NewNewtError(err.Error())
		}
	}

	dl.SetBranch("master")
	if err := dl.FetchFile("repository.yml",
		cpath+"/"+"repository.yml"); err != nil {
		util.StatusMessage(util.VERBOSITY_VERBOSE, " failed\n")
		return err
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE, " success!\n")

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
		repoVars := repoItf.(map[interface{}]interface{})

		if repoVars["type"] != "github" {
			return nil, util.NewNewtError("Only github repositories are currently supported.")
		}

		rversreq := repoVars["vers"].(string)
		dl := downloader.NewGithubDownloader()
		dl.User = repoVars["user"].(string)
		dl.Repo = repoVars["repo"].(string)

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

	if r.name == REPO_NAME_LOCAL {
		r.localPath = filepath.Clean(path)
	} else {
		r.localPath = filepath.Clean(path + "/" + REPOS_DIR + "/" + r.name)
	}

	return nil
}

func NewRepo(repoName string, rversreq string, d downloader.Downloader) (*Repo, error) {
	r := &Repo{}

	if err := r.Init(repoName, rversreq, d); err != nil {
		return nil, err
	}

	return r, nil
}

func NewLocalRepo() (*Repo, error) {
	r := &Repo{}

	if err := r.Init(REPO_NAME_LOCAL, "", nil); err != nil {
		return nil, err
	}

	return r, nil
}
