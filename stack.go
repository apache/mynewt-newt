/*
 Copyright 2015 Stack Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package main

import (
	"fmt"
	"github.com/micosa/stack/cli"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var ExitOnFailure bool = false
var ExportAll bool = false
var ImportAll bool = false
var StackVersion string = "1.0"
var StackRepo *cli.Repo
var StackLogLevel string = ""
var InstallSubmodule bool = false
var InstallSubtree bool = false
var InstallBranch string = "master"

func StackUsage(cmd *cobra.Command, err error) {
	if err != nil {
		sErr := err.(*cli.StackError)
		log.Printf("[DEBUG] %s", sErr.StackTrace)
		fmt.Println("Error: ", sErr.Text)
	}

	if cmd != nil {
		cmd.Usage()
	}
	os.Exit(1)
}

func addInstallFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVarP(&InstallSubmodule, "submodule", "m", false,
		"Install remote repository as a git submodule")
	cmd.PersistentFlags().BoolVarP(&InstallSubtree, "subtree", "t", false,
		"Install remote repostiory as a git subtree")
	cmd.PersistentFlags().StringVarP(&InstallBranch, "branch", "b", "master",
		"Branch name to fetch from remote git repository")
}

func targetSetCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		StackUsage(cmd,
			cli.NewStackError("Must specify two arguments (sect & k=v) to set"))
	}

	t, err := cli.LoadTarget(StackRepo, args[0])
	if err != nil {
		StackUsage(cmd, err)
	}
	ar := strings.Split(args[1], "=")

	t.Vars[ar[0]] = ar[1]

	err = t.Save()
	if err != nil {
		StackUsage(cmd, err)
	}

	fmt.Printf("Target %s successfully set %s to %s\n", args[0],
		ar[0], ar[1])
}

func targetShowCmd(cmd *cobra.Command, args []string) {
	dispSect := ""
	if len(args) == 1 {
		dispSect = args[0]
	}

	targets, err := cli.GetTargets(StackRepo)
	if err != nil {
		StackUsage(cmd, err)
	}

	for _, target := range targets {
		if dispSect == "" || dispSect == target.Vars["name"] {
			fmt.Println(target.Vars["name"])
			vars := target.GetVars()
			for k, v := range vars {
				fmt.Printf("	%s: %s\n", k, v)
			}
		}
	}
}

func targetCreateCmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		StackUsage(cmd, cli.NewStackError("Wrong number of args to create cmd."))
	}

	fmt.Println("Creating target " + args[0])

	if cli.TargetExists(StackRepo, args[0]) {
		StackUsage(cmd, cli.NewStackError(
			"Target already exists, cannot create target with same name."))
	}

	target := &cli.Target{
		Repo: StackRepo,
		Vars: map[string]string{},
	}
	target.Vars["name"] = args[0]
	target.Vars["arch"] = args[1]

	err := target.Save()
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Printf("Target %s sucessfully created!\n", args[0])
	}
}

func targetBuildCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		StackUsage(cmd, cli.NewStackError("Must specify target to build"))
	}

	t, err := cli.LoadTarget(StackRepo, args[0])
	if err != nil {
		StackUsage(cmd, err)
	}

	if len(args) > 1 && args[1] == "clean" {
		if len(args) > 2 && args[2] == "all" {
			err = t.BuildClean(true)
		} else {
			err = t.BuildClean(false)
		}
	} else {
		err = t.Build()
	}

	if err != nil {
		StackUsage(cmd, err)
	} else {
		fmt.Println("Successfully run!")
	}
}

func targetDelCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		StackUsage(cmd, cli.NewStackError("Must specify target to delete"))
	}

	t, err := cli.LoadTarget(StackRepo, args[0])
	if err != nil {
		StackUsage(cmd, err)
	}

	if err := t.Remove(); err != nil {
		StackUsage(cmd, err)
	}

	fmt.Printf("Target %s successfully removed\n", args[0])
}

func targetTestCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		StackUsage(cmd, cli.NewStackError("Must specify target to build"))
	}

	t, err := cli.LoadTarget(StackRepo, args[0])
	if err != nil {
		StackUsage(cmd, err)
	}

	if len(args) > 1 && args[1] == "clean" {
		if len(args) > 2 && args[2] == "all" {
			err = t.Test("testclean", true)
		} else {
			err = t.Test("testclean", false)
		}
	} else {
		err = t.Test("test", ExitOnFailure)
	}

	if err != nil {
		StackUsage(cmd, err)
	} else {
		fmt.Println("Successfully run!")
	}
}

func targetExportCmd(cmd *cobra.Command, args []string) {
	var targetName string
	if ExportAll {
		targetName = ""
	} else {
		if len(args) < 1 {
			StackUsage(cmd, cli.NewStackError("Must either specify -a flag or name of "+
				"target to export"))
		}
		targetName = args[0]
	}

	err := cli.ExportTargets(StackRepo, targetName, ExportAll, os.Stdout)
	if err != nil {
		StackUsage(cmd, err)
	}
}

func targetImportCmd(cmd *cobra.Command, args []string) {
	var targetName string
	if ImportAll {
		targetName = ""
	} else {
		if len(args) < 1 {
			StackUsage(cmd, cli.NewStackError("Must either specify -a flag or name of "+
				"target to import"))
		}

		targetName = args[0]
	}

	err := cli.ImportTargets(StackRepo, targetName, ImportAll, os.Stdin)
	if err != nil {
		StackUsage(cmd, err)
	}

	fmt.Println("Target(s) successfully imported!")
}

func targetAddCmds(base *cobra.Command) {
	targetCmd := &cobra.Command{
		Use:   "target",
		Short: "Set and view target information",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(StackLogLevel)

			var err error
			StackRepo, err = cli.NewRepo()
			if err != nil {
				StackUsage(nil, err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Set target configuration variable",
		Run:   targetSetCmd,
	}

	targetCmd.AddCommand(setCmd)

	delCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete target",
		Run:   targetDelCmd,
	}

	targetCmd.AddCommand(delCmd)

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a target",
		Run:   targetCreateCmd,
	}

	targetCmd.AddCommand(createCmd)

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "View target configuration variables",
		Run:   targetShowCmd,
	}

	targetCmd.AddCommand(showCmd)

	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build target",
		Run:   targetBuildCmd,
	}

	targetCmd.AddCommand(buildCmd)

	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test target",
		Run:   targetTestCmd,
	}

	targetCmd.AddCommand(testCmd)

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export target",
		Run:   targetExportCmd,
	}

	exportCmd.PersistentFlags().BoolVarP(&ExportAll, "export-all", "a", false,
		"If present, export all targets")

	targetCmd.AddCommand(exportCmd)

	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import target",
		Run:   targetImportCmd,
	}

	importCmd.PersistentFlags().BoolVarP(&ImportAll, "import-all", "a", false,
		"If present, import all targets")

	targetCmd.AddCommand(importCmd)

	base.AddCommand(targetCmd)
}

func repoCreateCmd(cmd *cobra.Command, args []string) {
	// must specify a repo name to create
	if len(args) != 1 {
		StackUsage(cmd, cli.NewStackError("Must specify a repo name to repo create"))
	}

	cwd, err := os.Getwd()
	if err != nil {
		StackUsage(cmd, err)
	}

	_, err = cli.CreateRepo(cwd, args[0])
	if err != nil {
		StackUsage(cmd, err)
	}

	fmt.Println("Repo " + args[0] + " successfully created!")
}

func repoAddCmds(baseCmd *cobra.Command) {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "Commands to manipulate the base repository",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a repository",
		Run:   repoCreateCmd,
	}

	repoCmd.AddCommand(createCmd)

	baseCmd.AddCommand(repoCmd)
}

func compilerCreateCmd(cmd *cobra.Command, args []string) {
	// must specify a compiler name to compiler create
	if len(args) != 1 {
		StackUsage(cmd, cli.NewStackError("Must specify a compiler name to compiler create"))
	}

	err := StackRepo.CreateCompiler(args[0])
	if err != nil {
		StackUsage(cmd, err)
	}

	fmt.Println("Compiler " + args[0] + " successfully created!")
}

func compilerInstallCmd(cmd *cobra.Command, args []string) {
	path := args[0]
	url := args[1]

	dirName := StackRepo.BasePath + "/" + path + "/"
	if cli.NodeExist(dirName) {
		StackUsage(cmd, cli.NewStackError("Compiler "+path+" already installed."))
	}

	// 0 = install clean
	// 1 = install submodule
	// 2 = install subtree
	installType := 0
	if InstallSubmodule && InstallSubtree {
		StackUsage(cmd,
			cli.NewStackError("Cannot specify both --submodule and --subtree options to "+
				"install command"))
	}

	if InstallSubmodule {
		installType = 1
	}

	if InstallSubtree {
		installType = 2
	}

	if err := cli.CopyUrl(url, InstallBranch, dirName, installType); err != nil {
		StackUsage(cmd, err)
	}

	fmt.Println("Compiler " + path + " successfully installed.")
}

func compilerAddCmds(baseCmd *cobra.Command) {
	compilerCmd := &cobra.Command{
		Use:   "compiler",
		Short: "Commands to install and create compiler definitions",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(StackLogLevel)

			var err error
			StackRepo, err = cli.NewRepo()
			if err != nil {
				StackUsage(nil, err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new compiler definition",
		Run:   compilerCreateCmd,
	}

	compilerCmd.AddCommand(createCmd)

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install a compiler from the specified URL",
		Run:   compilerInstallCmd,
	}

	addInstallFlags(installCmd)

	compilerCmd.AddCommand(installCmd)

	baseCmd.AddCommand(compilerCmd)
}

func runInstallCmd(cmd *cobra.Command, args []string) {
	inst, err := cli.NewInstaller(StackRepo)
	if err != nil {
		StackUsage(cmd, err)
	}

	if len(args) == 0 {
		StackUsage(cmd, cli.NewStackError("Must specify a URL to install"))
	}

	if err := inst.Install(args[0], cli.INSTALLER_GIT_CLONE_CLEAN); err != nil {
		StackUsage(cmd, err)
	}

	fmt.Println("Installation successfully completed!")
}

func installerAddCmds(baseCmd *cobra.Command) {
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install a package",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(StackLogLevel)

			var err error
			StackRepo, err = cli.NewRepo()
			if err != nil {
				StackUsage(nil, err)
			}
		},
		Run: runInstallCmd,
	}

	baseCmd.AddCommand(installCmd)
}

func dispPkg(pkg *cli.Package) error {
	fmt.Printf("Package %s, version %s\n", pkg.FullName, pkg.Version)
	fmt.Printf("  path: %s\n", filepath.Clean(pkg.BasePath))
	if pkg.Capabilities != nil {
		fmt.Printf("  capabilities: ")
		caps, err := pkg.GetCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range caps {
			fmt.Printf("%s ", capability)
		}
		fmt.Printf("\n")
	}
	if len(pkg.Deps) > 0 {
		fmt.Printf("  deps: ")
		for _, dep := range pkg.Deps {
			if dep == nil {
				continue
			}
			fmt.Printf("%s ", dep)
		}
		fmt.Printf("\n")
	}

	if pkg.LinkerScript != "" {
		fmt.Printf("  linkerscript: %s\n", pkg.LinkerScript)
	}
	return nil
}

func pkgListCmd(cmd *cobra.Command, args []string) {
	pkgMgr, err := cli.NewPkgMgr(StackRepo, nil)
	if err != nil {
		StackUsage(cmd, err)
	}

	for _, pkg := range pkgMgr.Packages {
		if err := dispPkg(pkg); err != nil {
			StackUsage(cmd, err)
		}
	}
}

func pkgCheckDepsCmd(cmd *cobra.Command, args []string) {
	pkgMgr, err := cli.NewPkgMgr(StackRepo, nil)
	if err != nil {
		StackUsage(cmd, err)
	}

	if err := pkgMgr.CheckDeps(); err != nil {
		StackUsage(cmd, err)
	}

	fmt.Println("Dependencies successfully resolved!")
}

func pkgAddCmds(baseCmd *cobra.Command) {
	pkgCmd := &cobra.Command{
		Use:   "pkg",
		Short: "Commands to list and inspect packages on a repo",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cli.Init(StackLogLevel)

			var err error
			StackRepo, err = cli.NewRepo()
			if err != nil {
				StackUsage(nil, err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			StackUsage(cmd, nil)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List packages in the current repository",
		Run:   pkgListCmd,
	}

	pkgCmd.AddCommand(listCmd)

	checkDepsCmd := &cobra.Command{
		Use:   "checkdeps",
		Short: "Check package dependencies",
		Run:   pkgCheckDepsCmd,
	}

	pkgCmd.AddCommand(checkDepsCmd)

	baseCmd.AddCommand(pkgCmd)
}

func parseCmds() *cobra.Command {
	stackCmd := &cobra.Command{
		Use:   "stack",
		Short: "Stack is a tool to help you compose and build your own OS",
		Long: `Stack allows you to create your own embedded project based on the
		     stack operating system`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}

	stackCmd.PersistentFlags().StringVarP(&StackLogLevel, "loglevel", "l",
		"WARN", "Log level, defaults to WARN.")

	versCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the stack version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Stack version: ", StackVersion)
		},
	}

	stackCmd.AddCommand(versCmd)

	targetAddCmds(stackCmd)
	repoAddCmds(stackCmd)
	compilerAddCmds(stackCmd)
	installerAddCmds(stackCmd)
	pkgAddCmds(stackCmd)

	return stackCmd
}

func main() {
	cmd := parseCmds()
	cmd.Execute()
}
