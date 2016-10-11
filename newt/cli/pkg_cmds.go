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
	"strings"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/project"
)

var NewTypeStr = "pkg"

func pkgNewCmd(cmd *cobra.Command, args []string) {
	NewTypeStr = strings.ToUpper(NewTypeStr)

	pw := project.NewPackageWriter()
	if err := pw.ConfigurePackage(NewTypeStr, args[0]); err != nil {
		NewtUsage(cmd, err)
	}
	if err := pw.WritePackage(); err != nil {
		NewtUsage(cmd, err)
	}
}

func AddPackageCommands(cmd *cobra.Command) {
	/* Add the base package command, on top of which other commands are
	 * keyed
	 */
	pkgHelpText := "Commands for creating and manipulating packages"
	pkgHelpEx := "newt pkg new --type=pkg libs/mylib"

	pkgCmd := &cobra.Command{
		Use:     "pkg",
		Short:   "Create and manage packages in the current workspace",
		Long:    pkgHelpText,
		Example: pkgHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cmd.AddCommand(pkgCmd)

	/* Package new command, create a new package */
	newCmdHelpText := ""
	newCmdHelpEx := ""

	newCmd := &cobra.Command{
		Use:     "new",
		Short:   "Create a new package, from a template",
		Long:    newCmdHelpText,
		Example: newCmdHelpEx,
		Run:     pkgNewCmd,
	}

	newCmd.PersistentFlags().StringVarP(&NewTypeStr, "type", "t",
		"pkg", "Type of package to create: pkg, bsp, sdk.  Default pkg.")

	pkgCmd.AddCommand(newCmd)
}
