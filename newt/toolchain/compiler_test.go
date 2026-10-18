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
	"os"
	"path/filepath"
	"slices"
	"testing"

	"mynewt.apache.org/newt/newt/cfgv"
)

const testCompilerYml = `
compiler.flags.base: [-DBASE]
compiler.flags.base.COMPILER_WERROR: [-Werror]
compiler.flags.debug: [compiler.flags.base, -O0]
compiler.flags.optimized: [compiler.flags.base, -Os]
`

func writeTestCompiler(t *testing.T, werrorDflt string) string {
	t.Helper()

	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, COMPILER_FILENAME),
		[]byte(testCompilerYml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	if werrorDflt != "" {
		syscfg := []byte("syscfg.defs:\n" +
			"    COMPILER_WERROR:\n" +
			"        description: Treat warnings as errors.\n" +
			"        value: " + werrorDflt + "\n")
		err = os.WriteFile(filepath.Join(dir, "syscfg.yml"), syscfg, 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

// Conditional keys in compiler.yml are evaluated against syscfg values and
// the build profile.
func TestCompilerSyscfgCond(t *testing.T) {
	tests := []struct {
		name       string
		werrorDflt string
		cfg        map[string]string
		profile    string
		expected   []string
	}{
		{
			name:     "werror enabled",
			cfg:      map[string]string{"COMPILER_WERROR": "1"},
			profile:  "debug",
			expected: []string{"-DBASE", "-Werror", "-O0"},
		},
		{
			name:     "werror disabled",
			cfg:      map[string]string{"COMPILER_WERROR": "0"},
			profile:  "debug",
			expected: []string{"-DBASE", "-O0"},
		},
		{
			name:     "werror enabled, other profile",
			cfg:      map[string]string{"COMPILER_WERROR": "1"},
			profile:  "optimized",
			expected: []string{"-DBASE", "-Werror", "-Os"},
		},
		{
			name:       "no cfg, default enabled",
			werrorDflt: "1",
			profile:    "debug",
			expected:   []string{"-DBASE", "-Werror", "-O0"},
		},
		{
			name:       "no cfg, default disabled",
			werrorDflt: "0",
			profile:    "debug",
			expected:   []string{"-DBASE", "-O0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeTestCompiler(t, tt.werrorDflt)

			var cfg *cfgv.Settings
			if tt.cfg != nil {
				cfg = cfgv.NewSettingsFromMap(tt.cfg)
			}

			c := &Compiler{}
			if err := c.load(dir, tt.profile, cfg); err != nil {
				t.Fatalf("load failed: %s", err)
			}

			if !slices.Equal(c.lclInfo.Cflags, tt.expected) {
				t.Errorf("got cflags %q, expected %q", c.lclInfo.Cflags,
					tt.expected)
			}
		})
	}
}

func TestReplaceVal(t *testing.T) {
	cfg := cfgv.NewSettingsFromMap(map[string]string{
		"SET_FLAG":   "-mset",
		"EMPTY_FLAG": "",
	})
	replace := replaceVal(cfg)

	t.Setenv("SET_FLAG", "-menv")
	t.Setenv("EMPTY_FLAG", "-mdfp=/path/to/dfp")
	t.Setenv("ENV_ONLY_FLAG", "-menv_only")

	tests := []struct {
		pattern string
		want    string
	}{
		{"$(SET_FLAG)", "-mset"},
		{"$(EMPTY_FLAG)", "-mdfp=/path/to/dfp"},
		{"$(ENV_ONLY_FLAG)", "-menv_only"},
		{"$(UNDEFINED_FLAG)", ""},
	}

	for _, tc := range tests {
		if got := replace(tc.pattern); got != tc.want {
			t.Errorf("replaceVal(%q) = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}
