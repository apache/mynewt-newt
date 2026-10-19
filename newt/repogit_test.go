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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var repoGitNoGitDir = filepath.Join("testdata", "repogit_nogit")

// repoGitInitProject turns dir into a git repository with a single commit
// and an "origin" remote. Returns the commit hash.
func repoGitInitProject(t *testing.T, env []string, dir string,
	origin string) string {

	t.Helper()

	env = append(env,
		"GIT_AUTHOR_NAME=newt test", "GIT_AUTHOR_EMAIL=test@dummy.invalid",
		"GIT_COMMITTER_NAME=newt test",
		"GIT_COMMITTER_EMAIL=test@dummy.invalid")

	runGit(t, env, nil, "-C", dir, "init", "-q")
	runGit(t, env, nil, "-C", dir, "add", ".")
	runGit(t, env, nil, "-C", dir, "commit", "-q", "-m", "project")
	runGit(t, env, nil, "-C", dir, "remote", "add", "origin", origin)

	return runGit(t, env, nil, "-C", dir, "rev-parse", "HEAD")
}

// If a directory in repos/ is not a git checkout, git commands for that repo
// must not be run against the enclosing project git repository.
func TestRepoGitNotCheckout(t *testing.T) {
	env := dummyReposEnv(t)
	projDir := copyToTemp(t, repoGitNoGitDir)

	const origin = "https://example.invalid/project.git"
	head := repoGitInitProject(t, env, projDir, origin)

	newtOutput(t, projDir, env, "upgrade")

	// Turn the installed repo into a plain copy of its files.
	repoDir := filepath.Join(projDir, "repos", "mynewt-dummy-repo-04")
	if err := os.Rename(filepath.Join(repoDir, ".git"),
		filepath.Join(t.TempDir(), "git")); err != nil {

		t.Fatal(err)
	}

	out, err := newtCombinedOutput(projDir, env, "upgrade")
	if err == nil {
		t.Fatalf("newt upgrade succeeded\n%s", out)
	}
	if !strings.Contains(out, "is not a git repository") {
		t.Fatalf("unexpected error\n%s", out)
	}

	gotOrigin := runGit(t, env, nil, "-C", projDir, "config", "--get",
		"remote.origin.url")
	if gotOrigin != origin {
		t.Fatalf("project origin changed to %q", gotOrigin)
	}

	gotHead := runGit(t, env, nil, "-C", projDir, "rev-parse", "HEAD")
	if gotHead != head {
		t.Fatalf("project HEAD changed to %s", gotHead)
	}

	if _, err := os.Stat(filepath.Join(projDir, "project.yml")); err != nil {
		t.Fatalf("project.yml missing: %v", err)
	}
}

// A project located in a subdirectory of a git repository must still work
// and report the commit of that repository.
func TestRepoGitProjectInSubdir(t *testing.T) {
	env := dummyReposEnv(t)
	projDir := copyToTemp(t, repoGitNoGitDir)

	head := repoGitInitProject(t, env, filepath.Dir(projDir),
		"https://example.invalid/project.git")

	newtOutput(t, projDir, env, "upgrade")

	out := newtOutput(t, projDir, env, "info")
	if !strings.Contains(out, "repogit_nogit (project): "+head) {
		t.Fatalf("project commit not reported\n%s", out)
	}
}
