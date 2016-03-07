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

package project

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/util"
)

func repoRunCmd(cmd *cobra.Command, args []string) {
	proj := GetProject()

	r := proj.FindRepo("apache-mynewt-world")
	r.DownloadDesc()
}

func projectRunCmd(cmd *cobra.Command, args []string) {
	wd, err := os.Getwd()
	if err != nil {
		cli.NewtUsage(cmd, util.NewNewtError(err.Error()))
	}

	proj, err := LoadProject(wd)
	if err != nil {
		cli.NewtUsage(cmd, err)
	}
	proj.LoadPackageList()
	for rName, list := range proj.PackageList() {
		fmt.Printf("repository name: %s\n", rName)
		for pkgName, _ := range *list {
			fmt.Printf("  %s\n", pkgName)
		}
	}

	fmt.Printf("Project %s\n", proj.Name)
	fmt.Printf("  BasePath: %s\n", proj.BasePath)
}

func AddCommands(cmd *cobra.Command) {
	projectHelpText := ""
	projectHelpEx := ""
	projectCmd := &cobra.Command{
		Use:     "project",
		Short:   "Command for manipulating projects",
		Long:    projectHelpText,
		Example: projectHelpEx,
		Run:     projectRunCmd,
	}

	repoCmd := &cobra.Command{
		Use: "repo",
		Run: repoRunCmd,
	}
	cmd.AddCommand(repoCmd)

	cmd.AddCommand(projectCmd)
}
