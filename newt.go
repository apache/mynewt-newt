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
	"github.com/mynewt/newt/cli"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var ExitOnFailure bool = false
var ExportAll bool = false
var ImportAll bool = false
var NewtVersion string = "1.0"
var NewtLogLevel string = ""
var NewtNest *cli.Nest

func NewtUsage(cmd *cobra.Command, err error) {
	if err != nil {
		sErr := err.(*cli.NewtError)
		log.Printf("[DEBUG] %s", sErr.StackTrace)
		fmt.Println("Error: ", sErr.Text)
	}

	if cmd != nil {
		cmd.Usage()
	}
	os.Exit(1)
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

	fmt.Printf("Target %s successfully set %s to %s\n", args[0],
		ar[0], ar[1])
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

	fmt.Printf("Target %s successfully unset %s\n", args[0], args[1])
}

func targetShowCmd(cmd *cobra.Command, args []string) {
	dispSect := ""
	if len(args) == 1 {
		dispSect = args[0]
	}

	targets, err := cli.GetTargets(NewtNest)
	if err != nil {
		NewtUsage(cmd, err)
	}

	for _, target := range targets {
		if dispSect == "" || dispSect == target.Vars["name"] {
			fmt.Println(target.Vars["name"])
			vars := target.GetVars()
			for k, v := range vars {
				fmt.Printf("	%s: %s\n", k, v)
			}
		}
	}
}

func targetCreateCmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		NewtUsage(cmd, cli.NewNewtError("Wrong number of args to create cmd."))
	}

	fmt.Println("Creating target " + args[0])

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
		fmt.Println(err)
	} else {
		fmt.Printf("Target %s sucessfully created!\n", args[0])
	}
}

func targetBuildCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target to build"))
	}

	t, err := cli.LoadTarget(NewtNest, args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	if len(args) > 1 && args[1] == "clean" {
		if len(args) > 2 && args[2] == "all" {
			err = t.BuildClean(true)
		} else {
			err = t.BuildClean(false)
		}
	} else {
		err = t.Build()
	}

	if err != nil {
		NewtUsage(cmd, err)
	} else {
		fmt.Println("Successfully run!")
	}
}

func targetDelCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target to delete"))
	}

	t, err := cli.LoadTarget(NewtNest, args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := t.Remove(); err != nil {
		NewtUsage(cmd, err)
	}

	fmt.Printf("Target %s successfully removed\n", args[0])
}

func targetTestCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, cli.NewNewtError("Must specify target to build"))
	}

	t, err := cli.LoadTarget(NewtNest, args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	if len(args) > 1 && args[1] == "clean" {
		if len(args) > 2 && args[2] == "all" {
			err = t.Test("testclean", true)
		} else {
			err = t.Test("testclean", false)
		}
	} else {
		err = t.Test("test", ExitOnFailure)
	}

	if err != nil {
		NewtUsage(cmd, err)
	} else {
		fmt.Println("Successfully run!")
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

	fmt.Println("Target(s) successfully imported!")
}

func targetAddCmds(base *cobra.Command) {
	targetCmd := &cobra.Command{
		Use:   "target",
		Short: "Set and view target information",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(NewtLogLevel)

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

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Set target configuration variable",
		Run:   targetSetCmd,
	}

	targetCmd.AddCommand(setCmd)

	unsetCmd := &cobra.Command{
		Use:   "unset",
		Short: "Unset target configuration variable",
		Run:   targetUnsetCmd,
	}

	targetCmd.AddCommand(unsetCmd)

	delCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete target",
		Run:   targetDelCmd,
	}

	targetCmd.AddCommand(delCmd)

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a target",
		Run:   targetCreateCmd,
	}

	targetCmd.AddCommand(createCmd)

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "View target configuration variables",
		Run:   targetShowCmd,
	}

	targetCmd.AddCommand(showCmd)

	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build target",
		Run:   targetBuildCmd,
	}

	targetCmd.AddCommand(buildCmd)

	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test target",
		Run:   targetTestCmd,
	}

	targetCmd.AddCommand(testCmd)

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export target",
		Run:   targetExportCmd,
	}

	exportCmd.PersistentFlags().BoolVarP(&ExportAll, "export-all", "a", false,
		"If present, export all targets")

	targetCmd.AddCommand(exportCmd)

	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import target",
		Run:   targetImportCmd,
	}

	importCmd.PersistentFlags().BoolVarP(&ImportAll, "import-all", "a", false,
		"If present, import all targets")

	targetCmd.AddCommand(importCmd)

	base.AddCommand(targetCmd)
}

