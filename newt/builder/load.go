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
	"strings"
	"syscall"

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

func RunOptionalCheck(checkScript string, env map[string]string) error {
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

func Load(binBasePath string, bspPkg *pkg.BspPackage,
	extraEnvSettings map[string]string) error {

	if bspPkg.DownloadScript == "" {
		return nil
	}

	env := BasicEnvVars(binBasePath, bspPkg)
	for k, v := range extraEnvSettings {
		env[k] = v
	}

	RunOptionalCheck(bspPkg.OptChkScript, env)
	// bspPath, binBasePath are passed in command line for backwards
	// compatibility
	cmd := []string{
		bspPkg.DownloadScript,
		bspPkg.BasePath(),
		binBasePath,
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

	env, err := b.EnvVars(imageSlot)
	if err != nil {
		return err
	}

	if extraJtagCmd != "" {
		env["EXTRA_JTAG_CMD"] = extraJtagCmd
	}

	if _, ok := env["BOOT_LOADER"]; ok {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Loading bootloader\n")
	} else {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Loading %s image into slot %d\n", b.buildName, imageSlot+1)
	}

	// Convert the binary path from absolute to relative.  This is required for
	// compatibility with unix-in-windows environemnts (e.g., cygwin).
	binPath := util.TryRelPath(b.AppBinBasePath())
	if err := Load(binPath, b.targetBuilder.bspPkg, env); err != nil {
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
	binBasePath := binPath
	bspPkg := b.targetBuilder.bspPkg

	env, err := b.EnvVars(0)
	if err != nil {
		return err
	}

	if extraJtagCmd != "" {
		env["EXTRA_JTAG_CMD"] = extraJtagCmd
	}
	if reset == true {
		env["RESET"] = "true"
	}
	if noGDB == true {
		env["NO_GDB"] = "1"
	}

	os.Chdir(project.GetProject().Path())

	RunOptionalCheck(bspPkg.OptChkScript, env)
	// bspPath, binBasePath are passed in command line for backwards
	// compatibility
	cmdLine := []string{
		b.targetBuilder.bspPkg.DebugScript, bspPath, binBasePath,
	}

	fmt.Printf("%s\n", cmdLine)
	return util.ShellInteractiveCommand(cmdLine, env, false)
}

func (b *Builder) Debug(extraJtagCmd string, reset bool, noGDB bool) error {
	binPath, err := b.binBasePath()
	if err != nil {
		return err
	}

	return b.debugBin(binPath, extraJtagCmd, reset, noGDB)
}
