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
	"strings"
	"testing"
)

// errorReportingExpectFail runs newt in dir and fails the test if newt
// succeeds or if its output does not contain all of msgs. It returns the
// combined output.
func errorReportingExpectFail(t *testing.T, dir string,
	msgs []string, args ...string) string {

	t.Helper()

	out, err := newtCombinedOutput(dir, nil, args...)
	if err == nil {
		t.Errorf("newt %s succeeded\n%s", strings.Join(args, " "), out)
	}

	for _, m := range msgs {
		if !strings.Contains(out, m) {
			t.Errorf("newt %s: missing %q in output\n%s",
				strings.Join(args, " "), m, out)
		}
	}

	return out
}

// Unknown commands and flags must make newt exit with an error.
func TestErrorReportingUnknownCommand(t *testing.T) {
	dir := t.TempDir()

	t.Run("command", func(t *testing.T) {
		errorReportingExpectFail(t, dir,
			[]string{`unknown command "bogus"`}, "bogus")
	})

	t.Run("flag", func(t *testing.T) {
		errorReportingExpectFail(t, dir,
			[]string{"unknown flag: --nosuchflag"},
			"target", "show", "--nosuchflag")
	})
}
