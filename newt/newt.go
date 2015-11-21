/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package main

import (
	"fmt"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt.git/newt/cli"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var ExitOnFailure bool = false
var ExportAll bool = false
var ImportAll bool = false
var NewtVersion string = "0.1"
var NewtLogLevel string = ""
var NewtNest *cli.Nest
var newtSilent bool
var newtQuiet bool
var newtVerbose bool
var NewtBranchClutch string
var NewtBranchEgg string

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

	t, err := cli.LoadTarget(NewtNest, args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}
	ar := strings.Split(args[1], "=")

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

	t, err := cli.LoadTarget(NewtNest, args[0])
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

	targets, err := cli.GetTargets(NewtNest)
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
				cli.StatusMessage(cli.VERBOSITY_QUIET, "	%s: %s\n", k, vars[k])
			}
		}
	}
}

func targetCreateCmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		NewtUsage(cmd, cli.NewNewtError("Wrong number of args to create cmd."))
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Creating target "+args[0]+"\n")

	if cli.TargetExists(NewtNest, args[0]) {
		NewtUsage(cmd, cli.NewNewtError(
			"Target already exists, cannot create target with same name."))
	}

	target := &cli.Target{
		Nest: NewtNest,
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

	t, err := cli.LoadTarget(NewtNest, args[0])
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

	t, err := cli.LoadTarget(NewtNest, args[0])
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

	t, err := cli.LoadTarget(NewtNest, args[0])
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

	t, err := cli.LoadTarget(NewtNest, args[0])
	if err != nil {
		NewtUsage(nil, err)
	}

	txt, err := t.GetSize()
	if err != nil {
		NewtUsage(nil, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "%s", txt)
}

func targetDownloadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target to download"))
	}

	t, err := cli.LoadTarget(NewtNest, args[0])
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

	t, err := cli.LoadTarget(NewtNest, args[0])
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

	err := cli.ExportTargets(NewtNest, targetName, ExportAll, os.Stdout)
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

	err := cli.ImportTargets(NewtNest, targetName, ImportAll, os.Stdin)
	if err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Target(s) successfully imported!\n")
}

