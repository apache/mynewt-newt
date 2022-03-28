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

package extcmd

import (
	"fmt"
	"mynewt.apache.org/newt/newt/cfgv"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/stage"
	"mynewt.apache.org/newt/newt/syscfg"
)

// ExtCmdCfg represents an ordered list of external commands that get run
// during the build (aka "user scripts").
type ExtCmdCfg struct {
	// Used in diagnostic messages only (example: "pre_build_cmds").
	Name string

	// Sorted in call order (stage-num,command-string).
	StageFuncs []stage.StageFunc

	// Strings describing errors encountered while parsing the extcmd config.
	InvalidSettings []string
}

// GetMapFn retrieves a set of ordered external commands from a package.  For
// example, one GetMapFn instance might retrieve a package's pre-build
// commands.
type GetMapFn func(
	lpkg *pkg.LocalPackage, settings *cfgv.Settings) map[string]string

func (ecfg *ExtCmdCfg) readOnePkg(lpkg *pkg.LocalPackage, cfg *syscfg.Cfg,
	getMapCb GetMapFn) {

	settings := cfg.AllSettingsForLpkg(lpkg)
	cmdMap := getMapCb(lpkg, settings)

	for name, stageStr := range cmdMap {
		sf, err := stage.NewStageFunc(name, stageStr, lpkg, cfg)
		if err != nil {
			text := fmt.Sprintf("%s: %s", lpkg.FullName(), err.Error())
			ecfg.InvalidSettings = append(ecfg.InvalidSettings, text)
		} else {
			ecfg.StageFuncs = append(ecfg.StageFuncs, sf)
		}
	}
}

// Read constructs an external command configuration from a full set of
// packages.
func Read(name string, lpkgs []*pkg.LocalPackage, cfg *syscfg.Cfg,
	getMapCb GetMapFn) ExtCmdCfg {

	ecfg := ExtCmdCfg{
		Name: name,
	}

	for _, lpkg := range lpkgs {
		ecfg.readOnePkg(lpkg, cfg, getMapCb)
	}

	stage.SortStageFuncs(ecfg.StageFuncs, ecfg.Name)

	return ecfg
}

// If any errors were encountered while parsing extcmd definitions, this
// function returns a string indicating the errors.  If no errors were
// encountered, "" is returned.
func (ecfg *ExtCmdCfg) ErrorText() string {
	str := ""

	if len(ecfg.InvalidSettings) > 0 {
		str += fmt.Sprintf("Invalid %s definitions detected:", ecfg.Name)
		for _, e := range ecfg.InvalidSettings {
			str += "\n    " + e
		}
	}

	return str
}
