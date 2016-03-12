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
	"sort"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

func packageListCmd(cmd *cobra.Command, args []string) {
	packNames := []string{}
	repoHash := project.GetProject().PackageList()
	for _, pkgHash := range repoHash {
		for _, pack := range *pkgHash {
			if pack.Type() != pkg.PACKAGE_TYPE_TARGET {
				packNames = append(packNames, pack.FullName())
			}
		}
	}

	sort.Strings(packNames)

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Packages in this project:\n")
	for _, name := range packNames {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "    * %s\n", name)
	}
}

func AddPackageCommands(cmd *cobra.Command) {
	packageHelpText := ""
	packageHelpEx := ""
	packageCmd := &cobra.Command{
		Use:     "package",
		Short:   "Command for manipulating packages",
		Long:    packageHelpText,
		Example: packageHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	cmd.AddCommand(packageCmd)

	listHelpText := "List all packages in the project."
	listHelpEx := "  newt package list\n"

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List all packages",
		Long:    listHelpText,
		Example: listHelpEx,
		Run:     packageListCmd,
	}

	packageCmd.AddCommand(listCmd)
}
