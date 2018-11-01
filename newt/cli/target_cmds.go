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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/logcfg"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/resolve"
	"mynewt.apache.org/newt/newt/stage"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/sysdown"
	"mynewt.apache.org/newt/newt/sysinit"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/val"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

var amendDelete bool = false

// target variables that can have values amended with the amend command.
var amendVars = []string{"aflags", "cflags", "cxxflags", "lflags", "syscfg"}

var setVars = []string{"aflags", "app", "build_profile", "bsp", "cflags",
	"cxxflags", "lflags", "loader", "syscfg"}

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
	vals := pack.PkgY.GetValStringSlice(key, nil)
	sort.Strings(vals)
	var buffer bytes.Buffer
	for _, v := range vals {
		buffer.WriteString(v)
		buffer.WriteString(" ")
	}
	return buffer.String()
}

//Process amend command for syscfg target variable
func amendSysCfg(value string, t *target.Target) error {
	// Get the current syscfg.vals name-value pairs
	sysVals := t.Package().SyscfgY.GetValStringMapString("syscfg.vals", nil)

	// Convert the input syscfg into name-value pairs
	amendSysVals, err := syscfg.KeyValueFromStr(value)
	if err != nil {
		return err
	}
	// Have current syscfg.vals in syscfg.yml file
	if sysVals != nil {
		// Either delete syscfg variable or replace with new value
		for k, v := range amendSysVals {
			if amendDelete {
				delete(sysVals, k)
			} else {
				sysVals[k] = v
			}
		}
	} else {
		// No syscfg.vals in syscfg.yml file. Use all the new
		// syscfg name-value  pairs if not deleting
		if !amendDelete {
			sysVals = amendSysVals
		}
	}

	itfMap := util.StringMapStringToItfMapItf(sysVals)
	t.Package().SyscfgY.Replace("syscfg.vals", itfMap)
	return nil
}

//Process amend command for aflags, cflags, cxxflags, and lflags target variables.
func amendBuildFlags(kv []string, t *target.Target) error {
	pkgVar := "pkg." + kv[0]
	curFlags := t.Package().PkgY.GetValStringSlice(pkgVar, nil)
	amendFlags := strings.Fields(kv[1])

	newFlags := []string{}
	exist := false

	// add flags
	if !amendDelete {
		newFlags = curFlags
		for _, amendVal := range amendFlags {
			exist = false
			for _, curVal := range curFlags {
				if amendVal == curVal {
					exist = true
				}
			}
			// Add flag if flag is not already set
			if !exist {
				newFlags = append(newFlags, amendVal)
			}
		}
	} else {
		// Delete Flag if it exist.
		for _, curVal := range curFlags {
			exist = false
			for _, deleteVal := range amendFlags {
				if deleteVal == curVal {
					exist = true
					break
				}
			}
			// Not deleting this flag, add it to the set of new
			// flags to save
			if !exist {
				newFlags = append(newFlags, curVal)
			}
		}
	}
	t.Package().PkgY.Replace(pkgVar, newFlags)
	return nil
}

