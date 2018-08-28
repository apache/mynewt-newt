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
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/settings"
	"mynewt.apache.org/newt/util"
)

type DownloaderCommitType int

const (
	COMMIT_TYPE_BRANCH DownloaderCommitType = iota
	COMMIT_TYPE_TAG
	COMMIT_TYPE_HASH
)

type Downloader interface {
	// Fetches all remotes and downloads the specified file.
	FetchFile(commit string, path string, filename string, dstDir string) error

	// Clones the repo and checks out the specified commit.
	Clone(commit string, dstPath string) error

	// Determines the equivalent commit hash for the specified commit string.
	HashFor(path string, commit string) (string, error)

	// Collects all commits that are equivalent to the specified commit string
	// (i.e., 1 hash, n tags, and n branches).
	CommitsFor(path string, commit string) ([]string, error)

	// Fetches all remotes and merges the specified branch into the local repo.
	Pull(path string, branchName string) error

	// Indicates whether the repo is in a clean or dirty state.
	DirtyState(path string) (string, error)

	// Determines the type of the specified commit.
	CommitType(path string, commit string) (DownloaderCommitType, error)

	// Configures the `origin` remote with the correct URL, according the the
	// user's `project.yml` file and / or the repo dependency lists.
	FixupOrigin(path string) error

	// Retrieves the name of the currently checked out branch, or "" if no
	// branch is checked out.
	CurrentBranch(path string) (string, error)

	// Retrieves the name of the remote branch being tracked by the specified
	// local branch, or "" if there is no tracked remote branch.
	UpstreamFor(repoDir string, branch string) (string, error)
}

type GenericDownloader struct {
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
		return nil, util.ChildNewtError(err)
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

func commitExists(repoDir string, commit string) bool {
	cmd := []string{
		"show-ref",
		"--verify",
		"--quiet",
		"refs/heads/" + commit,
	}
	_, err := executeGitCommand(repoDir, cmd, true)
	return err == nil
}

func initSubmodules(path string) error {
	cmd := []string{
		"submodule",
		"init",
	}

	_, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return err
	}

	return nil
}

func updateSubmodules(path string) error {
	cmd := []string{
		"submodule",
		"update",
	}

	_, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return err
	}

	return nil
}

// checkout does checkout a branch, or create a new branch from a tag name
// if the commit supplied is a tag. sha1 based commits have no special
// handling and result in dettached from HEAD state.
func checkout(repoDir string, commit string) error {
	var cmd []string
	ct, err := commitType(repoDir, commit)
	if err != nil {
		return err
	}

	full, err := remoteCommitName(repoDir, commit)
	if err != nil {
		return err
	}

	if ct == COMMIT_TYPE_TAG {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Will create new branch %s"+
			" from %s\n", commit, full)
		cmd = []string{
			"checkout",
			full,
			"-b",
			commit,
		}
	} else {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Will checkout %s\n", full)
		cmd = []string{
			"checkout",
			commit,
		}
	}
	if _, err := executeGitCommand(repoDir, cmd, true); err != nil {
		return err
	}

	// Always initialize and update submodules on checkout.  This prevents the
	// repo from being in a modified "(new commits)" state immediately after
	// switching commits.  If the submodules have already been updated, this
	// does not generate any network activity.
	if err := initSubmodules(repoDir); err != nil {
		return err
	}
	if err := updateSubmodules(repoDir); err != nil {
		return err
	}

	return nil
}

