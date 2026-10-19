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
	"path/filepath"
	"strings"
	"testing"
)

var localRepoDir = filepath.Join("testdata", "localrepo_dep")

// localRepoGitInit turns dir into a git repository with a single commit and
// an "origin" remote. Returns the commit hash.
func localRepoGitInit(t *testing.T, env []string, dir string,
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

// A repo dependency with the same name as the project (e.g., the project is
// a checkout of a repo that apache-mynewt-core depends on) must be satisfied
// by the project itself and must not modify the project's git checkout.
func TestLocalRepoDependency(t *testing.T) {
	env := dummyReposEnv(t)
	projDir := copyToTemp(t, localRepoDir)

	const origin = "https://example.invalid/project.git"
	head := localRepoGitInit(t, env, projDir, origin)

	for i := 0; i < 2; i++ {
		out, err := newtCombinedOutput(projDir, env, "upgrade")
		if err != nil {
			t.Fatalf("newt upgrade failed: %v\n%s", err, out)
		}

		gotOrigin := runGit(t, env, nil, "-C", projDir, "remote", "get-url",
			"origin")
		if gotOrigin != origin {
			t.Fatalf("project origin changed to %q", gotOrigin)
		}

		gotHead := runGit(t, env, nil, "-C", projDir, "rev-parse", "HEAD")
		if gotHead != head {
			t.Fatalf("project HEAD changed to %s", gotHead)
		}

		checkOutput(t, "installed repos",
			"mynewt-dummy-repo-01\nmynewt-dummy-repo-03\n"+
				"mynewt-dummy-repo-04",
			strings.Join(installedRepos(t, projDir), "\n"))
	}
}
