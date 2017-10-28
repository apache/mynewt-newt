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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/settings"
	"mynewt.apache.org/newt/util"
)

type Downloader interface {
	FetchFile(path string, filename string, dstDir string) error
	Branch() string
	SetBranch(branch string)
	DownloadRepo(commit string, dstPath string) error
	CurrentBranch(path string) (string, error)
	UpdateRepo(path string, branchName string) error
	CleanupRepo(path string, branchName string) error
	LocalDiff(path string) ([]byte, error)
	AreChanges(path string) (bool, error)
}

type GenericDownloader struct {
	branch string

	// Whether 'origin' has been fetched during this run.
	fetched bool
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

type GitDownloader struct {
	GenericDownloader
	Url string
}

type LocalDownloader struct {
	GenericDownloader

	// Path to parent directory of repository.yml file.
	Path string
}

func gitPath() (string, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return "", util.NewNewtError(fmt.Sprintf("Can't find git binary: %s\n",
			err.Error()))
	}

	return filepath.ToSlash(gitPath), nil
}

func executeGitCommand(dir string, cmd []string, logCmd bool) ([]byte, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, util.NewNewtError(err.Error())
	}

	gp, err := gitPath()
	if err != nil {
		return nil, err
	}

	if err := os.Chdir(dir); err != nil {
		return nil, util.NewNewtError(err.Error())
	}

	defer os.Chdir(wd)

	gitCmd := []string{gp}
	gitCmd = append(gitCmd, cmd...)
	output, err := util.ShellCommandLimitDbgOutput(gitCmd, nil, logCmd, -1)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func isTag(repoDir string, branchName string) bool {
	cmd := []string{"tag", "--list"}
	output, _ := executeGitCommand(repoDir, cmd, true)
	return strings.Contains(string(output), branchName)
}

func branchExists(repoDir string, branchName string) bool {
	cmd := []string{
		"show-ref",
		"--verify",
		"--quiet",
		"refs/heads/" + branchName,
	}
	_, err := executeGitCommand(repoDir, cmd, true)
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
	_, err := executeGitCommand(repoDir, cmd, true)
	return err
}

// mergeBranches applies upstream changes to the local copy and must be
// preceeded by a "fetch" to achieve any meaningful result.
func mergeBranch(repoDir string, branch string) error {
	if err := checkout(repoDir, branch); err != nil {
		return err
	}

	fullName := "origin/" + branch
	if _, err := executeGitCommand(
		repoDir, []string{"merge", fullName}, true); err != nil {

		util.StatusMessage(util.VERBOSITY_VERBOSE,
			"Merging changes from %s: %s\n", fullName, err)
		return err
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Merging changes from %s\n", fullName)
	return nil
}

// stash saves current changes locally and returns if a new stash was
// created (if there where no changes, there's no need to stash)
func stash(repoDir string) (bool, error) {
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Stashing local changes\n")
	output, err := executeGitCommand(repoDir, []string{"stash"}, true)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(output), "Saved"), nil
}

func stashPop(repoDir string) error {
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Un-stashing local changes\n")
	_, err := executeGitCommand(repoDir, []string{"stash", "pop"}, true)
	return err
}

func clean(repoDir string) error {
	_, err := executeGitCommand(repoDir, []string{"clean", "-f"}, true)
	return err
}

func diff(repoDir string) ([]byte, error) {
	return executeGitCommand(repoDir, []string{"diff"}, true)
}

func areChanges(repoDir string) (bool, error) {
	cmd := []string{
		"diff",
		"--name-only",
	}

	o, err := executeGitCommand(repoDir, cmd, true)
	if err != nil {
		return false, err
	}

	return len(o) > 0, nil
}

func showFile(
	path string, branch string, filename string, dstDir string) error {

	if err := os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return util.ChildNewtError(err)
	}

	cmd := []string{
		"show",
		fmt.Sprintf("origin/%s:%s", branch, filename),
	}

	dstPath := fmt.Sprintf("%s/%s", dstDir, filename)
	log.Debugf("Fetching file %s to %s", filename, dstPath)
	data, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(dstPath, data, os.ModePerm); err != nil {
		return util.ChildNewtError(err)
	}

	return nil
}

func (gd *GenericDownloader) Branch() string {
	return gd.branch
}

func (gd *GenericDownloader) SetBranch(branch string) {
	gd.branch = branch
}

// Fetches the downloader's origin remote if it hasn't been fetched yet during
// this run.
func (gd *GenericDownloader) cachedFetch(fn func() error) error {
	if gd.fetched {
		return nil
	}

	if err := fn(); err != nil {
		return err
	}

	gd.fetched = true
	return nil
}