func dispEgg(egg *cli.Egg) error {
	fmt.Printf("Egg %s, version %s\n", egg.FullName, egg.Version)
	fmt.Printf("  path: %s\n", filepath.Clean(egg.BasePath))
	if egg.Capabilities != nil {
		fmt.Printf("  capabilities: ")
		caps, err := egg.GetCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			fmt.Printf("%s ", capability)
		}
		fmt.Printf("\n")
	}
	if len(egg.Deps) > 0 {
		fmt.Printf("  deps: ")
		for _, dep := range egg.Deps {
			if dep == nil {
				continue
			}
			fmt.Printf("%s ", dep)
		}
		fmt.Printf("\n")
	}

	if egg.LinkerScript != "" {
		fmt.Printf("  linkerscript: %s\n", egg.LinkerScript)
	}
	return nil
}

func dispEggShell(eggShell *cli.EggShell) error {
	fmt.Printf("Egg %s from clutch %s, version %s\n", eggShell.FullName,
		eggShell.Clutch.Name,
		eggShell.Version)

	if eggShell.Caps != nil {
		fmt.Printf("  capabilities: ")
		caps, err := eggShell.GetCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			fmt.Printf("%s ", capability)
		}
		fmt.Printf("\n")
	}
	if len(eggShell.Deps) > 0 {
		fmt.Printf("  deps: ")
		for _, dep := range eggShell.Deps {
			if dep == nil {
				continue
			}
			fmt.Printf("%s ", dep)
		}
		fmt.Printf("\n")
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

	if err := eggMgr.CheckDeps(); err != nil {
		NewtUsage(cmd, err)
	}

	fmt.Println("Dependencies successfully resolved!")
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
			fmt.Printf("Installed egg %s@%s\n", egg.FullName,
				egg.Version)
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
				fmt.Printf("Clutch %s has egg %s@%s\n",
					clutch.Name, eggShell.FullName,
					eggShell.Version)
				found = true
			}
		}
	}

	if found == false {
		fmt.Println("No egg found!")
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

	eggShell = nil
	for _, tmpClutch := range clutches {
		if clutchName == "" || tmpClutch.Name == clutchName {
			tmpEggShell, err := tmpClutch.ResolveEggShellName(eggName)
			if err != nil && eggShell != nil {
				NewtUsage(cmd,
					cli.NewNewtError(fmt.Sprintf("Ambiguous source "+
						"egg %s in clutches %s and %s",
						eggName, clutch.Name, tmpClutch.Name)))
			}
			eggShell = tmpEggShell
			clutch = tmpClutch
		}
	}

	if eggShell == nil {
		NewtUsage(cmd,
			cli.NewNewtError(fmt.Sprintf("Can't find egg with name %s",
				eggName)))
	}

	err = eggShell.Download()
	if err != nil {
		NewtUsage(cmd, err)
	}
	fmt.Println("Installed successfully!")
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
	fmt.Println("Removed successfully!")
}