// rebase applies upstream changes to the local copy and must be
// preceeded by a "fetch" to achieve any meaningful result.
func rebase(repoDir string, commit string) error {
	if err := checkout(repoDir, commit); err != nil {
		return err
	}

	// We want to rebase the remote version of this branch.
	full, err := remoteCommitName(repoDir, commit)
	if err != nil {
		return err
	}

	cmd := []string{
		"rebase",
		full}
	if _, err := executeGitCommand(repoDir, cmd, true); err != nil {
		util.StatusMessage(util.VERBOSITY_VERBOSE,
			"Merging changes from %s: %s\n", full, err)
		return err
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Merging changes from %s\n", full)
	return nil
}

func mergeBase(repoDir string, commit string) (string, error) {
	cmd := []string{
		"merge-base",
		commit,
		commit,
	}
	o, err := executeGitCommand(repoDir, cmd, true)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(o)), nil
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

func commitType(repoDir string, commit string) (DownloaderCommitType, error) {
	if commit == "HEAD" {
		return COMMIT_TYPE_HASH, nil
	}

	if _, err := mergeBase(repoDir, commit); err == nil {
		// Distinguish local branch from hash.
		if branchExists(repoDir, commit) {
			return COMMIT_TYPE_BRANCH, nil
		} else {
			return COMMIT_TYPE_HASH, nil
		}
	}

	if _, err := mergeBase(repoDir, "tags/"+commit); err == nil {
		return COMMIT_TYPE_TAG, nil
	}

	return DownloaderCommitType(-1), util.FmtNewtError(
		"Cannot determine commit type of \"%s\"", commit)
}

func upstreamFor(path string, commit string) (string, error) {
	cmd := []string{
		"rev-parse",
		"--abbrev-ref",
		"--symbolic-full-name",
		commit + "@{u}",
	}

	up, err := executeGitCommand(path, cmd, true)
	if err != nil {
		if !util.IsExit(err) {
			return "", err
		} else {
			return "", nil
		}
	}

	return strings.TrimSpace(string(up)), nil
}

func remoteCommitName(path string, commit string) (string, error) {
	ct, err := commitType(path, commit)
	if err != nil {
		return "", err
	}

	switch ct {
	case COMMIT_TYPE_BRANCH:
		rmt, err := upstreamFor(path, commit)
		if err != nil {
			return "", err
		}
		if rmt == "" {
			return "",
				util.FmtNewtError("No remote upstream for branch \"%s\"",
					commit)
		}
		return rmt, nil
	case COMMIT_TYPE_TAG:
		return "tags/" + commit, nil
	case COMMIT_TYPE_HASH:
		return commit, nil
	default:
		return "", util.FmtNewtError("unknown commit type: %d", int(ct))
	}
}

func showFile(
	path string, branch string, filename string, dstDir string) error {

	if err := os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return util.ChildNewtError(err)
	}

	full, err := remoteCommitName(path, branch)
	if err != nil {
		return err
	}

	cmd := []string{
		"show",
		fmt.Sprintf("%s:%s", full, filename),
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

func getRemoteUrl(path string, remote string) (string, error) {
	cmd := []string{
		"remote",
		"get-url",
		remote,
	}

	o, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(o)), nil
}

func setRemoteUrlCmd(remote string, url string) []string {
	return []string{
		"remote",
		"set-url",
		remote,
		url,
	}
}

func setRemoteUrl(path string, remote string, url string, logCmd bool) error {
	cmd := setRemoteUrlCmd(remote, url)
	_, err := executeGitCommand(path, cmd, logCmd)
	return err
}

func warnWrongOriginUrl(path string, curUrl string, goodUrl string) {
	util.StatusMessage(util.VERBOSITY_QUIET,
		"WARNING: Repo's \"origin\" remote points to unexpected URL: "+
			"%s; correcting it to %s.  Repo contents may be incorrect.\n",
		curUrl, goodUrl)
}

func (gd *GenericDownloader) CommitType(
	path string, commit string) (DownloaderCommitType, error) {

	return commitType(path, commit)
}

func (gd *GenericDownloader) HashFor(path string, commit string) (string, error) {
	full, err := remoteCommitName(path, commit)
	if err != nil {
		return "", err
	}
	cmd := []string{"rev-parse", full}
	o, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(o)), nil
}

func (gd *GenericDownloader) CommitsFor(
	path string, commit string) ([]string, error) {

	// Hash.
	hash, err := gd.HashFor(path, commit)
	if err != nil {
		return nil, err
	}

	// Branches and tags.
	cmd := []string{
		"for-each-ref",
		"--format=%(refname:short)",
		"--points-at",
		hash,
	}
	o, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return nil, err
	}

	lines := []string{hash}
	text := strings.TrimSpace(string(o))
	if text != "" {
		lines = append(lines, strings.Split(text, "\n")...)
	}

	sort.Strings(lines)
	return lines, nil
}

func (gd *GenericDownloader) CurrentBranch(path string) (string, error) {
	cmd := []string{
		"rev-parse",
		"--abbrev-ref",
		"HEAD",
	}
	o, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return "", err
	}

	s := strings.TrimSpace(string(o))
	if s == "HEAD" {
		return "", nil
	} else {
		return s, nil
	}
}

func (gd *GenericDownloader) UpstreamFor(repoDir string,
	branch string) (string, error) {

	return upstreamFor(repoDir, branch)
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

// Indicates whether the specified git repo is in a clean or dirty state.
//
// @param path                  The path of the git repo to check.
//
// @return string               Text describing repo's dirty state, or "" if
//                                  clean.
// @return error                Error.
func (gd *GenericDownloader) DirtyState(path string) (string, error) {
	// Check for local changes.
	cmd := []string{
		"diff",
		"--name-only",
	}

	o, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return "", err
	}

	if len(o) > 0 {
		return "local changes", nil
	}

	// Check for staged changes.
	cmd = []string{
		"diff",
		"--name-only",
		"--staged",
	}

	o, err = executeGitCommand(path, cmd, true)
	if err != nil {
		return "", err
	}

	if len(o) > 0 {
		return "staged changes", nil
	}

	// If on a branch, check for unpushed commits.
	branch, err := gd.CurrentBranch(path)
	if err != nil {
		return "", err
	}

	if branch != "" {
		cmd = []string{
			"rev-list",
			"@{u}..",
		}

		o, err = executeGitCommand(path, cmd, true)
		if err != nil {
			return "", err
		}

		if len(o) > 0 {
			return "unpushed commits", nil
		}
	}

	return "", nil
}