func targetAddCmds(base *cobra.Command) {
	targetHelpText := formatHelp(`Targets tell the newt tool how to build the source
		code within a given nest.`)
	targetHelpEx := "  newt target create <target-name>\n"
	targetHelpEx += "  newt target set <target-name> <var-name>=<value>\n"
	targetHelpEx += "  newt target unset <target-name> <var-name>\n"
	targetHelpEx += "  newt target show <target-name>\n"
	targetHelpEx += "  newt target delete <target-name>\n"
	targetHelpEx += "  newt target build <target-name> [clean[ all]]\n"
	targetHelpEx += "  newt target test <target-name> [clean[ all]]\n"
	targetHelpEx += "  newt target size <target-name>\n"
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
			NewtNest, err = cli.NewNest()
			if err != nil {
				NewtUsage(nil, err)
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

	exportHelpText := formatHelp(`Export build targets from the current nest, and 
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

func dispEgg(egg *cli.Egg) error {
	cli.StatusMessage(cli.VERBOSITY_QUIET, "Egg %s, version %s\n",
		egg.FullName, egg.Version)
	cli.StatusMessage(cli.VERBOSITY_QUIET, "  path: %s\n",
		filepath.Clean(egg.BasePath))
	if egg.Capabilities != nil {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  capabilities: ")
		caps, err := egg.GetCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", capability)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
	if egg.ReqCapabilities != nil {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  required capabilities: ")
		caps, err := egg.GetReqCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", capability)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
	if len(egg.Deps) > 0 {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  deps: ")
		for _, dep := range egg.Deps {
			if dep == nil {
				continue
			}
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", dep)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}

	if egg.LinkerScript != "" {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  linkerscript: %s\n",
			egg.LinkerScript)
	}
	return nil
}

func dispEggShell(eggShell *cli.EggShell) error {
	cli.StatusMessage(cli.VERBOSITY_QUIET, "Egg %s from clutch %s, version %s\n",
		eggShell.FullName, eggShell.Clutch.Name, eggShell.Version)

	if eggShell.Caps != nil {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  capabilities: ")
		caps, err := eggShell.GetCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", capability)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
	if eggShell.ReqCaps != nil {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  required capabilities: ")
		caps, err := eggShell.GetReqCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", capability)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
	if len(eggShell.Deps) > 0 {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "  deps: ")
		for _, dep := range eggShell.Deps {
			if dep == nil {
				continue
			}
			cli.StatusMessage(cli.VERBOSITY_QUIET, "%s ", dep)
		}
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}

	return nil
}

func eggListCmd(cmd *cobra.Command, args []string) {
	eggMgr, err := cli.NewClutch(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}
	if err := eggMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}
	for _, egg := range eggMgr.Eggs {
		if err := dispEgg(egg); err != nil {
			NewtUsage(cmd, err)
		}
	}
}

func eggCheckDepsCmd(cmd *cobra.Command, args []string) {
	eggMgr, err := cli.NewClutch(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := eggMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}

	if err := eggMgr.CheckDeps(); err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Dependencies successfully resolved!\n")
}

func eggHuntCmd(cmd *cobra.Command, args []string) {
	var err error

	if len(args) != 1 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify string to hunt for"))
	}

	/*
	 * First check local.
	 */
	eggMgr, err := cli.NewClutch(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}
	if err := eggMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}

	found := false
	for _, egg := range eggMgr.Eggs {
		contains := strings.Contains(egg.FullName, args[0])
		if contains == true {
			cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Installed egg %s@%s\n",
				egg.FullName, egg.Version)
			found = true
		}
	}

	/*
	 * Then check remote clutches.
	 */
	clutches, err := NewtNest.GetClutches()
	if err != nil {
		NewtUsage(cmd, err)
	}
	for _, clutch := range clutches {
		for _, eggShell := range clutch.EggShells {
			contains := strings.Contains(eggShell.FullName, args[0])
			if contains == true {
				cli.StatusMessage(cli.VERBOSITY_DEFAULT,
					"Clutch %s has egg %s@%s\n",
					clutch.Name, eggShell.FullName,
					eggShell.Version)
				found = true
			}
		}
	}

	if found == false {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "No egg found!\n")
	}
}

func eggShowCmd(cmd *cobra.Command, args []string) {
	var eggName string
	var clutchName string = ""

	if len(args) < 1 || len(args) > 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify full name of the egg"))
	}

	if len(args) == 1 {
		eggName = args[0]
	} else {
		clutchName = args[0]
		eggName = args[1]
	}

	eggMgr, err := cli.NewClutch(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}
	if err := eggMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}

	egg, err := eggMgr.ResolveEggName(eggName)
	if err == nil {
		egg.LoadConfig(nil, false)
		dispEgg(egg)
	}

	clutches, err := NewtNest.GetClutches()
	if err != nil {
		NewtUsage(cmd, err)
	}
	for _, clutch := range clutches {
		if clutchName == "" || clutch.Name == clutchName {
			eggShell, err := clutch.ResolveEggShellName(eggName)
			if err == nil {
				dispEggShell(eggShell)
			}
		}
	}
}

func eggShellInstall(eggShell *cli.EggShell) error {
	eggMgr, err := cli.NewClutch(NewtNest)
	if err != nil {
		return err
	}

	if err := eggMgr.LoadConfigs(nil, false); err != nil {
		return err
	}

	_, err = eggMgr.ResolveEggName(eggShell.FullName)
	if err == nil {
		return cli.NewNewtError(fmt.Sprintf("Egg %s already installed!",
			eggShell.FullName))
	}
	err = eggShell.Install(eggMgr, NewtBranchEgg)
	if err != nil {
		return err
	}
	return nil
}

func eggInstallCmd(cmd *cobra.Command, args []string) {
	var eggName string
	var clutchName string = ""
	var clutch *cli.Clutch
	var eggShell *cli.EggShell

	if len(args) < 1 || len(args) > 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify full name of the egg"))
	}

	if len(args) == 1 {
		eggName = args[0]
	} else {
		clutchName = args[0]
		eggName = args[1]
	}

	/*
	 * Find the eggShell to install.
	 */
	clutches, err := NewtNest.GetClutches()
	if err != nil {
		NewtUsage(cmd, err)
	}

	if clutchName != "" {
		clutch = clutches[clutchName]
	}
	if clutch != nil {
		eggShell, err := clutch.ResolveEggShellName(eggName)
		if err != nil {
			NewtUsage(cmd, err)
		}
		err = eggShellInstall(eggShell)
		if err != nil {
			NewtUsage(cmd, err)
		}
		cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Installation was a success!\n")
		return
	}
	eggShell = nil
	for _, tmpClutch := range clutches {
		if clutchName == "" || tmpClutch.Name == clutchName {
			tmpEggShell, err := tmpClutch.ResolveEggShellName(eggName)
			if err != nil && eggShell != nil {
				NewtUsage(cmd,
					cli.NewNewtError(fmt.Sprintf("Ambiguous source "+
						"egg %s in clutches %s and %s",
						eggName, clutch.Name, tmpClutch.Name)))
			} else {
				eggShell = tmpEggShell
				clutch = tmpClutch
			}
		}
	}

	if eggShell == nil {
		NewtUsage(cmd,
			cli.NewNewtError(fmt.Sprintf("Can't find egg with name %s",
				eggName)))
	}

	err = eggShellInstall(eggShell)
	if err != nil {
		NewtUsage(cmd, err)
	}
	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Installation was a success!\n")
}

func eggRemoveCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 || len(args) > 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify full name of the egg"))
	}

	eggName := args[0]

	eggMgr, err := cli.NewClutch(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}
	if err := eggMgr.LoadConfigs(nil, false); err != nil {
		NewtUsage(cmd, err)
	}

	egg, err := eggMgr.ResolveEggName(eggName)
	if err != nil {
		NewtUsage(cmd,
			cli.NewNewtError(fmt.Sprintf("Egg %s has not been installed",
				eggName)))
	}
	err = egg.Remove()
	if err != nil {
		NewtUsage(cmd, err)
	}
	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Removed successfully!\n")
}

func eggAddCmds(baseCmd *cobra.Command) {
	eggHelpText := formatHelp(`Commands to search, display and install eggs
		in the current nest.  Eggs are newt's version of packages, and are 
		the fundamental building blocks of nests.`)
	eggHelpEx := "  newt egg list\n"
	eggHelpEx += "  newt egg checkdeps\n"
	eggHelpEx += "  newt egg hunt <egg-name>\n"
	eggHelpEx += "  newt egg show [<clutch-name> ] <egg-name>\n"
	eggHelpEx += "  newt egg install [<clutch-name> ] <egg-name>\n"
	eggHelpEx += "  newt egg remove [<clutch-name> ] <egg-name>"

	eggCmd := &cobra.Command{
		Use:     "egg",
		Short:   "Commands to list and inspect eggs on a nest",
		Long:    eggHelpText,
		Example: eggHelpEx,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(NewtLogLevel, newtSilent, newtQuiet, newtVerbose)

			var err error
			NewtNest, err = cli.NewNest()
			if err != nil {
				NewtUsage(nil, err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			NewtUsage(cmd, nil)
		},
	}

	listHelpText := formatHelp(`List all of the eggs in the current nest.`)
	listHelpEx := "  newt egg list"

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List eggs in the current nest",
		Long:    listHelpText,
		Example: listHelpEx,
		Run:     eggListCmd,
	}

	eggCmd.AddCommand(listCmd)

	checkDepsHelpText := formatHelp(`Resolve all dependencies in the local 
		nest.  This command goes through all eggs currently installed, checks
		their dependencies, and prints any unresolved dependencies between 
		eggs.`)
	checkDepsHelpEx := "  newt egg checkdeps"

	checkDepsCmd := &cobra.Command{
		Use:     "checkdeps",
		Short:   "Check egg dependencies",
		Long:    checkDepsHelpText,
		Example: checkDepsHelpEx,
		Run:     eggCheckDepsCmd,
	}

	eggCmd.AddCommand(checkDepsCmd)

	huntHelpText := formatHelp(`Hunt for an egg, specified by <egg-name>.  
		The local nest, along with all remote nests (clutches) are 
		searched.  All matched eggs are shown.`)
	huntHelpEx := "  newt egg hunt <egg-name>"

	huntCmd := &cobra.Command{
		Use:     "hunt",
		Short:   "Search for egg from clutches",
		Long:    huntHelpText,
		Example: huntHelpEx,
		Run:     eggHuntCmd,
	}

	eggCmd.AddCommand(huntCmd)

	showHelpText := formatHelp(`Show the contents of the egg, specified by 
		<egg-name>.  <egg-name> is resolved using all the clutches installed
		in the current nest or, if <clutch-name> is specified, only 
		<clutch-name> will be searched.`)
	showHelpEx := "  newt egg show [<clutch-name> ] <egg-name>"

	showCmd := &cobra.Command{
		Use:     "show",
		Short:   "Show the contents of an egg.",
		Long:    showHelpText,
		Example: showHelpEx,
		Run:     eggShowCmd,
	}

	eggCmd.AddCommand(showCmd)

	installHelpText := formatHelp(`Install the egg specified by <egg-name> to 
		the local nest. <egg-name> is searched for throughout the clutches in 
		the local nest.  If <clutch-name> is specified, then only <clutch-name>
		is searched for <egg-name>.`)
	installHelpEx := "  newt egg install [<clutch-name> ] <egg-name>"

	installCmd := &cobra.Command{
		Use:     "install",
		Short:   "Install an egg",
		Long:    installHelpText,
		Example: installHelpEx,
		Run:     eggInstallCmd,
	}

	installCmd.Flags().StringVarP(&NewtBranchEgg, "branch", "b", "",
		"Branch (or tag) of the clutch to install from.")

	eggCmd.AddCommand(installCmd)

	removeHelpText := formatHelp(`Remove the egg, specified by <egg-name> from 
		the local nest.  If present the egg is taking only from the clutch 
		specified by <clutch-name>.`)
	removeHelpEx := "  newt egg remove [<clutch-name> ] <egg-name>"

	removeCmd := &cobra.Command{
		Use:     "remove",
		Short:   "Remove an egg",
		Long:    removeHelpText,
		Example: removeHelpEx,
		Run:     eggRemoveCmd,
	}

	eggCmd.AddCommand(removeCmd)

	baseCmd.AddCommand(eggCmd)
}

func nestGenerateClutchCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify name and URL to lay clutch file"))
	}

	clutchName := args[0]
	clutchUrl := args[1]

	clutch, err := cli.NewClutch(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}
	clutch.Name = clutchName
	clutch.RemoteUrl = clutchUrl

	local, err := cli.NewClutch(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := clutch.LoadFromClutch(local); err != nil {
		NewtUsage(cmd, err)
	}

	clutchStr, err := clutch.Serialize()
	if err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "%s", clutchStr)
}

func nestAddClutchCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify both name and URL to "+
				"larva install command"))
	}

	name := args[0]
	url := args[1]

	clutch, err := cli.NewClutch(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := clutch.Install(name, url, NewtBranchClutch); err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Clutch "+name+" successfully installed to Nest.\n")
}

func nestListClutchesCmd(cmd *cobra.Command, args []string) {
	clutches, err := NewtNest.GetClutches()
	if err != nil {
		NewtUsage(cmd, err)
	}

	for name, clutch := range clutches {
		cli.StatusMessage(cli.VERBOSITY_QUIET,
			"Remote clutch %s (eggshells: %d)\n", name, len(clutch.EggShells))
	}
}

func nestCreateCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify a nest name to create"))
	}

	wd, err := os.Getwd()
	if err != nil {
		NewtUsage(cmd, cli.NewNewtError(err.Error()))
	}

	nestDir := wd + "/" + args[0]
	if len(args) > 1 {
		nestDir = args[1]
	}

	tadpoleUrl := ""
	if len(args) > 2 {
		tadpoleUrl = args[2]
	}

	if err := cli.CreateNest(args[0], nestDir, tadpoleUrl); err != nil {
		NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Nest %s successfully created in %s\n", args[0], nestDir)
}

func nestShowClutchCmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		NewtUsage(cmd,
			cli.NewNewtError("Must specify a clutch name to show-clutch command"))
	}

	clutch, err := cli.NewClutch(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := clutch.Load(args[0]); err != nil {
		NewtUsage(cmd, err)
	}

	// Clutch loaded, now print out clutch information
	cli.StatusMessage(cli.VERBOSITY_QUIET, "Clutch Name: %s\n", clutch.Name)
	cli.StatusMessage(cli.VERBOSITY_QUIET, "Clutch URL: %s\n",
		clutch.RemoteUrl)

	i := 0
	for _, eggShell := range clutch.EggShells {
		i++
		cli.StatusMessage(cli.VERBOSITY_QUIET, " %s@%s", eggShell.FullName,
			eggShell.Version)
		if i%4 == 0 {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
		}
	}
	if i%4 != 0 {
		cli.StatusMessage(cli.VERBOSITY_QUIET, "\n")
	}
}

func nestAddCmds(baseCmd *cobra.Command) {
	var nestCmd *cobra.Command
	var createCmd *cobra.Command

	nestHelpText := formatHelp(`The nest commands help manage the local nest.
		A nest represents the workspace for one or more projects, each project being a
                collection of eggs (packages.)  In addition to containing eggs, a local nest contains the 
		target (build) definitions, and a list of clutches (snapshots of remote nests 
		which contain eggs that can be installed into the current nest.)`)
	nestHelpEx := "  newt nest create <nest-name> [, <nest-skele-url>]\n"
	nestHelpEx += "  newt nest list-clutches\n"
	nestHelpEx += "  newt nest show-clutch <clutch-name>\n"
	nestHelpEx += "  newt nest add-clutch <clutch-name> <clutch-url>\n"
	nestHelpEx += "  newt nest generate-clutch <clutch-name> <clutch-url>"

	nestCmd = &cobra.Command{
		Use:     "nest",
		Short:   "Commands to manage nests & clutches (remote egg repositories)",
		Long:    nestHelpText,
		Example: nestHelpEx,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(NewtLogLevel, newtSilent, newtQuiet, newtVerbose)

			var err error

			if cmd != nestCmd && cmd != createCmd {
				NewtNest, err = cli.NewNest()
				if err != nil {
					NewtUsage(cmd, err)
				}
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			NewtUsage(cmd, nil)
		},
	}

	createHelpText := formatHelp(`Create a new nest, specified by <nest-name>. 
		If the optional <nest-url> parameter is specified, then download the 
		skeleton of the nest file from that URL, instead of using the default.`)

	createHelpEx := "  newt nest create <nest-name> [, <nest-url>]\n"
	createHelpEx += "  newt nest create mynest"

	createCmd = &cobra.Command{
		Use:     "create",
		Short:   "Create a new nest",
		Long:    createHelpText,
		Example: createHelpEx,
		Run:     nestCreateCmd,
	}

	nestCmd.AddCommand(createCmd)

	generateHelpText := formatHelp(`Generate a clutch file from a snapshot 
	    of the eggs in the current directory.  generate-clutch takes two 
		arguments, the name of the current nest and the URL at which 
		the nest is located.`)

	generateHelpEx := "  newt nest generate-clutch <name> <url>\n"
	generateHelpEx += "  newt nest generate-clutch larva " +
		"https://www.github.com/mynewt/larva"

	generateCmd := &cobra.Command{
		Use:     "generate-clutch",
		Short:   "Generate a clutch file from the eggs in the current directory",
		Long:    generateHelpText,
		Example: generateHelpEx,
		Run:     nestGenerateClutchCmd,
	}

	nestCmd.AddCommand(generateCmd)

	addClutchHelpText := formatHelp(`Add a remote clutch to the current nest.
	    When search for eggs to install, the clutch specified by clutch-name
		and clutch-url will be searched for eggs that match the search.  This
		includes both direct searches with newt egg hunt, as well as resolving
		dependencies in egg.yml files.`)

	addClutchHelpEx := "  newt nest add-clutch <clutch-name> <clutch-url>\n"
	addClutchHelpEx += "  newt nest add-clutch larva " +
		"https://www.github.com/mynewt/larva"

	addClutchCmd := &cobra.Command{
		Use:     "add-clutch",
		Short:   "Add a remote clutch, and put it in the current nest",
		Long:    addClutchHelpText,
		Example: addClutchHelpEx,
		Run:     nestAddClutchCmd,
	}

	addClutchCmd.Flags().StringVarP(&NewtBranchClutch, "branch", "b", "master",
		"Branch (or tag) of the clutch to install from.")

	nestCmd.AddCommand(addClutchCmd)

	listClutchesHelpText := formatHelp(`List the clutches installed in the current
		nest.  A clutch represents a collection of eggs in a nest.  List clutches
		includes the current nest, along with any remote clutches that have been 
		added using the add-clutch command.`)

	listClutchesHelpEx := "  newt nest list-clutches"

	listClutchesCmd := &cobra.Command{
		Use:     "list-clutches",
		Short:   "List the clutches installed in the current nest",
		Long:    listClutchesHelpText,
		Example: listClutchesHelpEx,
		Run:     nestListClutchesCmd,
	}

	nestCmd.AddCommand(listClutchesCmd)

	showClutchHelpText := formatHelp(`Show information about a clutch, given by the 
		<clutch-name> parameter.  Displays the clutch name, URL and packages 
		associated with a given clutch.`)

	showClutchHelpEx := "  newt nest show-clutch <clutch-name>\n"
	showClutchHelpEx += "  newt nest show-clutch larva"

	showClutchCmd := &cobra.Command{
		Use:     "show-clutch",
		Short:   "Show an individual clutch in the current nest",
		Long:    showClutchHelpText,
		Example: showClutchHelpEx,
		Run:     nestShowClutchCmd,
	}

	nestCmd.AddCommand(showClutchCmd)

	baseCmd.AddCommand(nestCmd)
}

func parseCmds() *cobra.Command {
	newtHelpText := formatHelp(`Newt allows you to create your own embedded 
		project based on the Mynewt operating system.  Newt provides both 
		build and package management in a single tool, which allows you to 
		compose an embedded workspace, and set of projects, and then build
		the necessary artifacts from those projects.  For more information 
		on the Mynewt operating system, please visit 
		https://www.github.com/mynewt/documentation.`)
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
	eggAddCmds(newtCmd)
	nestAddCmds(newtCmd)

	return newtCmd
}

func main() {
	cmd := parseCmds()
	cmd.Execute()
}
