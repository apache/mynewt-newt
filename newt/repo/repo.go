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
	"os"
	"path/filepath"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/util"
)

const REPO_NAME_LOCAL = "local"
const REPO_DEFAULT_PERMS = 0755

const REPOS_DIR = "repos"

type Repo struct {
	name       string
	downloader downloader.Downloader
	localPath  string
}

func (r *Repo) Name() string {
	return r.name
}

func (r *Repo) Path() string {
	return r.localPath
}

// Download the repository description.
func (r *Repo) DownloadDesc() error {
	dl := r.downloader

	dl.SetBranch("master")
	if err := dl.FetchFile("repository.yml",
		r.localPath+"/"+"repository.yml"); err != nil {
		return err
	}

	return nil
}

func (r *Repo) Init(basePath string, repoName string,
	d downloader.Downloader) error {
	r.name = repoName
	r.downloader = d

	if r.name == REPO_NAME_LOCAL {
		r.localPath = filepath.Clean(basePath)
	} else {
		r.localPath = filepath.Clean(basePath + "/" + REPOS_DIR + "/" + r.name)
	}

	// If local path doesn't exist, create it.
	if cli.NodeNotExist(r.localPath) {
		if err := os.MkdirAll(r.localPath, REPO_DEFAULT_PERMS); err != nil {
			return util.NewNewtError(err.Error())
		}
	}

	return nil
}

func NewRepo(basePath string, repoName string, d downloader.Downloader) (*Repo, error) {
	r := &Repo{}

	if err := r.Init(basePath, repoName, d); err != nil {
		return nil, err
	}

	return r, nil
}

func NewLocalRepo(basePath string) (*Repo, error) {
	r := &Repo{}

	if err := r.Init(basePath, REPO_NAME_LOCAL, nil); err != nil {
		return nil, err
	}

	return r, nil
}