func targetShowCmd(cmd *cobra.Command, args []string) {
	TryGetProject()
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
		settings := target.TargetY.AllSettingsAsStrings()
		for k, v := range settings {
			kvPairs[strings.TrimPrefix(k, "target.")] = v
		}

		// A few variables come from the base package rather than the target.
		kvPairs["syscfg"] = syscfg.KeyValueToStr(
			target.Package().SyscfgY.GetValStringMapString("syscfg.vals", nil))
		kvPairs["cflags"] = pkgVarSliceString(target.Package(), "pkg.cflags")
		kvPairs["cxxflags"] = pkgVarSliceString(target.Package(), "pkg.cxxflags")
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

func targetCmakeCmd(cmd *cobra.Command, args []string) {
	TryGetProject()

	// Verify and resolve each specified package.
	targets, err := ResolveTargets(args...)
	if err != nil {
		NewtUsage(cmd, err)
		return
	}

	if len(targets) != 1 {
		NewtUsage(cmd, err)
		return
	}

	err = builder.CMakeTargetGenerate(targets[0])
	if err != nil {
		NewtUsage(nil, err)
	}
}

func targetSetCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify at least two arguments "+
				"(target-name & k=v) to set"))
	}

	TryGetProject()

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
		key := strings.TrimPrefix(kv[0], "target.")
		supported := false
		for _, v := range setVars {
			if key == v {
				supported = true
				break
			}
		}

		if !supported {
			NewtUsage(cmd,
				util.NewNewtError("Not a valid variable: "+key))
		}
		if !strings.HasPrefix(kv[0], "target.") {
			kv[0] = "target." + kv[0]
		}

		// Make sure it is a valid variable.

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
		if kv[0] == "target.syscfg" {
			t.Package().SyscfgY = ycfg.YCfg{}
			kv, err := syscfg.KeyValueFromStr(kv[1])
			if err != nil {
				NewtUsage(cmd, err)
			}

			itfMap := util.StringMapStringToItfMapItf(kv)
			t.Package().SyscfgY.Replace("syscfg.vals", itfMap)
		} else if kv[0] == "target.cflags" ||
			kv[0] == "target.cxxflags" ||
			kv[0] == "target.lflags" ||
			kv[0] == "target.aflags" {

			kv[0] = "pkg." + strings.TrimPrefix(kv[0], "target.")
			if kv[1] == "" {
				// User specified empty value; delete variable.
				t.Package().PkgY.Replace(kv[0], nil)
			} else {
				t.Package().PkgY.Replace(kv[0], strings.Fields(kv[1]))
			}
		} else {
			if kv[1] == "" {
				// User specified empty value; delete variable.
				t.TargetY.Delete(kv[0])
			} else {
				// Assign value to specified variable.
				t.TargetY.Replace(kv[0], kv[1])
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

func targetAmendCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify at least two arguments "+
				"(target-name & variable=value) to append"))
	}

	TryGetProject()

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
		// Check that the variable can have values appended.
		valid := false
		for _, v := range amendVars {
			if kv[0] == v {
				valid = true
				break
			}
		}
		if !valid {
			NewtUsage(cmd,
				util.NewNewtError("Cannot amend values for "+kv[0]))
		}

		if len(kv) == 1 {
			// User entered a variable name without a '='
			NewtUsage(cmd, nil)
		}
		kv[1] = strings.TrimSpace(kv[1])
		if kv[1] == "" {
			NewtUsage(cmd,
				util.NewNewtError("Must provide a value to"+
					" append for variable "+kv[0]))

		}
		// Trim trailing slash from value.  This is necessary when tab
		// completion is used to fill in the value.
		kv[1] = strings.TrimSuffix(kv[1], "/")
		vars = append(vars, kv)
	}
	for _, kv := range vars {
		if kv[0] == "syscfg" {
			err = amendSysCfg(kv[1], t)
			if err != nil {
				NewtUsage(cmd, err)
			}
		} else if kv[0] == "cflags" ||
			kv[0] == "cxxflags" ||
			kv[0] == "lflags" ||
			kv[0] == "aflags" {
			err = amendBuildFlags(kv, t)
			if err != nil {
				NewtUsage(cmd, err)
			}
		}
	}
	if err := t.Save(); err != nil {
		NewtUsage(cmd, err)
	}

	for _, kv := range vars {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Amended %s for Target %s successfully\n",
			kv[0], t.FullName())
	}
}

func targetCreateCmd(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		NewtUsage(cmd, util.NewNewtError("Missing target name"))
	}

	proj := TryGetProject()

	pkgName, err := ResolveNewTargetName(args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	repo := proj.LocalRepo()
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
	if !newtutil.NewtForce {
		// Determine if the target directory contains extra user files.  If it
		// does, a prompt (or force) is required to delete it.
		userFiles, err := targetContainsUserFiles(t)
		if err != nil {
			return err
		}

		if userFiles {
			fmt.Printf("Target directory %s contains some extra content; "+
				"delete anyway? (y/N): ", t.Package().BasePath())
			rsp := PromptYesNo(false)
			if !rsp {
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
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify at least one "+
			"target to delete"))
	}

	TryGetProject()

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
	if len(args) != 2 {
		NewtUsage(cmd, util.NewNewtError("Must specify exactly one "+
			"source target and one destination target"))
	}

	proj := TryGetProject()

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
	dstTarget := srcTarget.Clone(proj.LocalRepo(), dstName)

	// Save the new target.
	err = dstTarget.Save()
	if err != nil {
		NewtUsage(nil, err)
	}

	// Copy syscfg.yml file.
	srcSyscfgPath := fmt.Sprintf("%s/%s",
		srcTarget.Package().BasePath(),
		pkg.SYSCFG_YAML_FILENAME)
	dstSyscfgPath := fmt.Sprintf("%s/%s",
		dstTarget.Package().BasePath(),
		pkg.SYSCFG_YAML_FILENAME)

	if err := util.CopyFile(srcSyscfgPath, dstSyscfgPath); err != nil {
		// If there is just no source syscfg.yml file, that is not an error.
		if !util.IsNotExist(err) {
			NewtUsage(nil, err)
		}
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Target successfully copied; %s --> %s\n",
		srcTarget.FullName(), dstTarget.FullName())
}

