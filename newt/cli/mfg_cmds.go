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

package cli

import (
	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/newt/mfg"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

func resolveMfgPkg(pkgName string) (*pkg.LocalPackage, error) {
	proj, err := project.TryGetProject()
	if err != nil {
		return nil, err
	}

	lpkg, err := proj.ResolvePackage(proj.LocalRepo(), pkgName)
	if err != nil {
		var err2 error
		lpkg, err2 = proj.ResolvePackage(proj.LocalRepo(),
			MFG_DEFAULT_DIR+"/"+pkgName)
		if err2 != nil {
			return nil, err
		}
	}

	if lpkg.Type() != pkg.PACKAGE_TYPE_MFG {
		return nil, util.FmtNewtError(
			"Package \"%s\" has incorrect type; expected mfg, got %s",
			pkgName, pkg.PackageTypeNames[lpkg.Type()])
	}

	return lpkg, nil
}

func mfgCreate(mi *mfg.MfgImage) {
	binPath, sectionPaths, err := mi.CreateMfgImage()
	if err != nil {
		NewtUsage(nil, err)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Created manufacturing image: %s\n", binPath)

	for _, sectionPath := range sectionPaths {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Created manufacturing section: %s\n", sectionPath)
	}
}

func mfgLoad(mi *mfg.MfgImage) {
	binPath, err := mi.Upload()
	if err != nil {
		NewtUsage(nil, err)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Uploaded manufacturing image: %s\n", binPath)
}

func mfgCreateRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify mfg package name"))
	}

	pkgName := args[0]
	lpkg, err := resolveMfgPkg(pkgName)
	if err != nil {
		NewtUsage(cmd, err)
	}

	mi, err := mfg.Load(lpkg)
	if err != nil {
		NewtUsage(nil, err)
	}

	mfgCreate(mi)
}

func mfgLoadRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify mfg package name"))
	}

	pkgName := args[0]
	lpkg, err := resolveMfgPkg(pkgName)
	if err != nil {
		NewtUsage(cmd, err)
	}

	mi, err := mfg.Load(lpkg)
	if err != nil {
		NewtUsage(nil, err)
	}

	mfgLoad(mi)
}

func mfgDeployRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify mfg package name"))
	}

	pkgName := args[0]
	lpkg, err := resolveMfgPkg(pkgName)
	if err != nil {
		NewtUsage(cmd, err)
	}

	mi, err := mfg.Load(lpkg)
	if err != nil {
		NewtUsage(nil, err)
	}

	mfgCreate(mi)
	mfgLoad(mi)
}

func AddMfgCommands(cmd *cobra.Command) {
	mfgHelpText := ""
	mfgHelpEx := ""
	mfgCmd := &cobra.Command{
		Use:     "mfg",
		Short:   "Manufacturing flash image commands",
		Long:    mfgHelpText,
		Example: mfgHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	cmd.AddCommand(mfgCmd)

	mfgCreateHelpText := "TBD"
	mfgCreateCmd := &cobra.Command{
		Use:       "create <mfg-package-name>",
		Short:     "Create a manufacturing flash image",
		Long:      mfgCreateHelpText,
		Run:       mfgCreateRunCmd,
		ValidArgs: mfgList(),
	}
	mfgCmd.AddCommand(mfgCreateCmd)

	mfgLoadHelpText := "TBD"
	mfgLoadCmd := &cobra.Command{
		Use:       "load <mfg-package-name>",
		Short:     "Load a manufacturing flash image onto a device",
		Long:      mfgLoadHelpText,
		Run:       mfgLoadRunCmd,
		ValidArgs: mfgList(),
	}
	mfgCmd.AddCommand(mfgLoadCmd)

	mfgDeployHelpText := "TBD"
	mfgDeployCmd := &cobra.Command{
		Use:       "deploy <mfg-package-name>",
		Short:     "Builds and uploads a manufacturing image (build + load)",
		Long:      mfgDeployHelpText,
		Run:       mfgDeployRunCmd,
		ValidArgs: mfgList(),
	}
	mfgCmd.AddCommand(mfgDeployCmd)
}
