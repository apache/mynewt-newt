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

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

const TARGET_TEST_NAME = "targets/unittest"

func buildRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, nil)
	}

	t := target.ResolveTargetName(args[0])
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("invalid target name: "+args[0]))
	}

	b, err := NewBuilder(t)
	if err != nil {
		fmt.Println(err)
	}
	err = b.Build()
	if err != nil {
		fmt.Println(err)
	}
}

func cleanRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, nil)
	}

	t := target.ResolveTargetName(args[0])
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("invalid target name"+args[0]))
	}

	b, err := NewBuilder(t)
	if err != nil {
		fmt.Println(err)
	}
	err = b.Clean()
	if err != nil {
		fmt.Println(err)
	}
}

func testRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, nil)
	}

	t := target.ResolveTargetName(TARGET_TEST_NAME)
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("can't find unit test target: "+
			TARGET_TEST_NAME))
	}

	b, err := NewBuilder(t)
	if err != nil {
		fmt.Println(err)
	}

	// Verify and resolve each specified package.
	packs := []*pkg.LocalPackage{}
	for _, pkgName := range args {
		dep, err := pkg.NewDependency(nil, pkgName)
		if err != nil {
			cli.NewtUsage(cmd, util.NewtErrorNoTrace(fmt.Sprintf("invalid "+
				"package name: %s (%s)", pkgName, err.Error())))
		}
		if dep == nil {
			cli.NewtUsage(cmd, util.NewtErrorNoTrace("invalid package name: "+
				pkgName))
		}
		pack := project.GetProject().ResolveDependency(dep)
		if pack == nil {
			cli.NewtUsage(cmd, util.NewtErrorNoTrace("unknown package: "+
				pkgName))
		}

		packs = append(packs, pack.(*pkg.LocalPackage))
	}

	// Test each package.
	for _, pack := range packs {
		err = b.Test(pack)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func downloadRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	t := target.ResolveTargetName(args[0])
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("Invalid target name"+args[0]))
	}

	b, err := NewBuilder(t)
	if err != nil {
		fmt.Println(err)
	}

	err = b.Download()
	if err != nil {
		fmt.Println(err)
	}
}

func debugRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	t := target.ResolveTargetName(args[0])
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("Invalid target name"+args[0]))
	}

	b, err := NewBuilder(t)
	if err != nil {
		fmt.Println(err)
	}

	err = b.Debug()
	if err != nil {
		fmt.Println(err)
	}
}

func sizeRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	t := target.ResolveTargetName(args[0])
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("Invalid target name"+args[0]))
	}

	b, err := NewBuilder(t)
	if err != nil {
		fmt.Println(err)
	}

	err = b.Size()
	if err != nil {
		fmt.Println(err)
	}
}

func AddCommands(cmd *cobra.Command) {
	buildHelpText := ""
	buildHelpEx := ""
	buildCmd := &cobra.Command{
		Use:     "build",
		Short:   "Command for building target",
		Long:    buildHelpText,
		Example: buildHelpEx,
		Run:     buildRunCmd,
	}

	cmd.AddCommand(buildCmd)

	cleanHelpText := ""
	cleanHelpEx := ""
	cleanCmd := &cobra.Command{
		Use:     "clean",
		Short:   "Command for cleaning target",
		Long:    cleanHelpText,
		Example: cleanHelpEx,
		Run:     cleanRunCmd,
	}

	cmd.AddCommand(cleanCmd)

	testHelpText := ""
	testHelpEx := ""
	testCmd := &cobra.Command{
		Use:     "test",
		Short:   "Executes unit tests for one or more packages",
		Long:    testHelpText,
		Example: testHelpEx,
		Run:     testRunCmd,
	}

	cmd.AddCommand(testCmd)

	downloadHelpText := "Download app image to target for <target-name>."
	downloadHelpEx := "  newt download <target-name>\n"

	downloadCmd := &cobra.Command{
		Use:     "download",
		Short:   "Download built target to board",
		Long:    downloadHelpText,
		Example: downloadHelpEx,
		Run:     downloadRunCmd,
	}
	cmd.AddCommand(downloadCmd)

	debugHelpText := "Open debugger session for <target-name>."
	debugHelpEx := "  newt debug <target-name>\n"

	debugCmd := &cobra.Command{
		Use:     "debug",
		Short:   "Open debugger session to target",
		Long:    debugHelpText,
		Example: debugHelpEx,
		Run:     debugRunCmd,
	}
	cmd.AddCommand(debugCmd)

	sizeHelpText := "Calculate the size of target components specified by " +
		"<target-name>."
	sizeHelpEx := "  newt size <target-name>\n"

	sizeCmd := &cobra.Command{
		Use:     "size",
		Short:   "Size of target components",
		Long:    sizeHelpText,
		Example: sizeHelpEx,
		Run:     sizeRunCmd,
	}
	cmd.AddCommand(sizeCmd)
}
