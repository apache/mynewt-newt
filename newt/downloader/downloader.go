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
	"regexp"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

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

	// Fetches all remotes.
	Fetch(path string) error

	// Checks out the specified commit (hash, tag, or branch).  Always puts the
	// repo in a "detached head" state.
	Checkout(path string, commit string) error

	// Recursively updates all submodules in a git repo
	UpdateSubmodules(path string) error

	// Recursively update a given submodule in a git repo
	UpdateSubmodule(path string, submodule string) error

	// Indicates whether the repo is in a clean or dirty state.
	DirtyState(path string) (string, error)

	// Determines the type of the specified commit.
	CommitType(path string, commit string) (DownloaderCommitType, error)

	// Configures the `origin` remote with the correct URL, according the the
	// user's `project.yml` file and / or the repo dependency lists.
	FixupOrigin(path string) error

	// Retrieves the name of the currently checked out local branch, or "" if
	// the repo is in a "detached head" state.
	CurrentBranch(path string) (string, error)

	// LatestRc finds the commit of the latest release candidate.  It looks
	// for commits with names matching the base commit string, but with with
	// "_rc#" inserted.  This is useful when a release candidate is being
	// tested.  In this case, the "rc" tags exist, but the official release
	// tag has not been created yet.
	//
	// If such a commit exists, it is returned.  Otherwise, "" is returned.
	LatestRc(path string, base string) (string, error)

	// Returns the branch that contains the YAML control files; this option
	// allows implementers to override "master" as the main branch.
	MainBranch() string
}

type Commit struct {
	hash string
	name string
	typ  DownloaderCommitType
}

type GenericDownloader struct {
	// [name-of-branch-or-tag]commit
	commits map[string]Commit

	// Hash of checked-out commit.
	head string

	// Whether 'origin' has been fetched during this run.
	fetched bool
}

type GithubDownloader struct {
	GenericDownloader
	Server string
	User   string
	Repo   string
	Branch string

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
	Url    string
	Branch string
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

// fixupCommitString strips "origin/" from the front of a commit, if it is
// present.  Newt only works with remote branches, and only with the "origin"
// remote.  The user is not required to prefix his branch specifiers with
// "origin/", but is allowed to.
func fixupCommitString(s string) string {
	return strings.TrimPrefix(s, "origin/")
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
	util.OneTimeWarning(
		"Repo's \"origin\" remote points to unexpected URL: "+
			"%s; correcting it to %s.  Repo contents may be incorrect.",
		curUrl, goodUrl)
}

// getCommits gathers all tags and remote branches.  It returns a mapping of
// [name]commit.
func getCommits(path string) (map[string]Commit, error) {
	cmd := []string{"show-ref", "--dereference"}
	o, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return nil, err
	}

	// Example output:
	// b7a5474d569d5b67152d1773627ddda010c080a3 refs/remotes/origin/1_7_0_dev
	// da13fb50c3b5824c47a44b62c3c9f693b922ce9c refs/tags/mynewt_1_7_0_tag
	// b7a5474d569d5b67152d1773627ddda010c080a3 refs/tags/mynewt_1_7_0_tag^{}

	m := map[string]Commit{}

	lines := strings.Split(strings.TrimSpace(string(o)), "\n")
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) != 2 {
			return nil, util.FmtNewtError(
				"git show-ref produced unexpected line: \"%s\"", line)
		}

		hash := f[0]
		ref := strings.TrimSuffix(f[1], "^{}")

		c := Commit{
			hash: hash,
		}
		if n := strings.TrimPrefix(ref, "refs/remotes/origin/"); n != ref {
			c.typ = COMMIT_TYPE_BRANCH
			c.name = n
		} else if n := strings.TrimPrefix(ref, "refs/tags/"); n != ref {
			c.typ = COMMIT_TYPE_TAG
			c.name = n
		}

		if c.name != "" {
			m[c.name] = c
		}
	}

	return m, nil
}

