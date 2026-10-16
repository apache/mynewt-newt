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
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/otiai10/copy"
)

// If set in the environment, the test executable runs newt instead of the
// tests. This allows tests to execute newt as a separate process without
// building it first.
const execNewtEnv = "NEWT_TEST_EXEC_NEWT"

var newtExe string

func TestMain(m *testing.M) {
	if os.Getenv(execNewtEnv) == "1" {
		main()
		os.Exit(0)
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get test executable: %s\n", err)
		os.Exit(1)
	}
	newtExe = exe

	os.Exit(m.Run())
}

func newtExecCmd(dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command(newtExe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), execNewtEnv+"=1")
	cmd.Env = append(cmd.Env, env...)

	return cmd
}

// newtOutput runs newt in dir and returns its standard output. The test
// fails if newt fails.
func newtOutput(t *testing.T, dir string, env []string,
	args ...string) string {

	t.Helper()

	cmd := newtExecCmd(dir, env, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("newt %s failed: %s\n%s%s", strings.Join(args, " "), err,
			out, stderr.String())
	}

	return string(out)
}

// newtCombinedOutput runs newt in dir and returns its combined standard
// output and standard error.
func newtCombinedOutput(dir string, env []string,
	args ...string) (string, error) {

	out, err := newtExecCmd(dir, env, args...).CombinedOutput()

	return string(out), err
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// subdirs returns names of all subdirectories of dir.
func subdirs(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}

	return names
}

// copyToTemp copies dir to a new temporary directory, so that tests can
// modify it, and returns path to the copy.
func copyToTemp(t *testing.T, dir string) string {
	t.Helper()

	dst := filepath.Join(t.TempDir(), filepath.Base(dir))
	if err := copy.Copy(dir, dst); err != nil {
		t.Fatal(err)
	}

	return dst
}

// linesNoSpace splits s into lines and removes all white space from them.
// Comparing results is equivalent to "diff -w".
func linesNoSpace(s string) []string {
	s = strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if s == "" {
		return nil
	}

	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, l)
	}

	return lines
}

// checkOutput fails the test if got differs from expected, ignoring white
// space.
func checkOutput(t *testing.T, what string, expected string, got string) {
	t.Helper()

	if !slices.Equal(linesNoSpace(expected), linesNoSpace(got)) {
		t.Errorf("unexpected %s\n--- expected:\n%s\n--- got:\n%s", what,
			expected, got)
	}
}

// checkOutputTail is like checkOutput, but only compares the last lines of
// got, as many as expected has.
func checkOutputTail(t *testing.T, what string, expected string,
	got string) {

	t.Helper()

	exp := linesNoSpace(expected)
	lines := linesNoSpace(got)
	if len(lines) >= len(exp) {
		lines = lines[len(lines)-len(exp):]
	}

	if !slices.Equal(exp, lines) {
		t.Errorf("unexpected %s\n--- expected (at the end):\n%s\n--- got:\n%s",
			what, expected, got)
	}
}
