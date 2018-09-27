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

package sysinit

import (
	"bytes"
	"fmt"
	"io"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/stage"
)

func initFuncs(pkgs []*pkg.LocalPackage) []stage.StageFunc {
	fns := make([]stage.StageFunc, 0, len(pkgs))
	for _, p := range pkgs {
		initMap := p.Init()
		for name, stageNum := range initMap {
			fn := stage.StageFunc{
				Name:  name,
				Stage: stageNum,
				Pkg:   p,
			}
			fns = append(fns, fn)
		}
	}

	return fns
}

func sortedInitFuncs(pkgs []*pkg.LocalPackage) []stage.StageFunc {
	fns := initFuncs(pkgs)
	stage.SortStageFuncs(fns, "sysinit")
	return fns
}

func write(pkgs []*pkg.LocalPackage, isLoader bool,
	w io.Writer) {

	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	if isLoader {
		fmt.Fprintf(w, "#if SPLIT_LOADER\n\n")
	} else {
		fmt.Fprintf(w, "#if !SPLIT_LOADER\n\n")
	}

	fns := sortedInitFuncs(pkgs)

	stage.WritePrototypes(fns, w)

	var fnName string
	if isLoader {
		fnName = "sysinit_loader"
	} else {
		fnName = "sysinit_app"
	}

	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "void\n%s(void)\n{\n", fnName)

	stage.WriteCalls(fns, "", w)

	fmt.Fprintf(w, "}\n\n")
	fmt.Fprintf(w, "#endif\n")
}

func EnsureWritten(pkgs []*pkg.LocalPackage, srcDir string, targetName string,
	isLoader bool) error {

	buf := bytes.Buffer{}
	write(pkgs, isLoader, &buf)

	var path string
	if isLoader {
		path = fmt.Sprintf("%s/%s-sysinit-loader.c", srcDir, targetName)
	} else {
		path = fmt.Sprintf("%s/%s-sysinit-app.c", srcDir, targetName)
	}

	return stage.EnsureWritten(path, buf.Bytes())
}
