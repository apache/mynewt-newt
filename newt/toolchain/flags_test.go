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

func TestMergeFlags(t *testing.T) {
	tests := []struct {
		flags []string
		exp   []string
	}{
		// Order is kept.
		{[]string{"-Wall -O2", "-mthumb"},
			[]string{"-Wall", "-O2", "-mthumb"}},
		// A repeated flag is kept at its last position.
		{[]string{"-Wall -Wextra", "-Wall"},
			[]string{"-Wextra", "-Wall"}},
		// Conflicting options are kept; the compiler uses the last one.
		{[]string{"-O2 -fno-common", "-Os -fcommon -fno-common"},
			[]string{"-O2", "-Os", "-fcommon", "-fno-common"}},
		// The last definition or undefinition of a macro wins.
		{[]string{"-DFOO=1 -DBAR", "-UBAR -DFOO=2", "-D", "BAZ=1",
			"-D BAZ=2"},
			[]string{"-UBAR", "-DFOO=2", "-D", "BAZ=2"}},
		{[]string{"-UFOO", "-DFOO"}, []string{"-DFOO"}},
		// Other options with arguments are never removed.
		{[]string{"-include a.h -Wall", "-include", "b.h",
			"-Xlinker -z -Xlinker relro", "-include a.h"},
			[]string{"-include", "a.h", "-Wall", "-include", "b.h",
				"-Xlinker", "-z", "-Xlinker", "relro", "-include", "a.h"}},
		// An option without argument at the end is kept.
		{[]string{"-b", "-a", "-include"},
			[]string{"-b", "-a", "-include"}},
		{nil, []string{}},
	}

	for _, tt := range tests {
		got := MergeFlags(tt.flags...)
		if !slices.Equal(got, tt.exp) {
			t.Errorf("MergeFlags(%q):\nexpected %q\ngot      %q",
				tt.flags, tt.exp, got)
		}
	}
}

func TestFlagMacro(t *testing.T) {
	tests := map[string]string{
		"-DFOO":      "FOO",
		"-DFOO=1":    "FOO",
		"-DFOO=a=b":  "FOO",
		"-UFOO":      "FOO",
		"-Wall":      "",
		"-include":   "",
		"-DFOO(x)=x": "FOO(x)",
	}

	for flag, exp := range tests {
		if got := FlagMacro(flag); got != exp {
			t.Errorf("FlagMacro(%q) = %q, expected %q", flag, got, exp)
		}
	}
}

func TestMacroFlags(t *testing.T) {
	got := MacroFlags("-DFOO=1 -Wall", "-D", "BAR", "-U BAZ -include a.h",
		"-UFOO")
	exp := []string{"-DFOO=1", "-DBAR", "-UBAZ", "-UFOO"}
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

	exp := []string{"-include", "a.h", "-include", "b.h", "-Wall"}
	if got := c.cflagsStrings(); !slices.Equal(got, exp) {
		t.Errorf("unexpected flags:\nexpected %q\ngot      %q", exp, got)
	}
}

// Flags of the compiler package have the lowest priority: they precede flags
// of packages, so conflicting flags of packages (e.g., optimization level or
// a define with a value) override them.  Flags added later override flags
// added earlier.
func TestCompilerFlagsPriority(t *testing.T) {
	c := newTestCompiler()
	c.lclInfo.Cflags = []string{"-O1", "-DFOO=2", "-Wall", "-mthumb"}
	c.lclInfo.CCflags = []string{"-std=c11"}

	c.AddInfo(&CompilerInfo{Cflags: []string{"-Os", "-DBAR=1"}})
	c.AddInfo(&CompilerInfo{Cflags: []string{"-O3", "-DFOO=1", "-Wall",
		"-std=gnu99", "-DBAR=2"}})

	exp := []string{"-O1", "-mthumb", "-std=c11", "-Os", "-O3", "-DFOO=1",
		"-Wall", "-std=gnu99", "-DBAR=2"}
	got, err := c.CompileFlags(COMPILER_TYPE_C)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, exp) {
		t.Errorf("unexpected flags:\nexpected %q\ngot      %q", exp, got)
	}
}

// Language specific flags follow the common flags of the same source.
func TestLangFlagsOrder(t *testing.T) {
	c := newTestCompiler()
	c.lclInfo.Cflags = []string{"-O1"}
	c.lclInfo.CXXflags = []string{"-std=c++11"}
	c.lclInfo.Aflags = []string{"-x", "assembler-with-cpp"}

	c.AddInfo(&CompilerInfo{
		Cflags:   []string{"-DFOO"},
		CXXflags: []string{"-std=c++17"},
		Aflags:   []string{"-DASM"},
	})

	tests := []struct {
		ctype int
		exp   []string
	}{
		{COMPILER_TYPE_C, []string{"-O1", "-DFOO"}},
		{COMPILER_TYPE_CPP,
			[]string{"-O1", "-std=c++11", "-DFOO", "-std=c++17"}},
		{COMPILER_TYPE_ASM,
			[]string{"-O1", "-x", "assembler-with-cpp", "-DFOO", "-DASM"}},
	}

	for _, tt := range tests {
		got, err := c.CompileFlags(tt.ctype)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, tt.exp) {
			t.Errorf("unexpected flags for type %d:\nexpected %q\ngot      %q",
				tt.ctype, tt.exp, got)
		}
	}
}

// Linker flags of the compiler package have the lowest priority: they are
// discarded if they conflict with flags of packages.
func TestLflagsPriority(t *testing.T) {
	c := newTestCompiler()
	c.lclInfo.Lflags = []string{"-specs=nosys.specs", "-static"}

	c.AddInfo(&CompilerInfo{Lflags: []string{"-specs=nano.specs"}})

	exp := []string{"-specs=nano.specs", "-static"}
	if got := c.lflagsStrings(); !slices.Equal(got, exp) {
		t.Errorf("unexpected flags:\nexpected %q\ngot      %q", exp, got)
	}
}
