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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

type Downloader interface {
	FetchFile(name string, dest string) error
	Branch() string
	SetBranch(branch string)
	DownloadRepo(branch string) (string, error)
	CurrentBranch(path string) (string, error)
	UpdateRepo(path string, branchName string) error
	CleanupRepo(path string, branchName string) error
	LocalDiff(path string) ([]byte, error)
}

type GenericDownloader struct {
	branch string
}

type GithubDownloader struct {
	GenericDownloader
	Server string
	User   string
	Repo   string

	// Login for private repos.
	Login string

	// Password for private repos.
	Password string

	// Name of environment variable containing the password for private repos.
	// Only used if the Password field is empty.
	PasswordEnv string
}

type LocalDownloader struct {
	GenericDownloader

	// Path to parent directory of repository.yml file.
	Path string
}

func executeGitCommand(dir string, cmd []string) ([]byte, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, util.NewNewtError(err.Error())
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Can't find git binary: %s\n",
			err.Error()))
	}
	gitPath = filepath.ToSlash(gitPath)

	if err := os.Chdir(dir); err != nil {
		return nil, util.NewNewtError(err.Error())
	}

	defer os.Chdir(wd)

	gitCmd := []string{gitPath}
	gitCmd = append(gitCmd, cmd...)
	output, err := util.ShellCommand(gitCmd, nil)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func isTag(repoDir string, branchName string) bool {
	cmd := []string{"tag", "--list"}
	output, _ := executeGitCommand(repoDir, cmd)
	return strings.Contains(string(output), branchName)
}

func branchExists(repoDir string, branchName string) bool {
	cmd := []string{"show-ref", "--verify", "--quiet", "refs/heads/" + branchName}
	_, err := executeGitCommand(repoDir, cmd)
	return err == nil
}

// checkout does checkout a branch, or create a new branch from a tag name
// if the commit supplied is a tag. sha1 based commits have no special
// handling and result in dettached from HEAD state.
func checkout(repoDir string, commit string) error {
	var cmd []string
	if isTag(repoDir, commit) && !branchExists(repoDir, commit) {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Will create new branch %s"+
			" from tag %s\n", commit, "tags/"+commit)
		cmd = []string{
			"checkout",
			"tags/" + commit,
			"-b",
			commit,
		}
	} else {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Will checkout branch %s\n",
			commit)
		cmd = []string{
			"checkout",
			commit,
		}
	}
	_, err := executeGitCommand(repoDir, cmd)
	return err
}

// mergeBranches applies upstream changes to the local copy and must be
// preceeded by a "fetch" to achieve any meaningful result.
func mergeBranches(repoDir string) {
	branches := []string{"master", "develop"}
	for _, branch := range branches {
		err := checkout(repoDir, branch)
		if err != nil {
			continue
		}
		_, err = executeGitCommand(repoDir, []string{"merge", "origin/" + branch})
		if err != nil {
			util.StatusMessage(util.VERBOSITY_VERBOSE, "Merging changes from origin/%s: %s\n",
				branch, err)
		} else {
			util.StatusMessage(util.VERBOSITY_VERBOSE, "Merging changes from origin/%s\n",
				branch)
		}
		// XXX: ignore error, probably resulting from a branch not available at
		//      origin anymore.
	}
}

func fetch(repoDir string) error {
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Fetching new remote branches/tags\n")
	_, err := executeGitCommand(repoDir, []string{"fetch", "--tags"})
	return err
}

// stash saves current changes locally and returns if a new stash was
// created (if there where no changes, there's no need to stash)
func stash(repoDir string) (bool, error) {
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Stashing local changes\n")
	output, err := executeGitCommand(repoDir, []string{"stash"})
	if err != nil {
		return false, err
	}
	return strings.Contains(string(output), "Saved"), nil
}

func stashPop(repoDir string) error {
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Un-stashing local changes\n")
	_, err := executeGitCommand(repoDir, []string{"stash", "pop"})
	return err
}