// urlsEquivalent determines if two URLs point to the same repo.  URLs are
// equivalent if:
//     1. The strings are identical after the optional ".git" suffixes are
//        stripped,
//          OR
//     2. One is a "git@" URL and the other is an "https://" URL for the same
//        repo.  For example:
//            git@github.com:apache/mynewt-core.git
//            https://github.com/apache/mynewt-core
func urlsEquivalent(a string, b string) bool {
	// Strip optional `.git` suffix.
	a = strings.TrimSuffix(a, ".git")
	b = strings.TrimSuffix(b, ".git")

	if a == b {
		return true
	}

	gitRE := regexp.MustCompile(`git@([^:]+):(.*)`)

	parseGit := func(s string) string {
		groups := gitRE.FindStringSubmatch(s)
		if len(groups) != 3 {
			return ""
		}

		return groups[1] + "/" + groups[2]
	}

	parseHttps := func(s string) string {
		if !strings.HasPrefix(s, "https://") {
			return ""
		}

		return strings.TrimPrefix(s, "https://")
	}

	var git string
	var https string

	git = parseGit(a)
	if git == "" {
		git = parseGit(b)
		https = parseHttps(a)
	} else {
		https = parseHttps(b)
	}

	return git != "" && git == https
}

// init populates a generic downloader with branch and tag information.
func (gd *GenericDownloader) init(path string) error {
	cmap, err := getCommits(path)
	if err != nil {
		return err
	}
	gd.commits = cmap

	cmd := []string{"rev-parse", "HEAD"}
	o, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return err
	}
	gd.head = strings.TrimSpace(string(o))

	return nil
}

// ensureInited calls init on the provided downloader if it has not already
// been initialized.
func (gd *GenericDownloader) ensureInited(path string) error {
	if gd.commits != nil {
		// Already initialized.
		return nil
	}

	return gd.init(path)
}

// untrackedFilesFromCheckoutErr collects the list of untracked files that
// prevented a checkout from succeeding.  It returns nil if the provided error
// does not indicate that untracked files are in the way.
func untrackedFilesFromCheckoutErr(err error) []string {
	var files []string

	text := err.Error()
	lines := strings.Split(text, "\n")

	collecting := false
	for _, line := range lines {
		if !collecting {
			if strings.Contains(line,
				"The following untracked working tree files would "+
					"be overwritten by checkout:") {
				collecting = true
			}
		} else {
			if strings.Contains(line, "Please move or remove them before") {
				collecting = false
			} else {
				files = append(files, strings.TrimSpace(line))
			}
		}
	}

	return files
}

func (gd *GenericDownloader) Checkout(repoDir string, commit string) error {
	// Get the hash corresponding to the commit in case the caller specified a
	// branch or tag.  We always want to check out a hash and end up in a
	// "detached head" state.
	hash, err := gd.HashFor(repoDir, commit)
	if err != nil {
		return err
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Will checkout %s\n", hash)
	cmd := []string{
		"checkout",
		hash,
	}

	_, err = executeGitCommand(repoDir, cmd, true)
	return err
}

// Update one submodule tree in a repo (under path)
func (gd *GenericDownloader) UpdateSubmodule(path string, submodule string) error {
	cmd := []string{
		"submodule",
		"update",
		"--init",
		"--recursive",
		submodule,
	}

	_, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return err
	}

	return nil
}

// Update all submodules in a repo (under path)
func (gd *GenericDownloader) UpdateSubmodules(path string) error {
	cmd := []string{
		"submodule",
		"update",
		"--init",
		"--recursive",
	}

	_, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return err
	}

	return nil
}

func (gd *GenericDownloader) showFile(
	path string, commit string, filename string, dstDir string) error {

	if err := os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return util.ChildNewtError(err)
	}

	hash, err := gd.HashFor(path, commit)
	if err != nil {
		return err
	}

	dstPath := fmt.Sprintf("%s/%s", dstDir, filename)
	log.Debugf("Fetching file %s to %s", filename, dstPath)

	cmd := []string{"show", fmt.Sprintf("%s:%s", hash, filename)}
	data, err := executeGitCommand(path, cmd, true)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(dstPath, data, os.ModePerm); err != nil {
		return util.ChildNewtError(err)
	}

	return nil
}

func (gd *GenericDownloader) findCommit(s string) *Commit {
	c, ok := gd.commits[fixupCommitString(s)]
	if !ok {
		return nil
	} else {
		return &c
	}
}

func (gd *GenericDownloader) CommitType(
	path string, commit string) (DownloaderCommitType, error) {

	if err := gd.ensureInited(path); err != nil {
		return -1, err
	}

	// HEAD is always a commit hash (detached).
	if commit == "HEAD" {
		return COMMIT_TYPE_HASH, nil
	}

	// Check if user provided a branch or tag name.
	if c := gd.findCommit(commit); c != nil {
		return c.typ, nil
	}

	// Check if user provided a commit hash.
	if _, err := mergeBase(path, commit); err == nil {
		return COMMIT_TYPE_HASH, nil
	}

	return -1, util.FmtNewtError(
		"cannot determine commit type of \"%s\"", commit)
}

