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
	NewTypeStr = strings.ToUpper(NewTypeStr)

	pw := project.NewPackageWriter()
	if err := pw.ConfigurePackage(NewTypeStr, args[0]); err != nil {
		NewtUsage(cmd, err)
	}
	if err := pw.WritePackage(); err != nil {
		NewtUsage(cmd, err)
	}
}

func pkgMoveCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		NewtUsage(cmd, util.NewNewtError("Exactly two arguments required to pkg move"))
	}

	srcLoc := args[0]
	dstLoc := args[1]

	proj := TryGetProject()
	interfaces.SetProject(proj)

	wd, err := os.Getwd()
	if err != nil {
		NewtUsage(cmd, util.NewNewtError(err.Error()))
	}

	if err := os.Chdir(proj.Path() + "/"); err != nil {
		NewtUsage(cmd, util.NewNewtError(err.Error()))
	}

	/* Find source package, defaulting search to the local project if no
	 * repository descriptor is found.
	 */
	srcRepoName, srcName, err := newtutil.ParsePackageString(srcLoc)
	if err != nil {
		os.Chdir(wd)
		NewtUsage(cmd, err)
	}

	srcRepo := proj.LocalRepo()
	if srcRepoName != "" {
		srcRepo = proj.FindRepo(srcRepoName)
	}

	srcPkg, err := proj.ResolvePackage(srcRepo, srcName)
	if err != nil {
		os.Chdir(wd)
		NewtUsage(cmd, err)
	}

	/* Resolve the destination package to a physical location, and then
	 * move the source package to that location.
	 * dstLoc is assumed to be in the format "@repo/pkg/loc"
	 */
	repoName, pkgName, err := newtutil.ParsePackageString(dstLoc)
	if err != nil {
		os.Chdir(wd)
		NewtUsage(cmd, err)
	}

	dstPath := proj.Path() + "/"
	repo := proj.LocalRepo()
	if repoName != "" {
		dstPath += "repos/" + repoName + "/"
		repo = proj.FindRepo(repoName)
		if repo == nil {
			os.Chdir(wd)
			NewtUsage(cmd, util.NewNewtError("Destination repo "+
				repoName+" does not exist"))
		}
	}
	dstPath += pkgName + "/"

	if util.NodeExist(dstPath) {
		os.Chdir(wd)
		NewtUsage(cmd, util.NewNewtError("Cannot overwrite existing package, "+
			"use pkg delete first"))
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Moving package %s to %s\n",
		srcLoc, dstLoc)

	if err := util.MoveDir(srcPkg.BasePath(), dstPath); err != nil {
		os.Chdir(wd)
		NewtUsage(cmd, err)
	}

	/* Replace the package name in the pkg.yml file */
	pkgData, err := ioutil.ReadFile(dstPath + "/pkg.yml")
	if err != nil {
		os.Chdir(wd)
		NewtUsage(cmd, err)
	}

	re := regexp.MustCompile(regexp.QuoteMeta(srcName))
	res := re.ReplaceAllString(string(pkgData), pkgName)

	if err := ioutil.WriteFile(dstPath+"/pkg.yml", []byte(res), 0666); err != nil {
		NewtUsage(cmd, util.ChildNewtError(err))
	}

	dstPkg, err := pkg.LoadLocalPackage(repo, pkgName)
	if err != nil {
		os.Chdir(wd)
		NewtUsage(cmd, err)
	}

	dstPkg.SetName(pkgName)
	dstPkg.Save()

	/* If the last element of the package path changes, rename the include
	 * directory.
	 */
	if path.Base(pkgName) != path.Base(srcPkg.Name()) {
		util.MoveDir(dstPath+"/include/"+path.Base(srcPkg.Name()),
			dstPath+"/include/"+path.Base(pkgName))
	}

	os.Chdir(wd)
}

func AddPackageCommands(cmd *cobra.Command) {
	/* Add the base package command, on top of which other commands are
	 * keyed
	 */
	pkgHelpText := "Commands for creating and manipulating packages"
	pkgHelpEx := "newt pkg new --type=pkg libs/mylib"

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
		Use:     "new",
		Short:   "Create a new package, from a template",
		Long:    newCmdHelpText,
		Example: newCmdHelpEx,
		Run:     pkgNewCmd,
	}

	newCmd.PersistentFlags().StringVarP(&NewTypeStr, "type", "t",
		"pkg", "Type of package to create: pkg, bsp, sdk.  Default pkg.")

	pkgCmd.AddCommand(newCmd)

	moveCmdHelpText := ""
	moveCmdHelpEx := ""

	moveCmd := &cobra.Command{
		Use:     "move",
		Short:   "Move a package from one location to another",
		Long:    moveCmdHelpText,
		Example: moveCmdHelpEx,
		Run:     pkgMoveCmd,
	}

	pkgCmd.AddCommand(moveCmd)
}
