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

package pkg

import (
	"strings"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
)

const BSP_YAML_FILENAME = "bsp.yml"

type BspPackage struct {
	*LocalPackage
	CompilerName      string
	Arch              string
	LinkerScript      string
	Part2LinkerScript string /* script to link app to second partition */
	DownloadScript    string
	DebugScript       string
	BspV              *viper.Viper
}

func (bsp *BspPackage) Reload(features map[string]bool) error {
	var err error
	bsp.BspV, err = util.ReadConfig(bsp.BasePath(),
		strings.TrimSuffix(BSP_YAML_FILENAME, ".yml"))
	if err != nil {
		return err
	}
	bsp.AddCfgFilename(bsp.BasePath() + BSP_YAML_FILENAME)

	bsp.CompilerName = newtutil.GetStringFeatures(bsp.BspV,
		features, "bsp.compiler")
	bsp.Arch = newtutil.GetStringFeatures(bsp.BspV,
		features, "bsp.arch")
	bsp.LinkerScript = newtutil.GetStringFeatures(bsp.BspV,
		features, "bsp.linkerscript")
	bsp.Part2LinkerScript = newtutil.GetStringFeatures(bsp.BspV,
		features, "bsp.part2linkerscript")
	bsp.DownloadScript = newtutil.GetStringFeatures(bsp.BspV,
		features, "bsp.downloadscript")
	bsp.DebugScript = newtutil.GetStringFeatures(bsp.BspV,
		features, "bsp.debugscript")

	if bsp.CompilerName == "" {
		return util.NewNewtError("BSP does not specify a compiler " +
			"(bsp.compiler)")
	}
	if bsp.Arch == "" {
		return util.NewNewtError("BSP does not specify an architecture " +
			"(bsp.arch)")
	}

	return nil
}

func NewBspPackage(lpkg *LocalPackage) (*BspPackage, error) {
	bsp := &BspPackage{
		CompilerName:   "",
		LinkerScript:   "",
		DownloadScript: "",
		DebugScript:    "",
		BspV:           viper.New(),
	}
	lpkg.Load()
	bsp.LocalPackage = lpkg
	err := bsp.Reload(nil)

	return bsp, err
}