func clean(repoDir string) error {
	_, err := executeGitCommand(repoDir, []string{"clean", "-f"})
	return err
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

func (gd *GithubDownloader) password() string {
	if gd.Password != "" {
		return gd.Password
	} else if gd.PasswordEnv != "" {
		return os.Getenv(gd.PasswordEnv)
	} else {
		return ""
	}
}

func (gd *GithubDownloader) FetchFile(name string, dest string) error {
	var url string
	if gd.Server != "" {
		// Use the github API
		url = fmt.Sprintf("https://%s/api/v3/repos/%s/%s/%s?ref=%s", gd.Server, gd.User, gd.Repo, name, gd.Branch())
	} else {
		// The public github API is ratelimited. Avoid rate limit issues with the raw endpoint.
		url = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", gd.User, gd.Repo, gd.Branch(), name)
	}

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Add("Accept", "application/vnd.github.v3.raw")

	pw := gd.password()
	if pw != "" {
		// XXX: Add command line option to include password in log.
		log.Debugf("Using basic auth; login=%s", gd.Login)
		req.SetBasicAuth(gd.Login, pw)
	}

	log.Debugf("Fetching file %s (url: %s) to %s", name, url, dest)
	client := &http.Client{}
	rsp, err := client.Do(req)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer rsp.Body.Close()

	if rsp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("Failed to download '%s'; status=%s",
			url, rsp.Status)
		switch rsp.StatusCode {
		case http.StatusNotFound:
			errMsg += "; URL incorrect or repository private?"
		case http.StatusUnauthorized:
			errMsg += "; credentials incorrect?"
		}

		return util.NewNewtError(errMsg)
	}

	handle, err := os.Create(dest)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer handle.Close()

	_, err = io.Copy(handle, rsp.Body)

	return nil
}

func (gd *GithubDownloader) CurrentBranch(path string) (string, error) {
	cmd := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	branch, err := executeGitCommand(path, cmd)
	return strings.Trim(string(branch), "\r\n"), err
}

func (gd *GithubDownloader) UpdateRepo(path string, branchName string) error {
	err := fetch(path)
	if err != nil {
		return err
	}

	stashed, err := stash(path)
	if err != nil {
		return err
	}

	mergeBranches(path)

	err = checkout(path, branchName)
	if err != nil {
		return err
	}

	if stashed {
		return stashPop(path)
	}

	return nil
}

func (gd *GithubDownloader) CleanupRepo(path string, branchName string) error {
	_, err := stash(path)
	if err != nil {
		return err
	}

	err = clean(path)
	if err != nil {
		return err
	}

	// TODO: needs handling of non-tracked files

	return gd.UpdateRepo(path, branchName)
}

func (gd *GithubDownloader) LocalDiff(path string) ([]byte, error) {
	return executeGitCommand(path, []string{"diff"})
}

func (gd *GithubDownloader) DownloadRepo(commit string) (string, error) {
	// Get a temporary directory, and copy the repository into that directory.
	tmpdir, err := ioutil.TempDir("", "newt-repo")
	if err != nil {
		return "", err
	}

	// Currently only the master branch is supported.
	branch := "master"
	server := "github.com"

	if gd.Server != "" {
		server = gd.Server
	}

	var auth string
	var publicAuth string
	if gd.Login != "" {
		pw := gd.password()
		auth = fmt.Sprintf("%s:%s@", gd.Login, pw)
		if pw == "" {
			publicAuth = auth
		} else {
			publicAuth = fmt.Sprintf("%s:<password-hidden>@", gd.Login)
		}
	}
	url := fmt.Sprintf("https://%s%s/%s/%s.git", auth, server, gd.User, gd.Repo)
	publicUrl := fmt.Sprintf("https://%s%s/%s/%s.git", publicAuth, server, gd.User, gd.Repo)
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Downloading "+
		"repository %s (branch: %s; commit: %s) at %s\n", gd.Repo, branch,
		commit, publicUrl)

	gitPath, err := exec.LookPath("git")
	if err != nil {
		os.RemoveAll(tmpdir)
		return "", util.NewNewtError(fmt.Sprintf("Can't find git binary: %s\n",
			err.Error()))
	}
	gitPath = filepath.ToSlash(gitPath)

	// Clone the repository.
	cmd := []string{
		gitPath,
		"clone",
		"-b",
		branch,
		url,
		tmpdir,
	}

	if util.Verbosity >= util.VERBOSITY_VERBOSE {
		if err := util.ShellInteractiveCommand(cmd, nil); err != nil {
			os.RemoveAll(tmpdir)
			return "", err
		}
	} else {
		if _, err := util.ShellCommand(cmd, nil); err != nil {
			return "", err
		}
	}

	// Checkout the specified commit.
	if err := checkout(tmpdir, commit); err != nil {
		return "", err
	}

	return tmpdir, nil
}

