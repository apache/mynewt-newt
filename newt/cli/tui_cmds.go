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
	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/tui"
)

// RunTui starts the ranger-style project browser ("newt --tui").
func RunTui() {
	proj := TryGetProject()

	load := func() ([]*tui.Category, error) {
		// Re-read the project so that changes made from the browser (e.g.
		// an edited target.yml) are picked up.
		if err := ResetGlobalState(); err != nil {
			return nil, err
		}
		p, err := project.TryGetProject()
		if err != nil {
			return nil, err
		}
		return tui.Load(p), nil
	}

	title, note := tui.ProjectTitle(proj.Path(), proj.Name())
	if err := tui.Run(title, note, load); err != nil {
		NewtUsage(nil, err)
	}
}

func AddTuiCommands(cmd *cobra.Command) {
	tuiHelpText := FormatHelp(`Browse the targets, apps, BSPs and other
packages of the project in a ranger-style terminal UI. Targets can be
configured (app, bsp, build profile, flash tool and syscfg), built, loaded
onto a chosen board and debugged from the browser, and the board's console
opened next to it. A help pane at the bottom lists the keys that work at the
current position; press ? to hide or show it.`)

	tuiCmd := &cobra.Command{
		Use:     "tui",
		Short:   "Browse project targets and packages interactively",
		Long:    tuiHelpText,
		Example: "  newt tui\n  newt --tui",
		Run: func(cmd *cobra.Command, args []string) {
			RunTui()
		},
	}

	cmd.AddCommand(tuiCmd)
}
