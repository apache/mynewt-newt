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
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

var targetForce bool = false

func resolveExistingTargetArg(arg string) (*target.Target, error) {
	t := ResolveTarget(arg)
	if t == nil {
		return nil, util.NewNewtError("Unknown target: " + arg)
	}

	return t, nil
}

// Tells you if a target's directory contains extra user files (i.e., files
// other than pkg.yml).
func targetContainsUserFiles(t *target.Target) (bool, error) {
	contents, err := ioutil.ReadDir(t.Package().BasePath())
	if err != nil {
		return false, err
	}

	userFiles := false
	for _, node := range contents {
		name := node.Name()
		if name != "." && name != ".." &&
			name != pkg.PACKAGE_FILE_NAME && name != target.TARGET_FILENAME {

			userFiles = true
			break
		}
	}

	return userFiles, nil
}

func pkgVarSliceString(pack *pkg.LocalPackage, key string) string {
	features := pack.Viper.GetStringSlice(key)
	sort.Strings(features)

	var buffer bytes.Buffer
	for _, f := range features {
		buffer.WriteString(f)
		buffer.WriteString(" ")
	}
	return buffer.String()
}

func targetShowCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	targetNames := []string{}
	if len(args) == 0 {
		for name, _ := range target.GetTargets() {
			// Don't display the special unittest target; this is used
			// internally by newt, so the user doesn't need to know about it.
			// XXX: This is a hack; come up with a better solution for unit
			// testing.
			if !strings.HasSuffix(name, "/unittest") {
				targetNames = append(targetNames, name)
			}
		}
	} else {
		targetSlice, err := ResolveTargets(args...)
		if err != nil {
			NewtUsage(cmd, err)
		}

		for _, t := range targetSlice {
			targetNames = append(targetNames, t.FullName())
		}
	}

	sort.Strings(targetNames)

	for _, name := range targetNames {
		kvPairs := map[string]string{}

		util.StatusMessage(util.VERBOSITY_DEFAULT, name+"\n")

		target := target.GetTargets()[name]
		for k, v := range target.Vars {
			kvPairs[strings.TrimPrefix(k, "target.")] = v
		}

		// A few variables come from the base package rather than the target.
		kvPairs["features"] = pkgVarSliceString(target.Package(),
			"pkg.features")
		kvPairs["cflags"] = pkgVarSliceString(target.Package(), "pkg.cflags")
		kvPairs["lflags"] = pkgVarSliceString(target.Package(), "pkg.lflags")
		kvPairs["aflags"] = pkgVarSliceString(target.Package(), "pkg.aflags")

		keys := []string{}
		for k, _ := range kvPairs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := kvPairs[k]
			if len(val) > 0 {
				util.StatusMessage(util.VERBOSITY_DEFAULT, "    %s=%s\n",
					k, kvPairs[k])
			}
		}
	}
}

func targetSetCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) < 2 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify at least two arguments "+
				"(target-name & k=v) to set"))
	}

	// Parse target name.
	t, err := resolveExistingTargetArg(args[0])
	if err != nil {
		NewtUsage(cmd, err)
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
			// User entered a variable name without a value.
			NewtUsage(cmd, nil)
		}

		// Trim trailing slash from value.  This is necessary when tab
		// completion is used to fill in the value.
		kv[1] = strings.TrimSuffix(kv[1], "/")

		vars = append(vars, kv)
	}

	// Set each specified variable in the target.
	for _, kv := range vars {
		// A few variables are special cases; they get set in the base package
		// instead of the target.
		if kv[0] == "target.features" ||
			kv[0] == "target.cflags" ||
			kv[0] == "target.lflags" ||
			kv[0] == "target.aflags" {

			kv[0] = "pkg." + strings.TrimPrefix(kv[0], "target.")
			if kv[1] == "" {
				// User specified empty value; delete variable.
				t.Package().Viper.Set(kv[0], nil)
			} else {
				t.Package().Viper.Set(kv[0], strings.Fields(kv[1]))
			}
		} else {
			if kv[1] == "" {
				// User specified empty value; delete variable.
				delete(t.Vars, kv[0])
			} else {
				// Assign value to specified variable.
				t.Vars[kv[0]] = kv[1]
			}
		}
	}

	if err := t.Save(); err != nil {
		NewtUsage(cmd, err)
	}

	for _, kv := range vars {
		if kv[1] == "" {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"Target %s successfully unset %s\n", t.FullName(), kv[0])
		} else {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"Target %s successfully set %s to %s\n", t.FullName(), kv[0],
				kv[1])
		}
	}
}

func targetCreateCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) != 1 {
		NewtUsage(cmd, util.NewNewtError("Missing target name"))
	}

	pkgName, err := ResolveNewTargetName(args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	repo := project.GetProject().LocalRepo()
	pack := pkg.NewLocalPackage(repo, repo.Path()+"/"+pkgName)
	pack.SetName(pkgName)
	pack.SetType(pkg.PACKAGE_TYPE_TARGET)

	t := target.NewTarget(pack)
	err = t.Save()
	if err != nil {
		NewtUsage(nil, err)
	} else {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Target %s successfully created\n", pkgName)
	}
}

