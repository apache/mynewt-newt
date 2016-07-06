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
	"os"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

const TARGET_TEST_NAME = "unittest"

func pkgIsTestable(pack *pkg.LocalPackage) bool {
	return util.NodeExist(pack.BasePath() + "/src/test")
}

func buildRunCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) < 1 {
		NewtUsage(cmd, nil)
	}

	// Verify that all target names are valid.
	_, err := ResolveTargets(args...)
	if err != nil {
		NewtUsage(cmd, err)
	}

	for _, targetName := range args {
		// Reset the global state for the next build.
		if err := ResetGlobalState(); err != nil {
			NewtUsage(nil, err)
		}

		// Lookup the target by name.  This has to be done a second time here
		// now that the project has been reset.
		t := ResolveTarget(targetName)
		if t == nil {
			NewtUsage(nil, util.NewNewtError("Failed to resolve target: "+
				targetName))
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT, "Building target %s\n",
			t.FullName())

		b, err := builder.NewTargetBuilder(t)
		if err != nil {
			NewtUsage(nil, err)
		}

		err = b.Build()
		if err != nil {
			NewtUsage(nil, err)
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT, "Target successfully built: "+
			"%s\n", targetName)

		/* TODO */
	}
}

func cleanRunCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	cleanAll := false
	targets := []*target.Target{}
	for _, arg := range args {
		if arg == TARGET_KEYWORD_ALL {
			cleanAll = true
		} else {
			t := ResolveTarget(arg)
			if t == nil {
				NewtUsage(cmd, util.NewNewtError("invalid target name: "+arg))
			}
			targets = append(targets, t)
		}
	}

	if cleanAll {
		path := builder.BinRoot()
		util.StatusMessage(util.VERBOSITY_VERBOSE,
			"Cleaning directory %s\n", path)

		err := os.RemoveAll(path)
		if err != nil {
			NewtUsage(cmd, err)
		}
	} else {
		for _, t := range targets {
			b, err := builder.NewTargetBuilder(t)
			if err != nil {
				NewtUsage(cmd, err)
			}
			err = b.Clean()
			if err != nil {
				NewtUsage(cmd, err)
			}
		}
	}
}

func testRunCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) < 1 {
		NewtUsage(cmd, nil)
	}

	// Verify and resolve each specified package.
	testAll := false
	packs := []*pkg.LocalPackage{}
	for _, pkgName := range args {
		if pkgName == "all" {
			testAll = true
		} else {
			pack, err := ResolvePackage(pkgName)
			if err != nil {
				NewtUsage(cmd, err)
			}

			if !pkgIsTestable(pack) {
				NewtUsage(nil, util.FmtNewtError("Package %s contains no "+
					"unit tests", pack.FullName()))
			}

			packs = append(packs, pack)
		}
	}

	if testAll {
		packs = []*pkg.LocalPackage{}
		for _, repoHash := range project.GetProject().PackageList() {
			for _, pack := range *repoHash {
				lclPack := pack.(*pkg.LocalPackage)

				if pkgIsTestable(lclPack) {
					packs = append(packs, lclPack)
				}
			}
		}
	}

	if len(packs) == 0 {
		NewtUsage(nil, util.NewNewtError("No testable packages found"))
	}

	passedPkgs := []*pkg.LocalPackage{}
	failedPkgs := []*pkg.LocalPackage{}
	for _, pack := range packs {
		// Reset the global state for the next test.
		if err := ResetGlobalState(); err != nil {
			NewtUsage(nil, err)
		}

		// Use the standard unit test target for all tests.
		t := ResolveTarget(TARGET_TEST_NAME)
		if t == nil {
			NewtUsage(nil, util.NewNewtError("Can't find unit test target: "+
				TARGET_TEST_NAME))
		}

		b, err := builder.NewTargetBuilder(t)
		if err != nil {
			NewtUsage(nil, err)
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT, "Testing package %s\n",
			pack.FullName())

		// The package under test needs to be resolved again now that the
		// project has been reset.
		newPack, err := ResolvePackage(pack.FullName())
		if err != nil {
			NewtUsage(nil, util.NewNewtError("Failed to resolve package: "+
				pack.Name()))
		}
		pack = newPack

		err = b.Test(pack)
		if err == nil {
			passedPkgs = append(passedPkgs, pack)
		} else {
			newtError := err.(*util.NewtError)
			util.StatusMessage(util.VERBOSITY_QUIET, newtError.Text)
			failedPkgs = append(failedPkgs, pack)
		}
	}

	passStr := fmt.Sprintf("Passed tests: [%s]", PackageNameList(passedPkgs))
	failStr := fmt.Sprintf("Failed tests: [%s]", PackageNameList(failedPkgs))

	if len(failedPkgs) > 0 {
		NewtUsage(nil, util.FmtNewtError("Test failure(s):\n%s\n%s", passStr,
			failStr))
	} else {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "%s\n", passStr)
		util.StatusMessage(util.VERBOSITY_DEFAULT, "All tests passed\n")
	}
}

func loadRunCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	t := ResolveTarget(args[0])
	if t == nil {
		NewtUsage(cmd, util.NewNewtError("Invalid target name: "+args[0]))
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		NewtUsage(cmd, err)
	}

	err = b.Load()
	if err != nil {
		NewtUsage(cmd, err)
	}
}

func debugRunCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	t := ResolveTarget(args[0])
	if t == nil {
		NewtUsage(cmd, util.NewNewtError("Invalid target name: "+args[0]))
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		NewtUsage(cmd, err)
	}

	err = b.Debug()
	if err != nil {
		NewtUsage(cmd, err)
	}
}

func sizeRunCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	t := ResolveTarget(args[0])
	if t == nil {
		NewtUsage(cmd, util.NewNewtError("Invalid target name: "+args[0]))
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		NewtUsage(cmd, err)
	}

	err = b.Size()
	if err != nil {
		NewtUsage(cmd, err)
	}
}

func AddBuildCommands(cmd *cobra.Command) {
	buildCmd := &cobra.Command{
		Use:   "build <target-name> [target-names...]",
		Short: "Builds one or more targets.",
		Run:   buildRunCmd,
	}

	cmd.AddCommand(buildCmd)

	cleanCmd := &cobra.Command{
		Use:   "clean <target-name> [target-names...] | all",
		Short: "Deletes build artifacts for one or more targets.",
		Run:   cleanRunCmd,
	}

	cmd.AddCommand(cleanCmd)

	testCmd := &cobra.Command{
		Use:   "test <package-name> [package-names...] | all",
		Short: "Executes unit tests for one or more packages",
		Run:   testRunCmd,
	}

	cmd.AddCommand(testCmd)

	loadHelpText := "Load app image to target for <target-name>."

	loadCmd := &cobra.Command{
		Use:   "load <target-name>",
		Short: "Load built target to board",
		Long:  loadHelpText,
		Run:   loadRunCmd,
	}
	cmd.AddCommand(loadCmd)

	debugHelpText := "Open debugger session for <target-name>."

	debugCmd := &cobra.Command{
		Use:   "debug <target-name>",
		Short: "Open debugger session to target",
		Long:  debugHelpText,
		Run:   debugRunCmd,
	}
	cmd.AddCommand(debugCmd)

	sizeHelpText := "Calculate the size of target components specified by " +
		"<target-name>."

	sizeCmd := &cobra.Command{
		Use:   "size <target-name>",
		Short: "Size of target components",
		Long:  sizeHelpText,
		Run:   sizeRunCmd,
	}
	cmd.AddCommand(sizeCmd)
}
