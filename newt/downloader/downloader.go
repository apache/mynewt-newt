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

package downloader

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/util"
)

type Downloader interface {
	FetchFile(name string, dest string) error
	Branch() string
	SetBranch(branch string)
	DownloadRepo(branch string) (string, error)
}

type GenericDownloader struct {
	branch string
}

type GithubDownloader struct {
	GenericDownloader
	User string
	Repo string
}

func (gd *GenericDownloader) Branch() string {
	return gd.branch
}

func (gd *GenericDownloader) SetBranch(branch string) {
	gd.branch = branch
}

func (gd *GenericDownloader) TempDir() (string, error) {
	dir, err := ioutil.TempDir("", "newt-tmp")
	return dir, err
}

func (gd *GithubDownloader) FetchFile(name string, dest string) error {

	fmtStr := "https://raw.githubusercontent.com/%s/%s/%s/%s"
	url := fmt.Sprintf(fmtStr, gd.User, gd.Repo, gd.Branch(), name)

	log.Printf("[DEBUG] Fetching file %s (url: %s) to %s", name, url, dest)

	rsp, err := http.Get(url)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer rsp.Body.Close()

	handle, err := os.Create(dest)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer handle.Close()

	_, err = io.Copy(handle, rsp.Body)

	return nil
}

func (gd *GithubDownloader) DownloadRepo(branch string) (string, error) {
	// Get a temporary directory, and copy the repository into that directory.
	tmpdir, err := ioutil.TempDir("", "newt-repo")
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://github.com/%s/%s.git", gd.User, gd.Repo)

	_, err = cli.ShellCommand(fmt.Sprintf("git clone -b %s %s %s",
		branch, url, tmpdir))
	if err != nil {
		return "", err
	}

	return tmpdir, nil
}

func NewGithubDownloader() *GithubDownloader {
	return &GithubDownloader{}
}
