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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// URL used by test projects and dummy repos to refer to dummy repos.
const dummyRepoURL = "https://dummy.invalid/"

var upgradeDir = filepath.Join("testdata", "upgrade")

func runGit(t *testing.T, env []string, stdin *os.File,
	args ...string) string {

	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = stdin

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s\n%s", strings.Join(args, " "), err, out)
	}

	return strings.TrimSpace(string(out))
}

// dummyReposEnv imports dummy repos into a temporary directory and returns
// environment for git that makes the dummy repo URL refer to them. Global
// and system git configuration is not used.
func dummyReposEnv(t *testing.T) []string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	dir := t.TempDir()
	gitConfig := filepath.Join(dir, "gitconfig")
	env := []string{"GIT_CONFIG_GLOBAL=" + gitConfig, "GIT_CONFIG_NOSYSTEM=1"}

	reposURL := filepath.ToSlash(dir)
	if !strings.HasPrefix(reposURL, "/") {
		reposURL = "/" + reposURL
	}
	cfg := fmt.Sprintf("[url \"file://%s/\"]\n\tinsteadOf = %s\n", reposURL,
		dummyRepoURL)
	if err := os.WriteFile(gitConfig, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	streams, err := filepath.Glob(filepath.Join(upgradeDir, "dummy-repos",
		"*.fi"))
	if err != nil {
		t.Fatal(err)
	}

	for _, stream := range streams {
		repo := filepath.Join(dir,
			strings.TrimSuffix(filepath.Base(stream), ".fi")+".git")

		f, err := os.Open(stream)
		if err != nil {
			t.Fatal(err)
		}

		runGit(t, env, nil, "init", "-q", "--bare", repo)
		runGit(t, env, f, "-C", repo, "fast-import", "--quiet")
		runGit(t, env, nil, "-C", repo, "symbolic-ref", "HEAD",
			"refs/heads/master")
		f.Close()
	}

	return env
}

// installedRepos returns names of repos installed in project.
func installedRepos(t *testing.T, projDir string) []string {
	t.Helper()

	names := []string{}
	for _, name := range subdirs(t, filepath.Join(projDir, "repos")) {
		if !strings.HasPrefix(name, ".") {
			names = append(names, name)
		}
	}

	return names
}

// checkDummyRepoVersions compares versions of dummy repos reported by
// "newt info" with expected.txt in the project.
func checkDummyRepoVersions(t *testing.T, projDir string, env []string) {
	t.Helper()

	lines := []string{}
	out := newtOutput(t, projDir, env, "info")
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "mynewt-dummy") {
			lines = append(lines, line)
		}
	}

	checkOutput(t, "repo versions",
		readFile(t, filepath.Join(projDir, "expected.txt")),
		strings.Join(lines, "\n"))
}

// Each project in testdata/upgrade/success must install repos with versions
// listed in its expected.txt, both on initial install and on upgrade from the
// latest commit.
func TestUpgradeSuccess(t *testing.T) {
	env := dummyReposEnv(t)
	successDir := filepath.Join(upgradeDir, "success")

	for _, name := range subdirs(t, successDir) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			projDir := copyToTemp(t, filepath.Join(successDir, name))

			newtOutput(t, projDir, env, "upgrade")
			checkDummyRepoVersions(t, projDir, env)

			for _, repo := range installedRepos(t, projDir) {
				runGit(t, env, nil, "-C",
					filepath.Join(projDir, "repos", repo),
					"checkout", "-q", "origin/master")
			}

			newtOutput(t, projDir, env, "upgrade")
			checkDummyRepoVersions(t, projDir, env)
		})
	}
}

// Each project in testdata/upgrade/fail must fail to upgrade with error
// message ending with its expected.txt.
func TestUpgradeFail(t *testing.T) {
	env := dummyReposEnv(t)
	failDir := filepath.Join(upgradeDir, "fail")

	for _, name := range subdirs(t, failDir) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			projDir := copyToTemp(t, filepath.Join(failDir, name))

			out, err := newtCombinedOutput(projDir, env, "upgrade")
			if err == nil {
				t.Fatalf("newt upgrade succeeded\n%s", out)
			}

			checkOutputTail(t, "error",
				readFile(t, filepath.Join(projDir, "expected.txt")), out)
		})
	}
}

