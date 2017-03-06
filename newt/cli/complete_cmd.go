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
	"regexp"
	"sort"
	"strings"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type TabCompleteFn func() []string

var tabCompleteEntries = map[*cobra.Command]TabCompleteFn{}

func AddTabCompleteFn(cmd *cobra.Command, cb TabCompleteFn) {
	if cmd.ValidArgs != nil || tabCompleteEntries[cmd] != nil {
		panic("tab completion values generated twice for command " +
			cmd.Name())
	}

	tabCompleteEntries[cmd] = cb
}

func GenerateTabCompleteValues() {
	for cmd, cb := range tabCompleteEntries {
		cmd.ValidArgs = cb()
	}
}

func pkgNameList(filterCb func(*pkg.LocalPackage) bool) []string {
	names := []string{}

	proj, err := project.TryGetProject()
	if err != nil {
		return names
	}

	for _, pack := range proj.PackagesOfType(-1) {
		if filterCb(pack.(*pkg.LocalPackage)) {
			names = append(names, pack.FullName())
		}
	}

	sort.Strings(names)
	return names
}

func targetList() []string {
	targetNames := pkgNameList(func(pack *pkg.LocalPackage) bool {
		return pack.Type() == pkg.PACKAGE_TYPE_TARGET &&
			!strings.HasSuffix(pack.Name(), "/unittest")
	})

	// Remove "targets/" prefix.
	for i, _ := range targetNames {
		targetNames[i] = strings.TrimPrefix(
			targetNames[i], TARGET_DEFAULT_DIR+"/")
	}

	return targetNames
}

/* @return                      A slice of all testable package names. */
func testablePkgList() []string {
	packs := testablePkgs()
	names := make([]string, 0, len(packs))
	for pack, _ := range packs {
		if pack.Type() != pkg.PACKAGE_TYPE_UNITTEST {
			names = append(names, pack.FullName())
		}
	}

	return names
}

func unittestList() []string {
	return pkgNameList(func(pack *pkg.LocalPackage) bool {
		return pack.Type() == pkg.PACKAGE_TYPE_UNITTEST
	})
}

func mfgList() []string {
	targetNames := pkgNameList(func(pack *pkg.LocalPackage) bool {
		return pack.Type() == pkg.PACKAGE_TYPE_MFG
	})

	// Remove "targets/" prefix.
	for i, _ := range targetNames {
		targetNames[i] = strings.TrimPrefix(
			targetNames[i], MFG_DEFAULT_DIR+"/")
	}

	return targetNames
}

func completeRunCmd(cmd *cobra.Command, args []string) {
	cmd_line := os.Getenv("COMP_LINE")

	if cmd_line == "" {
		fmt.Println("This command is intended to be used as part of " +
			" bash autocomplete.  Its not intended to be called directly from " +
			" the command line ")
		return
	}

	root_cmd := cmd.Root()

	args = strings.Split(cmd_line, " ")
	found_cmd, _, _ := root_cmd.Find(args[1:])
	if found_cmd == nil {
		return
	}

	/* this is worth a long comment.  We have to find a command line
	 * with the flags removed.  To do this, I look at the command
	 * path for the command without flags, and remove anything that
	 * doesn't match */
	found_args := strings.Split(found_cmd.CommandPath(), " ")
	last_arg := found_args[len(found_args)-1]

	/* what is remaining after the last parsed argument */
	ind := strings.Index(cmd_line, last_arg)
	ind += len(last_arg)
	extra_str := cmd_line[ind:]

	if len(extra_str) == 0 {
		/* this matched an exact command with no space afterwards.  There
		 * is no autocomplete except this command (to add a space) */
		fmt.Println(found_cmd.Name())
		return
	}

	/* skip flags for now. This just removes them */
	/* skip over complete flags. So the current bash autocomplete will
	 * not complete flags */
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		flg := fmt.Sprintf("--%s", flag.Name)
		if flag.Value.Type() == "bool" {
			/* skip the flag */
			r := regexp.MustCompile(flg + "[\\W]+")
			extra_str = r.ReplaceAllString(extra_str, "")
		} else if flag.Value.Type() == "string" {
			/* skip the string and the next word */
			r := regexp.MustCompile(flg + "[\\W]+[^\\W]+[\\W]+")
			extra_str = r.ReplaceAllString(extra_str, "")
		}

		sflg := fmt.Sprintf("-%s", flag.Shorthand)
		if flag.Value.Type() == "bool" {
			/* skip the flag */
			r := regexp.MustCompile(sflg + "[\\W]+")
			extra_str = r.ReplaceAllString(extra_str, "")
		} else if flag.Value.Type() == "string" {
			/* skip the string and the next word */
			r := regexp.MustCompile(sflg + "[\\W]+[^\\W]+[\\W]+")
			extra_str = r.ReplaceAllString(extra_str, "")
		}
	})

	if len(extra_str) == 0 {
		/* this matched an exact command with no space afterwards.  There
		 * is no autocomplete except this command (to add a space) */
		return
	}

	extra_str = strings.TrimLeft(extra_str, " ")

	/* give flag hints if the person asks for them */
	showShort := strings.HasPrefix(extra_str, "-") &&
		!strings.HasPrefix(extra_str, "--")

	showLong := strings.HasPrefix(extra_str, "--") ||
		extra_str == "-"

	if showLong {
		r := regexp.MustCompile("^--[^\\W]+")
		partial_flag := r.FindString(extra_str)
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			flg := fmt.Sprintf("--%s", flag.Name)
			if strings.HasPrefix(flg, partial_flag) {
				fmt.Println(flg)
			}
		})
	}

	if showShort {
		r := regexp.MustCompile("^-[^\\W]+")
		partial_flag := r.FindString(extra_str)
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if len(flag.Shorthand) > 0 {
				flg := fmt.Sprintf("-%s", flag.Shorthand)
				if strings.HasPrefix(flg, partial_flag) {
					fmt.Println(flg)
				}
			}
		})
	}

	/* dump out valid arguments */
	for _, c := range found_cmd.ValidArgs {
		if strings.HasPrefix(c, extra_str) {
			fmt.Printf("%s\n", c)
		}
	}

	/* dump out possible sub commands */
	for _, child_cmd := range found_cmd.Commands() {
		if strings.HasPrefix(child_cmd.Name(), extra_str) {
			fmt.Printf("%s\n", child_cmd.Name())
		}
	}
}

func AddCompleteCommands(cmd *cobra.Command) {

	completeCmd := &cobra.Command{
		Use:    "complete",
		Short:  "",
		Long:   "",
		Run:    completeRunCmd,
		Hidden: true,
	}

	/* silence errors on the complete command because we have partial flags */
	completeCmd.SilenceErrors = true
	cmd.AddCommand(completeCmd)
}
