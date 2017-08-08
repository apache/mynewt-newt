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
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

const TARGET_TEST_NAME = "unittest"

var testablePkgMap map[*pkg.LocalPackage]struct{}

func testablePkgs() map[*pkg.LocalPackage]struct{} {
	if testablePkgMap != nil {
		return testablePkgMap
	}

	testablePkgMap := map[*pkg.LocalPackage]struct{}{}

	// Create a map of path => lclPkg.
	proj, err := project.TryGetProject()
	if err != nil {
		return nil
	}

	allPkgs := proj.PackagesOfType(-1)
	pathLpkgMap := make(map[string]*pkg.LocalPackage, len(allPkgs))
	for _, p := range allPkgs {
		lpkg := p.(*pkg.LocalPackage)
		pathLpkgMap[lpkg.BasePath()] = lpkg
	}

	// Add all unit test packages to the testable package map.
	testPkgs := proj.PackagesOfType(pkg.PACKAGE_TYPE_UNITTEST)
	for _, p := range testPkgs {
		lclPack := p.(*pkg.LocalPackage)
		testablePkgMap[lclPack] = struct{}{}
	}

	// Next add first ancestor of each test package.
	for _, testPkgItf := range testPkgs {
		testPkg := testPkgItf.(*pkg.LocalPackage)
		for cur := filepath.ToSlash(filepath.Dir(testPkg.BasePath())); cur != proj.BasePath; cur = filepath.ToSlash(filepath.Dir(cur)) {
			lpkg := pathLpkgMap[cur]
			if lpkg != nil && lpkg.Type() != pkg.PACKAGE_TYPE_UNITTEST {
				testablePkgMap[lpkg] = struct{}{}
				break
			}
		}
	}

	return testablePkgMap
}

func pkgToUnitTests(pack *pkg.LocalPackage) []*pkg.LocalPackage {
	// If the user specified a unittest package, just test that one.
	if pack.Type() == pkg.PACKAGE_TYPE_UNITTEST {
		return []*pkg.LocalPackage{pack}
	}

	// Otherwise, return all the package's direct descendants that are unit
	// test packages.
	result := []*pkg.LocalPackage{}
	srcPath := pack.BasePath()
	for p, _ := range testablePkgs() {
		dirPath := filepath.ToSlash(filepath.Dir(p.BasePath()))
		if p.Type() == pkg.PACKAGE_TYPE_UNITTEST &&
			dirPath == srcPath {

			result = append(result, p)
		}
	}

	return result
}

var extraJtagCmd string
var noGDB_flag bool

func buildRunCmd(cmd *cobra.Command, args []string, printShellCmds bool, executeShell bool) {
	if len(args) < 1 {
		NewtUsage(cmd, nil)
	}

	util.PrintShellCmds = printShellCmds
	util.ExecuteShell = executeShell

	TryGetProject()

	// Verify and resolve each specified package.
	targets, all, err := ResolveTargetsOrAll(args...)
	if err != nil {
		NewtUsage(cmd, err)
	}

	if all {
		// Collect all targets that specify an app package.
		targets = []*target.Target{}
		for _, name := range targetList() {
			t := ResolveTarget(name)
			if t != nil && t.AppName != "" {
				targets = append(targets, t)
			}
		}
	}

	for i, _ := range targets {
		// Reset the global state for the next build.
		// XXX: It is not good that this is necessary.  This is certainly going
		// to bite us...
		if i > 0 {
			if err := ResetGlobalState(); err != nil {
				NewtUsage(nil, err)
			}
		}

		// Look up the target by name.  This has to be done a second time here
		// now that the project has been reset.
		t := ResolveTarget(targets[i].FullName())
		if t == nil {
			NewtUsage(nil, util.NewNewtError("Failed to resolve target: "+
				targets[i].Name()))
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT, "Building target %s\n",
			t.FullName())

		b, err := builder.NewTargetBuilder(t)
		if err != nil {
			NewtUsage(nil, err)
		}

		if err := b.Build(); err != nil {
			NewtUsage(nil, err)
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Target successfully built: %s\n", t.Name())
	}
}

func cleanDir(path string) {
	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Cleaning directory %s\n", path)

	err := os.RemoveAll(path)
	if err != nil {
		NewtUsage(nil, util.NewNewtError(err.Error()))
	}
}

func cleanRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	TryGetProject()

	cleanAll := false
	targets := []*target.Target{}
	for _, arg := range args {
		if arg == TARGET_KEYWORD_ALL {
			cleanAll = true
		} else {
			t, _, err := ResolveTargetOrUnittest(arg)
			if err != nil {
				NewtUsage(cmd, err)
			}
			targets = append(targets, t)
		}
	}

	if cleanAll {
		cleanDir(builder.BinRoot())
	} else {
		for _, t := range targets {
			cleanDir(builder.TargetBinDir(t.Name()))
		}
	}
}

