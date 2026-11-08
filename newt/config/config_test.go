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
	"mynewt.apache.org/newt/newt/ycfg"
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

func configCheckStringSlice(t *testing.T, yc ycfg.YCfg, key string,
	exp []string) {

	t.Helper()
	v, err := yc.GetValStringSlice(key, nil)
	if err != nil {
		t.Fatalf("unexpected error reading %s: %v", key, err)
	}
	if strings.Join(v, ",") != strings.Join(exp, ",") {
		t.Errorf("%s: expected %v, got %v", key, exp, v)
	}
}

// A local file applied on top of syscfg.yml and pkg.yml style settings
// merges maps per key, appends to and removes from sequences and deletes
// keys.
func TestLocalFileApply(t *testing.T) {
	interfaces.SetProject(nil)

	dir := t.TempDir()
	base := filepath.Join(dir, "base.yml")
	configWriteFile(t, base, `
syscfg.vals:
    FOO: 1
    BAR: 2
pkg.cflags:
    - -DA
    - -DB
pkg.lflags:
    - -lm
pkg.deps.FOO:
    - foo
`)
	local := filepath.Join(dir, "local.yml")
	configWriteFile(t, local, `
$import:
    - imp.yml
syscfg.vals:
    BAR: 3
pkg.cflags:
    - -DC
pkg.cflags.REMOVE:
    - -DA
pkg.lflags.OVERWRITE:
    - -lc
pkg.deps.FOO.DELETE: true
`)
	configWriteFile(t, filepath.Join(dir, "imp.yml"), `
syscfg.vals:
    BAZ: 4
`)

	yc, err := ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}

	lf, err := ReadLocalFile(local)
	if err != nil {
		t.Fatal(err)
	}

	expKeys := []string{"pkg.cflags", "pkg.deps.FOO", "pkg.lflags",
		"syscfg.vals"}
	if strings.Join(lf.Keys(), ",") != strings.Join(expKeys, ",") {
		t.Errorf("keys: expected %v, got %v", expKeys, lf.Keys())
	}

	if err := lf.Apply(&yc, nil); err != nil {
		t.Fatal(err)
	}

	vals, err := yc.GetValStringMapString("syscfg.vals", nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, exp := range map[string]string{"FOO": "1", "BAR": "3",
		"BAZ": "4"} {

		if vals[k] != exp {
			t.Errorf("syscfg.vals.%s: expected %q, got %q", k, exp, vals[k])
		}
	}

	configCheckStringSlice(t, yc, "pkg.cflags", []string{"-DB", "-DC"})
	configCheckStringSlice(t, yc, "pkg.lflags", []string{"-lc"})
	if yc.HasKey("pkg.deps.FOO") {
		t.Errorf("pkg.deps.FOO not deleted")
	}
}

// Only keys accepted by the filter are applied, including keys from imported
// files and keys with operation suffixes.
func TestLocalFileApplyFilter(t *testing.T) {
	interfaces.SetProject(nil)

	dir := t.TempDir()
	local := filepath.Join(dir, "local.yml")
	configWriteFile(t, local, `
$import:
    - imp.yml
syscfg.vals:
    FOO: 1
pkg.cflags.OVERWRITE:
    - -DC
`)
	configWriteFile(t, filepath.Join(dir, "imp.yml"), `
pkg.lflags:
    - -lm
`)

	lf, err := ReadLocalFile(local)
	if err != nil {
		t.Fatal(err)
	}

	syscfgYc := ycfg.NewYCfg("syscfg")
	pkgYc := ycfg.NewYCfg("pkg")
	pkgYc.Replace("pkg.cflags", []interface{}{"-DA"})

	isSyscfg := func(key string) bool {
		return strings.HasPrefix(key, "syscfg.")
	}
	if err := lf.Apply(&syscfgYc, isSyscfg); err != nil {
		t.Fatal(err)
	}
	if err := lf.Apply(&pkgYc, func(key string) bool {
		return !isSyscfg(key)
	}); err != nil {
		t.Fatal(err)
	}

	if syscfgYc.HasKey("pkg.cflags") || syscfgYc.HasKey("pkg.lflags") {
		t.Errorf("pkg settings applied to syscfg tree")
	}
	if pkgYc.HasKey("syscfg.vals") {
		t.Errorf("syscfg settings applied to pkg tree")
	}

	vals, err := syscfgYc.GetValStringMapString("syscfg.vals", nil)
	if err != nil {
		t.Fatal(err)
	}
	if vals["FOO"] != "1" {
		t.Errorf("syscfg.vals.FOO: expected \"1\", got %q", vals["FOO"])
	}
	configCheckStringSlice(t, pkgYc, "pkg.cflags", []string{"-DC"})
	configCheckStringSlice(t, pkgYc, "pkg.lflags", []string{"-lm"})
}

// REMOVE and DELETE are rejected in files imported by a local file.
func TestLocalFileImportRemove(t *testing.T) {
	interfaces.SetProject(nil)

	dir := t.TempDir()
	local := filepath.Join(dir, "local.yml")
	configWriteFile(t, local, "$import:\n    - imp.yml\n")
	configWriteFile(t, filepath.Join(dir, "imp.yml"),
		"pkg.cflags.REMOVE:\n    - -DA\n")

	_, err := ReadLocalFile(local)
	if err == nil || !strings.Contains(err.Error(), "only supported in") {
		t.Fatalf("expected error, got %v", err)
	}
}

// Sections of a local file include settings from the files it imports and
// support the same operations as the local file itself.
func TestLocalFileSections(t *testing.T) {
	interfaces.SetProject(nil)

	dir := t.TempDir()
	local := filepath.Join(dir, "local.yml")
	configWriteFile(t, local, `
$import:
    - imp.yml
secs:
    a:
        pkg.cflags.REMOVE:
            - -DA
`)
	configWriteFile(t, filepath.Join(dir, "imp.yml"), `
secs:
    a:
        pkg.cflags:
            - -DIMP
    b:
        pkg.cflags:
            - -DC
`)

	lf, err := ReadLocalFile(local)
	if err != nil {
		t.Fatal(err)
	}

	secs, err := lf.Sections("secs")
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(secs))
	}

	for name, exp := range map[string][]string{
		"a": {"-DB", "-DIMP"},
		"b": {"-DA", "-DB", "-DC"},
	} {
		yc := ycfg.NewYCfg(name)
		yc.Replace("pkg.cflags", []interface{}{"-DA", "-DB"})
		if err := secs[name].Apply(&yc, nil); err != nil {
			t.Fatal(err)
		}
		configCheckStringSlice(t, yc, "pkg.cflags", exp)
	}

	// REMOVE and DELETE are rejected in imported sections.
	configWriteFile(t, filepath.Join(dir, "imp.yml"), `
secs:
    a:
        pkg.cflags.REMOVE:
            - -DA
`)
	lf, err = ReadLocalFile(local)
	if err != nil {
		t.Fatal(err)
	}
	_, err = lf.Sections("secs")
	if err == nil || !strings.Contains(err.Error(), "only supported in") {
		t.Fatalf("expected error, got %v", err)
	}

	// Empty sections are allowed.
	configWriteFile(t, local, "$import:\n    - imp.yml\nsecs:\n")
	configWriteFile(t, filepath.Join(dir, "imp.yml"), "secs:\n    a:\n")
	lf, err = ReadLocalFile(local)
	if err != nil {
		t.Fatal(err)
	}
	secs, err = lf.Sections("secs")
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 1 || len(secs["a"].Keys()) != 0 {
		t.Errorf("expected one empty section, got %v", secs)
	}
}