func targetDelOne(t *target.Target) error {
	if !targetForce {
		// Determine if the target directory contains extra user files.  If it
		// does, a prompt (or force) is required to delete it.
		userFiles, err := targetContainsUserFiles(t)
		if err != nil {
			return err
		}

		if userFiles {
			scanner := bufio.NewScanner(os.Stdin)
			fmt.Printf("Target directory %s contains some extra content; "+
				"delete anyway? (y/N): ", t.Package().BasePath())
			rc := scanner.Scan()
			if !rc || strings.ToLower(scanner.Text()) != "y" {
				return nil
			}
		}
	}

	if err := os.RemoveAll(t.Package().BasePath()); err != nil {
		return util.NewNewtError(err.Error())
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Target %s successfully deleted.\n", t.FullName())

	return nil
}

func targetDelCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify at least one "+
			"target to delete"))
	}

	targets, err := ResolveTargets(args...)
	if err != nil {
		NewtUsage(cmd, err)
	}

	for _, t := range targets {
		if err := targetDelOne(t); err != nil {
			NewtUsage(cmd, err)
		}
	}
}

func targetCopyCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) != 2 {
		NewtUsage(cmd, util.NewNewtError("Must specify exactly one "+
			"source target and one destination target"))
	}

	srcTarget, err := resolveExistingTargetArg(args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	dstName, err := ResolveNewTargetName(args[1])
	if err != nil {
		NewtUsage(cmd, err)
	}

	// Copy the source target's base package and adjust the fields which need
	// to change.
	dstTarget := srcTarget.Clone(project.GetProject().LocalRepo(), dstName)

	// Save the new target.
	err = dstTarget.Save()
	if err != nil {
		NewtUsage(nil, err)
	} else {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Target successfully copied; %s --> %s\n",
			srcTarget.FullName(), dstTarget.FullName())
	}
}

func printSetting(entry syscfg.CfgEntry) {
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"  * Setting: %s\n", entry.Name)

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"    * Description: %s\n", entry.Description)

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"    * Value: %s", entry.Value)

	unfixed := syscfg.UnfixedValue(entry)
	if unfixed != entry.Value {
		util.StatusMessage(util.VERBOSITY_DEFAULT, " [%s]", unfixed)
	}
	util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")

	if len(entry.History) > 1 {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"    * Overridden: ")
		for i := 1; i < len(entry.History); i++ {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "%s, ",
				entry.History[i].Source.Name())
		}
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"default=%s\n", entry.History[0].Value)
	}
}

func printPkgCfg(pkgName string, cfg syscfg.Cfg, entries []syscfg.CfgEntry) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "* PACKAGE: %s\n", pkgName)

	settingNames := make([]string, len(entries))
	for i, entry := range entries {
		settingNames[i] = entry.Name
	}
	sort.Strings(settingNames)

	for _, name := range settingNames {
		printSetting(cfg[name])
	}
}

func printCfg(cfg syscfg.Cfg) {
	pkgNameEntryMap := syscfg.EntriesByPkg(cfg)

	pkgNames := make([]string, 0, len(pkgNameEntryMap))
	for pkgName, _ := range pkgNameEntryMap {
		pkgNames = append(pkgNames, pkgName)
	}
	sort.Strings(pkgNames)

	for i, pkgName := range pkgNames {
		if i > 0 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
		printPkgCfg(pkgName, cfg, pkgNameEntryMap[pkgName])
	}
}

func targetConfigCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) != 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target name"))
	}

	t, err := resolveExistingTargetArg(args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		NewtUsage(nil, err)
	}

	if err := b.PrepBuild(); err != nil {
		NewtUsage(nil, err)
	}

	printCfg(b.App.Cfg)
}

func AddTargetCommands(cmd *cobra.Command) {
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
	showCmd.ValidArgs = targetList()
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
	delCmd.PersistentFlags().BoolVarP(&targetForce, "force", "f", false,
		"Force delete of targets with user files without prompt")

	targetCmd.AddCommand(delCmd)

	copyHelpText := "Create a new target by cloning <src-target>."
	copyHelpEx := "  newt target copy <src-target> <dst-target>\n"
	copyHelpEx += "  newt target copy blinky_sim my_target"

	copyCmd := &cobra.Command{
		Use:       "copy",
		Short:     "Copy target",
		Long:      copyHelpText,
		Example:   copyHelpEx,
		Run:       targetCopyCmd,
		ValidArgs: targetList(),
	}

	targetCmd.AddCommand(copyCmd)

	configHelpText := "View a target's system configuration."

	configCmd := &cobra.Command{
		Use:       "config <target-name>",
		Short:     "View target system configuration",
		Long:      configHelpText,
		Run:       targetConfigCmd,
		ValidArgs: targetList(),
	}

	targetCmd.AddCommand(configCmd)
}