func pkgnames(pkgs []*pkg.LocalPackage) string {
	s := ""

	for _, p := range pkgs {
		s += p.Name() + " "
	}

	return s
}

func testRunCmd(cmd *cobra.Command, args []string, exclude string, executeShell bool) {
	if len(args) < 1 {
		NewtUsage(cmd, nil)
	}

	util.ExecuteShell = executeShell

	proj := TryGetProject()

	// Verify and resolve each specified package.
	testAll := false
	packs := []*pkg.LocalPackage{}
	for _, pkgName := range args {
		if pkgName == "all" {
			testAll = true
		} else {
			pack, err := proj.ResolvePackage(proj.LocalRepo(), pkgName)
			if err != nil {
				NewtUsage(cmd, err)
			}

			testPkgs := pkgToUnitTests(pack)
			if len(testPkgs) == 0 {
				NewtUsage(nil, util.FmtNewtError("Package %s contains no "+
					"unit tests", pack.FullName()))
			}

			packs = append(packs, testPkgs...)
		}
	}

	if testAll {
		packItfs := proj.PackagesOfType(pkg.PACKAGE_TYPE_UNITTEST)
		packs = make([]*pkg.LocalPackage, len(packItfs))
		for i, p := range packItfs {
			packs[i] = p.(*pkg.LocalPackage)
		}

		packs = pkg.SortLclPkgs(packs)
	}

	if len(exclude) > 0 {
		// filter out excluded tests
		orig := packs
		packs = packs[:0]
		excls := strings.Split(exclude, ",")
	packLoop:
		for _, pack := range orig {
			for _, excl := range excls {
				if pack.Name() == excl || strings.HasPrefix(pack.Name(), excl+"/") {
					continue packLoop
				}
			}
			packs = append(packs, pack)
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

		t, err := ResolveUnittest(pack.Name())
		if err != nil {
			NewtUsage(nil, err)
		}

		b, err := builder.NewTargetTester(t, pack)
		if err != nil {
			NewtUsage(nil, err)
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT, "Testing package %s\n",
			pack.FullName())

		err = b.SelfTestExecute()
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
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	TryGetProject()

	t := ResolveTarget(args[0])
	if t == nil {
		NewtUsage(cmd, util.NewNewtError("Invalid target name: "+args[0]))
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		NewtUsage(nil, err)
	}

	if err := b.Load(extraJtagCmd); err != nil {
		NewtUsage(cmd, err)
	}
}

func debugRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	TryGetProject()

	t := ResolveTarget(args[0])
	if t == nil {
		NewtUsage(cmd, util.NewNewtError("Invalid target name: "+args[0]))
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		NewtUsage(nil, err)
	}

	if err := b.Debug(extraJtagCmd, false, noGDB_flag); err != nil {
		NewtUsage(cmd, err)
	}
}

func sizeRunCmd(cmd *cobra.Command, args []string, ram bool, flash bool) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	TryGetProject()

	t := ResolveTarget(args[0])
	if t == nil {
		NewtUsage(cmd, util.NewNewtError("Invalid target name: "+args[0]))
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		NewtUsage(nil, err)
	}

	if ram || flash {
		if err := b.SizeReport(ram, flash); err != nil {
			NewtUsage(cmd, err)
		}
		return
	}

	if err := b.Size(); err != nil {
		NewtUsage(cmd, err)
	}
}

