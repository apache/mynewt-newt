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
	"mynewt.apache.org/newt/newt/cfgv"
	"runtime"
	"strings"

	"mynewt.apache.org/newt/newt/config"
	"mynewt.apache.org/newt/newt/flashmap"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

const BSP_YAML_FILENAME = "bsp.yml"

type BspYCfgOverride struct {
	Pkg  *LocalPackage
	PkgY *ycfg.YCfg
}

type BspPackage struct {
	*LocalPackage
	yov                *BspYCfgOverride
	CompilerName       string
	CompilerNamePkg    *LocalPackage /* package which defines compiler name */
	Arch               string
	LinkerScripts      []string
	Part2LinkerScripts []string /* scripts to link app to second partition */
	DownloadScript     string
	DebugScript        string
	OptChkScript       string
	ImageOffset        int
	ImagePad           int
	FlashMap           flashmap.FlashMap
	BspV               ycfg.YCfg
}

func (bsp *BspPackage) BspYamlPath() string {
	return fmt.Sprintf("%s/%s", bsp.BasePath(), BSP_YAML_FILENAME)
}

func (bsp *BspPackage) resolvePathSetting(
	settings *cfgv.Settings, key string) (string, error) {
	var ypkg *LocalPackage
	var ycfg *ycfg.YCfg

	proj := interfaces.GetProject()

	ypkg, ycfg = bsp.selectKey(key)
	val, err := ycfg.GetValString(key, settings)
	util.OneTimeWarningError(err)
	if val == "" {
		return "", nil
	}
	path, err := proj.ResolvePath(ypkg.Repo().Path(), val)
	if err != nil {
		return "", util.PreNewtError(err,
			"Package \"%s\" specifies invalid %s setting",
			ypkg.FullName(), key)
	}
	return path, nil
}

// Interprets a setting as either a single linker script or a list of linker
// scripts.
func (bsp *BspPackage) resolveLinkerScriptSetting(
	settings *cfgv.Settings, key string) ([]string, error) {
	var ypkg *LocalPackage
	var ycfg *ycfg.YCfg

	paths := []string{}

	// Assume config file specifies a list of scripts.
	ypkg, ycfg = bsp.selectKey(key)
	vals, err := ycfg.GetValStringSlice(key, settings)
	util.OneTimeWarningError(err)
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
			path, err := proj.ResolvePath(ypkg.Repo().Path(), val)
			if err != nil {
				return nil, util.PreNewtError(err,
					"Package \"%s\" specifies invalid %s setting",
					ypkg.FullName(), key)
			}

			if path != "" {
				paths = append(paths, path)
			}
		}
	}

	return paths, nil
}

func (bsp *BspPackage) selectKey(key string) (*LocalPackage, *ycfg.YCfg) {
	if bsp.yov != nil && bsp.yov.PkgY.HasKey(key) {
		return bsp.yov.Pkg, bsp.yov.PkgY
	} else {
		return bsp.LocalPackage, &bsp.BspV
	}
}

func (bsp *BspPackage) Reload(settings *cfgv.Settings) error {
	var ypkg *LocalPackage
	var ycfg *ycfg.YCfg
	var err error

	if settings == nil {
		settings = cfgv.NewSettings(nil)
	}
	settings.Set(strings.ToUpper(runtime.GOOS), "1")

	bsp.BspV, err = config.ReadFile(bsp.BspYamlPath())
	if err != nil {
		return err
	}
	bsp.AddCfgFilename(bsp.BspYamlPath())

	ypkg, ycfg = bsp.selectKey("bsp.compiler")
	bsp.CompilerNamePkg = ypkg
	bsp.CompilerName, err = ycfg.GetValString("bsp.compiler", settings)
	util.OneTimeWarningError(err)

	bsp.Arch, err = bsp.BspV.GetValString("bsp.arch", settings)
	util.OneTimeWarningError(err)

	_, ycfg = bsp.selectKey("bsp.image_offset")
	bsp.ImageOffset, err = ycfg.GetValInt("bsp.image_offset", settings)
	util.OneTimeWarningError(err)

	_, ycfg = bsp.selectKey("bsp.image_pad")
	bsp.ImagePad, err = ycfg.GetValInt("bsp.image_pad", settings)
	util.OneTimeWarningError(err)

	bsp.LinkerScripts, err = bsp.resolveLinkerScriptSetting(settings, "bsp.linkerscript")
	if err != nil {
		return err
	}

	bsp.Part2LinkerScripts, err = bsp.resolveLinkerScriptSetting(settings, "bsp.part2linkerscript")
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
	/* Optional Target Checker Script, not an err if not found */
	bsp.OptChkScript, err = bsp.resolvePathSetting(
		settings, "bsp.optionalcheckscript")

	if bsp.CompilerName == "" {
		return util.NewNewtError("BSP does not specify a compiler " +
			"(bsp.compiler)")
	}
	if bsp.Arch == "" {
		return util.NewNewtError("BSP does not specify an architecture " +
			"(bsp.arch)")
	}

	ypkg, ycfg = bsp.selectKey("bsp.flash_map")
	ymlFlashMap, err := ycfg.GetValStringMap("bsp.flash_map", settings)
	util.OneTimeWarningError(err)
	if ymlFlashMap == nil {
		return util.NewNewtError("BSP does not specify a flash map " +
			"(bsp.flash_map)")
	}
	bsp.FlashMap, err = flashmap.Read(ymlFlashMap, ypkg.FullName())
	if err != nil {
		return err
	}

	return nil
}

func NewBspPackage(lpkg *LocalPackage, yov *BspYCfgOverride) (*BspPackage, error) {
	bsp := &BspPackage{
		yov:            yov,
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

func NewBspYCfgOverride(lpkg *LocalPackage, ycfg *ycfg.YCfg) *BspYCfgOverride {
	ov := &BspYCfgOverride{
		Pkg:  lpkg,
		PkgY: ycfg,
	}

	return ov
}