func printSetting(entry syscfg.CfgEntry) {
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"  * Setting: %s\n", entry.Name)

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"    * Description: %s\n", entry.Description)

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"    * Value: %s", entry.Value)

	util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")

	if len(entry.History) > 1 {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"    * Overridden: ")
		for i := 1; i < len(entry.History); i++ {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "%s, ",
				entry.History[i].Source.FullName())
		}
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"default=%s\n", entry.History[0].Value)
	}
	if len(entry.ValueRefName) > 0 {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"    * Copied from: %s\n",
			entry.ValueRefName)
	}
}

func printBriefSetting(entry syscfg.CfgEntry) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "  %s: %s",
		entry.Name, entry.Value)

	var extras []string

	if len(entry.History) > 1 {
		s := fmt.Sprintf("overridden by %s",
			entry.History[len(entry.History)-1].Source.FullName())
		extras = append(extras, s)
	}
	if len(entry.ValueRefName) > 0 {
		s := fmt.Sprintf("copied from %s", entry.ValueRefName)
		extras = append(extras, s)
	}

	if len(extras) > 0 {
		util.StatusMessage(util.VERBOSITY_DEFAULT, " (%s)",
			strings.Join(extras, ", "))
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
}

func printPkgCfg(pkgName string, cfg syscfg.Cfg, entries []syscfg.CfgEntry) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "* PACKAGE: %s\n", pkgName)

	settingNames := make([]string, len(entries))
	for i, entry := range entries {
		settingNames[i] = entry.Name
	}
	sort.Strings(settingNames)

	for _, name := range settingNames {
		printSetting(cfg.Settings[name])
	}
}

func printCfg(targetName string, cfg syscfg.Cfg) {
	if errText := cfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Syscfg for %s:\n", targetName)
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

func printPkgBriefCfg(pkgName string, cfg syscfg.Cfg, entries []syscfg.CfgEntry) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "[%s]\n", pkgName)

	settingNames := make([]string, len(entries))
	for i, entry := range entries {
		settingNames[i] = entry.Name
	}
	sort.Strings(settingNames)

	for _, name := range settingNames {
		printBriefSetting(cfg.Settings[name])
	}
}

func printBriefCfg(targetName string, cfg syscfg.Cfg) {
	if errText := cfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Brief syscfg for %s:\n", targetName)
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
		printPkgBriefCfg(pkgName, cfg, pkgNameEntryMap[pkgName])
	}
}

func yamlPkgCfg(w io.Writer, pkgName string, cfg syscfg.Cfg,
	entries []syscfg.CfgEntry) {

	settingNames := make([]string, len(entries))
	for i, entry := range entries {
		settingNames[i] = entry.Name
	}
	sort.Strings(settingNames)

	fmt.Fprintf(w, "    ### %s\n", pkgName)
	for _, name := range settingNames {
		fmt.Fprintf(w, "    %s: '%s'\n", name, cfg.Settings[name].Value)
	}
}

func yamlCfg(cfg syscfg.Cfg) string {
	if errText := cfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

	pkgNameEntryMap := syscfg.EntriesByPkg(cfg)

	pkgNames := make([]string, 0, len(pkgNameEntryMap))
	for pkgName, _ := range pkgNameEntryMap {
		pkgNames = append(pkgNames, pkgName)
	}
	sort.Strings(pkgNames)

	buf := bytes.Buffer{}

	fmt.Fprintf(&buf, "syscfg.vals:\n")
	for i, pkgName := range pkgNames {
		if i > 0 {
			fmt.Fprintf(&buf, "\n")
		}
		yamlPkgCfg(&buf, pkgName, cfg, pkgNameEntryMap[pkgName])
	}

	return string(buf.Bytes())
}

