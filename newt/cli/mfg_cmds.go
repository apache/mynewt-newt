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
	"fmt"

	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/newt/mfg"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/util"
)

func ResolveMfgPkg(pkgName string) (*pkg.LocalPackage, error) {
	proj := TryGetProject()

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

func mfgCreate(me mfg.MfgEmitter) {
	srcPaths, dstPaths, err := me.Emit()
	if err != nil {
		NewtUsage(nil, err)
	}

	srcStr := ""
	dstStr := ""

	for _, p := range srcPaths {
		srcStr += fmt.Sprintf("    %s\n", p)
	}

	for _, p := range dstPaths {
		dstStr += fmt.Sprintf("    %s\n", p)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Creating a manufacturing image from the following files:\n%s\n",
		srcStr)

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Generated the following files:\n%s\n",
		dstStr)
}

func mfgLoad(basePkg *pkg.LocalPackage) {
	binPath, err := mfg.Upload(basePkg)
	if err != nil {
		NewtUsage(nil, err)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Uploaded manufacturing image: %s\n", binPath)
}

func mfgCreateRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		NewtUsage(cmd, util.NewNewtError(
			"Must specify mfg package name and version number"))
	}

	pkgName := args[0]
	lpkg, err := ResolveMfgPkg(pkgName)
	if err != nil {
		NewtUsage(cmd, err)
	}

	versStr := args[1]
	ver, err := image.ParseVersion(versStr)
	if err != nil {
		NewtUsage(cmd, err)
	}

	keys, _, err := parseKeyArgs(args[2:])
	if err != nil {
		NewtUsage(nil, err)
	}

	me, err := mfg.LoadMfgEmitter(lpkg, ver, keys)
	if err != nil {
		NewtUsage(nil, err)
	}

	mfgCreate(me)
}

func mfgLoadRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify mfg package name"))
	}

	pkgName := args[0]
	lpkg, err := ResolveMfgPkg(pkgName)
	if err != nil {
		NewtUsage(cmd, err)
	}

	mfgLoad(lpkg)
}

func mfgDeployRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify mfg package name"))
	}

	pkgName := args[0]
	lpkg, err := ResolveMfgPkg(pkgName)
	if err != nil {
		NewtUsage(cmd, err)
	}

	ver := image.ImageVersion{}
	if len(args) >= 2 {
		versStr := args[1]
		ver, err = image.ParseVersion(versStr)
		if err != nil {
			NewtUsage(cmd, err)
		}
	}

	me, err := mfg.LoadMfgEmitter(lpkg, ver, nil)
	if err != nil {
		NewtUsage(nil, err)
	}

	mfgCreate(me)

	mfgLoad(lpkg)
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

	mfgCreateCmd := &cobra.Command{
		Use: "create <mfg-package-name> <version #.#.#.#> [signing-key-1] " +
			"[signing-key-2] [...]",
		Short: "Create a manufacturing flash image",
		Run:   mfgCreateRunCmd,
	}
	mfgCmd.AddCommand(mfgCreateCmd)
	AddTabCompleteFn(mfgCreateCmd, mfgList)

	mfgLoadCmd := &cobra.Command{
		Use:   "load <mfg-package-name>",
		Short: "Load a manufacturing flash image onto a device",
		Run:   mfgLoadRunCmd,
	}
	mfgCmd.AddCommand(mfgLoadCmd)
	AddTabCompleteFn(mfgLoadCmd, mfgList)

	mfgDeployCmd := &cobra.Command{
		Use:   "deploy <mfg-package-name> [version #.#.#.#]",
		Short: "Build and upload a manufacturing image (create + load)",
		Run:   mfgDeployRunCmd,
	}
	mfgCmd.AddCommand(mfgDeployCmd)
	AddTabCompleteFn(mfgDeployCmd, mfgList)
}
