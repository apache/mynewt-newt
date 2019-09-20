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

package builder

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/resolve"
	"mynewt.apache.org/newt/util"
)

func TestTargetName(testPkgName string) string {
	return strings.Replace(testPkgName, "/", "_", -1)
}

// FeatureString converts a syscfg map to a string.  The string is a
// space-separate list of "enabled" settings.
func FeatureString(settings map[string]string) string {
	var buffer bytes.Buffer

	featureSlice := make([]string, 0, len(settings))
	for k, v := range settings {
		if parse.ValueIsTrue(v) {
			featureSlice = append(featureSlice, k)
		}
	}
	sort.Strings(featureSlice)

	for i, feature := range featureSlice {
		if i != 0 {
			buffer.WriteString(" ")
		}

		buffer.WriteString(feature)
	}

	return buffer.String()
}

type bpkgSorter struct {
	bpkgs []*BuildPackage
}

func (b bpkgSorter) Len() int {
	return len(b.bpkgs)
}
func (b bpkgSorter) Swap(i, j int) {
	b.bpkgs[i], b.bpkgs[j] = b.bpkgs[j], b.bpkgs[i]
}
func (b bpkgSorter) Less(i, j int) bool {
	return b.bpkgs[i].rpkg.Lpkg.Name() < b.bpkgs[j].rpkg.Lpkg.Name()
}

func (b *Builder) sortedBuildPackages() []*BuildPackage {
	sorter := bpkgSorter{
		bpkgs: make([]*BuildPackage, 0, len(b.PkgMap)),
	}

	for _, bpkg := range b.PkgMap {
		sorter.bpkgs = append(sorter.bpkgs, bpkg)
	}

	sort.Sort(sorter)
	return sorter.bpkgs
}

func (b *Builder) SortedRpkgs() []*resolve.ResolvePackage {
	bpkgs := b.sortedBuildPackages()

	rpkgs := make([]*resolve.ResolvePackage, len(bpkgs), len(bpkgs))
	for i, bpkg := range bpkgs {
		rpkgs[i] = bpkg.rpkg
	}

	return rpkgs
}

func logDepInfo(res *resolve.Resolution) {
	// Log API set.
	apis := []string{}
	for api, _ := range res.ApiMap {
		apis = append(apis, api)
	}
	sort.Strings(apis)

	log.Debugf("API set:")
	for _, api := range apis {
		rpkg := res.ApiMap[api]
		log.Debugf("    * " + api + " (" + rpkg.Lpkg.FullName() + ")")
	}

	// Log dependency graph.
	dg, err := depGraph(res.MasterSet)
	if err != nil {
		log.Debugf("Error while constructing dependency graph: %s\n",
			err.Error())
	} else {
		log.Debugf("%s", DepGraphText(dg))
	}

	// Log reverse dependency graph.
	rdg, err := revdepGraph(res.MasterSet)
	if err != nil {
		log.Debugf("Error while constructing reverse dependency graph: %s\n",
			err.Error())
	} else {
		log.Debugf("%s", RevdepGraphText(rdg))
	}
}

// binBasePath calculates the relative path of the "application binary"
// directory.  Examples:
//     * bin/targets/my_blinky_sim/app
//     * bin/targets/splitty-nrf52dk/loader
func (b *Builder) binBasePath() (string, error) {
	if b.appPkg == nil {
		return "", util.NewNewtError("app package not specified")
	}

	// Convert the binary path from absolute to relative.  This is required for
	// Windows compatibility.
	return util.TryRelPath(b.AppBinBasePath()), nil
}

// BasicEnvVars calculates the basic set of environment variables passed to all
// external scripts.  `binBase` is the result of calling `binBasePath()`.
func BasicEnvVars(binBase string, bspPkg *pkg.BspPackage) map[string]string {
	coreRepo := project.GetProject().FindRepo("apache-mynewt-core")
	bspPath := bspPkg.BasePath()

	return map[string]string{
		"CORE_PATH":           coreRepo.Path(),
		"BSP_PATH":            bspPath,
		"BIN_BASENAME":        binBase,
		"BIN_ROOT":            BinRoot(),
		"MYNEWT_PROJECT_ROOT": ProjectRoot(),
	}
}

