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
	"mynewt.apache.org/newt/newt/syscfg"
)

type SysdownCfg struct {
	// Sorted in call order (stage-num,function-name).
	StageFuncs []stage.StageFunc

	// Strings describing errors encountered while parsing the sysdown config.
	InvalidSettings []string

	// Contains sets of entries with conflicting function names.
	//     [function-name] => <slice-of-stages-with-function-name>
	Conflicts map[string][]stage.StageFunc
}

func (scfg *SysdownCfg) readOnePkg(lpkg *pkg.LocalPackage, cfg *syscfg.Cfg) {
	settings := cfg.AllSettingsForLpkg(lpkg)
	initMap := lpkg.DownFuncs(settings)
	for name, stageStr := range initMap {
		sf, err := stage.NewStageFunc(name, stageStr, lpkg, cfg)
		if err != nil {
			scfg.InvalidSettings = append(scfg.InvalidSettings, err.Error())
		}
		sf.ReturnType = "int"
		sf.ArgList = "int reason"

		scfg.StageFuncs = append(scfg.StageFuncs, sf)
	}
}

// Searches the sysdown configuration for entries with identical function
// names.  The sysdown configuration object is populated with the results.
func (scfg *SysdownCfg) detectConflicts() {
	m := map[string][]stage.StageFunc{}

	for _, sf := range scfg.StageFuncs {
		m[sf.Name] = append(m[sf.Name], sf)
	}

	for name, sfs := range m {
		if len(sfs) > 1 {
			scfg.Conflicts[name] = sfs
		}
	}
}

func Read(lpkgs []*pkg.LocalPackage, cfg *syscfg.Cfg) SysdownCfg {
	scfg := SysdownCfg{}

	for _, lpkg := range lpkgs {
		scfg.readOnePkg(lpkg, cfg)
	}

	scfg.detectConflicts()
	stage.SortStageFuncs(scfg.StageFuncs, "sysdown")

	return scfg
}

func (scfg *SysdownCfg) filter(lpkgs []*pkg.LocalPackage) []stage.StageFunc {
	m := make(map[*pkg.LocalPackage]struct{}, len(lpkgs))

	for _, lpkg := range lpkgs {
		m[lpkg] = struct{}{}
	}

	filtered := []stage.StageFunc{}
	for _, sf := range scfg.StageFuncs {
		if _, ok := m[sf.Pkg]; ok {
			filtered = append(filtered, sf)
		}
	}

	return filtered
}

// If any errors were encountered while parsing sysdown definitions, this
// function returns a string indicating the errors.  If no errors were
// encountered, "" is returned.
func (scfg *SysdownCfg) ErrorText() string {
	str := ""

	if len(scfg.InvalidSettings) > 0 {
		str += "Invalid sysdown definitions detected:"
		for _, e := range scfg.InvalidSettings {
			str += "\n    " + e
		}
	}

	if len(scfg.Conflicts) > 0 {
		str += "Sysdown function name conflicts detected:\n"
		for name, sfs := range scfg.Conflicts {
			for _, sf := range sfs {
				str += fmt.Sprintf("    Function=%s Package=%s\n",
					name, sf.Pkg.FullName())
			}
		}

		str += "\nResolve the problem by assigning unique function names " +
			"to each entry."
	}

	return str
}

func (scfg *SysdownCfg) write(lpkgs []*pkg.LocalPackage, isLoader bool,
	w io.Writer) error {

	var sfs []stage.StageFunc
	if lpkgs == nil {
		sfs = scfg.StageFuncs
	} else {
		sfs = scfg.filter(lpkgs)
	}

	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	if isLoader {
		fmt.Fprintf(w, "#if SPLIT_LOADER\n\n")
	} else {
		fmt.Fprintf(w, "#if !SPLIT_LOADER\n\n")
	}

	stage.WritePrototypes(sfs, w)

	var arrName string

	// XXX: Assign a different array name depending on isLoader.
	arrName = "sysdown_cbs"

	fmt.Fprintf(w, "\nint (* const %s[])(int reason) = {\n", arrName)
	stage.WriteArr(sfs, w)
	fmt.Fprintf(w, "};\n")

	fmt.Fprintf(w, "#endif\n")

	return nil
}

func (scfg *SysdownCfg) EnsureWritten(lpkgs []*pkg.LocalPackage, srcDir string,
	targetName string, isLoader bool) error {

	buf := bytes.Buffer{}
	if err := scfg.write(lpkgs, isLoader, &buf); err != nil {
		return err
	}

	var path string
	if isLoader {
		path = fmt.Sprintf("%s/%s-sysdown-loader.c", srcDir, targetName)
	} else {
		path = fmt.Sprintf("%s/%s-sysdown-app.c", srcDir, targetName)
	}

	return stage.EnsureWritten(path, buf.Bytes())
}
