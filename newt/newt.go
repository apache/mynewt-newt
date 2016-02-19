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

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newt/cli"
	"github.com/spf13/cobra"
)

var ExitOnFailure bool = false
var ExportAll bool = false
var ImportAll bool = false
var NewtVersion string = "0.8.0"
var NewtLogLevel string = ""
var NewtRepo *cli.Repo
var newtSilent bool
var newtQuiet bool
var newtVerbose bool
var AutoTargets string = "autotargets"
var NewtBranchPkgList string
var NewtBranchPkg string

func NewtUsage(cmd *cobra.Command, err error) {
	if err != nil {
		sErr := err.(*cli.NewtError)
		log.Printf("[DEBUG] %s", sErr.StackTrace)
		fmt.Fprintf(os.Stderr, "Error: %s\n", sErr.Text)
	}

	if cmd != nil {
		cmd.Help()
	}
	os.Exit(1)
}

// Display help text with a max line width of 79 characters
func formatHelp(text string) string {
	// first compress all new lines and extra spaces
	words := regexp.MustCompile("\\s+").Split(text, -1)
	linelen := 0
	fmtText := ""
	for _, word := range words {
		word = strings.Trim(word, "\n ") + " "
		tmplen := linelen + len(word)
		if tmplen >= 80 {
			fmtText += "\n"
			linelen = 0
		}
		fmtText += word
		linelen += len(word)
	}
	return fmtText
}

// Extracts "<key>=<value>" strings from the supplied slice and inserts them
// into the specified target's variable map.
func extractTargetVars(args []string, t *cli.Target) error {
	for i := 0; i < len(args); i++ {
		pair := strings.SplitN(args[i], "=", 2)
		if len(pair) != 2 {
			return cli.NewNewtError("invalid argument: " + args[i])
		}

		t.Vars[pair[0]] = pair[1]
	}

	return nil
}

func targetSetCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify two arguments (sect & k=v) to set"))
	}

	t, err := cli.LoadTarget(NewtRepo, args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}
	ar := strings.SplitN(args[1], "=", 2)

	t.Vars[ar[0]] = ar[1]

	if err := t.Save(); err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Target %s successfully set %s to %s\n", args[0], ar[0], ar[1])
}

func targetUnsetCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify two arguments (sect & k) to unset"))
	}

	t, err := cli.LoadTarget(NewtRepo, args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := t.DeleteVar(args[1]); err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Target %s successfully unset %s\n", args[0], args[1])
}

// Type for sorting an array of target pointers alphabetically by name.
type ByName []*cli.Target

func (a ByName) Len() int           { return len(a) }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool { return a[i].Vars["name"] < a[j].Vars["name"] }

func targetShowCmd(cmd *cobra.Command, args []string) {
	dispSect := ""
	if len(args) == 1 {
		dispSect = args[0]
	}

	targets, err := cli.GetTargets(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}

	sort.Sort(ByName(targets))

	for _, target := range targets {
		if dispSect == "" || dispSect == target.Vars["name"] {
			cli.StatusMessage(cli.VERBOSITY_QUIET, target.Vars["name"]+"\n")

			vars := target.GetVars()
			var keys []string
			for k := range vars {
				keys = append(keys, k)
			}

			sort.Strings(keys)
			for _, k := range keys {
				cli.StatusMessage(cli.VERBOSITY_QUIET, "	%s=%s\n", k, vars[k])
			}
		}
	}
}

func targetCreateCmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		NewtUsage(cmd, cli.NewNewtError("Wrong number of args to create cmd."))
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Creating target "+args[0]+"\n")

	if cli.TargetExists(NewtRepo, args[0]) {
		NewtUsage(cmd, cli.NewNewtError(
			"Target already exists, cannot create target with same name."))
	}

	target := &cli.Target{
		Repo: NewtRepo,
		Vars: map[string]string{},
	}
	target.Vars["name"] = args[0]

	err := target.Save()
	if err != nil {
		NewtUsage(nil, err)
	} else {
		cli.StatusMessage(cli.VERBOSITY_DEFAULT,
			"Target %s successfully created!\n", args[0])
	}
}

func targetBuildCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target to build"))
	}

	t, err := cli.LoadTarget(NewtRepo, args[0])
	if err != nil {
		NewtUsage(nil, err)
	}

	if len(args) > 1 && args[1] == "clean" {
		if len(args) > 2 && args[2] == "all" {
			err = t.BuildClean(true)
		} else {
			err = t.BuildClean(false)
		}
		if err != nil {
			NewtUsage(nil, err)
		}
	} else {
		// Parse any remaining key-value pairs and insert them into the target.
		err = extractTargetVars(args[1:], t)
		if err != nil {
			NewtUsage(nil, err)
		}

		err = t.Build()
		if err != nil {
			NewtUsage(nil, err)
		}
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Successfully run!\n")
}

func targetDelCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target to delete"))
	}

	t, err := cli.LoadTarget(NewtRepo, args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	// Clean target prior to deletion; ignore errors during clean.
	t.BuildClean(false)

	if err := t.Remove(); err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Target %s successfully removed\n", args[0])
}

func targetTestCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target to build"))
	}

	t, err := cli.LoadTarget(NewtRepo, args[0])
	if err != nil {
		NewtUsage(nil, err)
	}

	if len(args) > 1 && args[1] == "clean" {
		if len(args) > 2 && args[2] == "all" {
			err = t.Test("testclean", true)
		} else {
			err = t.Test("testclean", false)
		}
		if err != nil {
			NewtUsage(nil, err)
		}
	} else {
		// Parse any remaining key-value pairs and insert them into the target.
		err = extractTargetVars(args[1:], t)
		if err != nil {
			NewtUsage(cmd, err)
		}

		err = t.Test("test", ExitOnFailure)
		if err != nil {
			NewtUsage(nil, err)
		}
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Successfully run!\n")
}

func targetSizeCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target for sizing"))
	}

	t, err := cli.LoadTarget(NewtRepo, args[0])
	if err != nil {
		NewtUsage(nil, err)
	}

	txt, err := t.GetSize()
	if err != nil {
		NewtUsage(nil, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "%s", txt)
}

func targetLabelCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target and version"))
	}

	t, err := cli.LoadTarget(NewtRepo, args[0])
	if err != nil {
		NewtUsage(nil, err)
	}

	err = t.Build()
	if err != nil {
		NewtUsage(nil, err)
	}

	err = t.Label(args[1])
	if err != nil {
		NewtUsage(nil, err)
	}
}

func targetDownloadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target to download"))
	}

	t, err := cli.LoadTarget(NewtRepo, args[0])
	if err != nil {
		NewtUsage(nil, err)
	}

	err = t.Build()
	if err != nil {
		NewtUsage(nil, err)
	}

	err = t.Download()
	if err != nil {
		NewtUsage(nil, err)
	}
}

func targetDebugCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target for debug"))
	}

	t, err := cli.LoadTarget(NewtRepo, args[0])
	if err != nil {
		NewtUsage(nil, err)
	}

	err = t.Debug()
	if err != nil {
		NewtUsage(nil, err)
	}
}

func targetExportCmd(cmd *cobra.Command, args []string) {
	var targetName string
	if ExportAll {
		targetName = ""
	} else {
		if len(args) < 1 {
			NewtUsage(cmd, cli.NewNewtError("Must either specify -a flag or name of "+
				"target to export"))
		}
		targetName = args[0]
	}

	err := cli.ExportTargets(NewtRepo, targetName, ExportAll, os.Stdout)
	if err != nil {
		NewtUsage(cmd, err)
	}
}

func targetImportCmd(cmd *cobra.Command, args []string) {
	var targetName string
	if ImportAll {
		targetName = ""
	} else {
		if len(args) < 1 {
			NewtUsage(cmd, cli.NewNewtError("Must either specify -a flag or name of "+
				"target to import"))
		}

		targetName = args[0]
	}

	err := cli.ImportTargets(NewtRepo, targetName, ImportAll, os.Stdin)
	if err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Target(s) successfully imported!\n")
}

