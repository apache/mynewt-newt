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

package sysdown

import (
	"bytes"
	"fmt"
	"io"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/stage"
)

// downFuncs collects the sysdown functions corresponding to the provided
// packages.
func downFuncs(pkgs []*pkg.LocalPackage) []stage.StageFunc {
	fns := make([]stage.StageFunc, 0, len(pkgs))
	for _, p := range pkgs {
		downMap := p.DownFuncs()
		for name, stageNum := range downMap {
			fn := stage.StageFunc{
				Name:       name,
				Stage:      stageNum,
				ReturnType: "int",
				ArgList:    "int reason",
				Pkg:        p,
			}
			fns = append(fns, fn)
		}
	}

	return fns
}

func sortedDownFuncs(pkgs []*pkg.LocalPackage) []stage.StageFunc {
	fns := downFuncs(pkgs)
	stage.SortStageFuncs(fns, "sysdown")
	return fns
}

func write(pkgs []*pkg.LocalPackage, w io.Writer) {
	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	fns := sortedDownFuncs(pkgs)
	stage.WritePrototypes(fns, w)

	fmt.Fprintf(w, "\nint (* const sysdown_cbs[])(int reason) = {\n")
	stage.WriteArr(fns, w)
	fmt.Fprintf(w, "};\n")
}

func EnsureWritten(pkgs []*pkg.LocalPackage, srcDir string,
	targetName string) error {

	buf := bytes.Buffer{}
	write(pkgs, &buf)

	path := fmt.Sprintf("%s/%s-sysdown.c", srcDir, targetName)
	return stage.EnsureWritten(path, buf.Bytes())
}
