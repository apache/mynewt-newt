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
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/otiai10/copy"
)

// buildSpaceWriteFile writes a file, creating its directory if needed.
func buildSpaceWriteFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestBuildSpaceInPath builds a project located in a directory with a space
// in its name, including a source file with a space in its name, using the
// host C compiler.
func TestBuildSpaceInPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("host build not supported on Windows")
	}

	var cc string
	for _, name := range []string{"cc", "gcc"} {
		if path, err := exec.LookPath(name); err == nil {
			cc = path
			break
		}
	}
	if cc == "" {
		t.Skip("no C compiler found in PATH")
	}

	dir := filepath.Join(t.TempDir(), "sp ace", "proj")
	if err := copy.Copy(filepath.Join("testdata", "syscfg_collision"),
		dir); err != nil {

		t.Fatal(err)
	}

	core := filepath.Join(dir, "repos", "apache-mynewt-core")
	buildSpaceWriteFile(t, filepath.Join(core, "compiler", "sim",
		"compiler.yml"),
		"compiler.path.cc: \""+cc+"\"\n"+
			"compiler.path.as: \""+cc+"\"\n"+
			"compiler.path.archive: ar\n"+
			"compiler.path.objsize: size\n"+
			"compiler.flags.default: [-O1, -Wall]\n"+
			"compiler.ld.binfile: false\n")
	buildSpaceWriteFile(t, filepath.Join(core, "sys", "foo", "src",
		"foo.c"), "void foo_init(void) {}\n")
	// Linked as a list of object files (@file).
	buildErrAppendFile(t, filepath.Join(core, "sys", "foo", "pkg.yml"),
		"\npkg.whole_archive: true\n")
	buildSpaceWriteFile(t, filepath.Join(core, "sys", "foo", "include",
		"foo dir", "foo.h"), "#define FOO 1\n")
	buildSpaceWriteFile(t, filepath.Join(dir, "targets", "success",
		"include", "flash_map", "flash_map.h"),
		"#include <stdint.h>\n"+
			"struct flash_area { uint8_t fa_id; uint8_t fa_device_id;\n"+
			"    uint16_t pad16; uint32_t fa_off; uint32_t fa_size; };\n")
	buildSpaceWriteFile(t, filepath.Join(dir, "apps", "app", "src",
		"main.c"), "#include \"foo dir/foo.h\"\n"+
		"int main(void) { return FOO - 1; }\n")
	buildSpaceWriteFile(t, filepath.Join(dir, "apps", "app", "src",
		"my file.c"), "int my_file(void) { return 0; }\n")

	// Second and third builds use the generated dependency files.
	for i := 0; i < 3; i++ {
		out, err := newtCombinedOutput(dir, nil, "build", "success")
		if err != nil {
			t.Fatalf("newt build #%d failed: %s\n%s", i+1, err, out)
		}
	}
}