func (gd *GithubDownloader) fetch(repoDir string) error {
	return gd.cachedFetch(func() error {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Fetching repo %s\n",
			gd.Repo)

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
	commit string, path string, filename string, dstDir string) error {

	if err := gd.fetch(path); err != nil {
		return err
	}

	if err := showFile(path, commit, filename, dstDir); err != nil {
		return err
	}

	return nil
}

func (gd *GithubDownloader) Pull(path string, branchName string) error {
	err := gd.fetch(path)
	if err != nil {
		return err
	}

	// Ignore error, probably resulting from a branch not available at origin
	// anymore.
	rebase(path, branchName)

	if err := checkout(path, branchName); err != nil {
		return err
	}

	return nil
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
	// Hide password in logged command.
	safeUrl := url
	pw := gd.password()
	if pw != "" {
		safeUrl = strings.Replace(safeUrl, pw, "<password-hidden>", -1)
	}
	util.LogShellCmd(setRemoteUrlCmd("origin", safeUrl), nil)

	return setRemoteUrl(path, "origin", url, false)
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

func (gd *GithubDownloader) Clone(commit string, dstPath string) error {
	// Currently only the master branch is supported.
	branch := "master"

	url, publicUrl := gd.remoteUrls()

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Downloading repository %s (commit: %s) from %s\n",
		gd.Repo, commit, publicUrl)

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

func (gd *GithubDownloader) FixupOrigin(path string) error {
	curUrl, err := getRemoteUrl(path, "origin")
	if err != nil {
		return err
	}

	// Use the public URL, i.e., hide the login and password.
	_, publicUrl := gd.remoteUrls()
	if curUrl == publicUrl {
		return nil
	}

	warnWrongOriginUrl(path, curUrl, publicUrl)
	return gd.setOriginUrl(path, publicUrl)
}

func NewGithubDownloader() *GithubDownloader {
	return &GithubDownloader{}
}

func (gd *GitDownloader) fetch(repoDir string) error {
	return gd.cachedFetch(func() error {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Fetching repo %s\n",
			gd.Url)
		_, err := executeGitCommand(repoDir, []string{"fetch", "--tags"}, true)
		return err
	})
}

func (gd *GitDownloader) FetchFile(
	commit string, path string, filename string, dstDir string) error {

	if err := gd.fetch(path); err != nil {
		return err
	}

	if err := showFile(path, commit, filename, dstDir); err != nil {
		return err
	}

	return nil
}

func (gd *GitDownloader) Pull(path string, branchName string) error {
	err := gd.fetch(path)
	if err != nil {
		return err
	}

	// Ignore error, probably resulting from a branch not available at origin
	// anymore.
	rebase(path, branchName)

	if err := checkout(path, branchName); err != nil {
		return err
	}

	return nil
}

func (gd *GitDownloader) Clone(commit string, dstPath string) error {
	// Currently only the master branch is supported.
	branch := "master"

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Downloading repository %s (commit: %s)\n", gd.Url, commit)

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

func (gd *GitDownloader) FixupOrigin(path string) error {
	curUrl, err := getRemoteUrl(path, "origin")
	if err != nil {
		return err
	}

	if curUrl == gd.Url {
		return nil
	}

	warnWrongOriginUrl(path, curUrl, gd.Url)
	return setRemoteUrl(path, "origin", gd.Url, true)
}

func NewGitDownloader() *GitDownloader {
	return &GitDownloader{}
}

func (ld *LocalDownloader) FetchFile(
	commit string, path string, filename string, dstDir string) error {

	srcPath := ld.Path + "/" + filename
	dstPath := dstDir + "/" + filename

	log.Debugf("Fetching file %s to %s", srcPath, dstPath)
	if err := util.CopyFile(srcPath, dstPath); err != nil {
		return err
	}

	return nil
}

func (ld *LocalDownloader) Pull(path string, branchName string) error {
	os.RemoveAll(path)
	return ld.Clone(branchName, path)
}

func (ld *LocalDownloader) Clone(commit string, dstPath string) error {
	util.StatusMessage(util.VERBOSITY_DEFAULT,
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

func (ld *LocalDownloader) FixupOrigin(path string) error {
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
