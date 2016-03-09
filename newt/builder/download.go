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

func (b *Builder) Download() error {
	if b.target.App() == nil {
		return util.NewNewtError("app package not specified")
	}

	/*
	 * Populate the package list and feature sets.
	 */
	err := b.PrepBuild()
	if err != nil {
		return err
	}

	bspPath := b.Bsp.BasePath()
	downloadScript := filepath.Join(bspPath, b.Bsp.DownloadScript)
	binBaseName := b.appBinBasePath()
	featureString := b.featureString()

	downloadCmd := fmt.Sprintf("%s %s %s %s",
		downloadScript, bspPath, binBaseName, featureString)

	rsp, err := util.ShellCommand(downloadCmd)
	fmt.Printf("%s", rsp)

	return err
}

func (b *Builder) Debug() error {
	if b.target.App() == nil {
		return util.NewNewtError("app package not specified")
	}

	/*
	 * Populate the package list and feature sets.
	 */
	err := b.PrepBuild()
	if err != nil {
		return err
	}

	bspPath := b.Bsp.BasePath()
	debugScript := filepath.Join(bspPath, b.Bsp.DebugScript)
	binBaseName := b.appBinBasePath()
	featureString := b.featureString()

	os.Chdir(project.GetProject().Path())

	cmdLine := []string{debugScript, bspPath, binBaseName, featureString}
	return util.ShellInteractiveCommand(cmdLine)
}