// Only allowed and not ignored repos must be installed.
func TestUpgradeAllowedIgnored(t *testing.T) {
	env := dummyReposEnv(t)
	projDir := copyToTemp(t, filepath.Join(upgradeDir, "allowed-ignored"))

	newtOutput(t, projDir, env, "upgrade")

	checkOutput(t, "installed repos",
		readFile(t, filepath.Join(projDir, "expected.txt")),
		strings.Join(installedRepos(t, projDir), "\n"))
}

// commitToDummyRepo clones the master branch of a dummy repo, lets edit
// change files in the clone and pushes them as a new commit, optionally
// tagged, back to the dummy repo. Returns the commit hash.
func commitToDummyRepo(t *testing.T, env []string, repo string,
	edit func(dir string), tag string) string {

	t.Helper()

	env = append(env,
		"GIT_AUTHOR_NAME=newt test", "GIT_AUTHOR_EMAIL=test@dummy.invalid",
		"GIT_COMMITTER_NAME=newt test",
		"GIT_COMMITTER_EMAIL=test@dummy.invalid")

	dir := filepath.Join(t.TempDir(), repo)
	runGit(t, env, nil, "clone", "-q", dummyRepoURL+repo+".git", dir)

	edit(dir)

	runGit(t, env, nil, "-C", dir, "commit", "-q", "-a", "-m", "test commit")
	runGit(t, env, nil, "-C", dir, "push", "-q", "origin", "master")
	if tag != "" {
		runGit(t, env, nil, "-C", dir, "tag", "-a", "-m", tag, tag)
		runGit(t, env, nil, "-C", dir, "push", "-q", "origin", tag)
	}

	return runGit(t, env, nil, "-C", dir, "rev-parse", "HEAD")
}

// replaceInFile replaces the first occurrence of old in a file with new.
func replaceInFile(t *testing.T, path string, old string, new string) {
	t.Helper()

	data := readFile(t, path)
	if !strings.Contains(data, old) {
		t.Fatalf("%s does not contain %q", path, old)
	}

	data = strings.Replace(data, old, new, 1)
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

// Upgrade must fetch commits and versions added to repos after they were
// installed.
func TestUpgradeFetch(t *testing.T) {
	env := dummyReposEnv(t)
	projDir := copyToTemp(t, filepath.Join(upgradeDir, "fetch"))

	newtOutput(t, projDir, env, "upgrade")
	checkDummyRepoVersions(t, projDir, env)

	// repo-04 is installed at its master branch, which gets a new commit.
	commit04 := commitToDummyRepo(t, env, "mynewt-dummy-repo-04",
		func(dir string) {
			replaceInFile(t, filepath.Join(dir, "README.md"),
				"# mynewt-dummy-repo-04", "# mynewt-dummy-repo-04 (updated)")
		}, "")

	// repo-05 gets a new release, which the project then requires.
	commit05 := commitToDummyRepo(t, env, "mynewt-dummy-repo-05",
		func(dir string) {
			replaceInFile(t, filepath.Join(dir, "repository.yml"),
				`"2.0.0": "mynewt_2_0_0_tag"`,
				`"2.0.0": "mynewt_2_0_0_tag"`+"\n"+
					`    "3.0.0": "mynewt_3_0_0_tag"`)
		}, "mynewt_3_0_0_tag")
	replaceInFile(t, filepath.Join(projDir, "project.yml"), "vers: 2.0.0",
		"vers: 3.0.0")

	newtOutput(t, projDir, env, "upgrade")

	lines := []string{}
	for _, line := range strings.Split(newtOutput(t, projDir, env, "info"),
		"\n") {

		if strings.Contains(line, "mynewt-dummy") {
			lines = append(lines, line)
		}
	}
	checkOutput(t, "repo versions",
		fmt.Sprintf("* mynewt-dummy-repo-04: %s, 0.0.0\n"+
			"* mynewt-dummy-repo-05: %s, 3.0.0", commit04, commit05),
		strings.Join(lines, "\n"))
}
