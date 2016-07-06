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

	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

func (t *TargetBuilder) Load() error {

	err := t.PrepBuild()

	if err != nil {
		return err
	}

	if t.Loader != nil {
		err = t.App.Load(1)
		if err == nil {
			err = t.Loader.Load(0)
		}
	} else {
		err = t.App.Load(0)
	}

	return err
}

func (b *Builder) Load(image_slot int) error {
	if b.appPkg == nil {
		return util.NewNewtError("app package not specified")
	}

	/*
	 * Populate the package list and feature sets.
	 */
	err := b.target.PrepBuild()
	if err != nil {
		return err
	}

	if b.target.Bsp.DownloadScript == "" {
		/*
		 *
		 */
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"No download script for BSP %s\n", b.target.Bsp.Name())
		return nil
	}

	bspPath := b.target.Bsp.BasePath()
	downloadScript := filepath.Join(bspPath, b.target.Bsp.DownloadScript)
	binBaseName := b.AppBinBasePath()
	featureString := b.FeatureString()

	downloadCmd := fmt.Sprintf("%s %s %s %d %s",
		downloadScript, bspPath, binBaseName, image_slot, featureString)

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Loading %s image int slot %d\n", b.buildName, image_slot)
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

func (t *TargetBuilder) Debug() error {
	//var additional_libs []string
	err := t.PrepBuild()

	if err != nil {
		return err
	}

	//	if t.Loader != nil {
	//		basename := t.Loader.AppElfPath()
	//		name := strings.TrimSuffix(basename, filepath.Ext(basename))
	//		additional_libs = append(additional_libs, name)
	//	}

	//	return t.App.Debug(additional_libs)
	if t.Loader == nil {
		return t.App.Debug(nil)
	}
	return t.Loader.Debug(nil)
}

func (b *Builder) Debug(addlibs []string) error {
	if b.appPkg == nil {
		return util.NewNewtError("app package not specified")
	}

	/*
	 * Populate the package list and feature sets.
	 */
	err := b.target.PrepBuild()
	if err != nil {
		return err
	}

	bspPath := b.target.Bsp.BasePath()
	debugScript := filepath.Join(bspPath, b.target.Bsp.DebugScript)
	binBaseName := b.AppBinBasePath()

	os.Chdir(project.GetProject().Path())

	cmdLine := []string{debugScript, bspPath, binBaseName}
	cmdLine = append(cmdLine, addlibs...)

	fmt.Printf("%s\n", cmdLine)
	return util.ShellInteractiveCommand(cmdLine)
}
