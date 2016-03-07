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
	"sort"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/project"
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

	targets, err := TargetList()
	if err != nil {
		cli.NewtUsage(cmd, cli.NewNewtError(err.Error()))
	}
	sort.Sort(ByName(targets))

	for _, target := range targets {
		if dispTarget == "" || dispTarget == target.Package().Name() {
			cli.StatusMessage(cli.VERBOSITY_QUIET, target.Package().Name()+"\n")

			settings := target.v.AllSettings()
			keys := []string{}
			for k, _ := range settings {
				keys = append(keys, k)
			}

			sort.Strings(keys)
			for _, k := range keys {
				cli.StatusMessage(cli.VERBOSITY_QUIET,
					"    %s=%s\n", k, settings[k])
			}
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
}
