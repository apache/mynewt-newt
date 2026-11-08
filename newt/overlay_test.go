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

// overlayProject copies the test project, writes the specified files into it
// and returns the project directory.
func overlayProject(t *testing.T, files map[string]string) string {
	t.Helper()

	projDir := copyToTemp(t, filepath.Join("testdata", "syscfg_collision"))
	for path, content := range files {
		overlayWriteFile(t, filepath.Join(projDir, path), content)
	}

	return projDir
}

func overlayWriteFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func overlayCheckContains(t *testing.T, desc string, out string,
	exp ...string) {

	t.Helper()

	for _, e := range exp {
		if !strings.Contains(out, e) {
			t.Errorf("%s: missing %q\n%s", desc, e, out)
		}
	}
}

func overlayCheckNotContains(t *testing.T, desc string, out string,
	unexp ...string) {

	t.Helper()

	for _, e := range unexp {
		if strings.Contains(out, e) {
			t.Errorf("%s: unexpected %q\n%s", desc, e, out)
		}
	}
}

// newtConfigShow runs "newt target config show" and returns its output.
func newtConfigShow(t *testing.T, projDir string, args ...string) string {
	t.Helper()

	args = append(args, "target", "config", "show", "success")
	out, err := newtCombinedOutput(projDir, nil, args...)
	if err != nil {
		t.Fatalf("newt %v failed: %s\n%s", args, err, out)
	}

	return out
}

const overlayLocalPath = "targets/success/local.overlay.yml"

const overlayLocal = `
target.build_profile: debug
pkg.cflags:
    - -DLOCAL
syscfg.vals:
    FOO_IMPLEMENTATION: baz
syscfg.vals.FOO_COND_1.DELETE: true
syscfg.vals.FOO_COND_2.DELETE: true
`

// A target's local overlay is applied on top of its target.yml, pkg.yml and
// syscfg.yml.
func TestOverlayLocal(t *testing.T) {
	projDir := overlayProject(t, map[string]string{
		overlayLocalPath: overlayLocal,
	})

	out := newtOutput(t, projDir, nil, "target", "show", "success")
	overlayCheckContains(t, "target show", out,
		"build_profile=debug",
		"cflags=-DLOCAL",
		"FOO_IMPLEMENTATION=baz")

	out = newtConfigShow(t, projDir)
	overlayCheckContains(t, "target config show", out,
		"Using overlay targets/success/local.overlay.yml",
		"* Value: baz")
}

// An overlay can change a setting defined by the target itself (here in a
// file imported by its syscfg.yml), which enables conditional settings
// depending on it.
func TestOverlayTargetSetting(t *testing.T) {
	projDir := overlayProject(t, map[string]string{
		"config/debug.yml": `
syscfg.defs:
    DEBUG_ENABLE:
        description: Enable debug.
        value: 0
syscfg.vals.DEBUG_ENABLE:
    FOO_IMPLEMENTATION: debug
`,
		"targets/success/syscfg.yml": `
$import:
    - config/debug.yml
`,
		overlayLocalPath: "syscfg.vals:\n    DEBUG_ENABLE: 1\n",
	})

	out := newtConfigShow(t, projDir)
	overlayCheckContains(t, "target config show", out, "* Value: debug")
}

// Local overlays of other packages than targets are ignored.
func TestOverlayOnlyTargets(t *testing.T) {
	projDir := overlayProject(t, map[string]string{
		"repos/apache-mynewt-core/sys/foo/local.overlay.yml": `
syscfg.defs:
    FOO_LOCAL:
        description: Local setting.
        value: 7
`,
	})

	out := newtConfigShow(t, projDir)
	overlayCheckNotContains(t, "target config show", out,
		"Setting: FOO_LOCAL", "Using overlay")
}

// The overlay is reported even if it breaks dependency resolution.
func TestOverlayResolveError(t *testing.T) {
	projDir := overlayProject(t, map[string]string{
		overlayLocalPath: "pkg.deps:\n    - sys/missing\n",
	})

	out, err := newtCombinedOutput(projDir, nil, "target", "config", "show",
		"success")
	if err == nil {
		t.Fatalf("newt target config show didn't fail\n%s", out)
	}
	overlayCheckContains(t, "target config show", out,
		"Using overlay targets/success/local.overlay.yml")
}

// Overlays can only contain settings of the target's configuration files and
// can't change the target's name or type.
func TestOverlayInvalidSettings(t *testing.T) {
	for overlay, exp := range map[string]string{
		"foo.bar: 1\n":      "unrecognized setting \"foo.bar\"",
		"pkg.type: lib\n":   "\"pkg.type\" can't be changed by an overlay",
		"pkg.name.FOO: x\n": "\"pkg.name.FOO\" can't be changed by an overlay",
	} {
		projDir := overlayProject(t, map[string]string{
			overlayLocalPath: overlay,
		})

		out, _ := newtCombinedOutput(projDir, nil, "target", "show",
			"success")
		overlayCheckContains(t, "target show", out, exp)
	}
}

