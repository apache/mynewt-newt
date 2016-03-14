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
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

var projectForce bool = false

func newRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify "+
			"a project directory to newt new"))
	}

	newDir := args[0]

	if util.NodeExist(newDir) {
		NewtUsage(cmd, util.NewNewtError("Cannot create new project, "+
			"directory already exists"))
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Downloading "+
		"project skeleton from apache/incubator-mynewt-blinky...\n")
	dl := downloader.NewGithubDownloader()
	dl.User = "apache"
	dl.Repo = "incubator-mynewt-blinky"

	dir, err := dl.DownloadRepo("develop")
	if err != nil {
		NewtUsage(cmd, err)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Installing "+
		"skeleton in %s...\n", newDir)

	if err := util.CopyDir(dir, newDir); err != nil {
		NewtUsage(cmd, err)
	}

	if err := os.RemoveAll(newDir + "/" + "/.git/"); err != nil {
		NewtUsage(cmd, err)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Project %s successfully created.\n", newDir)
}

func installRunCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	proj := project.GetProject()
	interfaces.SetProject(proj)

	if err := proj.Install(false, projectForce); err != nil {
		NewtUsage(cmd, err)
	}
}

func upgradeRunCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	proj := project.GetProject()
	interfaces.SetProject(proj)

	if err := proj.Upgrade(projectForce); err != nil {
		NewtUsage(cmd, err)
	}
}

func infoRunCmd(cmd *cobra.Command, args []string) {
	reqRepoName := ""
	if len(args) >= 1 {
		reqRepoName = strings.TrimPrefix(args[0], "@")
	}

	if err := project.Initialize(); err != nil {
		NewtUsage(nil, err)
	}
	proj := project.GetProject()

	repoNames := []string{}
	for repoName, _ := range proj.PackageList() {
		repoNames = append(repoNames, repoName)
	}
	sort.Strings(repoNames)

	if reqRepoName == "" {
		util.StatusMessage(util.VERBOSITY_DEFAULT, "Repositories in %s:\n",
			proj.Name())

		for _, repoName := range repoNames {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "    * @%s\n", repoName)
		}

		// Now display the packages in the local repository.
		util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		reqRepoName = "local"
	}

	firstRepo := true
	for _, repoName := range repoNames {
		if reqRepoName == "all" || reqRepoName == repoName {
			packNames := []string{}
			for _, pack := range *proj.PackageList()[repoName] {
				packNames = append(packNames, pack.Name())
			}

			sort.Strings(packNames)
			if !firstRepo {
				util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
			} else {
				firstRepo = false
			}
			util.StatusMessage(util.VERBOSITY_DEFAULT, "Packages in @%s:\n",
				repoName)
			for _, pkgName := range packNames {
				util.StatusMessage(util.VERBOSITY_DEFAULT, "    * %s\n",
					pkgName)
			}
		}
	}
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

	newHelpText := ""
	newHelpEx := ""
	newCmd := &cobra.Command{
		Use:     "new <project-dir>",
		Short:   "Create a new project",
		Long:    newHelpText,
		Example: newHelpEx,
		Run:     newRunCmd,
	}

	cmd.AddCommand(newCmd)

	infoHelpText := "Show information about the current project."
	infoHelpEx := "  newt info\n"

	infoCmd := &cobra.Command{
		Use:     "info",
		Short:   "Show project info",
		Long:    infoHelpText,
		Example: infoHelpEx,
		Run:     infoRunCmd,
	}

	cmd.AddCommand(infoCmd)
}
