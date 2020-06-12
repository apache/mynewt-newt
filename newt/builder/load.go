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

func (t *TargetBuilder) loadLoader(slot int, extraJtagCmd string, imgFilename string) error {
	if err := t.bspPkg.Reload(t.LoaderBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	return t.LoaderBuilder.Load(slot, extraJtagCmd, imgFilename)
}

func (t *TargetBuilder) loadApp(slot int, extraJtagCmd string, imgFilename string) error {
	if err := t.bspPkg.Reload(t.AppBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	return t.AppBuilder.Load(slot, extraJtagCmd, imgFilename)
}

func (t *TargetBuilder) debugLoader(extraJtagCmd string, reset bool,
	noGDB bool, elfBase string) error {

	if err := t.bspPkg.Reload(t.LoaderBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	return t.LoaderBuilder.Debug(extraJtagCmd, reset, noGDB, elfBase)
}

func (t *TargetBuilder) debugApp(extraJtagCmd string, reset bool,
	noGDB bool, elfBase string) error {

	if err := t.bspPkg.Reload(t.AppBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	return t.AppBuilder.Debug(extraJtagCmd, reset, noGDB, elfBase)
}

// Load loads a .img file onto a device.  If imgFileOverride is not empty, it
// specifies the path of the image file to load.  If it is empty, the image in
// the target's `bin` directory is loaded.
func (t *TargetBuilder) Load(extraJtagCmd string, imgFileOverride string) error {
	err := t.PrepBuild()
	if err != nil {
		return err
	}

	if t.LoaderBuilder != nil && imgFileOverride != "" {
		return util.FmtNewtError(
			"cannot specify image file override for split images")
	}

	var imgBase string
	if imgFileOverride == "" {
		imgBase = t.AppBuilder.AppBinBasePath()
	} else {
		// The download script appends ".img" to the basename.  Make sure we
		// can strip the extension here and the script will reconstruct the
		// original filename.
		imgBase = strings.TrimSuffix(imgFileOverride, ".img")
		if imgBase == imgFileOverride {
			return util.FmtNewtError(
				"invalid img filename: must end in \".img\": filename=%s",
				imgFileOverride)
		}
	}

	if t.LoaderBuilder != nil {
		err = t.loadApp(1, extraJtagCmd, imgBase)
		if err == nil {
			err = t.loadLoader(0, extraJtagCmd, t.LoaderBuilder.AppBinBasePath())
		}
	} else {
		err = t.loadApp(0, extraJtagCmd, imgBase)
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

func (b *Builder) Load(imageSlot int, extraJtagCmd string, imgFilename string) error {
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
			"Loading bootloader (%s)\n", imgFilename)
	} else {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Loading %s image into slot %d (%s)\n", b.buildName, imageSlot+1, imgFilename)
	}

	// Convert the binary path from absolute to relative.  This is required for
	// compatibility with unix-in-windows environemnts (e.g., cygwin).
	binPath := util.TryRelPath(imgFilename)

	// Make sure the img override (if any) gets used.
	env["BIN_BASENAME"] = binPath

	if err := Load(binPath, b.targetBuilder.bspPkg, env); err != nil {
		return err
	}

	return nil
}

// Debug runs gdb on the .elf file corresponding to what is running on a
// device.  If elfFileOverride is not empty, it specifies the path of the .elf
// file to debug.  If it is empty, the .elf file in the target's `bin`
// directory is loaded.
func (t *TargetBuilder) Debug(extraJtagCmd string, reset bool, noGDB bool, elfFileOverride string) error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	var elfBase string // Everything except ".elf"

	if elfFileOverride != "" {
		// The debug script appends ".elf" to the basename.  Make sure we can strip
		// the extension here and the script will reconstruct the original
		// filename.
		elfBase = strings.TrimSuffix(elfFileOverride, ".elf")
		if elfBase == elfFileOverride {
			return util.FmtNewtError(
				"invalid elf filename: must end in \".elf\": filename=%s",
				elfFileOverride)
		}
	}

	if t.LoaderBuilder == nil {
		if elfBase == "" {
			elfBase = t.AppBuilder.AppBinBasePath()
		}
		return t.debugApp(extraJtagCmd, reset, noGDB, elfBase)
	} else {
		if elfBase == "" {
			elfBase = t.LoaderBuilder.AppBinBasePath()
		}
		return t.debugLoader(extraJtagCmd, reset, noGDB, elfBase)
	}
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

	// Make sure the elf override (if any) gets used.
	env["BIN_BASENAME"] = binPath

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

func (b *Builder) Debug(extraJtagCmd string, reset bool, noGDB bool, binBase string) error {
	return b.debugBin(binBase, extraJtagCmd, reset, noGDB)
}
