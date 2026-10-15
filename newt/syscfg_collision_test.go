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
	"regexp"
	"strings"
	"testing"
)

// Two true conditions overriding the same setting are allowed only if they
// set the same value.
func TestSyscfgCollision(t *testing.T) {
	projDir := filepath.Join("testdata", "syscfg_collision")

	t.Run("success", func(t *testing.T) {
		out := newtOutput(t, projDir, nil, "target", "config", "show",
			"success")

		re := regexp.MustCompile(`Setting: FOO_IMPLEMENTATION\r?\n.*\r?\n` +
			`.*Value: bar\r?\n`)
		if !re.MatchString(out) {
			t.Errorf("FOO_IMPLEMENTATION not set to bar\n%s", out)
		}
	})

	t.Run("fail", func(t *testing.T) {
		out, err := newtCombinedOutput(projDir, nil, "target", "config",
			"show", "fail")
		if err == nil {
			t.Fatalf("newt target config show succeeded\n%s", out)
		}

		msg := "setting FOO_IMPLEMENTATION collision - two conditions true:"
		if !strings.Contains(out, msg) {
			t.Errorf("missing collision error\n%s", out)
		}
	})
}
