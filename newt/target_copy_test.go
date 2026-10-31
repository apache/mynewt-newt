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

const targetCopySrcPkgYml = `pkg.name: targets/success
pkg.type: target
pkg.description: desc
pkg.keywords:
    - kw
pkg.deps:
    - "@apache-mynewt-core/sys/foo"
pkg.deps.FOO_COND_1:
    - "@apache-mynewt-core/sys/foo"
pkg.cflags:
    - -DFOO
pkg.cflags.FOO_COND_1:
    - -DBAR
pkg.lflags:
    - -lm
pkg.init:
    foo_init: 500
`

const targetCopyDstPkgYml = `pkg.name: targets/dst
pkg.type: target
pkg.description: desc
pkg.author:
pkg.homepage:

pkg.deps:
    - "@apache-mynewt-core/sys/foo"
pkg.cflags:
    - "-DFOO"
pkg.lflags:
    - "-lm"
pkg.cflags.FOO_COND_1:
    - "-DBAR"
pkg.deps.FOO_COND_1:
    - "@apache-mynewt-core/sys/foo"
pkg.init:
    foo_init: 500
pkg.keywords:
    - kw
`

// "target copy" must preserve all pkg.yml settings of the source target,
// including conditional ones, and "target set" must not drop them either.
func TestTargetCopyPkgFields(t *testing.T) {
	projDir := copyToTemp(t, filepath.Join("testdata", "syscfg_collision"))

	srcPath := filepath.Join(projDir, "targets", "success", "pkg.yml")
	if err := os.WriteFile(srcPath, []byte(targetCopySrcPkgYml),
		0644); err != nil {
		t.Fatal(err)
	}

	newtOutput(t, projDir, nil, "target", "copy", "success", "dst")

	dstPath := filepath.Join(projDir, "targets", "dst", "pkg.yml")
	checkOutput(t, "pkg.yml after copy", targetCopyDstPkgYml,
		readFile(t, dstPath))

	newtOutput(t, projDir, nil, "target", "set", "dst",
		"target.build_profile=optimized")

	checkOutput(t, "pkg.yml after set", targetCopyDstPkgYml,
		readFile(t, dstPath))
}