func (gd *GithubDownloader) fetch(repoDir string) error {
	return gd.cachedFetch(func() error {
		util.StatusMessage(util.VERBOSITY_VERBOSE,
			"Fetching new remote branches/tags\n")

		_, err := gd.authenticatedCommand(repoDir, []string{"fetch", "--tags"})
		return err
	})
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

func (gd *GithubDownloader) authenticatedCommand(path string,
	args []string) ([]byte, error) {

	if err := gd.setRemoteAuth(path); err != nil {
		return nil, err
	}
	defer gd.clearRemoteAuth(path)

	return executeGitCommand(path, args, true)
}

func (gd *GithubDownloader) FetchFile(
	path string, filename string, dstDir string) error {

	if err := gd.fetch(path); err != nil {
		return err
	}

	if err := showFile(path, gd.Branch(), filename, dstDir); err != nil {
		return err
	}

	return nil
}

func (gd *GithubDownloader) CurrentBranch(path string) (string, error) {
	cmd := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	branch, err := executeGitCommand(path, cmd, true)
	return strings.Trim(string(branch), "\r\n"), err
}

func (gd *GithubDownloader) UpdateRepo(path string, branchName string) error {
	err := gd.fetch(path)
	if err != nil {
		return err
	}

	stashed, err := stash(path)
	if err != nil {
		return err
	}

	// Ignore error, probably resulting from a branch not available at origin
	// anymore.
	mergeBranch(path, branchName)

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
	return diff(path)
}

func (gd *GithubDownloader) AreChanges(path string) (bool, error) {
	return areChanges(path)
}

func (gd *GithubDownloader) remoteUrls() (string, string) {
	server := "github.com"

	if gd.Server != "" {
		server = gd.Server
	}

	var auth string
	if gd.Login != "" {
		pw := gd.password()
		auth = fmt.Sprintf("%s:%s@", gd.Login, pw)
	}

	url := fmt.Sprintf("https://%s%s/%s/%s.git", auth, server, gd.User,
		gd.Repo)
	publicUrl := fmt.Sprintf("https://%s/%s/%s.git", server, gd.User, gd.Repo)

	return url, publicUrl
}

func (gd *GithubDownloader) setOriginUrl(path string, url string) error {
	genCmdStrs := func(urlStr string) []string {
		return []string{
			"remote",
			"set-url",
			"origin",
			urlStr,
		}
	}

	// Hide password in logged command.
	safeUrl := url
	pw := gd.password()
	if pw != "" {
		safeUrl = strings.Replace(safeUrl, pw, "<password-hidden>", -1)
	}
	util.LogShellCmd(genCmdStrs(safeUrl), nil)

	_, err := executeGitCommand(path, genCmdStrs(url), false)
	return err
}

func (gd *GithubDownloader) clearRemoteAuth(path string) error {
	url, publicUrl := gd.remoteUrls()
	if url == publicUrl {
		return nil
	}

	return gd.setOriginUrl(path, publicUrl)
}

func (gd *GithubDownloader) setRemoteAuth(path string) error {
	url, publicUrl := gd.remoteUrls()
	if url == publicUrl {
		return nil
	}

	return gd.setOriginUrl(path, url)
}

func (gd *GithubDownloader) DownloadRepo(commit string, dstPath string) error {
	// Currently only the master branch is supported.
	branch := "master"

	url, publicUrl := gd.remoteUrls()

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Downloading "+
		"repository %s (branch: %s; commit: %s) at %s\n", gd.Repo, branch,
		commit, publicUrl)

	gp, err := gitPath()
	if err != nil {
		return err
	}

	// Clone the repository.
	cmd := []string{
		gp,
		"clone",
		"-b",
		branch,
		url,
		dstPath,
	}

	if util.Verbosity >= util.VERBOSITY_VERBOSE {
		err = util.ShellInteractiveCommand(cmd, nil)
	} else {
		_, err = util.ShellCommand(cmd, nil)
	}
	if err != nil {
		return err
	}

	defer gd.clearRemoteAuth(dstPath)

	// Checkout the specified commit.
	if err := checkout(dstPath, commit); err != nil {
		return err
	}

	return nil
}

func NewGithubDownloader() *GithubDownloader {
	return &GithubDownloader{}
}

func (gd *GitDownloader) fetch(repoDir string) error {
	return gd.cachedFetch(func() error {
		util.StatusMessage(util.VERBOSITY_VERBOSE,
			"Fetching new remote branches/tags\n")
		_, err := executeGitCommand(repoDir, []string{"fetch", "--tags"}, true)
		return err
	})
}

