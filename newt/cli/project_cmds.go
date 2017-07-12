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
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

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
		"project skeleton from apache/mynewt-blinky...\n")
	dl := downloader.NewGithubDownloader()
	dl.User = "apache"
	dl.Repo = "mynewt-blinky"

	dir, err := dl.DownloadRepo(newtutil.NewtBlinkyTag)
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
	proj := TryGetProject()
	interfaces.SetProject(proj)

	if err := proj.Install(false, newtutil.NewtForce); err != nil {
		NewtUsage(cmd, err)
	}
}

func upgradeRunCmd(cmd *cobra.Command, args []string) {
	proj := TryGetProject()
	interfaces.SetProject(proj)

	if err := proj.Upgrade(newtutil.NewtForce); err != nil {
		NewtUsage(cmd, err)
	}
}

func infoRunCmd(cmd *cobra.Command, args []string) {
	reqRepoName := ""
	if len(args) >= 1 {
		reqRepoName = strings.TrimPrefix(args[0], "@")
	}

	proj := TryGetProject()

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
				// Don't display the special unittest target; this is used
				// internally by newt, so the user doesn't need to know about
				// it.
				// XXX: This is a hack; come up with a better solution for
				// unit testing.
				if !strings.HasSuffix(pack.Name(), "/unittest") {
					packNames = append(packNames, pack.Name())
				}
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

func syncRunCmd(cmd *cobra.Command, args []string) {
	proj := TryGetProject()
	repos := proj.Repos()

	ps, err := project.LoadProjectState()
	if err != nil {
		NewtUsage(nil, err)
	}

	var failedRepos []string
	for _, repo := range repos {
		var exists bool
		var updated bool
		if repo.IsLocal() {
			continue
		}
		vers := ps.GetInstalledVersion(repo.Name())
		if vers == nil {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"No installed version of %s found, skipping\n\n",
				repo.Name())
		}
		exists, updated, err = repo.Sync(vers, newtutil.NewtForce)
		if exists && !updated {
			failedRepos = append(failedRepos, repo.Name())
		}
	}
	if len(failedRepos) > 0 {
		var forceMsg string
		if !newtutil.NewtForce {
			forceMsg = " To force resync, add the -f (force) option."
		}
		err = util.NewNewtError(fmt.Sprintf("Failed for repos: %v."+
			forceMsg, failedRepos))
		NewtUsage(nil, err)
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
	installCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
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
	upgradeCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
		"Force upgrade of the repositories to latest state in project.yml")

	cmd.AddCommand(upgradeCmd)

	syncHelpText := ""
	syncHelpEx := ""
	syncCmd := &cobra.Command{
		Use:     "sync",
		Short:   "Synchronize project dependencies",
		Long:    syncHelpText,
		Example: syncHelpEx,
		Run:     syncRunCmd,
	}
	syncCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
		"Force overwrite of existing remote repositories.")
	cmd.AddCommand(syncCmd)

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