// SettingsEnvVars calculates the syscfg set of environment variables required
// by image loading scripts.
func SettingsEnvVars(settings map[string]string) map[string]string {
	env := map[string]string{}

	// Add all syscfg settings to the environment with the MYNEWT_VAL_ prefix.
	for k, v := range settings {
		env["MYNEWT_VAL_"+k] = v
	}

	if parse.ValueIsTrue(settings["BOOT_LOADER"]) {
		env["BOOT_LOADER"] = "1"
	}

	env["FEATURES"] = FeatureString(settings)

	return env
}

// SlotEnvVars calculates the image-slot set of environment variables required
// by image loading scripts.  Pass a negative `imageSlot` value if the target
// is a boot loader.
func SlotEnvVars(bspPkg *pkg.BspPackage,
	imageSlot int) (map[string]string, error) {

	env := map[string]string{}

	var flashTargetArea string
	if imageSlot < 0 {
		flashTargetArea = "FLASH_AREA_BOOTLOADER"
	} else {
		env["IMAGE_SLOT"] = strconv.Itoa(imageSlot)
		switch imageSlot {
		case 0:
			flashTargetArea = "FLASH_AREA_IMAGE_0"
		case 1:
			flashTargetArea = "FLASH_AREA_IMAGE_1"
		default:
			return nil, util.FmtNewtError(
				"invalid image slot: have=%d want=0or1", imageSlot)
		}
	}

	tgtArea := bspPkg.FlashMap.Areas[flashTargetArea]
	if tgtArea.Name == "" {
		return nil, util.FmtNewtError(
			"No flash target area %s", flashTargetArea)
	}

	env["FLASH_OFFSET"] = "0x" + strconv.FormatInt(int64(tgtArea.Offset), 16)
	env["FLASH_AREA_SIZE"] = "0x" + strconv.FormatInt(int64(tgtArea.Size), 16)

	return env, nil
}

type UserEnvParams struct {
	Lpkg       *pkg.LocalPackage
	TargetName string // Short name
	AppName    string
	BuildName  string // "app" or "loader"
	UserSrcDir string // "" if none
	UserIncDir string // "" if none
	WorkDir    string
}

// UserEnvVars calculates the set of environment variables required by external
// user scripts.
func UserEnvVars(params UserEnvParams) map[string]string {
	m := map[string]string{}

	m["MYNEWT_APP_BIN_DIR"] = FileBinDir(
		params.TargetName, params.BuildName, params.AppName)
	m["MYNEWT_PKG_BIN_ARCHIVE"] = ArchivePath(
		params.TargetName, params.BuildName, params.Lpkg.FullName(),
		params.Lpkg.Type())
	m["MYNEWT_PKG_BIN_DIR"] = PkgBinDir(
		params.TargetName, params.BuildName, params.Lpkg.FullName(),
		params.Lpkg.Type())
	m["MYNEWT_PKG_NAME"] = params.Lpkg.FullName()
	m["MYNEWT_USER_WORK_DIR"] = params.WorkDir

	if params.UserSrcDir != "" {
		m["MYNEWT_USER_SRC_DIR"] = params.UserSrcDir
	}
	if params.UserIncDir != "" {
		m["MYNEWT_USER_INCLUDE_DIR"] = params.UserIncDir
	}

	return m
}

// EnvVars calculates the full set of environment variables passed to external
// scripts.
func (b *Builder) EnvVars(imageSlot int) (map[string]string, error) {
	bspPkg := b.targetBuilder.bspPkg
	settings := b.cfg.SettingValues()

	binBasePath, err := b.binBasePath()
	if err != nil {
		return nil, err
	}

	// Calculate all three sets of environment variables: basic, settings, and
	// slot.  Then merge the three sets into one.

	env := BasicEnvVars(binBasePath, bspPkg)
	setEnv := SettingsEnvVars(settings)

	if parse.ValueIsTrue(settings["BOOT_LOADER"]) {
		imageSlot = -1
	}

	slotEnv, err := SlotEnvVars(bspPkg, imageSlot)
	if err != nil {
		return nil, err
	}

	for k, v := range setEnv {
		env[k] = v
	}
	for k, v := range slotEnv {
		env[k] = v
	}

	return env, nil
}

// EnvVarsToSlice converts an environment variable map into a slice of strings
// suitable for "shell command" functions defined in `util` (e.g.,
// util.ShellCommand).
func EnvVarsToSlice(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k, _ := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	slice := make([]string, 0, len(env))
	for _, key := range keys {
		slice = append(slice, fmt.Sprintf("%s=%s", key, env[key]))
	}

	return slice
}