func eggAddCmds(baseCmd *cobra.Command) {
	eggCmd := &cobra.Command{
		Use:   "egg",
		Short: "Commands to list and inspect eggs on a nest",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(NewtLogLevel)

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

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List eggs in the current nest",
		Run:   eggListCmd,
	}

	eggCmd.AddCommand(listCmd)

	checkDepsCmd := &cobra.Command{
		Use:   "checkdeps",
		Short: "Check egg dependencies",
		Run:   eggCheckDepsCmd,
	}

	eggCmd.AddCommand(checkDepsCmd)

	huntCmd := &cobra.Command{
		Use:   "hunt",
		Short: "Search for egg from clutches",
		Run:   eggHuntCmd,
	}

	eggCmd.AddCommand(huntCmd)

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show what we know of an egg",
		Run:   eggShowCmd,
	}

	eggCmd.AddCommand(showCmd)

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install an egg",
		Run:   eggInstallCmd,
	}

	eggCmd.AddCommand(installCmd)

	removeCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove an egg",
		Run:   eggRemoveCmd,
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

	fmt.Print(clutchStr)
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

	if err := clutch.Install(name, url); err != nil {
		NewtUsage(cmd, err)
	}

	fmt.Println("Clutch " + name + " sucessfully installed to Nest.")
}

func nestListClutchesCmd(cmd *cobra.Command, args []string) {
	clutches, err := NewtNest.GetClutches()
	if err != nil {
		NewtUsage(cmd, err)
	}

	for name, clutch := range clutches {
		fmt.Printf("Remote clutch %s (eggshells: %d)\n", name,
			len(clutch.EggShells))
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

	fmt.Printf("Nest %s successfully created in %s\n", args[0], nestDir)
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
	fmt.Printf("Clutch Name: %s\n", clutch.Name)
	fmt.Printf("Clutch URL: %s\n", clutch.RemoteUrl)

	i := 0
	for _, eggShell := range clutch.EggShells {
		i++
		fmt.Printf(" %s@%s", eggShell.FullName, eggShell.Version)
		if i%4 == 0 {
			fmt.Printf("\n")
		}
	}
	if i%4 != 0 {
		fmt.Printf("\n")
	}
}

func nestAddCmds(baseCmd *cobra.Command) {
	nestCmd := &cobra.Command{
		Use:   "nest",
		Short: "Commands to manage nests & clutches (remote egg repositories)",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(NewtLogLevel)

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

	createCmd := &cobra.Command{
		Use:              "create",
		Short:            "Create a new nest",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
		Run:              nestCreateCmd,
	}

	nestCmd.AddCommand(createCmd)

	generateCmd := &cobra.Command{
		Use:   "generate-clutch",
		Short: "Generate a clutch file from the eggs in the current directory",
		Run:   nestGenerateClutchCmd,
	}

	nestCmd.AddCommand(generateCmd)

	addClutchCmd := &cobra.Command{
		Use:   "add-clutch",
		Short: "Add a remote clutch, and put it in the current nest",
		Run:   nestAddClutchCmd,
	}

	nestCmd.AddCommand(addClutchCmd)

	listClutchesCmd := &cobra.Command{
		Use:   "list-clutches",
		Short: "List the clutches installed in the current nest",
		Run:   nestListClutchesCmd,
	}

	nestCmd.AddCommand(listClutchesCmd)

	showClutchCmd := &cobra.Command{
		Use:   "show-clutch",
		Short: "Show an individual clutch in the current nest",
		Run:   nestShowClutchCmd,
	}

	nestCmd.AddCommand(showClutchCmd)

	baseCmd.AddCommand(nestCmd)
}

func parseCmds() *cobra.Command {
	newtCmd := &cobra.Command{
		Use:   "newt",
		Short: "Newt is a tool to help you compose and build your own OS",
		Long: `Newt allows you to create your own embedded project based on 
			the Newt operating system`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	newtCmd.PersistentFlags().IntVarP(&cli.Verbosity, "verbosity", "v",
		cli.VERBOSITY_DEFAULT, "How verbose the Newt tool should be "+
			"about it's operation")
	newtCmd.PersistentFlags().StringVarP(&NewtLogLevel, "loglevel", "l",
		"WARN", "Log level, defaults to WARN.")

	versCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the Newt version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Newt version: ", NewtVersion)
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
