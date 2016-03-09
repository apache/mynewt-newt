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

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/project"
)

var projectForce bool = false

func installRunCmd(cmd *cobra.Command, args []string) {
	proj := project.GetProject()
	interfaces.SetProject(proj)

	if err := proj.Install(false, projectForce); err != nil {
		NewtUsage(cmd, err)
	}

	fmt.Println("Repos successfully installed")
}

func upgradeRunCmd(cmd *cobra.Command, args []string) {
	proj := project.GetProject()
	interfaces.SetProject(proj)

	if err := proj.Upgrade(projectForce); err != nil {
		NewtUsage(cmd, err)
	}

	fmt.Println("Repos successfully upgrade")
}

func AddProjectCommands(cmd *cobra.Command) {
	installHelpText := ""
	installHelpEx := ""
	installCmd := &cobra.Command{
		Use:     "install",
		Short:   "Install project dependencies",
		Long:    installHelpText,
		Example: installHelpEx,
		Run:     installRunCmd,
	}
	installCmd.PersistentFlags().BoolVarP(&projectForce, "force", "f", false,
		"Force install of the repositories in project, regardless of what "+
			"exists in repos directory")

	cmd.AddCommand(installCmd)

	upgradeHelpText := ""
	upgradeHelpEx := ""
	upgradeCmd := &cobra.Command{
		Use:     "upgrade",
		Short:   "Upgrade project dependencies",
		Long:    upgradeHelpText,
		Example: upgradeHelpEx,
		Run:     upgradeRunCmd,
	}
	upgradeCmd.PersistentFlags().BoolVarP(&projectForce, "force", "f", false,
		"Force upgrade of the repositories to latest state in project.yml")

	cmd.AddCommand(upgradeCmd)

}
