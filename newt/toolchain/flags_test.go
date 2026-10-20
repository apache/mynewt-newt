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

package toolchain

import (
	"slices"
	"testing"

	"github.com/kballard/go-shellquote"
)

func TestIncludesStringsSpaces(t *testing.T) {
	c := newTestCompiler()
	c.baseDir = "/sp ace/proj"
	c.info.Includes = []string{
		"/sp ace/proj/b dir/include",
		"/sp ace/proj/a/include",
		"/other dir/include",
		"/sp ace/proj/a/include",
		" ",
	}

	exp := []string{
		"-I/other dir/include",
		"-Ia/include",
		"-Ib dir/include",
	}
	if got := c.includesStrings(); !slices.Equal(got, exp) {
		t.Errorf("unexpected include flags: expected %q, got %q", exp, got)
	}
}

func TestCommandString(t *testing.T) {
	cmd := []string{
		"gcc",
		"-DSTR=\"a b\"",
		"-I/sp ace/inc",
		"-DQ='x'",
		"-c",
		"a\\b.c",
		"plain.c",
	}

	s := commandString(cmd)
	got, err := shellquote.Split(s)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, cmd) {
		t.Errorf("round trip failed: %q -> %q -> %q", cmd, s, got)
	}

	// Arguments without special characters are not quoted.
	if s := commandString([]string{"gcc", "-O2", "a.c"}); s != "gcc -O2 a.c" {
		t.Errorf("unexpected command string: %q", s)
	}
}

func TestSortFlags(t *testing.T) {
	got := SortFlags("-Wall -O2", "-include a.h", "-Wall",
		"-include", "b.h", "-Xlinker -z -Xlinker relro",
		"-Xlinker -z -Xlinker noexecstack", "-DFOO=1")

	exp := []string{
		"-DFOO=1", "-O2", "-Wall",
		"-include", "a.h", "-include", "b.h",
		"-Xlinker", "-z", "-Xlinker", "relro",
		"-Xlinker", "-z", "-Xlinker", "noexecstack",
	}
	if !slices.Equal(got, exp) {
		t.Errorf("unexpected flags:\nexpected %q\ngot      %q", exp, got)
	}

	// An option without argument at the end is kept.
	got = SortFlags("-b", "-a", "-include")
	exp = []string{"-a", "-b", "-include"}
	if !slices.Equal(got, exp) {
		t.Errorf("unexpected flags:\nexpected %q\ngot      %q", exp, got)
	}
}

func TestCflagsStringsPairs(t *testing.T) {
	c := newTestCompiler()

	// Flags from two packages, both using forced includes as separate list
	// elements.
	c.AddInfo(&CompilerInfo{Cflags: []string{"-Wall", "-include", "a.h"}})
	c.AddInfo(&CompilerInfo{Cflags: []string{"-include", "b.h", "-Wall"}})

	exp := []string{"-Wall", "-include", "a.h", "-include", "b.h"}
	if got := c.cflagsStrings(); !slices.Equal(got, exp) {
		t.Errorf("unexpected flags:\nexpected %q\ngot      %q", exp, got)
	}
}

func TestCcflagsStrings(t *testing.T) {
	tests := []struct {
		cflags  []string
		ccflags []string
		exp     []string
	}{
		{[]string{"-Wall"}, []string{"-std=c11"},
			[]string{"-Wall", "-std=c11"}},
		{[]string{"-Wall", "-std=gnu99"}, []string{"-std=c11"},
			[]string{"-Wall", "-std=gnu99"}},
		{[]string{"-Wall"}, []string{}, []string{"-Wall"}},
		{[]string{"-Wall"}, []string{"-include", "a.h", "-std=c11"},
			[]string{"-Wall", "-std=c11", "-include", "a.h"}},
	}

	for _, tt := range tests {
		c := &Compiler{}
		c.info.Cflags = tt.cflags
		c.info.CCflags = tt.ccflags
		got := c.ccflagsStrings()
		if !slices.Equal(got, tt.exp) {
			t.Errorf("ccflagsStrings() with cflags %q ccflags %q = %q, "+
				"expected %q", tt.cflags, tt.ccflags, got, tt.exp)
		}
	}
}