// Commands which save targets ignore overlays, so that they are not written
// to the target's files nor copied to another target.
func TestOverlayTargetSave(t *testing.T) {
	projDir := overlayProject(t, map[string]string{
		overlayLocalPath: overlayLocal,
	})

	newtOutput(t, projDir, nil, "target", "set", "success",
		"target.app=apps/app")
	newtOutput(t, projDir, nil, "target", "amend", "success",
		"cflags=-DAMEND")
	newtOutput(t, projDir, nil, "target", "copy", "success", "dst")

	for _, name := range []string{"success", "dst"} {
		dir := filepath.Join(projDir, "targets", name)
		for _, f := range []string{"target.yml", "pkg.yml", "syscfg.yml"} {
			overlayCheckNotContains(t, name+"/"+f,
				readFile(t, filepath.Join(dir, f)), "debug", "LOCAL", "baz")
		}
		overlayCheckContains(t, name+"/pkg.yml",
			readFile(t, filepath.Join(dir, "pkg.yml")), "-DAMEND")
		overlayCheckContains(t, name+"/syscfg.yml",
			readFile(t, filepath.Join(dir, "syscfg.yml")), "FOO_COND_1")
	}

	// The copy doesn't get the overlay.
	out := newtOutput(t, projDir, nil, "target", "show", "dst")
	overlayCheckContains(t, "target show dst", out, "build_profile=default")

	// The source target still uses it.
	out = newtOutput(t, projDir, nil, "target", "show", "success")
	overlayCheckContains(t, "target show success", out,
		"build_profile=debug", "-DAMEND -DLOCAL")
}

// Overlays from the command line are applied after the local overlay, in the
// order given.  Relative imports are resolved relative to the overlay file.
func TestOverlayCli(t *testing.T) {
	projDir := overlayProject(t, map[string]string{
		overlayLocalPath: overlayLocal,
		"ovl/ovl1.yml": `
$import:
    - imp.yml
target.build_profile: optimized
pkg.cflags:
    - -DOVL1
`,
		"ovl/imp.yml": "syscfg.vals:\n    FOO_IMPLEMENTATION: ovl\n",
		"ovl2.yml":    "pkg.cflags:\n    - -DOVL2\n",
	})

	args := []string{"--overlay", "ovl/ovl1.yml", "--overlay", "ovl2.yml"}

	out := newtOutput(t, projDir, nil,
		append(args, "target", "show", "success")...)
	overlayCheckContains(t, "target show", out,
		"build_profile=optimized",
		"cflags=-DLOCAL -DOVL1 -DOVL2",
		"FOO_IMPLEMENTATION=ovl")

	out = newtConfigShow(t, projDir, args...)
	overlayCheckContains(t, "target config show", out,
		"Using overlay targets/success/local.overlay.yml",
		"Using overlay ovl/ovl1.yml",
		"Using overlay ovl2.yml",
		"* Value: ovl")
}

// --no-local disables the local overlay, but not overlays from the command
// line.
func TestOverlayNoLocal(t *testing.T) {
	projDir := overlayProject(t, map[string]string{
		overlayLocalPath: overlayLocal,
		"ovl.yml":        "pkg.cflags:\n    - -DOVERLAY\n",
	})

	out := newtOutput(t, projDir, nil, "--no-local", "target", "show",
		"success")
	overlayCheckContains(t, "target show", out, "build_profile=default",
		"FOO_COND_1=1")
	overlayCheckNotContains(t, "target show", out, "LOCAL", "baz")

	out = newtOutput(t, projDir, nil, "--no-local", "--overlay", "ovl.yml",
		"target", "show", "success")
	overlayCheckContains(t, "target show", out, "build_profile=default",
		"cflags=-DOVERLAY")
	overlayCheckNotContains(t, "target show", out, "-DLOCAL")
}

func TestOverlayCliErrors(t *testing.T) {
	projDir := overlayProject(t, map[string]string{
		"unknown.yml": "syscfg.vals:\n    FOO: 1\nfoo.bar: 1\n",
		"ok.yml":      "pkg.cflags:\n    - -DOVERLAY\n",
	})

	for _, tc := range []struct {
		args []string
		err  string
	}{
		{[]string{"--overlay", "unknown.yml", "target", "show", "success"},
			"unrecognized setting \"foo.bar\""},
		{[]string{"--overlay", "missing.yml", "target", "show", "success"},
			"missing.yml"},
		{[]string{"--overlay", "ok.yml", "target", "set", "success",
			"target.build_profile=debug"},
			"--overlay can't be used with commands which modify targets"},
	} {
		out, err := newtCombinedOutput(projDir, nil, tc.args...)
		if err == nil {
			t.Errorf("newt %v: expected failure\n%s", tc.args, out)
		}
		overlayCheckContains(t, strings.Join(tc.args, " "), out, tc.err)
	}

	overlayCheckNotContains(t, "target.yml",
		readFile(t, filepath.Join(projDir, "targets", "success",
			"target.yml")), "debug")
}
