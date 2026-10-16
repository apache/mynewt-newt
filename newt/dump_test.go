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
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Parses output of "newt target dump". The sysinit section is not compared
// since it is not stable between platforms.
func parseDump(t *testing.T, data string) map[string]interface{} {
	t.Helper()

	var dump map[string]interface{}
	if err := json.Unmarshal([]byte(data), &dump); err != nil {
		t.Fatalf("invalid dump: %s", err)
	}
	delete(dump, "sysinit")

	return dump
}

// firstDiff returns lines around the first line that differs between a and
// b.
func firstDiff(a string, b string) string {
	al := strings.Split(a, "\n")
	bl := strings.Split(b, "\n")

	i := 0
	for i < len(al) && i < len(bl) && al[i] == bl[i] {
		i++
	}

	ctx := func(lines []string) string {
		start := max(i-3, 0)
		end := min(i+4, len(lines))
		return strings.Join(lines[start:end], "\n")
	}

	return fmt.Sprintf("first difference at line %d\n--- expected:\n%s\n"+
		"--- got:\n%s", i+1, ctx(al), ctx(bl))
}

// Compares "newt target dump" output for each target of the test project
// with known answers. The answers can be regenerated with
// testdata/dump/generate-answers.sh.
func TestTargetDump(t *testing.T) {
	projDir := filepath.Join("testdata", "dump", "proj")
	answersDir := filepath.Join("testdata", "dump", "answers")

	for _, target := range subdirs(t, filepath.Join(projDir, "targets")) {
		if target == "unittest" {
			continue
		}

		t.Run(target, func(t *testing.T) {
			t.Parallel()

			answerPath := filepath.Join(answersDir, target+".json")
			expected := parseDump(t, readFile(t, answerPath))
			got := parseDump(t, newtOutput(t, projDir, nil,
				"target", "dump", target))

			if !reflect.DeepEqual(expected, got) {
				e, _ := json.MarshalIndent(expected, "", "  ")
				g, _ := json.MarshalIndent(got, "", "  ")
				t.Errorf("dump differs from %s, %s", answerPath,
					firstDiff(string(e), string(g)))
			}
		})
	}
}
