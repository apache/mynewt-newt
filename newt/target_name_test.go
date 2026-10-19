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
	"testing"
)

// targetNameSnapshot returns contents of all files under the grandparent
// of dir, so that tests can verify nothing was written inside or near dir.
func targetNameSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()

	files := map[string]string{}
	err := filepath.Walk(filepath.Dir(filepath.Dir(dir)),
		func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				files[p] = readFile(t, p)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}

	return files
}

func targetNameCheckUnchanged(t *testing.T, dir string,
	before map[string]string) {

	t.Helper()

	after := targetNameSnapshot(t, dir)
	for p, data := range after {
		old, ok := before[p]
		if !ok {
			t.Errorf("file created: %s", p)
		} else if old != data {
			t.Errorf("file modified: %s", p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("file removed: %s", p)
		}
	}
}

// Invalid or conflicting names passed to "target create" and "target copy"
// must be rejected without touching any files.
func TestTargetNameInvalid(t *testing.T) {
	names := []string{
		"",
		"/",
		".",
		"..",
		"x/..",
		"../../x",
		"targets/../x",
		"/tmp/x",
		"apps/app",
		"success",
		"targets/success",
	}

	for _, name := range names {
		for _, cmd := range [][]string{
			{"target", "create", name},
			{"target", "copy", "success", name},
		} {
			projDir := copyToTemp(t,
				filepath.Join("testdata", "syscfg_collision"))
			before := targetNameSnapshot(t, projDir)

			out, err := newtCombinedOutput(projDir, nil, cmd...)
			if err == nil {
				t.Errorf("newt %q succeeded\n%s", cmd, out)
			}

			targetNameCheckUnchanged(t, projDir, before)
		}
	}
}

func TestTargetNameValid(t *testing.T) {
	projDir := copyToTemp(t, filepath.Join("testdata", "syscfg_collision"))

	if err := os.MkdirAll(filepath.Join(projDir, "targets", "empty"),
		0755); err != nil {
		t.Fatal(err)
	}

	newtOutput(t, projDir, nil, "target", "create", "new")
	newtOutput(t, projDir, nil, "target", "create", "apps/newtgt")
	newtOutput(t, projDir, nil, "target", "copy", "success", "empty")

	for _, p := range []string{
		"targets/new/pkg.yml",
		"targets/new/target.yml",
		"apps/newtgt/pkg.yml",
		"apps/newtgt/target.yml",
		"targets/empty/pkg.yml",
		"targets/empty/target.yml",
		"targets/empty/syscfg.yml",
	} {
		if _, err := os.Stat(filepath.Join(projDir, p)); err != nil {
			t.Error(err)
		}
	}
}
