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

package util

import (
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kballard/go-shellquote"
)

func TestShellCmdStringQuoting(t *testing.T) {
	cmd := []string{
		"arm-none-eabi-gcc",
		"-DMBEDTLS_USER_CONFIG_FILE=\"mbedtls/config_mynewt.h\"",
		"-DCONFIG_FILE=<mbedtls/config.h>",
		"-DSTR='a b'",
		"-Ipath with spaces/include",
		`-IC:\Users\user\proj\repos\apache-mynewt-core\kernel\os\include`,
		`-DFOO=\{1\}`,
		"-c",
		"file.c",
	}

	s := ShellCmdString(cmd, nil)
	got, err := shellquote.Split(s)
	if err != nil {
		t.Fatalf("failed to split %q: %v", s, err)
	}
	if !slices.Equal(got, cmd) {
		t.Errorf("got %q, want %q", got, cmd)
	}
}

func TestExecuteShellArgs(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("ExecuteShell is supported on Linux and MacOS only")
	}

	args := []string{
		"-DMBEDTLS_USER_CONFIG_FILE=\"mbedtls/config_mynewt.h\"",
		"-DCONFIG_FILE=<mbedtls/config.h>",
		"-DSTR='a b'",
		"-Ipath with spaces/include",
		"$HOME",
		"a;b",
	}

	ExecuteShell = true
	defer func() { ExecuteShell = false }()

	o, err := ShellCommand(append([]string{"printf", "%s\\n"}, args...),
		nil)
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := strings.Split(strings.TrimSuffix(string(o), "\n"), "\n")
	if !slices.Equal(got, args) {
		t.Errorf("got %q, want %q", got, args)
	}
}

func TestShellCmdStringEnv(t *testing.T) {
	env := map[string]string{
		"MYNEWT_CFLAGS": "-DA=\"x y\" -DB",
		"MYNEWT_APP":    "app",
	}
	s := ShellCmdString([]string{"script.sh"}, env)
	want := "MYNEWT_APP=app MYNEWT_CFLAGS='-DA=\"x y\" -DB' script.sh"
	if s != want {
		t.Errorf("got %q, want %q", s, want)
	}
}
