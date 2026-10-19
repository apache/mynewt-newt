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
	"testing"
)

// mfgErrorsWrite writes a file with the given contents into the project,
// creating its parent directories as needed.
func mfgErrorsWrite(t *testing.T, projDir string, path string,
	contents string) {

	t.Helper()

	path = filepath.Join(projDir, path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

// mfgErrorsProject creates a project with a target named "targets/mfgtgt"
// using the given target.yml contents and an mfg package "mfgs/mfgtest"
// referencing this target.
func mfgErrorsProject(t *testing.T, targetYml string) string {
	t.Helper()

	projDir := copyToTemp(t, filepath.Join("testdata", "syscfg_collision"))

	mfgErrorsWrite(t, projDir, "targets/mfgtgt/pkg.yml",
		"pkg.name: targets/mfgtgt\npkg.type: target\n")
	mfgErrorsWrite(t, projDir, "targets/mfgtgt/target.yml", targetYml)
	mfgErrorsWrite(t, projDir, "mfgs/mfgtest/pkg.yml",
		"pkg.name: mfgs/mfgtest\npkg.type: mfg\n")
	mfgErrorsWrite(t, projDir, "mfgs/mfgtest/mfg.yml", `
mfg.targets:
    - name: targets/mfgtgt
      area: FLASH_AREA_IMAGE_0
      offset: 0
mfg.meta:
    area: FLASH_AREA_IMAGE_0
`)

	return projDir
}

// Targets referenced by mfg.yml without a valid app or BSP must be
// reported as errors.
func TestMfgErrorsTargetPkgs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		targetYml string
		msg       string
	}{
		{
			name:      "no_bsp",
			targetYml: "target.app: apps/app\n",
			msg:       `target "targets/mfgtgt" has missing or unresolvable BSP`,
		},
		{
			name: "bad_bsp",
			targetYml: "target.app: apps/app\n" +
				"target.bsp: \"@apache-mynewt-core/hw/bsp/nosuch\"\n",
			msg: `target "targets/mfgtgt" has missing or unresolvable BSP`,
		},
		{
			name:      "no_app",
			targetYml: "target.bsp: \"@apache-mynewt-core/hw/bsp/sim\"\n",
			msg:       `target "targets/mfgtgt" has missing or unresolvable app`,
		},
		{
			name: "bad_app",
			targetYml: "target.app: apps/nosuch\n" +
				"target.bsp: \"@apache-mynewt-core/hw/bsp/sim\"\n",
			msg: `target "targets/mfgtgt" has missing or unresolvable app`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projDir := mfgErrorsProject(t, tc.targetYml)

			cliErrorCheck(t, projDir, nil, tc.msg,
				"mfg", "create", "mfgtest", "1.0.0")
		})
	}
}
