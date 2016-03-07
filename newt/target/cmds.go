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

package target

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

// Type for sorting an array of target pointers alphabetically by name.
type ByName []*Target

func (a ByName) Len() int           { return len(a) }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool { return a[i].Package().Name() < a[j].Package().Name() }

func targetShowCmd(cmd *cobra.Command, args []string) {
	proj := project.GetProject()
	proj.LoadPackageList()

	dispTarget := ""
	if len(args) == 1 {
		dispTarget = args[0]
	}

	targetNames := []string{}
	for name, _ := range GetTargets() {
		targetNames = append(targetNames, name)
	}
	sort.Strings(targetNames)

	for _, name := range targetNames {
		if dispTarget == "" || dispTarget == name {
			cli.StatusMessage(cli.VERBOSITY_QUIET, name+"\n")

			target := GetTargets()[name]
			keys := []string{}
			for k, _ := range target.Vars {
				keys = append(keys, k)
			}

			sort.Strings(keys)
			for _, k := range keys {
				cli.StatusMessage(cli.VERBOSITY_QUIET,
					"    %s=%s\n", k, target.Vars[k])
			}
		}
	}
}

func showValidSettings(varName string) error {
	var err error = nil
	var values []string

	fmt.Printf("Valid values for target variable \"%s\":\n", varName)

	values, err = VarValues(varName)
	if err != nil {
		return err
	}

	for _, value := range values {
		fmt.Printf("    %s\n", value)
	}

	return nil
}

func targetSetCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		cli.NewtUsage(cmd,
			util.NewNewtError("Must specify two arguments "+
				"(target-name & k=v) to set"))
	}

	t := GetTargets()[args[0]]
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("Unknown target"))
	}

	ar := strings.SplitN(args[1], "=", 2)
	if !strings.HasPrefix(ar[0], "target.") {
		ar[0] = "target." + ar[0]
	}

	if len(ar) == 1 {
		// User entered a variable name without a value.  Display valid values
		// for the specified variable.
		err := showValidSettings(ar[0])
		if err != nil {
			cli.NewtUsage(cmd, err)
		}
	} else {
		if ar[1] == "" {
			// User specified empty value; delete variable.
			delete(t.Vars, ar[0])
		} else {
			// Assign value to specified variable.
			t.Vars[ar[0]] = ar[1]
		}

		if err := t.Save(); err != nil {
			cli.NewtUsage(cmd, err)
		}

		if ar[1] == "" {
			cli.StatusMessage(cli.VERBOSITY_DEFAULT,
				"Target %s successfully unset %s\n", args[0], ar[0])
		} else {
			cli.StatusMessage(cli.VERBOSITY_DEFAULT,
				"Target %s successfully set %s to %s\n", args[0], ar[0], ar[1])
		}
	}
}

func targetCreateCmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		cli.NewtUsage(cmd, util.NewNewtError("Wrong number of args to create cmd."))
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Creating target "+args[0]+"\n")

	t := GetTargets()[args[0]]
	if t != nil {
		cli.NewtUsage(cmd, util.NewNewtError(
			"Target already exists, cannot create target with same name."))
	}

	repo := project.GetProject().LocalRepo()
	pack := pkg.NewLocalPackage(repo, repo.BasePath+"/"+args[0])
	pack.SetName(args[0])
	pack.SetType(pkg.PACKAGE_TYPE_TARGET)
	pack.SetVers(&pkg.Version{0, 0, 1})

	t = NewTarget(pack)
	err := t.Save()
	if err != nil {
		cli.NewtUsage(nil, err)
	} else {
		cli.StatusMessage(cli.VERBOSITY_DEFAULT,
			"Target %s successfully created!\n", args[0])
	}
}

func targetDelCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, util.NewNewtError("Must specify target to delete"))
	}

	t := GetTargets()[args[0]]
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("Target does not exist"))
	}

	if !cli.Force {
		// Determine if the target directory contains extra user files.  If it
		// does, a prompt (or force) is required to delete it.
		userFiles, err := t.ContainsUserFiles()
		if err != nil {
			cli.NewtUsage(cmd, err)
		}

		if userFiles {
			scanner := bufio.NewScanner(os.Stdin)
			fmt.Printf("Target directory %s contains some extra content; "+
				"delete anyway? (y/N): ", t.basePkg.Name())
			rc := scanner.Scan()
			if !rc || strings.ToLower(scanner.Text()) != "y" {
				return
			}
		}
	}

	// Clean target prior to deletion; ignore errors during clean.
	//t.BuildClean(false)

	if err := t.Delete(); err != nil {
		cli.NewtUsage(cmd, err)
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Target %s successfully removed\n", args[0])
}

func AddCommands(cmd *cobra.Command) {
	targetHelpText := ""
	targetHelpEx := ""
	targetCmd := &cobra.Command{
		Use:     "target",
		Short:   "Command for manipulating targets",
		Long:    targetHelpText,
		Example: targetHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	cmd.AddCommand(targetCmd)

	showHelpText := "Show all the variables for the target specified " +
		"by <target-name>."
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

	setHelpText := "Set a target variable (<var-name>) on target " +
		"<target-name> to value <value>."
	setHelpEx := "  newt target set <target-name> <var-name>=<value>\n"
	setHelpEx += "  newt target set my_target1 var_name=value\n"
	setHelpEx += "  newt target set my_target1 arch=cortex_m4\n"
	setHelpEx += "  newt target set my_target1 var_name   (display valid values for <var_name>)"

	setCmd := &cobra.Command{
		Use:     "set",
		Short:   "Set target configuration variable",
		Long:    setHelpText,
		Example: setHelpEx,
		Run:     targetSetCmd,
	}

	targetCmd.AddCommand(setCmd)

	createHelpText := "Create a target specified by <target-name>."
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

	delHelpText := "Delete the target specified by <target-name>."
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
}