func (gd *GenericDownloader) HashFor(path string,
	commit string) (string, error) {

	if err := gd.ensureInited(path); err != nil {
		return "", err
	}

	if commit == "HEAD" {
		return gd.head, nil
	}

	if c := gd.findCommit(commit); c != nil {
		return c.hash, nil
	}

	return commit, nil
}

func (gd *GenericDownloader) CommitsFor(
	path string, commit string) ([]string, error) {

	if err := gd.ensureInited(path); err != nil {
		return nil, err
	}

	commit = fixupCommitString(commit)

	cm := map[string]struct{}{}

	// Add all cm that are equivalent to the specified string.
	for _, c := range gd.commits {
		if commit == c.hash {
			// User specified a hash; add the corresponding branch or tag name.
			cm[c.name] = struct{}{}
		} else if commit == c.name {
			// User specified a branch or tag; add the corresponding hash.
			cm[c.hash] = struct{}{}
		}
	}

	// If the user specified a hash, add the hash itself.
	ct, err := gd.CommitType(path, commit)
	if err != nil {
		return nil, err
	}

	if ct == COMMIT_TYPE_HASH {
		hash, err := gd.HashFor(path, commit)
		if err != nil {
			return nil, err
		}
		cm[hash] = struct{}{}
	}

	// Sort the list of commit strings.
	var commits []string
	for cstring, _ := range cm {
		commits = append(commits, cstring)
	}
	sort.Strings(commits)

	return commits, nil
}

