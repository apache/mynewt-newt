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
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

var NewTypeStr = "pkg"

func pkgNewCmd(cmd *cobra.Command, args []string) {

	if len(args) == 0 {
		NewtUsage(cmd, util.NewNewtError("Must specify a package name"))
	}

	if len(args) != 1 {
		NewtUsage(cmd, util.NewNewtError("Exactly one argument required"))
	}

	NewTypeStr = strings.ToUpper(NewTypeStr)

	pw := project.NewPackageWriter()
	if err := pw.ConfigurePackage(NewTypeStr, args[0]); err != nil {
		NewtUsage(cmd, err)
	}
	if err := pw.WritePackage(); err != nil {
		NewtUsage(cmd, err)
	}
}

type dirOperation func(string, string) error

func pkgCopyCmd(cmd *cobra.Command, args []string) {
	pkgCloneOrMoveCmd(cmd, args, util.CopyDir, "Copying")
}

func pkgMoveCmd(cmd *cobra.Command, args []string) {
	pkgCloneOrMoveCmd(cmd, args, util.MoveDir, "Moving")
}

func pkgCloneOrMoveCmd(cmd *cobra.Command, args []string, dirOpFn dirOperation, opStr string) {
	if len(args) != 2 {
		NewtUsage(cmd, util.NewNewtError("Exactly two arguments required to pkg move"))
	}

	srcLoc := args[0]
	dstLoc := args[1]

	proj := TryGetProject()
	interfaces.SetProject(proj)

	wd, err := os.Getwd()
	if err != nil {
		NewtUsage(cmd, util.ChildNewtError(err))
	}

	if err := os.Chdir(proj.Path() + "/"); err != nil {
		NewtUsage(cmd, util.ChildNewtError(err))
	}

	defer os.Chdir(wd)

	/* Find source package, defaulting search to the local project if no
	 * repository descriptor is found.
	 */
	srcRepoName, srcName, err := newtutil.ParsePackageString(srcLoc)
	if err != nil {
		NewtUsage(cmd, err)
	}

	srcRepo := proj.LocalRepo()
	if srcRepoName != "" {
		srcRepo = proj.FindRepo(srcRepoName)
	}

	srcPkg, err := proj.ResolvePackage(srcRepo, srcName)
	if err != nil {
		NewtUsage(cmd, err)
	}

	/* Resolve the destination package to a physical location, and then
	 * move the source package to that location.
	 * dstLoc is assumed to be in the format "@repo/pkg/loc"
	 */
	repoName, pkgName, err := newtutil.ParsePackageString(dstLoc)
	if err != nil {
		NewtUsage(cmd, err)
	}

	dstPath := proj.Path() + "/"
	repo := proj.LocalRepo()
	if repoName != "" {
		dstPath += "repos/" + repoName + "/"
		repo = proj.FindRepo(repoName)
		if repo == nil {
			NewtUsage(cmd, util.NewNewtError("Destination repo "+
				repoName+" does not exist"))
		}
	}
	dstPath += pkgName + "/"

	if util.NodeExist(dstPath) {
		NewtUsage(cmd, util.NewNewtError("Cannot overwrite existing package, "+
			"use pkg delete first"))
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "%s package %s to %s\n",
		opStr, srcLoc, dstLoc)

	if err := dirOpFn(srcPkg.BasePath(), dstPath); err != nil {
		NewtUsage(cmd, err)
	}

	/* Replace the package name in the pkg.yml file */
	pkgData, err := ioutil.ReadFile(dstPath + "/pkg.yml")
	if err != nil {
		NewtUsage(cmd, err)
	}

	re := regexp.MustCompile(regexp.QuoteMeta(srcName))
	res := re.ReplaceAllString(string(pkgData), pkgName)

	if err := ioutil.WriteFile(dstPath+"/pkg.yml", []byte(res), 0666); err != nil {
		NewtUsage(cmd, util.ChildNewtError(err))
	}

	/* If the last element of the package path changes, rename the include
	 * directory.
	 */
	if path.Base(pkgName) != path.Base(srcPkg.Name()) {
		dirOpFn(dstPath+"/include/"+path.Base(srcPkg.Name()),
			dstPath+"/include/"+path.Base(pkgName))
	}
}

func pkgRemoveCmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify a package name to delete"))
	}

	proj := TryGetProject()
	interfaces.SetProject(proj)

	wd, err := os.Getwd()
	if err != nil {
		NewtUsage(cmd, util.ChildNewtError(err))
	}

	if err := os.Chdir(proj.Path() + "/"); err != nil {
		NewtUsage(cmd, util.ChildNewtError(err))
	}

	defer os.Chdir(wd)

	/* Resolve package, and get path from package to ensure we're being asked
	 * to remove a valid path.
	 */
	repoName, pkgName, err := newtutil.ParsePackageString(args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	repo := proj.LocalRepo()
	if repoName != "" {
		repo = proj.FindRepo(repoName)
		if repo == nil {
			NewtUsage(cmd, util.NewNewtError("Destination repo "+
				repoName+" does not exist"))
		}
	}

	pkg, err := pkg.LoadLocalPackage(repo, pkgName)
	if err != nil {
		NewtUsage(cmd, err)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Removing package %s\n",
		args[0])

	if err := os.RemoveAll(pkg.BasePath()); err != nil {
		NewtUsage(cmd, util.ChildNewtError(err))
	}
}

func AddPackageCommands(cmd *cobra.Command) {
	/* Add the base package command, on top of which other commands are
	 * keyed
	 */
	pkgHelpText := "Commands for creating and manipulating packages"
	pkgHelpEx := "  newt pkg new --type=pkg sys/mylib"

	pkgCmd := &cobra.Command{
		Use:     "pkg",
		Short:   "Create and manage packages in the current workspace",
		Long:    pkgHelpText,
		Example: pkgHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cmd.AddCommand(pkgCmd)

	/* Package new command, create a new package */
	newCmdHelpText := ""
	newCmdHelpEx := ""

	newCmd := &cobra.Command{
		Use:     "new <package-name>",
		Short:   "Create a new package in the current directory, from a template",
		Long:    newCmdHelpText,
		Example: newCmdHelpEx,
		Run:     pkgNewCmd,
	}

	newCmd.PersistentFlags().StringVarP(&NewTypeStr, "type", "t",
		"lib", "Type of package to create: app, bsp, lib, sdk, unittest.")

	pkgCmd.AddCommand(newCmd)

	copyCmdHelpText := "Create a new package <dst-pkg> by cloning <src-pkg>"
	copyCmdHelpEx := "  newt pkg copy apps/blinky apps/myapp"

	copyCmd := &cobra.Command{
		Use:     "copy <src-pkg> <dst-pkg>",
		Short:   "Copy an existing package into another",
		Long:    copyCmdHelpText,
		Example: copyCmdHelpEx,
		Run:     pkgCopyCmd,
	}

	pkgCmd.AddCommand(copyCmd)

	moveCmdHelpText := ""
	moveCmdHelpEx := "  newt pkg move apps/blinky apps/myapp"

	moveCmd := &cobra.Command{
		Use:     "move <oldpkg> <newpkg>",
		Short:   "Move a package from one location to another",
		Long:    moveCmdHelpText,
		Example: moveCmdHelpEx,
		Run:     pkgMoveCmd,
	}

	pkgCmd.AddCommand(moveCmd)

	removeCmdHelpText := ""
	removeCmdHelpEx := ""

	removeCmd := &cobra.Command{
		Use:     "remove <package-name>",
		Short:   "Remove a package",
		Long:    removeCmdHelpText,
		Example: removeCmdHelpEx,
		Run:     pkgRemoveCmd,
	}

	pkgCmd.AddCommand(removeCmd)
}