func (gd *GitDownloader) FetchFile(
	path string, filename string, dstDir string) error {

	if err := gd.fetch(path); err != nil {
		return err
	}

	if err := showFile(path, gd.Branch(), filename, dstDir); err != nil {
		return err
	}

	return nil
}

func (gd *GitDownloader) CurrentBranch(path string) (string, error) {
	cmd := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	branch, err := executeGitCommand(path, cmd, true)
	return strings.Trim(string(branch), "\r\n"), err
}

func (gd *GitDownloader) UpdateRepo(path string, branchName string) error {
	err := gd.fetch(path)
	if err != nil {
		return err
	}

	stashed, err := stash(path)
	if err != nil {
		return err
	}

	// Ignore error, probably resulting from a branch not available at origin
	// anymore.
	mergeBranch(path, branchName)

	err = checkout(path, branchName)
	if err != nil {
		return err
	}

	if stashed {
		return stashPop(path)
	}

	return nil
}

func (gd *GitDownloader) CleanupRepo(path string, branchName string) error {
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

func (gd *GitDownloader) LocalDiff(path string) ([]byte, error) {
	return diff(path)
}

func (gd *GitDownloader) AreChanges(path string) (bool, error) {
	return areChanges(path)
}

func (gd *GitDownloader) DownloadRepo(commit string, dstPath string) error {
	// Currently only the master branch is supported.
	branch := "master"

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Downloading "+
		"repository %s (branch: %s; commit: %s)\n", gd.Url, branch, commit)

	gp, err := gitPath()
	if err != nil {
		return err
	}

	// Clone the repository.
	cmd := []string{
		gp,
		"clone",
		"-b",
		branch,
		gd.Url,
		dstPath,
	}

	if util.Verbosity >= util.VERBOSITY_VERBOSE {
		err = util.ShellInteractiveCommand(cmd, nil)
	} else {
		_, err = util.ShellCommand(cmd, nil)
	}
	if err != nil {
		return err
	}

	// Checkout the specified commit.
	if err := checkout(dstPath, commit); err != nil {
		return err
	}

	return nil
}

func NewGitDownloader() *GitDownloader {
	return &GitDownloader{}
}

func (ld *LocalDownloader) FetchFile(
	path string, filename string, dstDir string) error {

	srcPath := ld.Path + "/" + filename
	dstPath := dstDir + "/" + filename

	log.Debugf("Fetching file %s to %s", srcPath, dstPath)
	if err := util.CopyFile(srcPath, dstPath); err != nil {
		return err
	}

	return nil
}

func (ld *LocalDownloader) CurrentBranch(path string) (string, error) {
	cmd := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	branch, err := executeGitCommand(path, cmd, true)
	return strings.Trim(string(branch), "\r\n"), err
}

// NOTE: intentionally always error...
func (ld *LocalDownloader) UpdateRepo(path string, branchName string) error {
	return util.NewNewtError(fmt.Sprintf("Can't pull from a local repo\n"))
}

func (ld *LocalDownloader) CleanupRepo(path string, branchName string) error {
	os.RemoveAll(path)

	tmpdir, err := newtutil.MakeTempRepoDir()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	return ld.DownloadRepo(branchName, tmpdir)
}

func (ld *LocalDownloader) LocalDiff(path string) ([]byte, error) {
	return diff(path)
}

func (ld *LocalDownloader) AreChanges(path string) (bool, error) {
	return areChanges(path)
}

func (ld *LocalDownloader) DownloadRepo(commit string, dstPath string) error {
	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Downloading local repository %s\n", ld.Path)

	if err := util.CopyDir(ld.Path, dstPath); err != nil {
		return err
	}

	// Checkout the specified commit.
	if err := checkout(dstPath, commit); err != nil {
		return err
	}

	return nil
}

func NewLocalDownloader() *LocalDownloader {
	return &LocalDownloader{}
}

func loadError(format string, args ...interface{}) error {
	return util.NewNewtError(
		"error loading project.yml: " + fmt.Sprintf(format, args...))
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
		newtrc := settings.Newtrc()
		privRepo := newtrc.GetValStringMapString("repository."+repoName, nil)
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

	case "git":
		gd := NewGitDownloader()
		gd.Url = repoVars["url"]
		if gd.Url == "" {
			return nil, loadError("repo \"%s\" missing required field \"url\"",
				repoName)
		}
		return gd, nil

	case "local":
		ld := NewLocalDownloader()
		ld.Path = repoVars["path"]
		return ld, nil

	default:
		return nil, loadError("invalid repository type: %s", repoVars["type"])
	}
}