func AddBuildCommands(cmd *cobra.Command) {
	var printShellCmds bool
	var executeShell bool

	buildCmd := &cobra.Command{
		Use:   "build <target-name> [target-names...]",
		Short: "Build one or more targets",
		Run: func(cmd *cobra.Command, args []string) {
			buildRunCmd(cmd, args, printShellCmds, executeShell)
		},
	}

	buildCmd.Flags().BoolVarP(&printShellCmds, "printCmds", "p", false,
		"Print executed build commands")

	buildCmd.Flags().BoolVar(&executeShell, "executeShell", false,
		"Execute build command using /bin/sh (Linux and MacOS only)")

	cmd.AddCommand(buildCmd)
	AddTabCompleteFn(buildCmd, func() []string {
		return append(targetList(), "all")
	})

	cleanCmd := &cobra.Command{
		Use:   "clean <target-name> [target-names...] | all",
		Short: "Delete build artifacts for one or more targets",
		Run:   cleanRunCmd,
	}

	cmd.AddCommand(cleanCmd)
	AddTabCompleteFn(cleanCmd, func() []string {
		return append(append(targetList(), unittestList()...), "all")
	})

	var exclude string
	testCmd := &cobra.Command{
		Use:   "test <package-name> [package-names...] | all",
		Short: "Executes unit tests for one or more packages",
		Run: func(cmd *cobra.Command, args []string) {
			testRunCmd(cmd, args, exclude, executeShell)
		},
	}
	testCmd.Flags().StringVarP(&exclude, "exclude", "e", "", "Comma separated list of packages to exclude")
	testCmd.Flags().BoolVar(&executeShell, "executeShell", false,
		"Execute build command using /bin/sh (Linux and MacOS only)")
	cmd.AddCommand(testCmd)
	AddTabCompleteFn(testCmd, func() []string {
		return append(testablePkgList(), "all", "allexcept")
	})

	loadHelpText := "Load application image on to the board for <target-name>"

	loadCmd := &cobra.Command{
		Use:   "load <target-name>",
		Short: "Load built target to board",
		Long:  loadHelpText,
		Run:   loadRunCmd,
	}

	cmd.AddCommand(loadCmd)
	AddTabCompleteFn(loadCmd, targetList)

	loadCmd.PersistentFlags().StringVarP(&extraJtagCmd, "extrajtagcmd", "", "",
		"Extra commands to send to JTAG software")

	debugHelpText := "Open a debugger session for <target-name>"

	debugCmd := &cobra.Command{
		Use:   "debug <target-name>",
		Short: "Open debugger session to target",
		Long:  debugHelpText,
		Run:   debugRunCmd,
	}

	debugCmd.PersistentFlags().StringVarP(&extraJtagCmd, "extrajtagcmd", "",
		"", "Extra commands to send to JTAG software")
	debugCmd.PersistentFlags().BoolVarP(&noGDB_flag, "noGDB", "n", false,
		"Do not start GDB from command line")

	cmd.AddCommand(debugCmd)
	AddTabCompleteFn(debugCmd, targetList)

	sizeHelpText := "Calculate the size of target components specified by " +
		"<target-name>."

	var ram, flash bool
	sizeCmd := &cobra.Command{
		Use:   "size <target-name>",
		Short: "Size of target components",
		Long:  sizeHelpText,
		Run: func(cmd *cobra.Command, args []string) {
			sizeRunCmd(cmd, args, ram, flash)
		},
	}

	sizeCmd.Flags().BoolVarP(&ram, "ram", "R", false, "Print RAM statistics")
	sizeCmd.Flags().BoolVarP(&flash, "flash", "F", false,
		"Print FLASH statistics")

	cmd.AddCommand(sizeCmd)
	AddTabCompleteFn(sizeCmd, targetList)
}
