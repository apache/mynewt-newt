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
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

func (t *TargetBuilder) loadLoader(slot int, extraJtagCmd string) error {
	if err := t.bspPkg.Reload(t.LoaderBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	return t.LoaderBuilder.Load(slot, extraJtagCmd)
}

func (t *TargetBuilder) loadApp(slot int, extraJtagCmd string) error {
	if err := t.bspPkg.Reload(t.AppBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	return t.AppBuilder.Load(slot, extraJtagCmd)
}

func (t *TargetBuilder) debugLoader(extraJtagCmd string, reset bool,
	noGDB bool) error {

	if err := t.bspPkg.Reload(t.LoaderBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	return t.LoaderBuilder.Debug(extraJtagCmd, reset, noGDB)
}

func (t *TargetBuilder) debugApp(extraJtagCmd string, reset bool,
	noGDB bool) error {

	if err := t.bspPkg.Reload(t.AppBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	return t.AppBuilder.Debug(extraJtagCmd, reset, noGDB)
}

func (t *TargetBuilder) Load(extraJtagCmd string) error {
	err := t.PrepBuild()
	if err != nil {
		return err
	}

	if t.LoaderBuilder != nil {
		err = t.loadApp(1, extraJtagCmd)
		if err == nil {
			err = t.loadLoader(0, extraJtagCmd)
		}
	} else {
		err = t.loadApp(0, extraJtagCmd)
	}

	return err
}

func RunOptionalCheck(checkScript string, env []string) error {
	if checkScript == "" {
		return nil
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)

	cmd := []string{
		checkScript,
	}

	/* Handle Ctrl-C, terminate newt, as it is the
	   intended behavior */
	go func() {
		sig := <-sigs
		fmt.Println(sig)
		os.Exit(0)
	}()

	util.StatusMessage(util.VERBOSITY_SILENT,
		"Optional target check: %s\n", strings.Join(cmd, " "))
	util.ShellInteractiveCommand(cmd, env, true)

	/* Unregister SIGTERM handler */
	signal.Reset(syscall.SIGTERM)
	return nil
}

func Load(binBaseName string, bspPkg *pkg.BspPackage,
	extraEnvSettings map[string]string) error {

	if bspPkg.DownloadScript == "" {
		return nil
	}

	bspPath := bspPkg.BasePath()

	sortedKeys := make([]string, 0, len(extraEnvSettings))
	for k, _ := range extraEnvSettings {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	env := []string{}
	for _, key := range sortedKeys {
		env = append(env, fmt.Sprintf("%s=%s", key, extraEnvSettings[key]))
	}

	coreRepo := project.GetProject().FindRepo("apache-mynewt-core")
	env = append(env, fmt.Sprintf("CORE_PATH=%s", coreRepo.Path()))
	env = append(env, fmt.Sprintf("BSP_PATH=%s", bspPath))
	env = append(env, fmt.Sprintf("BIN_BASENAME=%s", binBaseName))
	env = append(env, fmt.Sprintf("BIN_ROOT=%s", BinRoot()))
	env = append(env, fmt.Sprintf("MYNEWT_PROJECT_ROOT=%s", ProjectRoot()))

	RunOptionalCheck(bspPkg.OptChkScript, env)
	// bspPath, binBaseName are passed in command line for backwards
	// compatibility
	cmd := []string{
		bspPkg.DownloadScript,
		bspPath,
		binBaseName,
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Load command: %s\n",
		strings.Join(cmd, " "))
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Environment:\n")
	for _, v := range env {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "* %s\n", v)
	}
	if _, err := util.ShellCommand(cmd, env); err != nil {
		return err
	}
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Successfully loaded image.\n")

	return nil
}

func (b *Builder) Load(imageSlot int, extraJtagCmd string) error {
	if b.appPkg == nil {
		return util.NewNewtError("app package not specified")
	}

	/* Populate the package list and feature sets. */
	err := b.targetBuilder.PrepBuild()
	if err != nil {
		return err
	}

	envSettings := map[string]string{
		"IMAGE_SLOT": strconv.Itoa(imageSlot),
		"FEATURES":   b.FeatureString(),
	}
	if extraJtagCmd != "" {
		envSettings["EXTRA_JTAG_CMD"] = extraJtagCmd
	}
	settings := b.cfg.SettingValues()

	var flashTargetArea string
	if parse.ValueIsTrue(settings["BOOT_LOADER"]) {
		envSettings["BOOT_LOADER"] = "1"

		flashTargetArea = "FLASH_AREA_BOOTLOADER"
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Loading bootloader\n")
	} else {
		if imageSlot == 0 {
			flashTargetArea = "FLASH_AREA_IMAGE_0"
		} else if imageSlot == 1 {
			flashTargetArea = "FLASH_AREA_IMAGE_1"
		}
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Loading %s image into slot %d\n", b.buildName, imageSlot+1)
	}

	bspPkg := b.targetBuilder.bspPkg
	tgtArea := bspPkg.FlashMap.Areas[flashTargetArea]
	if tgtArea.Name == "" {
		return util.NewNewtError(fmt.Sprintf("No flash target area %s\n",
			flashTargetArea))
	}
	envSettings["FLASH_OFFSET"] = "0x" + strconv.FormatInt(int64(tgtArea.Offset), 16)
	envSettings["FLASH_AREA_SIZE"] = "0x" + strconv.FormatInt(int64(tgtArea.Size), 16)

	// Add all syscfg settings to the environment with the MYNEWT_VAL_ prefix.
	for k, v := range settings {
		envSettings["MYNEWT_VAL_"+k] = v
	}

	// Convert the binary path from absolute to relative.  This is required for
	// compatibility with unix-in-windows environemnts (e.g., cygwin).
	binPath := util.TryRelPath(b.AppBinBasePath())
	if err := Load(binPath, b.targetBuilder.bspPkg, envSettings); err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) Debug(extraJtagCmd string, reset bool, noGDB bool) error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	if t.LoaderBuilder == nil {
		return t.debugApp(extraJtagCmd, reset, noGDB)
	}
	return t.debugLoader(extraJtagCmd, reset, noGDB)
}

func (b *Builder) debugBin(binPath string, extraJtagCmd string, reset bool,
	noGDB bool) error {
	/*
	 * Populate the package list and feature sets.
	 */
	err := b.targetBuilder.PrepBuild()
	if err != nil {
		return err
	}

	bspPath := b.bspPkg.rpkg.Lpkg.BasePath()
	binBaseName := binPath
	featureString := b.FeatureString()
	bspPkg := b.targetBuilder.bspPkg

	coreRepo := project.GetProject().FindRepo("apache-mynewt-core")
	envSettings := []string{
		fmt.Sprintf("CORE_PATH=%s", coreRepo.Path()),
		fmt.Sprintf("BSP_PATH=%s", bspPath),
		fmt.Sprintf("BIN_BASENAME=%s", binBaseName),
		fmt.Sprintf("FEATURES=%s", featureString),
		fmt.Sprintf("MYNEWT_PROJECT_ROOT=%s", ProjectRoot()),
	}
	if extraJtagCmd != "" {
		envSettings = append(envSettings,
			fmt.Sprintf("EXTRA_JTAG_CMD=%s", extraJtagCmd))
	}
	if reset == true {
		envSettings = append(envSettings, fmt.Sprintf("RESET=true"))
	}
	if noGDB == true {
		envSettings = append(envSettings, fmt.Sprintf("NO_GDB=1"))
	}

	os.Chdir(project.GetProject().Path())

	RunOptionalCheck(bspPkg.OptChkScript, envSettings)
	// bspPath, binBaseName are passed in command line for backwards
	// compatibility
	cmdLine := []string{
		b.targetBuilder.bspPkg.DebugScript, bspPath, binBaseName,
	}

	fmt.Printf("%s\n", cmdLine)
	return util.ShellInteractiveCommand(cmdLine, envSettings, false)
}

func (b *Builder) Debug(extraJtagCmd string, reset bool, noGDB bool) error {
	if b.appPkg == nil {
		return util.NewNewtError("app package not specified")
	}

	// Convert the binary path from absolute to relative.  This is required for
	// Windows compatibility.
	binPath := util.TryRelPath(b.AppBinBasePath())
	return b.debugBin(binPath, extraJtagCmd, reset, noGDB)
}