func targetBuilderConfigResolve(b *builder.TargetBuilder) *resolve.Resolution {
	res, err := b.Resolve()
	if err != nil {
		NewtUsage(nil, err)
	}

	warningText := strings.TrimSpace(res.WarningText())
	if warningText != "" {
		log.Warn(warningText + "\n")
	}

	return res
}

func targetConfigShowCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	for i, arg := range args {
		b, err := TargetBuilderForTargetOrUnittest(arg)
		if err != nil {
			NewtUsage(cmd, err)
		}

		res := targetBuilderConfigResolve(b)
		printCfg(b.GetTarget().Name(), res.Cfg)

		if i < len(args)-1 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
	}
}

func targetConfigBriefCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	for i, arg := range args {
		b, err := TargetBuilderForTargetOrUnittest(arg)
		if err != nil {
			NewtUsage(cmd, err)
		}

		res := targetBuilderConfigResolve(b)
		printBriefCfg(b.GetTarget().Name(), res.Cfg)

		if i < len(args)-1 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
	}
}

func valSettingString(vs val.ValSetting) string {
	intVal, _ := vs.IntVal()

	s := fmt.Sprintf("%d", intVal)
	if vs.RefName != "" {
		s += fmt.Sprintf("%*s [%s]", 16-len(s), "", vs.RefName)
	}

	return s
}

func logLevelString(ls val.ValSetting) string {
	intVal, _ := ls.IntVal()

	s := fmt.Sprintf("%d (%s)", intVal, logcfg.LogLevelString(intVal))
	if ls.RefName != "" {
		s += fmt.Sprintf("%*s [%s]", 16-len(s), "", ls.RefName)
	}

	return s
}

func printLogCfgOne(l logcfg.Log) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "%s:\n", l.Name)
	util.StatusMessage(util.VERBOSITY_DEFAULT, "    Package: %s\n",
		l.Source.FullName())
	util.StatusMessage(util.VERBOSITY_DEFAULT, "    Module:  %s\n",
		valSettingString(l.Module))
	util.StatusMessage(util.VERBOSITY_DEFAULT, "    Level:   %s\n",
		logLevelString(l.Level))
}

func printLogCfg(targetName string, lcfg logcfg.LCfg) {
	if errText := lcfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Log config for %s:\n",
		targetName)

	logNames := make([]string, 0, len(lcfg.Logs))
	for name, _ := range lcfg.Logs {
		logNames = append(logNames, name)
	}
	sort.Strings(logNames)

	for i, logName := range logNames {
		if i > 0 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
		printLogCfgOne(lcfg.Logs[logName])
	}
}

func targetLogShowCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	for i, arg := range args {
		b, err := TargetBuilderForTargetOrUnittest(arg)
		if err != nil {
			NewtUsage(cmd, err)
		}

		res := targetBuilderConfigResolve(b)
		printLogCfg(b.GetTarget().Name(), res.LCfg)

		if i < len(args)-1 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
	}
}

func printLogCfgBriefOne(l logcfg.Log, colWidth int) {
	intMod, _ := l.Module.IntVal()
	intLevel, _ := l.Level.IntVal()

	levelStr := fmt.Sprintf("%d (%s)", intLevel,
		logcfg.LogLevelString(intLevel))

	util.StatusMessage(util.VERBOSITY_DEFAULT, "%*s | %-8d | %-12s\n",
		colWidth, l.Name, intMod, levelStr)
}

func printLogCfgBrief(targetName string, lcfg logcfg.LCfg) {
	if errText := lcfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Brief log config for %s:\n",
		targetName)

	longest := 6
	logNames := make([]string, 0, len(lcfg.Logs))
	for name, _ := range lcfg.Logs {
		logNames = append(logNames, name)
		if len(name) > longest {
			longest = len(name)
		}
	}
	sort.Strings(logNames)

	colWidth := longest + 4
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"%*s | MODULE   | LEVEL\n", colWidth, "LOG")
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"%s-+----------+--------------\n",
		strings.Repeat("-", colWidth))
	for _, logName := range logNames {
		printLogCfgBriefOne(lcfg.Logs[logName], colWidth)
	}
}

func targetLogBriefCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	for i, arg := range args {
		b, err := TargetBuilderForTargetOrUnittest(arg)
		if err != nil {
			NewtUsage(cmd, err)
		}

		res := targetBuilderConfigResolve(b)
		printLogCfgBrief(b.GetTarget().Name(), res.LCfg)

		if i < len(args)-1 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
	}
}

func printStage(sf stage.StageFunc) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "%s:\n", sf.Name)
	util.StatusMessage(util.VERBOSITY_DEFAULT, "    Package: %s\n",
		sf.Pkg.FullName())
	util.StatusMessage(util.VERBOSITY_DEFAULT, "    Stage:  %s\n",
		valSettingString(sf.Stage))
}

func printStageBriefOne(sf stage.StageFunc, fnWidth int, pkgWidth int) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, " %-*s | %-*s | %s\n",
		fnWidth, sf.Name,
		pkgWidth, sf.Pkg.FullName(),
		valSettingString(sf.Stage))
}

func printStageBriefTable(sfs []stage.StageFunc) {
	longestPkg := 6
	longestFn := 6
	for _, sf := range sfs {
		if len(sf.Name) > longestFn {
			longestFn = len(sf.Name)
		}
		if len(sf.Pkg.FullName()) > longestPkg {
			longestPkg = len(sf.Pkg.FullName())
		}
	}

	pkgWidth := longestPkg + 2
	fnWidth := longestFn + 2

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		" %-*s | %-*s | STAGE\n",
		fnWidth, "FUNCTION",
		pkgWidth, "PACKAGE")
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"-%s-+-%s-+----------\n",
		strings.Repeat("-", fnWidth), strings.Repeat("-", pkgWidth))
	for _, sf := range sfs {
		printStageBriefOne(sf, fnWidth, pkgWidth)
	}
}

func printSysinitCfg(targetName string, scfg sysinit.SysinitCfg) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "Sysinit config for %s:\n",
		targetName)

	for i, sf := range scfg.StageFuncs {
		if i > 0 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
		printStage(sf)
	}
}

func printSysinitBrief(targetName string, scfg sysinit.SysinitCfg) {
	if errText := scfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Brief sysinit config for %s:\n",
		targetName)

	printStageBriefTable(scfg.StageFuncs)
}

func targetSysinitShowCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	for i, arg := range args {
		b, err := TargetBuilderForTargetOrUnittest(arg)
		if err != nil {
			NewtUsage(cmd, err)
		}

		res := targetBuilderConfigResolve(b)
		printSysinitCfg(b.GetTarget().Name(), res.SysinitCfg)

		if i < len(args)-1 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
	}
}

func targetSysinitBriefCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	for i, arg := range args {
		b, err := TargetBuilderForTargetOrUnittest(arg)
		if err != nil {
			NewtUsage(cmd, err)
		}

		res := targetBuilderConfigResolve(b)
		printSysinitBrief(b.GetTarget().Name(), res.SysinitCfg)

		if i < len(args)-1 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
	}
}

func printSysdownCfg(targetName string, scfg sysdown.SysdownCfg) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "Sysdown config for %s:\n",
		targetName)

	for i, sf := range scfg.StageFuncs {
		if i > 0 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
		printStage(sf)
	}
}

func targetSysdownShowCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	for i, arg := range args {
		b, err := TargetBuilderForTargetOrUnittest(arg)
		if err != nil {
			NewtUsage(cmd, err)
		}

		res := targetBuilderConfigResolve(b)
		printSysdownCfg(b.GetTarget().Name(), res.SysdownCfg)

		if i < len(args)-1 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
	}
}

func printSysdownBrief(targetName string, scfg sysdown.SysdownCfg) {
	if errText := scfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Brief sysdown config for %s:\n",
		targetName)

	printStageBriefTable(scfg.StageFuncs)
}

func targetSysdownBriefCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	for i, arg := range args {
		b, err := TargetBuilderForTargetOrUnittest(arg)
		if err != nil {
			NewtUsage(cmd, err)
		}

		res := targetBuilderConfigResolve(b)
		printSysdownBrief(b.GetTarget().Name(), res.SysdownCfg)

		if i < len(args)-1 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}
	}
}

func targetConfigInitCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	type entry struct {
		lpkg   *pkg.LocalPackage
		path   string
		b      *builder.TargetBuilder
		exists bool
	}

	TryGetProject()

	anyExist := false
	entries := make([]entry, len(args))
	for i, pkgName := range args {
		e := &entries[i]

		b, err := TargetBuilderForTargetOrUnittest(pkgName)
		if err != nil {
			NewtUsage(cmd, err)
		}
		e.b = b

		e.lpkg = b.GetTestPkg()
		if e.lpkg == nil {
			e.lpkg = b.GetTarget().Package()
		}

		e.path = builder.PkgSyscfgPath(e.lpkg.BasePath())

		if util.NodeExist(e.path) {
			e.exists = true
			anyExist = true
		}
	}

	if anyExist && !newtutil.NewtForce {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Configuration files already exist:\n")
		for _, e := range entries {
			if e.exists {
				util.StatusMessage(util.VERBOSITY_DEFAULT, "    * %s\n",
					e.path)
			}
		}
		util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")

		fmt.Printf("Overwrite them? (y/N): ")
		rsp := PromptYesNo(false)
		if !rsp {
			return
		}
	}

	for _, e := range entries {
		res := targetBuilderConfigResolve(e.b)
		yaml := yamlCfg(res.Cfg)

		if err := ioutil.WriteFile(e.path, []byte(yaml), 0644); err != nil {
			NewtUsage(nil, util.FmtNewtError("Error writing file \"%s\"; %s",
				e.path, err.Error()))
		}
	}
}

func targetDepCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	b, err := TargetBuilderForTargetOrUnittest(args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	res, err := b.Resolve()
	if err != nil {
		NewtUsage(nil, err)
	}

	dg, err := b.CreateDepGraph()
	if err != nil {
		NewtUsage(nil, err)
	}

	// If user specified any package names, only include specified packages.
	if len(args) > 1 {
		rpkgs, err := ResolveRpkgs(res, args[1:])
		if err != nil {
			NewtUsage(cmd, err)
		}

		var missingRpkgs []*resolve.ResolvePackage
		dg, missingRpkgs = builder.FilterDepGraph(dg, rpkgs)
		for _, rpkg := range missingRpkgs {
			util.StatusMessage(util.VERBOSITY_QUIET,
				"Warning: Package \"%s\" not included in target \"%s\"\n",
				rpkg.Lpkg.FullName(), b.GetTarget().FullName())
		}
	}

	if len(dg) > 0 {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			builder.DepGraphText(dg)+"\n")
	}
}

func targetRevdepCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target name"))
	}

	TryGetProject()

	b, err := TargetBuilderForTargetOrUnittest(args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	res, err := b.Resolve()
	if err != nil {
		NewtUsage(nil, err)
	}

	dg, err := b.CreateRevdepGraph()
	if err != nil {
		NewtUsage(nil, err)
	}

	// If user specified any package names, only include specified packages.
	if len(args) > 1 {
		rpkgs, err := ResolveRpkgs(res, args[1:])
		if err != nil {
			NewtUsage(cmd, err)
		}

		var missingRpkgs []*resolve.ResolvePackage
		dg, missingRpkgs = builder.FilterDepGraph(dg, rpkgs)
		for _, rpkg := range missingRpkgs {
			util.StatusMessage(util.VERBOSITY_QUIET,
				"Warning: Package \"%s\" not included in target \"%s\"\n",
				rpkg.Lpkg.FullName(), b.GetTarget().FullName())
		}
	}

	if len(dg) > 0 {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			builder.RevdepGraphText(dg)+"\n")
	}
}

