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
	"path/filepath"
	"strings"
	"testing"
)

// cliErrorCheck runs newt and verifies that it fails gracefully, i.e.,
// with an error message containing msg rather than with a panic.
func cliErrorCheck(t *testing.T, dir string, env []string, msg string,
	args ...string) {

	t.Helper()

	out, err := newtCombinedOutput(dir, env, args...)
	if err == nil {
		t.Errorf("newt %q succeeded\n%s", args, out)
	}
	if strings.Contains(out, "panic") || strings.Contains(out, "goroutine ") {
		t.Errorf("newt %q panicked\n%s", args, out)
	}
	if !strings.Contains(out, msg) {
		t.Errorf("newt %q: missing %q in output\n%s", args, msg, out)
	}
}

// "newt man" and "newt apropos" require an argument.
func TestCliManNoArgs(t *testing.T) {
	projDir := filepath.Join("testdata", "syscfg_collision")

	cliErrorCheck(t, projDir, nil, "Exactly one argument required", "man")
	cliErrorCheck(t, projDir, nil, "Exactly one argument required",
		"apropos")
}
