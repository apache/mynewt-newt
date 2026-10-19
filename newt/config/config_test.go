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

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mynewt.apache.org/newt/newt/interfaces"
)

func configWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// Relative imports must not require a loaded project (e.g. newtrc is read
// before the project).  They are resolved relative to the importing file.
func TestReadFileImportNoProject(t *testing.T) {
	interfaces.SetProject(nil)

	dir := t.TempDir()
	top := filepath.Join(dir, "newtrc.yml")
	configWriteFile(t, top, "$import:\n    - sub/foo.yml\nkey1: top\n")
	configWriteFile(t, filepath.Join(dir, "sub", "foo.yml"),
		"$import:\n    - bar.yml\nkey2: foo\n")
	configWriteFile(t, filepath.Join(dir, "sub", "bar.yml"), "key3: bar\n")

	yc, err := ReadFile(top)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for k, exp := range map[string]string{
		"key1": "top",
		"key2": "foo",
		"key3": "bar",
	} {
		v, err := yc.GetValString(k, nil)
		if err != nil {
			t.Fatalf("unexpected error reading %s: %v", k, err)
		}
		if v != exp {
			t.Errorf("%s: expected %q, got %q", k, exp, v)
		}
	}
}

func TestReadFileRepoImportNoProject(t *testing.T) {
	interfaces.SetProject(nil)

	dir := t.TempDir()
	top := filepath.Join(dir, "newtrc.yml")
	configWriteFile(t, top, "$import:\n    - \"@foo/bar.yml\"\n")

	_, err := ReadFile(top)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "no project") {
		t.Fatalf("unexpected error: %v", err)
	}
}
