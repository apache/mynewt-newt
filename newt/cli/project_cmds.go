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
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
)

var infoRemote bool

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

	tmpdir, err := newtutil.MakeTempRepoDir()
	if err != nil {
		NewtUsage(nil, err)
	}
	defer os.RemoveAll(tmpdir)

	if err := dl.Clone(newtutil.NewtBlinkyTag, tmpdir); err != nil {
		NewtUsage(nil, err)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Installing "+
		"skeleton in %s...\n", newDir)

	if err := util.CopyDir(tmpdir, newDir); err != nil {
		NewtUsage(cmd, err)
	}

	if err := os.RemoveAll(newDir + "/" + "/.git/"); err != nil {
		NewtUsage(cmd, err)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Project %s successfully created.\n", newDir)
}

// Builds a repo selection predicate based on the specified names.  If no names
// are specified, the resulting function selects all non-local repos.
// Otherwise, the function selects each non-local repo whose name is specified.
func makeRepoPredicate(repoNames []string) func(r *repo.Repo) bool {
	// If the user didn't specify any repo names, apply the operation to all
	// repos in `project.yml`.
	if len(repoNames) == 0 {
		proj := project.GetProject()
		return func(r *repo.Repo) bool { return proj.RepoIsRoot(r.Name()) }
	}

	return func(r *repo.Repo) bool {
		if !r.IsLocal() {
			for _, arg := range repoNames {
				if strings.TrimPrefix(r.Name(), "@") == arg {
					return true
				}
			}
		}
		return false
	}
}

func installRunCmd(cmd *cobra.Command, args []string) {
	proj := TryGetProject()
	interfaces.SetProject(proj)

	pred := makeRepoPredicate(args)
	if err := proj.InstallIf(
		false, newtutil.NewtForce, newtutil.NewtAsk, pred); err != nil {

		NewtUsage(nil, err)
	}
}

func upgradeRunCmd(cmd *cobra.Command, args []string) {
	proj := TryGetProject()
	interfaces.SetProject(proj)

	pred := makeRepoPredicate(args)
	if err := proj.InstallIf(
		true, newtutil.NewtForce, newtutil.NewtAsk, pred); err != nil {

		NewtUsage(nil, err)
	}
}

func infoRunCmd(cmd *cobra.Command, args []string) {
	proj := TryGetProject()

	// If no arguments specified, print status of all installed repos.
	if len(args) == 0 {
		pred := func(r *repo.Repo) bool { return !r.IsLocal() }

		if err := proj.InfoIf(pred, infoRemote); err != nil {
			NewtUsage(nil, err)
		}
		return
	}

	// Otherwise, list packages specified repo contains.
	reqRepoName := strings.TrimPrefix(args[0], "@")

	repoNames := []string{}
	for repoName, _ := range proj.PackageList() {
		repoNames = append(repoNames, repoName)
	}
	sort.Strings(repoNames)

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
	pred := makeRepoPredicate(args)

	if err := proj.SyncIf(
		newtutil.NewtForce, newtutil.NewtAsk, pred); err != nil {

		NewtUsage(nil, err)
	}
}

func AddProjectCommands(cmd *cobra.Command) {
	installHelpText := ""
	installHelpEx := "  newt install\n"
	installHelpEx += "    Installs all repositories specified in project.yml.\n\n"
	installHelpEx += "  newt install apache-mynewt-core\n"
	installHelpEx += "    Installs the apache-mynewt-core repository."
	installCmd := &cobra.Command{
		Use:     "install [repo-1] [repo-2] [...]",
		Short:   "Install project dependencies",
		Long:    installHelpText,
		Example: installHelpEx,
		Run:     installRunCmd,
	}
	installCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
		"Force install of the repositories in project, regardless of what "+
			"exists in repos directory")
	installCmd.PersistentFlags().BoolVarP(&newtutil.NewtAsk,
		"ask", "a", false, "Prompt user before installing any repos")

	cmd.AddCommand(installCmd)

	upgradeHelpText := ""
	upgradeHelpEx := "  newt upgrade\n"
	upgradeHelpEx += "    Upgrades all repositories specified in project.yml.\n\n"
	upgradeHelpEx += "  newt upgrade apache-mynewt-core\n"
	upgradeHelpEx += "    Upgrades the apache-mynewt-core repository."
	upgradeCmd := &cobra.Command{
		Use:     "upgrade [repo-1] [repo-2] [...]",
		Short:   "Upgrade project dependencies",
		Long:    upgradeHelpText,
		Example: upgradeHelpEx,
		Run:     upgradeRunCmd,
	}
	upgradeCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
		"Force upgrade of the repositories to latest state in project.yml")
	upgradeCmd.PersistentFlags().BoolVarP(&newtutil.NewtAsk,
		"ask", "a", false, "Prompt user before upgrading any repos")

	cmd.AddCommand(upgradeCmd)

	syncHelpText := ""
	syncHelpEx := "  newt sync\n"
	syncHelpEx += "    Syncs all repositories specified in project.yml.\n\n"
	syncHelpEx += "  newt sync apache-mynewt-core\n"
	syncHelpEx += "    Syncs the apache-mynewt-core repository."
	syncCmd := &cobra.Command{
		Use:     "sync [repo-1] [repo-2] [...]",
		Short:   "Synchronize project dependencies",
		Long:    syncHelpText,
		Example: syncHelpEx,
		Run:     syncRunCmd,
	}
	syncCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
		"Force overwrite of existing remote repositories.")
	syncCmd.PersistentFlags().BoolVarP(&newtutil.NewtAsk,
		"ask", "a", false, "Prompt user before syncing any repos")
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
	infoCmd.PersistentFlags().BoolVarP(&infoRemote,
		"remote", "r", false,
		"Fetch latest repos to determine if upgrades are required")

	cmd.AddCommand(infoCmd)
}