func (gd *GenericDownloader) CurrentBranch(path string) (string, error) {
	// Check if there is a git ref (branch) for the current commit.  If there
	// is none, git exits with a status of 1.  We need to distinguish this case
	// from an actual error.
	cmd := []string{"symbolic-ref", "-q", "HEAD"}
	o, err := executeGitCommand(path, cmd, true)
	if err != nil {
		ne := err.(*util.NewtError)
		ee, ok := ne.Parent.(*exec.ExitError)
		if ok && ee.ExitCode() == 1 {
			// No branch.
			return "", nil
		} else {
			return "", err
		}
	}

	s := strings.TrimSpace(string(o))
	branch := strings.TrimPrefix(s, "refs/heads/")
	if branch == s {
		return "", util.FmtNewtError(
			"%s produced unexpected output: %s", strings.Join(cmd, " "), s)
	}

	return branch, nil
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

	// If on a branch with a configured upstream, check for unpushed commits.
	branch, err := gd.CurrentBranch(path)
	if err != nil {
		return "", err
	}

	upstream, err := upstreamFor(path, "HEAD")
	if err != nil {
		return "", err
	}

	if upstream != "" && branch != "" {
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

func (gd *GenericDownloader) LatestRc(path string,
	base string) (string, error) {

	if err := gd.ensureInited(path); err != nil {
		return "", err
	}

	// Example:
	// [BASE] mynewt_1_7_0_tag
	// [RC]   mynewt_1_7_0_rc1_tag

	notag := strings.TrimSuffix(base, "_tag")
	if notag == base {
		return base, nil
	}

	restr := fmt.Sprintf("^%s_rc(\\d+)_tag$", regexp.QuoteMeta(notag))
	re := regexp.MustCompile(restr)

	bestNum := -1
	bestStr := ""
	for commit, _ := range gd.commits {
		match := re.FindStringSubmatch(commit)
		if len(match) >= 2 {
			num, _ := strconv.Atoi(match[1])
			if num > bestNum {
				bestNum = num
				bestStr = commit
			}
		}
	}

	if bestStr == "" {
		bestStr = base
	}

	return bestStr, nil
}

func (gd *GithubDownloader) Fetch(repoDir string) error {
	return gd.cachedFetch(func() error {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Fetching repo %s\n",
			gd.Repo)

		cmd := []string{"fetch", "--tags"}
		if util.ShallowCloneDepth > 0 {
			cmd = append(cmd, "--depth", strconv.Itoa(util.ShallowCloneDepth))
		}
		_, err := gd.authenticatedCommand(repoDir, cmd)
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

	if err := gd.Fetch(path); err != nil {
		return err
	}

	if err := gd.showFile(path, commit, filename, dstDir); err != nil {
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
	branch := gd.MainBranch()

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
	}

	if util.ShallowCloneDepth > 0 {
		cmd = append(cmd, "--depth", strconv.Itoa(util.ShallowCloneDepth))
	}

	cmd = append(cmd, url, dstPath)

	if util.Verbosity >= util.VERBOSITY_VERBOSE {
		err = util.ShellInteractiveCommand(cmd, nil, false)
	} else {
		_, err = util.ShellCommand(cmd, nil)
	}
	if err != nil {
		return err
	}
	defer gd.clearRemoteAuth(dstPath)

	if err := gd.Checkout(dstPath, commit); err != nil {
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
	if urlsEquivalent(curUrl, publicUrl) {
		return nil
	}

	warnWrongOriginUrl(path, curUrl, publicUrl)
	return gd.setOriginUrl(path, publicUrl)
}

func (gd *GithubDownloader) MainBranch() string {
	if gd.Branch != "" {
		return gd.Branch
	} else {
		return "master"
	}
}

func NewGithubDownloader() *GithubDownloader {
	return &GithubDownloader{}
}

func (gd *GitDownloader) Fetch(repoDir string) error {
	return gd.cachedFetch(func() error {
		cmd := []string{"fetch", "--tags"}
		if util.ShallowCloneDepth > 0 {
			cmd = append(cmd, "--depth", strconv.Itoa(util.ShallowCloneDepth))
		}
		_, err := executeGitCommand(repoDir, cmd, true)
		return err
	})
}

func (gd *GitDownloader) FetchFile(
	commit string, path string, filename string, dstDir string) error {

	if err := gd.Fetch(path); err != nil {
		return err
	}

	if err := gd.showFile(path, commit, filename, dstDir); err != nil {
		return err
	}

	return nil
}

func (gd *GitDownloader) Clone(commit string, dstPath string) error {
	branch := gd.MainBranch()

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
	}

	if util.ShallowCloneDepth > 0 {
		cmd = append(cmd, "--depth", strconv.Itoa(util.ShallowCloneDepth))
	}

	cmd = append(cmd, gd.Url, dstPath)

	if util.Verbosity >= util.VERBOSITY_VERBOSE {
		err = util.ShellInteractiveCommand(cmd, nil, false)
	} else {
		_, err = util.ShellCommand(cmd, nil)
	}
	if err != nil {
		return err
	}

	if err := gd.Checkout(dstPath, commit); err != nil {
		return err
	}

	return nil
}

func (gd *GitDownloader) FixupOrigin(path string) error {
	curUrl, err := getRemoteUrl(path, "origin")
	if err != nil {
		return err
	}

	if urlsEquivalent(curUrl, gd.Url) {
		return nil
	}

	warnWrongOriginUrl(path, curUrl, gd.Url)
	return setRemoteUrl(path, "origin", gd.Url, true)
}

func (gd *GitDownloader) MainBranch() string {
	if gd.Branch != "" {
		return gd.Branch
	} else {
		return "master"
	}
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

func (ld *LocalDownloader) Fetch(path string) error {
	os.RemoveAll(path)
	return ld.Clone(ld.MainBranch(), path)
}

func (ld *LocalDownloader) Checkout(path string, commit string) error {
	_, err := executeGitCommand(path, []string{"checkout", commit}, true)
	return err
}

func (ld *LocalDownloader) Clone(commit string, dstPath string) error {
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Downloading local repository %s\n", ld.Path)

	if err := util.CopyDir(ld.Path, dstPath); err != nil {
		return err
	}

	if err := ld.Checkout(dstPath, commit); err != nil {
		return err
	}

	return nil
}

func (ld *LocalDownloader) FixupOrigin(path string) error {
	return nil
}

func (gd *LocalDownloader) MainBranch() string {
	return "master"
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
		gd.Branch = repoVars["branch"]

		// The project.yml file can contain github access tokens and
		// authentication credentials, but this file is probably world-readable
		// and therefore not a great place for this.
		gd.Login = repoVars["login"]
		gd.Password = repoVars["password"]
		gd.PasswordEnv = repoVars["password_env"]

		// Alternatively, the user can put security material in
		// $HOME/.newt/repos.yml.
		newtrc := settings.Newtrc()
		privRepo, err := newtrc.GetValStringMapString("repository."+repoName, nil)
		util.OneTimeWarningError(err)
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
		gd.Branch = repoVars["branch"]
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
