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
	"log"
	"os"
	"path/filepath"
	"strings"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/interfaces"
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
	versreq    []interfaces.VersionReqInterface
	rdesc      *RepoDesc
}

type RepoDesc struct {
	name string
	vers map[*Version]string
}

func (rd *RepoDesc) MatchVersion(searchVers *Version) (string, *Version, bool) {
	for vers, curBranch := range rd.vers {
		if vers.CompareVersions(vers, searchVers) == 0 {
			return curBranch, vers, true
		}
	}
	return "", nil, false
}

func (rd *RepoDesc) Match(r *Repo) (string, *Version, bool) {
	for vers, branch := range rd.vers {
		if vers.SatisfiesVersion(r.VersionRequirements()) {
			log.Printf("[DEBUG] Found matching version %s for repo %s",
				vers.String(), r.Name())
			if vers.Stability() != "none" {
				searchVers, err := LoadVersion(branch)
				if err != nil {
					return "", nil, false
				}

				var ok bool
				branch, vers, ok = rd.MatchVersion(searchVers)
				if !ok {
					return "", nil, false
				}
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
		log.Printf("[DEBUG] Printing version %s for remote repo %s", versStr, name)
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

func (r *Repo) Name() string {
	return r.name
}

func (r *Repo) Path() string {
	return r.localPath
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

func (r *Repo) Install(rdesc *RepoDesc, force bool) (*Version, error) {
	// Copy the git repo into /repos/, error'ing out if the repo already exists
	if cli.NodeExist(r.Path()) {
		if force {
			if err := os.RemoveAll(r.Path()); err != nil {
				return nil, util.NewNewtError(err.Error())
			}
		} else {
			return nil, util.NewNewtError(fmt.Sprintf("Repository %s already "+
				"exists in local tree, cannot install.  Provide -f to override.", r.Path()))
		}
	}

	branchName, vers, found := rdesc.Match(r)
	if !found {
		return nil, util.NewNewtError(fmt.Sprintf("No repository matching description %s found",
			rdesc.String()))
	}

	dl := r.downloader

	// Download the git repo, returns the git repo, checked out to that branch
	tmpdir, err := dl.DownloadRepo(branchName)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Error download repository %s, : %s",
			r.Name(), err.Error()))
	}

	// Copy the Git repo into the the desired local path of the repo
	if err := cli.CopyDir(tmpdir, r.Path()); err != nil {
		// Cleanup any directory that might have been created if we error out
		// here.
		os.RemoveAll(r.Path())
		return nil, err
	}

	return vers, nil
}

// Download the repository description.
func (r *Repo) DownloadDesc() error {
	dl := r.downloader

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, fmt.Sprintf("Downloading "+
		"repository description for %s...", r.Name()))

	// Configuration path
	cpath := r.repoFilePath()
	if cli.NodeNotExist(cpath) {
		if err := os.MkdirAll(cpath, REPO_DEFAULT_PERMS); err != nil {
			return util.NewNewtError(err.Error())
		}
	}

	dl.SetBranch("master")
	if err := dl.FetchFile("repository.yml",
		cpath+"/"+"repository.yml"); err != nil {
		cli.StatusMessage(cli.VERBOSITY_DEFAULT, " failed\n")
		return err
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, " success!\n")

	return nil
}

func (r *Repo) ReadDesc() (*RepoDesc, error) {
	if cli.NodeNotExist(r.repoFilePath() + REPO_FILE_NAME) {
		return nil,
			util.NewNewtError("No configuration exists for repository " + r.name)
	}

	v, err := util.ReadConfig(r.repoFilePath(),
		strings.TrimSuffix(REPO_FILE_NAME, ".yml"))
	if err != nil {
		return nil, err
	}

	name := v.GetString("repo.name")
	versMap := v.GetStringMapString("repo.versions")

	rdesc, err := NewRepoDesc(name, versMap)
	if err != nil {
		return nil, err
	}
	r.rdesc = rdesc

	return rdesc, nil
}

func (r *Repo) Init(repoName string, rversreq string, d downloader.Downloader) error {
	var err error

	r.name = repoName
	r.downloader = d
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