func targetAddCmds(base *cobra.Command) {
	targetHelpText := formatHelp(`Targets tell the newt tool how to build the source
		code within a given application.`)
	targetHelpEx := "  newt target create <target-name>\n"
	targetHelpEx += "  newt target set <target-name> <var-name>=<value>\n"
	targetHelpEx += "  newt target unset <target-name> <var-name>\n"
	targetHelpEx += "  newt target show <target-name>\n"
	targetHelpEx += "  newt target delete <target-name>\n"
	targetHelpEx += "  newt target build <target-name> [clean[ all]]\n"
	targetHelpEx += "  newt target test <target-name> [clean[ all]]\n"
	targetHelpEx += "  newt target size <target-name>\n"
	targetHelpEx += "  newt target label <target-name> <version number>"
	targetHelpEx += "  newt target download <target-name>\n"
	targetHelpEx += "  newt target debug <target-name>\n"
	targetHelpEx += "  newt target export [-a -export-all] [<target-name>]\n"
	targetHelpEx += "  newt target import [-a -import-all] [<target-name>]"

	targetCmd := &cobra.Command{
		Use:     "target",
		Short:   "Set and view target information",
		Long:    targetHelpText,
		Example: targetHelpEx,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(NewtLogLevel, newtSilent, newtQuiet, newtVerbose)

			var err error
			NewtRepo, err = cli.NewRepo()
			if err != nil {
				NewtUsage(nil, err)
			}

			file, err := os.Open(NewtRepo.BasePath + "/" + AutoTargets)
			if err == nil {
				err = cli.ImportTargets(NewtRepo, "", true, file)
				if err != nil {
					log.Printf("[DEBUG] failed to import automatic "+
						"targets %s", err.Error())
				}
				file.Close()
			} else {
				log.Printf("[DEBUG] failed to import automatic "+
					"targets %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	setHelpText := formatHelp(`Set a target variable (<var-name>) on target 
		<target-name> to value <value>.`)
	setHelpEx := "  newt target set <target-name> <var-name>=<value>\n"
	setHelpEx += "  newt target set my_target1 var_name=value\n"
	setHelpEx += "  newt target set my_target1 arch=cortex_m4"

	setCmd := &cobra.Command{
		Use:     "set",
		Short:   "Set target configuration variable",
		Long:    setHelpText,
		Example: setHelpEx,
		Run:     targetSetCmd,
	}

	targetCmd.AddCommand(setCmd)

	unsetHelpText := formatHelp(`Unset a target variable (<var-name>) on target
		<target-name>.`)
	unsetHelpEx := "  newt target unset <target-name> <var-name>\n"
	unsetHelpEx += "  newt target unset my_target1 var_name"

	unsetCmd := &cobra.Command{
		Use:     "unset",
		Short:   "Unset target configuration variable",
		Long:    unsetHelpText,
		Example: unsetHelpEx,
		Run:     targetUnsetCmd,
	}

	targetCmd.AddCommand(unsetCmd)

	delHelpText := formatHelp(`Delete the target specified by <target-name>.`)
	delHelpEx := "  newt target delete <target-name>\n"
	delHelpEx += "  newt target delete my_target1"

	delCmd := &cobra.Command{
		Use:     "delete",
		Short:   "Delete target",
		Long:    delHelpText,
		Example: delHelpEx,
		Run:     targetDelCmd,
	}

	targetCmd.AddCommand(delCmd)

	createHelpText := formatHelp(`Create a target specified by <target-name>.`)
	createHelpEx := "  newt target create <target-name>\n"
	createHelpEx += "  newt target create my_target1"

	createCmd := &cobra.Command{
		Use:     "create",
		Short:   "Create a target",
		Long:    createHelpText,
		Example: createHelpEx,
		Run:     targetCreateCmd,
	}

	targetCmd.AddCommand(createCmd)

	showHelpText := formatHelp(`Show all the variables for the target specified 
		by <target-name>.`)
	showHelpEx := "  newt target show <target-name>\n"
	showHelpEx += "  newt target show my_target1"

	showCmd := &cobra.Command{
		Use:     "show",
		Short:   "View target configuration variables",
		Long:    showHelpText,
		Example: showHelpEx,
		Run:     targetShowCmd,
	}

	targetCmd.AddCommand(showCmd)

	buildHelpText := formatHelp(`Build the target specified by <target-name>.  
		If clean is specified, then all the binaries and object files for this 
		target will be removed.  If the all option is specified, all binaries 
		and object files for all targets will be removed.`)
	buildHelpEx := "  newt target build <target-name> [clean[ all]]\n"
	buildHelpEx += "  newt target build my_target1\n"
	buildHelpEx += "  newt target build my_target1 clean\n"
	buildHelpEx += "  newt target build my_target1 clean all\n"

	buildCmd := &cobra.Command{
		Use:     "build",
		Short:   "Build target",
		Long:    buildHelpText,
		Example: buildHelpEx,
		Run:     targetBuildCmd,
	}

	targetCmd.AddCommand(buildCmd)

	testHelpText := formatHelp(`Test the target specified by <target-name>.  If
		clean is specified, then all the test binaries and object files for this 
		target will be removed.  If the all option is specified, all test 
		binaries and object files for all targets will be removed.`)
	testHelpEx := "  newt target test <target-name> [clean, [all]]\n"
	testHelpEx += "  newt target test mytarget1\n"
	testHelpEx += "  newt target test mytarget1 clean\n"
	testHelpEx += "  newt target test mytarget1 clean all"

	testCmd := &cobra.Command{
		Use:     "test",
		Short:   "Test target",
		Long:    testHelpText,
		Example: testHelpEx,
		Run:     targetTestCmd,
	}

	targetCmd.AddCommand(testCmd)

	sizeHelpText := formatHelp(`Calculate the size of target components specified by
		<target-name>.`)
	sizeHelpEx := "  newt target size <target-name>\n"

	sizeCmd := &cobra.Command{
		Use:     "size",
		Short:   "Size of the target",
		Long:    sizeHelpText,
		Example: sizeHelpEx,
		Run:     targetSizeCmd,
	}

	targetCmd.AddCommand(sizeCmd)

	labelHelpText := formatHelp(`Label image by appending image header to created
		binary file <target-name>. Version number in the header is set to be
		<version>`)
	labelHelpEx := "  newt target label <target-name> <version>\n"
	labelHelpEx += "  newt target label my_target1 1.2.0\n"
	labelHelpEx += "  newt target label my_target1 1.2.0.3\n"

	labelCmd := &cobra.Command{
		Use:     "label",
		Short:   "Add image header to target binary",
		Long:    labelHelpText,
		Example: labelHelpEx,
		Run:     targetLabelCmd,
	}
	targetCmd.AddCommand(labelCmd)

	downloadHelpText := formatHelp(`Download project image to target for
		<target-name>.`)
	downloadHelpEx := "  newt target download <target-name>\n"

	downloadCmd := &cobra.Command{
		Use:     "download",
		Short:   "Download project to target",
		Long:    downloadHelpText,
		Example: downloadHelpEx,
		Run:     targetDownloadCmd,
	}
	targetCmd.AddCommand(downloadCmd)

	debugHelpText := formatHelp(`Download project image to target for
		<target-name>.`)
	debugHelpEx := "  newt target download <target-name>\n"

	debugCmd := &cobra.Command{
		Use:     "debug",
		Short:   "Open debugger session to target",
		Long:    debugHelpText,
		Example: debugHelpEx,
		Run:     targetDebugCmd,
	}
	targetCmd.AddCommand(debugCmd)

	exportHelpText := formatHelp(`Export build targets from the current application, and 
		print them to standard output.  If the -a (or -export-all) option is 
		specified, then all targets will be exported.  Otherwise, <target-name> 
		must be specified, and only that target will be exported.`)
	exportHelpEx := "  newt target export [-a -export-all] [<target-name>]\n"
	exportHelpEx += "  newt target export -a > my_exports.txt\n"
	exportHelpEx += "  newt target export my_target > my_target_export.txt"

	exportCmd := &cobra.Command{
		Use:     "export",
		Short:   "Export target",
		Long:    exportHelpText,
		Example: exportHelpEx,
		Run:     targetExportCmd,
	}

	exportCmd.PersistentFlags().BoolVarP(&ExportAll, "export-all", "a", false,
		"If present, export all targets")

	targetCmd.AddCommand(exportCmd)

	importHelpText := formatHelp(`Import build targets from standard input.  If 
		the -a (or -import-all) option is specified, then all targets will be 
		imported.  Otherwise, a <target-name> must be specified, and only that 
		target will be imported.`)
	importHelpEx := "  newt target import [-a -import-all] [<target-name>]\n"
	importHelpEx += "  newt target import -a < exported_targets.txt\n"
	importHelpEx += "  newt target import ex_tgt_1 < exported_targets.txt"

	importCmd := &cobra.Command{
		Use:     "import",
		Short:   "Import target",
		Long:    importHelpText,
		Example: importHelpEx,
		Run:     targetImportCmd,
	}

	importCmd.PersistentFlags().BoolVarP(&ImportAll, "import-all", "a", false,
		"If present, import all targets")

	targetCmd.AddCommand(importCmd)

	base.AddCommand(targetCmd)
}

func dispPkg(pkg *cli.Pkg) error {
	cli.StatusMessage(cli.VERBOSITY_QUIET, "Package %s, version %s\n",
		pkg.FullName, pkg.Version)
	cli.StatusMessage(cli.VERBOSITY_QUIET, "  path: %s\n",
		filepath.Clean(pkg.BasePath))
	if pkg.Capabilities != nil {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  capabilities: ")
		caps, err := pkg.GetCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", capability)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
	if pkg.ReqCapabilities != nil {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  required capabilities: ")
		caps, err := pkg.GetReqCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", capability)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
	if len(pkg.Deps) > 0 {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  deps: ")
		for _, dep := range pkg.Deps {
			if dep == nil {
				continue
			}
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", dep)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}

	if pkg.LinkerScript != "" {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  linkerscript: %s\n",
			pkg.LinkerScript)
	}
	return nil
}

func dispPkgDesc(pkgDesc *cli.PkgDesc) error {
	cli.StatusMessage(cli.VERBOSITY_QUIET, "Package %s from pkg-list %s, version %s\n",
		pkgDesc.FullName, pkgDesc.PkgList.Name, pkgDesc.Version)

	if pkgDesc.Caps != nil {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  capabilities: ")
		caps, err := pkgDesc.GetCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", capability)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
	if pkgDesc.ReqCaps != nil {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  required capabilities: ")
		caps, err := pkgDesc.GetReqCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", capability)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
	if len(pkgDesc.Deps) > 0 {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  deps: ")
		for _, dep := range pkgDesc.Deps {
			if dep == nil {
				continue
			}
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", dep)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}

	return nil
}

func pkgListCmd(cmd *cobra.Command, args []string) {
	pkgMgr, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}
	if err := pkgMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}
	for _, pkg := range pkgMgr.Pkgs {
		if err := dispPkg(pkg); err != nil {
			NewtUsage(cmd, err)
		}
	}
}

func pkgCheckDepsCmd(cmd *cobra.Command, args []string) {
	pkgMgr, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := pkgMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}

	if err := pkgMgr.CheckDeps(); err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Dependencies successfully resolved!\n")
}

func pkgSearchCmd(cmd *cobra.Command, args []string) {
	var err error

	if len(args) != 1 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify string to search for"))
	}

	/*
	 * First check local.
	 */
	pkgMgr, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}
	if err := pkgMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}

	found := false
	for _, pkg := range pkgMgr.Pkgs {
		contains := strings.Contains(pkg.FullName, args[0])
		if contains == true {
			cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Installed package %s@%s\n",
				pkg.FullName, pkg.Version)
			found = true
		}
	}

	/*
	 * Then check remote pkgLists.
	 */
	pkgLists, err := NewtRepo.GetPkgLists()
	if err != nil {
		NewtUsage(cmd, err)
	}
	for _, pkgList := range pkgLists {
		for _, pkgDesc := range pkgList.PkgDescs {
			contains := strings.Contains(pkgDesc.FullName, args[0])
			if contains == true {
				cli.StatusMessage(cli.VERBOSITY_DEFAULT,
					"Package list %s has package %s@%s\n",
					pkgList.Name, pkgDesc.FullName,
					pkgDesc.Version)
				found = true
			}
		}
	}

	if found == false {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "No package found!\n")
	}
}

func pkgShowCmd(cmd *cobra.Command, args []string) {
	var pkgName string
	var pkgListName string = ""

	if len(args) < 1 || len(args) > 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify full name of the package"))
	}

	if len(args) == 1 {
		pkgName = args[0]
	} else {
		pkgListName = args[0]
		pkgName = args[1]
	}

	pkgMgr, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}
	if err := pkgMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}

	pkg, err := pkgMgr.ResolvePkgName(pkgName)
	if err == nil {
		pkg.LoadConfig(nil, false)
		dispPkg(pkg)
	}

	pkgLists, err := NewtRepo.GetPkgLists()
	if err != nil {
		NewtUsage(cmd, err)
	}
	for _, pkgList := range pkgLists {
		if pkgListName == "" || pkgList.Name == pkgListName {
			pkgDesc, err := pkgList.ResolvePkgDescName(pkgName)
			if err == nil {
				dispPkgDesc(pkgDesc)
			}
		}
	}
}

func pkgDescInstall(pkgDesc *cli.PkgDesc) error {
	pkgMgr, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		return err
	}

	if err := pkgMgr.LoadConfigs(nil, false); err != nil {
		return err
	}

	_, err = pkgMgr.ResolvePkgName(pkgDesc.FullName)
	if err == nil {
		return cli.NewNewtError(fmt.Sprintf("Package %s already installed!",
			pkgDesc.FullName))
	}
	err = pkgDesc.Install(pkgMgr, NewtBranchPkg)
	if err != nil {
		return err
	}
	return nil
}

func pkgInstallCmd(cmd *cobra.Command, args []string) {
	var pkgName string
	var pkgListName string = ""
	var pkgList *cli.PkgList
	var pkgDesc *cli.PkgDesc

	if len(args) < 1 || len(args) > 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify full name of the package"))
	}

	if len(args) == 1 {
		pkgName = args[0]
	} else {
		pkgListName = args[0]
		pkgName = args[1]
	}

	/*
	 * Find the pkgDesc to install.
	 */
	pkgLists, err := NewtRepo.GetPkgLists()
	if err != nil {
		NewtUsage(cmd, err)
	}

	if pkgListName != "" {
		pkgList = pkgLists[pkgListName]
	}
	if pkgList != nil {
		pkgDesc, err := pkgList.ResolvePkgDescName(pkgName)
		if err != nil {
			NewtUsage(cmd, err)
		}
		err = pkgDescInstall(pkgDesc)
		if err != nil {
			NewtUsage(cmd, err)
		}
		cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Installation was a success!\n")
		return
	}
	pkgDesc = nil
	for _, tmpPkgList := range pkgLists {
		if pkgListName == "" || tmpPkgList.Name == pkgListName {
			tmpPkgDesc, err := tmpPkgList.ResolvePkgDescName(pkgName)
			if err != nil && pkgDesc != nil {
				NewtUsage(cmd,
					cli.NewNewtError(fmt.Sprintf("Ambiguous source "+
						"pkg %s in package-list %s and %s",
						pkgName, pkgList.Name, tmpPkgList.Name)))
			} else {
				pkgDesc = tmpPkgDesc
				pkgList = tmpPkgList
			}
		}
	}

	if pkgDesc == nil {
		NewtUsage(cmd,
			cli.NewNewtError(fmt.Sprintf("Can't find package with name %s",
				pkgName)))
	}

	err = pkgDescInstall(pkgDesc)
	if err != nil {
		NewtUsage(cmd, err)
	}
	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Installation was a success!\n")
}

func pkgRemoveCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 || len(args) > 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify full name of the package"))
	}

	pkgName := args[0]

	pkgMgr, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}
	if err := pkgMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}

	pkg, err := pkgMgr.ResolvePkgName(pkgName)
	if err != nil {
		NewtUsage(cmd,
			cli.NewNewtError(fmt.Sprintf("Package %s has not been installed",
				pkgName)))
	}
	err = pkg.Remove()
	if err != nil {
		NewtUsage(cmd, err)
	}
	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Removed successfully!\n")
}

func pkgAddCmds(baseCmd *cobra.Command) {
	pkgHelpText := formatHelp(`Commands to search, display and install packages
		in the current application.`)
	pkgHelpEx := "  newt pkg list\n"
	pkgHelpEx += "  newt pkg checkdeps\n"
	pkgHelpEx += "  newt pkg search <pkg-name>\n"
	pkgHelpEx += "  newt pkg show [<pkg-list> ] <pkg-name>\n"
	pkgHelpEx += "  newt pkg install [<pkg-list> ] <pkg-name>\n"
	pkgHelpEx += "  newt pkg remove [<pkg-list> ] <pkg-name>"

	pkgCmd := &cobra.Command{
		Use:     "pkg",
		Short:   "Commands to list and inspect packages on a application",
		Long:    pkgHelpText,
		Example: pkgHelpEx,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(NewtLogLevel, newtSilent, newtQuiet, newtVerbose)

			var err error
			NewtRepo, err = cli.NewRepo()
			if err != nil {
				NewtUsage(nil, err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			NewtUsage(cmd, nil)
		},
	}

	listHelpText := formatHelp(`List all of the packages in the current application.`)
	listHelpEx := "  newt pkg list"

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List packages in the current application",
		Long:    listHelpText,
		Example: listHelpEx,
		Run:     pkgListCmd,
	}

	pkgCmd.AddCommand(listCmd)

	checkDepsHelpText := formatHelp(`Resolve all dependencies in the local 
		application.  This command goes through all packages currently 
		installed, checks their dependencies, and prints any unresolved 
		dependencies between packages.`)
	checkDepsHelpEx := "  newt pkg checkdeps"

	checkDepsCmd := &cobra.Command{
		Use:     "checkdeps",
		Short:   "Check package dependencies",
		Long:    checkDepsHelpText,
		Example: checkDepsHelpEx,
		Run:     pkgCheckDepsCmd,
	}

	pkgCmd.AddCommand(checkDepsCmd)

	searchHelpText := formatHelp(`Search for an package, specified by <pkg-name>.  
		The local application, along with all remote applications are 
		searched.  All matched packages are shown.`)
	searchHelpEx := "  newt pkg search <pkg-name>"

	searchCmd := &cobra.Command{
		Use:     "search",
		Short:   "Search for package",
		Long:    searchHelpText,
		Example: searchHelpEx,
		Run:     pkgSearchCmd,
	}

	pkgCmd.AddCommand(searchCmd)

	showHelpText := formatHelp(`Show the contents of the package, specified by 
		<pkg-name>.  <pkg-name> is resolved using all the pkg-lists installed
		in the current application or, if <pkg-list-name> is specified, only 
		<pkg-list-name> will be searched.`)
	showHelpEx := "  newt pkg show [<pkg-list-name> ] <pkg-name>"

	showCmd := &cobra.Command{
		Use:     "show",
		Short:   "Show the contents of a package.",
		Long:    showHelpText,
		Example: showHelpEx,
		Run:     pkgShowCmd,
	}

	pkgCmd.AddCommand(showCmd)

	installHelpText := formatHelp(`Install the package specified by <pkg-name> to 
		the local application. <pkg-name> is searched for throughout the pkg-lists in 
		the local application.  If <pkg-list-name> is specified, then only <pkg-list-name>
		is searched for <pkg-name>.`)
	installHelpEx := "  newt pkg install [<pkgList-name> ] <pkg-name>"

	installCmd := &cobra.Command{
		Use:     "install",
		Short:   "Install a package",
		Long:    installHelpText,
		Example: installHelpEx,
		Run:     pkgInstallCmd,
	}

	installCmd.Flags().StringVarP(&NewtBranchPkg, "branch", "b", "",
		"Branch (or tag) of the pkg-list to install from.")

	pkgCmd.AddCommand(installCmd)

	removeHelpText := formatHelp(`Remove the package, specified by <pkg-name> from 
		the local application.  If present the package is resolved from the package
		list specified by <pkg-list-name>.`)
	removeHelpEx := "  newt pkg remove [<pkg-list-name> ] <pkg-name>"

	removeCmd := &cobra.Command{
		Use:     "remove",
		Short:   "Remove a package",
		Long:    removeHelpText,
		Example: removeHelpEx,
		Run:     pkgRemoveCmd,
	}

	pkgCmd.AddCommand(removeCmd)

	baseCmd.AddCommand(pkgCmd)
}

func repoGeneratePkgListCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify name and URL to generate a pkg-list file"))
	}

	pkgListName := args[0]
	pkgListUrl := args[1]

	pkgList, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}
	pkgList.Name = pkgListName
	pkgList.RemoteUrl = pkgListUrl

	local, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := pkgList.LoadFromPkgList(local); err != nil {
		NewtUsage(cmd, err)
	}

	pkgListStr, err := pkgList.Serialize()
	if err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "%s", pkgListStr)
}

func repoAddPkgListCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify both name and URL to "+
				"larva install command"))
	}

	name := args[0]
	url := args[1]

	pkgList, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := pkgList.Install(name, url, NewtBranchPkgList); err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Package list "+name+" successfully installed to application.\n")
}

func repoListPkgListsCmd(cmd *cobra.Command, args []string) {
	pkgLists, err := NewtRepo.GetPkgLists()
	if err != nil {
		NewtUsage(cmd, err)
	}

	for name, pkgList := range pkgLists {
		cli.StatusMessage(cli.VERBOSITY_QUIET,
			"Remote package list %s (num_pkgs: %d)\n", name, len(pkgList.PkgDescs))
	}
}

func repoCreateCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify an application name to create"))
	}

	wd, err := os.Getwd()
	if err != nil {
		NewtUsage(cmd, cli.NewNewtError(err.Error()))
	}

	repoDir := wd + "/" + args[0]
	if len(args) > 1 {
		repoDir = args[1]
	}

	tadpoleUrl := ""
	if len(args) > 2 {
		tadpoleUrl = args[2]
	}

	if err := cli.CreateRepo(args[0], repoDir, tadpoleUrl); err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Application %s successfully created in %s\n", args[0], repoDir)
}

func repoShowPkgListCmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify a pkg-list name to show-pkg-list command"))
	}

	pkgList, err := cli.NewPkgList(NewtRepo)
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := pkgList.Load(args[0]); err != nil {
		NewtUsage(cmd, err)
	}

	// PkgList loaded, now print out pkgList information
	cli.StatusMessage(cli.VERBOSITY_QUIET, "Package List Name: %s\n", pkgList.Name)
	cli.StatusMessage(cli.VERBOSITY_QUIET, "Package List URL: %s\n",
		pkgList.RemoteUrl)

	i := 0
	for _, pkgDesc := range pkgList.PkgDescs {
		i++
		cli.StatusMessage(cli.VERBOSITY_QUIET, " %s@%s", pkgDesc.FullName,
			pkgDesc.Version)
		if i%4 == 0 {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
		}
	}
	if i%4 != 0 {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
}

func repoAddCmds(baseCmd *cobra.Command) {
	var repoCmd *cobra.Command
	var createCmd *cobra.Command

	repoHelpText := formatHelp(`The app commands help manage the local application.
		An application represents the workspace for one or more projects, each project being a
                collection of packages.  In addition to containing packages, an application contains the 
		target (build) definitions, and a list of pkg-lists (snapshots of remote applications, 
		which contain packages that can be installed into the current application.)`)
	repoHelpEx := "  newt app list-pkg-lists\n"
	repoHelpEx += "  newt app show-pkg-list <pkg-list-name>\n"
	repoHelpEx += "  newt app add-pkg-list <pkg-list-name> <pkg-list-url>\n"
	repoHelpEx += "  newt app generate-pkg-list <pkg-list-name> <pkg-list-url>"

	repoCmd = &cobra.Command{
		Use:     "app",
		Short:   "Commands to manage application",
		Long:    repoHelpText,
		Example: repoHelpEx,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(NewtLogLevel, newtSilent, newtQuiet, newtVerbose)

			var err error

			if cmd != repoCmd {
				NewtRepo, err = cli.NewRepo()
				if err != nil {
					NewtUsage(cmd, err)
				}
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			NewtUsage(cmd, nil)
		},
	}

	createHelpText := formatHelp(`Create a new application, specified by <app-name>. 
		If the optional <app-url> parameter is specified, then download the 
		skeleton of the application from that URL, instead of using the default.`)

	createHelpEx := "  newt new <app-name> [, <app-url>]\n"
	createHelpEx += "  newt new myapp"

	createCmd = &cobra.Command{
		Use:     "new",
		Short:   "Create a new application",
		Long:    createHelpText,
		Example: createHelpEx,
		Run:     repoCreateCmd,
	}

	baseCmd.AddCommand(createCmd)

	generateHelpText := formatHelp(`Generate a pkg-list file from a snapshot 
	    of the packeges in the current directory.  generate-pkg-list takes two 
		arguments, the name of the current application and the URL at which 
		the application is located.`)

	generateHelpEx := "  newt app generate-pkg-list <name> <url>\n"
	generateHelpEx += "  newt app generate-pkg-list larva " +
		"https://git-wip-us.apache.org/repos/asf/incubator-mynewt-larva"

	generateCmd := &cobra.Command{
		Use:     "generate-pkg-list",
		Short:   "Generate a pkg-list file from the packages in the current directory",
		Long:    generateHelpText,
		Example: generateHelpEx,
		Run:     repoGeneratePkgListCmd,
	}

	repoCmd.AddCommand(generateCmd)

	addPkgListHelpText := formatHelp(`Add a remote pkg-list to the current application.
	    When search for pkgs to install, the pkg-list specified by pkg-list-name
		and pkg-list-url will be searched for packages that match the search.  This
		includes both direct searches with newt package search, as well as resolving
		dependencies in pkg.yml files.`)

	addPkgListHelpEx := "  newt app add-pkg-list <pkg-list-name> <pkg-list-url>\n"
	addPkgListHelpEx += "  newt app add-pkg-list larva " +
		"https://git-wip-us.apache.org/repos/asf/incubator-mynewt-larva"

	addPkgListCmd := &cobra.Command{
		Use:     "add-pkg-list",
		Short:   "Add a remote pkg-list, and put it in the current application",
		Long:    addPkgListHelpText,
		Example: addPkgListHelpEx,
		Run:     repoAddPkgListCmd,
	}

	addPkgListCmd.Flags().StringVarP(&NewtBranchPkgList, "branch", "b", "master",
		"Branch (or tag) of the pkg-list to install from.")

	repoCmd.AddCommand(addPkgListCmd)

	listPkgListsHelpText := formatHelp(`List the pkg-lists installed in the current
		application.  A pkg-list represents a collection of packages in an application.  List pkg-lists
		includes the current application, along with any remote pkgLists that have been 
		added using the add-pkg-list command.`)

	listPkgListsHelpEx := "  newt app list-pkg-lists"

	listPkgListsCmd := &cobra.Command{
		Use:     "list-pkg-lists",
		Short:   "List the pkg-lists installed in the current application",
		Long:    listPkgListsHelpText,
		Example: listPkgListsHelpEx,
		Run:     repoListPkgListsCmd,
	}

	repoCmd.AddCommand(listPkgListsCmd)

	showPkgListHelpText := formatHelp(`Show information about a pkg-list, given by the 
		<pkg-list-name> parameter.  Displays the pkg-list name, URL and packages 
		associated with a given pkg-list.`)

	showPkgListHelpEx := "  newt app show-pkg-list <pkg-list-name>\n"
	showPkgListHelpEx += "  newt app show-pkg-list larva"

	showPkgListCmd := &cobra.Command{
		Use:     "show-pkg-list",
		Short:   "Show an individual pkg-list in the current application",
		Long:    showPkgListHelpText,
		Example: showPkgListHelpEx,
		Run:     repoShowPkgListCmd,
	}

	repoCmd.AddCommand(showPkgListCmd)

	baseCmd.AddCommand(repoCmd)
}

func parseCmds() *cobra.Command {
	newtHelpText := formatHelp(`Newt allows you to create your own embedded 
		application based on the Mynewt operating system.  Newt provides both 
		build and package management in a single tool, which allows you to 
		compose an embedded application, and set of projects, and then build
		the necessary artifacts from those projects.  For more information 
		on the Mynewt operating system, please visit 
		https://mynewt.apache.org/.`)
	newtHelpText += "\n\n" + formatHelp(`Please use the newt help command, 
		and specify the name of the command you want help for, for help on 
		how to use a specific command`)
	newtHelpEx := "  newt\n"
	newtHelpEx += "  newt help [<command-name>]\n"
	newtHelpEx += "    For help on <command-name>.  If not specified, " +
		"print this message."

	newtCmd := &cobra.Command{
		Use:     "newt",
		Short:   "Newt is a tool to help you compose and build your own OS",
		Long:    newtHelpText,
		Example: newtHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	newtCmd.PersistentFlags().BoolVarP(&newtVerbose, "verbose", "v", false,
		"Enable verbose output when executing commands.")
	newtCmd.PersistentFlags().BoolVarP(&newtQuiet, "quiet", "q", false,
		"Be quiet; only display error output.")
	newtCmd.PersistentFlags().BoolVarP(&newtSilent, "silent", "s", false,
		"Be silent; don't output anything.")
	newtCmd.PersistentFlags().StringVarP(&NewtLogLevel, "loglevel", "l",
		"WARN", "Log level, defaults to WARN.")

	versHelpText := formatHelp(`Display the Newt version number.`)
	versHelpEx := "  newt version"
	versCmd := &cobra.Command{
		Use:     "version",
		Short:   "Display the Newt version number.",
		Long:    versHelpText,
		Example: versHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Newt version: %s\n", NewtVersion)
		},
	}

	newtCmd.AddCommand(versCmd)

	targetAddCmds(newtCmd)
	pkgAddCmds(newtCmd)
	repoAddCmds(newtCmd)

	return newtCmd
}

func main() {
	cmd := parseCmds()
	cmd.Execute()
}
