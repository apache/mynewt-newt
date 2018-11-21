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
	"fmt"
	"runtime"
	"strings"

	"mynewt.apache.org/newt/newt/flashmap"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
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
	FlashMap           flashmap.FlashMap
	BspV               ycfg.YCfg
}

func (bsp *BspPackage) BspYamlPath() string {
	return fmt.Sprintf("%s/%s", bsp.BasePath(), BSP_YAML_FILENAME)
}

func (bsp *BspPackage) resolvePathSetting(
	settings map[string]string, key string) (string, error) {

	proj := interfaces.GetProject()

	val := bsp.BspV.GetValString(key, settings)
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
	settings map[string]string, key string) ([]string, error) {

	paths := []string{}

	// Assume config file specifies a list of scripts.
	vals := bsp.BspV.GetValStringSlice(key, settings)
	if vals == nil {
		// Couldn't read a list of scripts; try to interpret setting as a
		// single script.
		path, err := bsp.resolvePathSetting(settings, key)
		if err != nil {
			return nil, err
		}

		if path != "" {
			paths = append(paths, path)
		}
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

			if path != "" {
				paths = append(paths, path)
			}
		}
	}

	return paths, nil
}

func (bsp *BspPackage) Reload(settings map[string]string) error {
	var err error

	if settings == nil {
		settings = map[string]string{}
	}
	settings[strings.ToUpper(runtime.GOOS)] = "1"

	bsp.BspV, err = newtutil.ReadConfigPath(bsp.BspYamlPath())
	if err != nil {
		return err
	}
	bsp.AddCfgFilename(bsp.BspYamlPath())

	bsp.CompilerName = bsp.BspV.GetValString("bsp.compiler", settings)
	bsp.Arch = bsp.BspV.GetValString("bsp.arch", settings)

	bsp.LinkerScripts, err = bsp.resolveLinkerScriptSetting(
		settings, "bsp.linkerscript")
	if err != nil {
		return err
	}

	bsp.Part2LinkerScripts, err = bsp.resolveLinkerScriptSetting(
		settings, "bsp.part2linkerscript")
	if err != nil {
		return err
	}

	bsp.DownloadScript, err = bsp.resolvePathSetting(
		settings, "bsp.downloadscript")
	if err != nil {
		return err
	}
	bsp.DebugScript, err = bsp.resolvePathSetting(
		settings, "bsp.debugscript")
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

	ymlFlashMap := bsp.BspV.GetValStringMap("bsp.flash_map", settings)
	if ymlFlashMap == nil {
		return util.NewNewtError("BSP does not specify a flash map " +
			"(bsp.flash_map)")
	}
	bsp.FlashMap, err = flashmap.Read(ymlFlashMap)
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
	}

	lpkg.Load()
	bsp.LocalPackage = lpkg
	bsp.BspV = ycfg.NewYCfg(bsp.BspYamlPath())

	err := bsp.Reload(nil)

	return bsp, err
}
