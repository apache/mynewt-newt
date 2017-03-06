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
	"runtime"
	"strings"

	"mynewt.apache.org/newt/newt/flash"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
)

const BSP_YAML_FILENAME = "bsp.yml"

type BspPackage struct {
	*LocalPackage
	CompilerName       string
	Arch               string
	LinkerScripts      []string
	Part2LinkerScripts []string /* scripts to link app to second partition */
	DownloadScript     string
	DebugScript        string
	FlashMap           flash.FlashMap
	BspV               *viper.Viper
}

func (bsp *BspPackage) resolvePathSetting(
	features map[string]bool, key string) (string, error) {

	proj := interfaces.GetProject()

	val := newtutil.GetStringFeatures(bsp.BspV, features, key)
	if val == "" {
		return "", nil
	}
	path, err := proj.ResolvePath(bsp.Repo().Path(), val)
	if err != nil {
		return "", util.PreNewtError(err,
			"BSP \"%s\" specifies invalid %s setting",
			bsp.Name(), key)
	}
	return path, nil
}

// Interprets a setting as either a single linker script or a list of linker
// scripts.
func (bsp *BspPackage) resolveLinkerScriptSetting(
	features map[string]bool, key string) ([]string, error) {

	paths := []string{}

	// Assume config file specifies a list of scripts.
	vals := newtutil.GetStringSliceFeatures(bsp.BspV, features, key)
	if vals == nil {
		// Couldn't read a list of scripts; try to interpret setting as a
		// single script.
		path, err := bsp.resolvePathSetting(features, key)
		if err != nil {
			return nil, err
		}

		paths = append(paths, path)
	} else {
		proj := interfaces.GetProject()

		// Read each linker script from the list.
		for _, val := range vals {
			path, err := proj.ResolvePath(bsp.Repo().Path(), val)
			if err != nil {
				return nil, util.PreNewtError(err,
					"BSP \"%s\" specifies invalid %s setting",
					bsp.Name(), key)
			}

			paths = append(paths, path)
		}
	}

	return paths, nil
}

func (bsp *BspPackage) Reload(features map[string]bool) error {
	var err error

	if features == nil {
		features = map[string]bool{
			strings.ToUpper(runtime.GOOS): true,
		}
	} else {
		features[strings.ToUpper(runtime.GOOS)] = true
	}
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

	bsp.LinkerScripts, err = bsp.resolveLinkerScriptSetting(
		features, "bsp.linkerscript")
	if err != nil {
		return err
	}

	bsp.Part2LinkerScripts, err = bsp.resolveLinkerScriptSetting(
		features, "bsp.part2linkerscript")
	if err != nil {
		return err
	}

	bsp.DownloadScript, err = bsp.resolvePathSetting(
		features, "bsp.downloadscript")
	if err != nil {
		return err
	}
	bsp.DebugScript, err = bsp.resolvePathSetting(
		features, "bsp.debugscript")
	if err != nil {
		return err
	}

	if bsp.CompilerName == "" {
		return util.NewNewtError("BSP does not specify a compiler " +
			"(bsp.compiler)")
	}
	if bsp.Arch == "" {
		return util.NewNewtError("BSP does not specify an architecture " +
			"(bsp.arch)")
	}

	ymlFlashMap := newtutil.GetStringMapFeatures(bsp.BspV, features,
		"bsp.flash_map")
	if ymlFlashMap == nil {
		return util.NewNewtError("BSP does not specify a flash map " +
			"(bsp.flash_map)")
	}
	bsp.FlashMap, err = flash.Read(ymlFlashMap)
	if err != nil {
		return err
	}

	return nil
}

func NewBspPackage(lpkg *LocalPackage) (*BspPackage, error) {
	bsp := &BspPackage{
		CompilerName:   "",
		DownloadScript: "",
		DebugScript:    "",
		BspV:           viper.New(),
	}
	lpkg.Load()
	bsp.LocalPackage = lpkg
	err := bsp.Reload(nil)

	return bsp, err
}
