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

func parseTargetName(arg string) (*Target, error) {
	t := ResolveTargetName(arg)
	if t == nil {
		return nil, util.NewNewtError("Unknown target: " + arg)
	}

	return t, nil
}

func targetShowCmd(cmd *cobra.Command, args []string) {
	proj := project.GetProject()
	err := proj.LoadPackageList()
	if err != nil {
		cli.NewtUsage(cmd, err)
	}

	targetNames := []string{}
	if len(args) == 0 {
		for name, _ := range GetTargets() {
			targetNames = append(targetNames, name)
		}
	} else {
		targetSlice, err := ResolveTargetNames(args...)
		if err != nil {
			cli.NewtUsage(cmd, err)
		}

		for _, t := range targetSlice {
			targetNames = append(targetNames, t.Name())
		}
	}

	sort.Strings(targetNames)

	for _, name := range targetNames {
		cli.StatusMessage(cli.VERBOSITY_QUIET, name+"\n")

		target := GetTargets()[name]
		keys := []string{}
		for k, _ := range target.Vars {
			keys = append(keys, k)
		}

		sort.Strings(keys)
		for _, k := range keys {
			cli.StatusMessage(cli.VERBOSITY_QUIET, "    %s=%s\n", k,
				target.Vars[k])
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
	if len(args) < 2 {
		cli.NewtUsage(cmd,
			util.NewNewtError("Must specify at least two arguments "+
				"(target-name & k=v) to set"))
	}

	// Parse target name.
	t, err := parseTargetName(args[0])
	if err != nil {
		cli.NewtUsage(cmd, err)
	}

	// Parse series of k=v pairs.  If an argument doesn't contain a '='
	// character, display the valid values for the variable and quit.
	vars := [][]string{}
	for i := 1; i < len(args); i++ {
		kv := strings.SplitN(args[i], "=", 2)
		if !strings.HasPrefix(kv[0], "target.") {
			kv[0] = "target." + kv[0]
		}

		if len(kv) == 1 {
			// User entered a variable name without a value.  Display valid
			// values for the specified variable.
			err := showValidSettings(kv[0])
			if err != nil {
				cli.NewtUsage(cmd, err)
			}
			return
		}

		vars = append(vars, kv)
	}

	// Set each specified variable in the target.
	for _, kv := range vars {
		if kv[1] == "" {
			// User specified empty value; delete variable.
			delete(t.Vars, kv[0])
		} else {
			// Assign value to specified variable.
			t.Vars[kv[0]] = kv[1]
		}
	}

	if err := t.Save(); err != nil {
		cli.NewtUsage(cmd, err)
	}

	for _, kv := range vars {
		if kv[1] == "" {
			cli.StatusMessage(cli.VERBOSITY_DEFAULT,
				"Target %s successfully unset %s\n", t.Name(), kv[0])
		} else {
			cli.StatusMessage(cli.VERBOSITY_DEFAULT,
				"Target %s successfully set %s to %s\n", t.Name(), kv[0],
				kv[1])
		}
	}
}

func targetCreateCmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		cli.NewtUsage(cmd, util.NewNewtError("Missing target name"))
	}

	tName := args[0]

	// "Naked" target names go in the "targets/" directory.
	if !strings.Contains(tName, "/") {
		tName = "targets/" + tName
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Creating target "+tName+"\n")

	t := ResolveTargetName(tName)
	if t != nil {
		cli.NewtUsage(cmd, util.NewNewtError(
			"Target already exists; cannot create target with same name."))
	}

	repo := project.GetProject().LocalRepo()
	pack := pkg.NewLocalPackage(repo, repo.Path()+"/"+tName)
	pack.SetName(tName)
	pack.SetType(pkg.PACKAGE_TYPE_TARGET)

	t = NewTarget(pack)
	err := t.Save()
	if err != nil {
		cli.NewtUsage(nil, err)
	} else {
		cli.StatusMessage(cli.VERBOSITY_DEFAULT,
			"Target %s successfully created!\n", tName)
	}
}

func targetDelOne(t *Target) error {
	if !cli.Force {
		// Determine if the target directory contains extra user files.  If it
		// does, a prompt (or force) is required to delete it.
		userFiles, err := t.ContainsUserFiles()
		if err != nil {
			return err
		}

		if userFiles {
			scanner := bufio.NewScanner(os.Stdin)
			fmt.Printf("Target directory %s contains some extra content; "+
				"delete anyway? (y/N): ", t.basePkg.Name())
			rc := scanner.Scan()
			if !rc || strings.ToLower(scanner.Text()) != "y" {
				return nil
			}
		}
	}

	if err := t.Delete(); err != nil {
		return err
	}

	cli.StatusMessage(cli.VERBOSITY_DEFAULT,
		"Target %s successfully removed\n", t.Name())

	return nil
}

func targetDelCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, util.NewNewtError("Must specify at least one "+
			"target to delete"))
	}

	targets, err := ResolveTargetNames(args...)
	if err != nil {
		cli.NewtUsage(cmd, err)
	}

	for _, t := range targets {
		if err := targetDelOne(t); err != nil {
			cli.NewtUsage(cmd, err)
		}
	}
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
