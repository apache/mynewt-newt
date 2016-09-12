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
	"strings"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func targetList() []string {
	err := project.Initialize()

	targetNames := []string{}

	if err != nil {
		return targetNames
	}

	for name, _ := range target.GetTargets() {
		// Don't display the special unittest target; this is used
		// internally by newt, so the user doesn't need to know about it.
		// XXX: This is a hack; come up with a better solution for unit
		// testing.
		if !strings.HasSuffix(name, "/unittest") {
			targetNames = append(targetNames, strings.TrimPrefix(name, "targets/"))
		}
	}
	return targetNames
}

/* return a list of all packages */
func packageList() []string {

	err := project.Initialize()

	var list []string

	if err != nil {
		return list
	}
	for _, repoHash := range project.GetProject().PackageList() {
		for _, pack := range *repoHash {
			lclPack := pack.(*pkg.LocalPackage)

			if pkgIsTestable(lclPack) {
				list = append(list, lclPack.FullName())
			}
		}
	}
	return list
}

func isValueInList(value string, list []string) int {
	for i, v := range list {
		if v == value {
			return i
		}
	}
	return -1
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
	completeShortHelp := "Performs Bash Autocompletion (-C)"

	completeLongHelp := completeShortHelp + ".\n\n" +
		" this command reads environment variables COMP_LINE and COMP_POINT " +
		" and will send completion options out stdout as one option per line  "

	completeCmd := &cobra.Command{
		Use:   "complete",
		Short: completeShortHelp,
		Long:  completeLongHelp,
		Run:   completeRunCmd,
	}

	/* silence errors on the complete command because we have partial flags */
	completeCmd.SilenceErrors = true
	cmd.AddCommand(completeCmd)
}
