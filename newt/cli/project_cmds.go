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

	/* For new command don't use shallow copy by default
	 * as release tag may not be present on tip of master
	 * branch.
	 */
	if util.ShallowCloneDepth < 0 {
		util.ShallowCloneDepth = 0
	}

	if err := dl.Clone("master", tmpdir); err != nil {
		NewtUsage(nil, err)
	}

	commit, err := dl.LatestRc(tmpdir, newtutil.NewtBlinkyTag)
	if err != nil {
		NewtUsage(nil, err)
	}

	err = dl.Checkout(tmpdir, commit)
	if err != nil {
		NewtUsage(nil, err)
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Installing "+
		"skeleton in %s (commit: %s)\n", newDir, commit)

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

func upgradeRunCmd(cmd *cobra.Command, args []string) {
	proj := TryGetOrDownloadProject()
	interfaces.SetProject(proj)

	proj.GetPkgRepos()

	pred := makeRepoPredicate(args)
	if err := proj.UpgradeIf(
		newtutil.NewtForce, newtutil.NewtAsk, pred); err != nil {

		NewtUsage(nil, err)
	}
}

func infoRunCmd(cmd *cobra.Command, args []string) {
	newtutil.PrintNewtVersion()

	proj := TryGetProject()

	// If no arguments specified, print status of all installed repos.
	if len(args) == 0 {
		pred := func(r *repo.Repo) bool { return true }
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

func AddProjectCommands(cmd *cobra.Command) {
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
