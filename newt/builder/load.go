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
	"path/filepath"
	"sort"
	"strconv"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

func (t *TargetBuilder) Load(extraJtagCmd string) error {

	err := t.PrepBuild()

	if err != nil {
		return err
	}

	if t.LoaderBuilder != nil {
		err = t.AppBuilder.Load(1, extraJtagCmd)
		if err == nil {
			err = t.LoaderBuilder.Load(0, extraJtagCmd)
		}
	} else {
		err = t.AppBuilder.Load(0, extraJtagCmd)
	}

	return err
}

func Load(binBaseName string, bspPkg *pkg.BspPackage,
	extraEnvSettings map[string]string) error {

	if bspPkg.DownloadScript == "" {
		return util.FmtNewtError("No download script for BSP %s\n",
			bspPkg.Name())
	}

	bspPath := bspPkg.BasePath()
	downloadScript := filepath.Join(bspPath, bspPkg.DownloadScript)

	sortedKeys := make([]string, 0, len(extraEnvSettings))
	for k, _ := range extraEnvSettings {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	envSettings := ""
	for _, key := range sortedKeys {
		envSettings += fmt.Sprintf("%s=\"%s\" ", key, extraEnvSettings[key])
	}

	envSettings += fmt.Sprintf("BSP_PATH=\"%s\" ", bspPath)
	envSettings += fmt.Sprintf("BIN_BASENAME=\"%s\" ", binBaseName)

	// bspPath, binBaseName are passed in command line for backwards
	// compatibility
	downloadCmd := fmt.Sprintf("%s %s %s %s", envSettings, downloadScript,
		bspPath, binBaseName)

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Load command: %s\n",
		downloadCmd)
	rsp, err := util.ShellCommand(downloadCmd)
	if err != nil {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "%s", rsp)
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

	features := b.cfg.Features()

	if _, ok := features["BOOT_LOADER"]; ok {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Loading bootloader\n")
	} else {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Loading %s image into slot %d\n", b.buildName, imageSlot+1)
	}

	if err := Load(b.AppBinBasePath(), b.targetBuilder.bspPkg,
		envSettings); err != nil {

		return err
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE, "Successfully loaded image.\n")

	return nil
}

func (t *TargetBuilder) Debug(extraJtagCmd string, reset bool) error {
	//var additional_libs []string
	err := t.PrepBuild()

	if err != nil {
		return err
	}

	if t.LoaderBuilder == nil {
		return t.AppBuilder.Debug(extraJtagCmd, reset)
	}
	return t.LoaderBuilder.Debug(extraJtagCmd, reset)
}

func (b *Builder) Debug(extraJtagCmd string, reset bool) error {
	if b.appPkg == nil {
		return util.NewNewtError("app package not specified")
	}

	/*
	 * Populate the package list and feature sets.
	 */
	err := b.targetBuilder.PrepBuild()
	if err != nil {
		return err
	}

	bspPath := b.bspPkg.BasePath()
	binBaseName := b.AppBinBasePath()
	featureString := b.FeatureString()

	envSettings := []string{
		fmt.Sprintf("BSP_PATH=%s", bspPath),
		fmt.Sprintf("BIN_BASENAME=%s", binBaseName),
		fmt.Sprintf("FEATURES=\"%s\"", featureString),
	}
	if extraJtagCmd != "" {
		envSettings = append(envSettings,
			fmt.Sprintf("EXTRA_JTAG_CMD=%s", extraJtagCmd))
	}
	if reset == true {
		envSettings = append(envSettings, fmt.Sprintf("RESET=true"))
	}
	debugScript := filepath.Join(bspPath, b.targetBuilder.bspPkg.DebugScript)

	os.Chdir(project.GetProject().Path())

	// bspPath, binBaseName are passed in command line for backwards
	// compatibility
	cmdLine := []string{debugScript, bspPath, binBaseName}

	fmt.Printf("%s\n", cmdLine)
	return util.ShellInteractiveCommand(cmdLine, envSettings)
}
