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

// Implements commands that show or modify a target's configuration.

package cli

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/dump"
	"mynewt.apache.org/newt/newt/logcfg"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/resolve"
	"mynewt.apache.org/newt/newt/stage"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/sysdown"
	"mynewt.apache.org/newt/newt/sysinit"
	"mynewt.apache.org/newt/newt/val"
	"mynewt.apache.org/newt/util"
)

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

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Brief syscfg for %s:\n", targetName)
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

func printFlatCfg(targetName string, cfg syscfg.Cfg) {
	if errText := cfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

	settings := cfg.SettingValues()
	names := make([]string, 0, len(settings))
	for name, _ := range settings {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"%s: %s\n", name, settings[name])
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

func targetConfigFlatCmd(cmd *cobra.Command, args []string) {
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
		printFlatCfg(b.GetTarget().Name(), res.Cfg)

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

func printStageBriefOne(sf stage.StageFunc,
	stageWidth int, pkgWidth int, fnWidth int, settingWidth int) {

	util.StatusMessage(util.VERBOSITY_DEFAULT, " %-*s | %-*s | %-*s | %-*s\n",
		stageWidth, sf.Stage.Value,
		pkgWidth, sf.Pkg.FullName(),
		fnWidth, sf.Name,
		settingWidth, sf.Stage.RefName)
}

func printStageBriefTable(sfs []stage.StageFunc) {
	longestStage := 5
	longestPkg := 7
	longestFn := 8
	longestSetting := 7
	for _, sf := range sfs {
		if len(sf.Stage.Value) > longestStage {
			longestStage = len(sf.Stage.Value)
		}
		if len(sf.Pkg.FullName()) > longestPkg {
			longestPkg = len(sf.Pkg.FullName())
		}
		if len(sf.Name) > longestFn {
			longestFn = len(sf.Name)
		}
		if len(sf.Stage.RefName) > longestSetting {
			longestSetting = len(sf.Stage.RefName)
		}
	}

	stageWidth := longestStage + 2
	pkgWidth := longestPkg + 2
	fnWidth := longestFn + 2
	settingWidth := longestSetting + 2

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		" %-*s | %-*s | %-*s | %-*s\n",
		stageWidth, "STAGE",
		pkgWidth, "PACKAGE",
		fnWidth, "FUNCTION",
		settingWidth, "SETTING")
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"-%s-+-%s-+-%s-+-%s-\n",
		strings.Repeat("-", stageWidth),
		strings.Repeat("-", pkgWidth),
		strings.Repeat("-", fnWidth),
		strings.Repeat("-", settingWidth))
	for _, sf := range sfs {
		printStageBriefOne(sf, stageWidth, pkgWidth, fnWidth, settingWidth)
	}
}

func printSysinitCfg(targetName string, scfg sysinit.SysinitCfg) {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "Sysinit config for %s:\n",
		targetName)

	if errText := scfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

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

	if errText := scfg.ErrorText(); errText != "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "!!! %s\n\n", errText)
	}

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

func targetDumpCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd,
			util.NewNewtError("Must specify target or unittest name"))
	}

	TryGetProject()

	for _, arg := range args {
		b, err := TargetBuilderForTargetOrUnittest(arg)
		if err != nil {
			NewtUsage(cmd, err)
		}

		rpt, err := dump.NewReport(b)
		if err != nil {
			NewtUsage(nil, err)
		}
		s, err := rpt.JSON()
		if err != nil {
			NewtUsage(nil, err)
		}
		util.StatusMessage(util.VERBOSITY_DEFAULT, s+"\n")
	}
}

func targetCfgCmdAll() []*cobra.Command {
	cmds := []*cobra.Command{}

	configHelpText := "View or populate a target's system configuration"

	configCmd := &cobra.Command{
		Use:   "config",
		Short: configHelpText,
		Long:  configHelpText,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	cmds = append(cmds, configCmd)

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

	configFlatCmd := &cobra.Command{
		Use:   "flat <target> [target...]",
		Short: "View a flat table of target's system configuration",
		Long:  "View a flat table of target's system configuration",
		Run:   targetConfigFlatCmd,
	}

	configCmd.AddCommand(configFlatCmd)
	AddTabCompleteFn(configFlatCmd, func() []string {
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

	cmds = append(cmds, logCmd)

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

	cmds = append(cmds, sysinitCmd)

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

	cmds = append(cmds, sysdownCmd)

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

	dumpCmd := &cobra.Command{
		Use:   "dump <target> [target...]",
		Short: "Dump a target's intermediate form in JSON",
		Run:   targetDumpCmd,
	}

	cmds = append(cmds, dumpCmd)
	AddTabCompleteFn(dumpCmd, func() []string {
		return append(targetList(), unittestList()...)
	})

	return cmds
}
