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

// buildErrProject copies the syscfg_collision project to a temporary
// directory and adds a compiler definition which does not need a real
// compiler as long as nothing gets compiled.
func buildErrProject(t *testing.T) string {
	t.Helper()

	dir := copyToTemp(t, filepath.Join("testdata", "syscfg_collision"))
	yml := "compiler.path.cc: nonexistent-cc\n" +
		"compiler.path.archive: nonexistent-ar\n" +
		"compiler.flags.default: [-O1]\n"
	path := filepath.Join(dir, "repos", "apache-mynewt-core", "compiler",
		"sim", "compiler.yml")
	if err := os.WriteFile(path, []byte(yml), 0644); err != nil {
		t.Fatal(err)
	}

	return dir
}

// buildErrAppendFile appends text to a file.
func buildErrAppendFile(t *testing.T, path string, text string) {
	t.Helper()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func TestBuildEmptyExtCmd(t *testing.T) {
	for _, key := range []string{"pre_build_cmds", "pre_link_cmds",
		"post_link_cmds"} {

		t.Run(key, func(t *testing.T) {
			dir := buildErrProject(t)
			buildErrAppendFile(t, filepath.Join(dir, "apps", "app",
				"pkg.yml"), "\npkg."+key+":\n    ' ': 100\n")

			out, err := newtCombinedOutput(dir, nil, "build", "success")
			if err == nil {
				t.Fatalf("newt build succeeded unexpectedly:\n%s", out)
			}
			if strings.Contains(out, "panic") {
				t.Fatalf("newt build panicked:\n%s", out)
			}
			if !strings.Contains(out, "apps/app: invalid command") {
				t.Fatalf("unexpected newt build output:\n%s", out)
			}
		})
	}
}

func TestBuildCmakeErrorKeepsFile(t *testing.T) {
	dir := buildErrProject(t)
	path := filepath.Join(dir, "CMakeLists.txt")
	content := "# user content\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := newtCombinedOutput(dir, nil, "target", "cmake", "fail")
	if err == nil {
		t.Fatalf("newt target cmake succeeded unexpectedly:\n%s", out)
	}

	if got := readFile(t, path); got != content {
		t.Fatalf("CMakeLists.txt modified on error; got:\n%q", got)
	}
}