func NewGithubDownloader() *GithubDownloader {
	return &GithubDownloader{}
}

func (ld *LocalDownloader) FetchFile(name string, dest string) error {
	srcPath := ld.Path + "/" + name

	log.Debugf("Fetching file %s to %s", srcPath, dest)
	if err := util.CopyFile(srcPath, dest); err != nil {
		return err
	}

	return nil
}

func (ld *LocalDownloader) CurrentBranch(path string) (string, error) {
	cmd := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	branch, err := executeGitCommand(path, cmd)
	return strings.Trim(string(branch), "\r\n"), err
}

// NOTE: intentionally always error...
func (ld *LocalDownloader) UpdateRepo(path string, branchName string) error {
	return util.NewNewtError(fmt.Sprintf("Can't pull from a local repo\n"))
}

func (ld *LocalDownloader) CleanupRepo(path string, branchName string) error {
	os.RemoveAll(path)
	_, err := ld.DownloadRepo(branchName)
	return err
}

func (ld *LocalDownloader) LocalDiff(path string) ([]byte, error) {
	return executeGitCommand(path, []string{"diff"})
}

func (ld *LocalDownloader) DownloadRepo(commit string) (string, error) {
	// Get a temporary directory, and copy the repository into that directory.
	tmpdir, err := ioutil.TempDir("", "newt-repo")
	if err != nil {
		return "", err
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Downloading local repository %s\n", ld.Path)

	if err := util.CopyDir(ld.Path, tmpdir); err != nil {
		return "", err
	}

	// Checkout the specified commit.
	if err := checkout(tmpdir, commit); err != nil {
		return "", err
	}

	return tmpdir, nil
}

func NewLocalDownloader() *LocalDownloader {
	return &LocalDownloader{}
}

func LoadDownloader(repoName string, repoVars map[string]string) (
	Downloader, error) {

	switch repoVars["type"] {
	case "github":
		gd := NewGithubDownloader()

		gd.Server = repoVars["server"]
		gd.User = repoVars["user"]
		gd.Repo = repoVars["repo"]

		// The project.yml file can contain github access tokens and
		// authentication credentials, but this file is probably world-readable
		// and therefore not a great place for this.
		gd.Login = repoVars["login"]
		gd.Password = repoVars["password"]
		gd.PasswordEnv = repoVars["password_env"]

		// Alternatively, the user can put security material in
		// $HOME/.newt/repos.yml.
		newtrc := newtutil.Newtrc()
		privRepo := newtrc.GetStringMapString("repository." + repoName)
		if privRepo != nil {
			if gd.Login == "" {
				gd.Login = privRepo["login"]
			}
			if gd.Password == "" {
				gd.Password = privRepo["password"]
			}
			if gd.PasswordEnv == "" {
				gd.PasswordEnv = privRepo["password_env"]
			}
		}
		return gd, nil

	case "local":
		ld := NewLocalDownloader()
		ld.Path = repoVars["path"]
		return ld, nil

	default:
		return nil, util.FmtNewtError("Invalid repository type: %s",
			repoVars["type"])
	}
}