func AddTargetCommands(cmd *cobra.Command) {
	targetHelpText := ""
	targetHelpEx := ""
	targetCmd := &cobra.Command{
		Use:     "target",
		Short:   "Commands to create, delete, configure, and query targets",
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
	AddTabCompleteFn(showCmd, targetList)

	cmakeHelpText := "Generate CMakeLists.txt for target specified " +
		"by <target-name>."
	cmakeHelpEx := "  newt target cmake <target-name>\n"
	cmakeHelpEx += "  newt target cmake my_target1"

	cmakeCmd := &cobra.Command{
		Use:     "cmake",
		Short:   "",
		Long:    cmakeHelpText,
		Example: cmakeHelpEx,
		Run:     targetCmakeCmd,
	}
	targetCmd.AddCommand(cmakeCmd)
	AddTabCompleteFn(cmakeCmd, targetList)

	setHelpText := "Set a target variable (<var-name>) on target "
	setHelpText += "<target-name> to value <value>.\n"
	setHelpText += "Variables that can be set are:\n"
	setHelpText += strings.Join(setVars, "\n") + "\n\n"
	setHelpText += "Warning: When setting the syscfg variable, a new syscfg.yml file\n"
	setHelpText += "is created and the current settings are deleted. Only the settings\n"
	setHelpText += "specified in the command are saved in the syscfg.yml file."
	setHelpText += "\nIf you want to change or add a new syscfg value and keep the other\n"
	setHelpText += "syscfg values, use the newt target amend command.\n"
	setHelpEx := "  newt target set my_target1 build_profile=optimized "
	setHelpEx += "cflags=\"-DNDEBUG\"\n"
	setHelpEx += "  newt target set my_target1 "
	setHelpEx += "syscfg=LOG_NEWTMGR=1:CONFIG_NEWTMGR=0\n"

	setCmd := &cobra.Command{
		Use: "set <target-name> <var-name>=<value> " +
			"[<var-name>=<value>...]",
		Short:   "Set target configuration variable",
		Long:    setHelpText,
		Example: setHelpEx,
		Run:     targetSetCmd,
	}
	targetCmd.AddCommand(setCmd)
	AddTabCompleteFn(setCmd, targetList)

	amendHelpText := "Add, change, or delete values for multi-value target variables\n\n"
	amendHelpText += "Variables that can have values amended are:\n"
	amendHelpText += strings.Join(amendVars, "\n") + "\n\n"
	amendHelpText += "To change the value for a single value variable, such as bsp, use the\nnewt target set command.\n"

	amendHelpEx := "  newt target amend my_target cflags=\"-DNDEBUG -DTEST\"\n"
	amendHelpEx += "    Adds -DDEBUG and -DTEST to cflags\n\n"
	amendHelpEx += "  newt target amend my_target lflags=\"-Lmylib\" "
	amendHelpEx += "syscfg=LOG_LEVEL:CONFIG_NEWTMGR=0\n"
	amendHelpEx += "    Adds -Lmylib to lflags and syscfg variables LOG_LEVEL=1 and CONFIG_NEWTMGR=0\n\n"
	amendHelpEx += "  newt target amend my_target -d syscfg=CONFIG_NEWTMGR "
	amendHelpEx += "cflags=\"-DNDEBUG\"\n"
	amendHelpEx += "    Deletes syscfg variable CONFIG_NEWTMGR and -DNDEBUG from cflags\n"

	amendCmd := &cobra.Command{
		Use: "amend <target-name> <var-name>=<value>" +
			"[<var-name>=<value>...]\n",
		Short:   "Add, change, or delete values for multi-value target variables",
		Long:    amendHelpText,
		Example: amendHelpEx,
		Run:     targetAmendCmd,
	}
	amendCmd.Flags().BoolVarP(&amendDelete, "delete", "d", false,
		"Delete Variable values")
	targetCmd.AddCommand(amendCmd)
	AddTabCompleteFn(amendCmd, targetList)

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
	delCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
		"Force delete of targets with user files without prompt")

	targetCmd.AddCommand(delCmd)

	copyHelpText := "Create a new target <dst-target> by cloning <src-target>"
	copyHelpEx := "  newt target copy blinky_sim my_target"

	copyCmd := &cobra.Command{
		Use:     "copy <src-target> <dst-target>",
		Short:   "Copy target",
		Long:    copyHelpText,
		Example: copyHelpEx,
		Run:     targetCopyCmd,
	}

	targetCmd.AddCommand(copyCmd)
	AddTabCompleteFn(copyCmd, targetList)

	configHelpText := "View or populate a target's system configuration"

	configCmd := &cobra.Command{
		Use:   "config",
		Short: configHelpText,
		Long:  configHelpText,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	targetCmd.AddCommand(configCmd)

	configShowCmd := &cobra.Command{
		Use:   "show <target> [target...]",
		Short: "View a target's system configuration",
		Long:  "View a target's system configuration",
		Run:   targetConfigShowCmd,
	}

	configCmd.AddCommand(configShowCmd)
	AddTabCompleteFn(configShowCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	configBriefCmd := &cobra.Command{
		Use:   "brief <target> [target...]",
		Short: "View a summary of target's system configuration",
		Long:  "View a summary of target's system configuration",
		Run:   targetConfigBriefCmd,
	}

	configCmd.AddCommand(configBriefCmd)
	AddTabCompleteFn(configBriefCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	configInitCmd := &cobra.Command{
		Use:   "init",
		Short: "Populate a target's system configuration file",
		Long: "Populate a target's system configuration file (syscfg). " +
			"Unspecified settings are given default values.",
		Run: targetConfigInitCmd,
	}
	configInitCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
		"Force overwrite of target configuration")

	configCmd.AddCommand(configInitCmd)
	AddTabCompleteFn(configInitCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	logHelpText := "View a target's log configuration"

	logCmd := &cobra.Command{
		Use:   "logcfg",
		Short: logHelpText,
		Long:  logHelpText,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	targetCmd.AddCommand(logCmd)

	logShowCmd := &cobra.Command{
		Use:   "show <target> [target...]",
		Short: "View a target's log configuration",
		Long:  "View a target's log configuration",
		Run:   targetLogShowCmd,
	}

	logCmd.AddCommand(logShowCmd)
	AddTabCompleteFn(logShowCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	logBriefCmd := &cobra.Command{
		Use:   "brief <target> [target...]",
		Short: "View a summary of target's log configuration",
		Long:  "View a summary of target's log configuration",
		Run:   targetLogBriefCmd,
	}

	logCmd.AddCommand(logBriefCmd)
	AddTabCompleteFn(logBriefCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	sysinitHelpText := "View a target's sysinit configuration"

	sysinitCmd := &cobra.Command{
		Use:   "sysinit",
		Short: sysinitHelpText,
		Long:  sysinitHelpText,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	targetCmd.AddCommand(sysinitCmd)

	sysinitShowCmd := &cobra.Command{
		Use:   "show <target> [target...]",
		Short: "View a target's sysinit configuration",
		Long:  "View a target's sysinit configuration",
		Run:   targetSysinitShowCmd,
	}

	sysinitCmd.AddCommand(sysinitShowCmd)
	AddTabCompleteFn(sysinitShowCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	sysinitBriefCmd := &cobra.Command{
		Use:   "brief <target> [target...]",
		Short: "View a summary of target's sysinit configuration",
		Long:  "View a summary of target's sysinit configuration",
		Run:   targetSysinitBriefCmd,
	}

	sysinitCmd.AddCommand(sysinitBriefCmd)
	AddTabCompleteFn(sysinitBriefCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	sysdownHelpText := "View a target's sysdown configuration"

	sysdownCmd := &cobra.Command{
		Use:   "sysdown",
		Short: sysdownHelpText,
		Long:  sysdownHelpText,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	targetCmd.AddCommand(sysdownCmd)

	sysdownShowCmd := &cobra.Command{
		Use:   "show <target> [target...]",
		Short: "View a target's sysdown configuration",
		Long:  "View a target's sysdown configuration",
		Run:   targetSysdownShowCmd,
	}

	sysdownCmd.AddCommand(sysdownShowCmd)
	AddTabCompleteFn(sysdownShowCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	sysdownBriefCmd := &cobra.Command{
		Use:   "brief <target> [target...]",
		Short: "View a summary of target's sysdown configuration",
		Long:  "View a summary of target's sysdown configuration",
		Run:   targetSysdownBriefCmd,
	}

	sysdownCmd.AddCommand(sysdownBriefCmd)
	AddTabCompleteFn(sysdownBriefCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	depHelpText := "View a target's dependency graph."

	depCmd := &cobra.Command{
		Use:   "dep <target> [pkg-1] [pkg-2] [...]",
		Short: "View target's dependency graph",
		Long:  depHelpText,
		Run:   targetDepCmd,
	}

	targetCmd.AddCommand(depCmd)
	AddTabCompleteFn(depCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	revdepHelpText := "View a target's reverse-dependency graph."

	revdepCmd := &cobra.Command{
		Use:   "revdep <target> [pkg-1] [pkg-2] [...]",
		Short: "View target's reverse-dependency graph",
		Long:  revdepHelpText,
		Run:   targetRevdepCmd,
	}

	targetCmd.AddCommand(revdepCmd)
	AddTabCompleteFn(revdepCmd, func() []string {
		return append(targetList(), unittestList()...)
	})
}
