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
	"bufio"
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
)

var globalProject *Project = nil

const PROJECT_FILE_NAME = "project.yml"

var PackageSearchDirs []string = []string{
	"apps/",
	"compiler/",
	"fs/",
	"libs/",
	"net/",
	"hw/bsp/",
	"hw/mcu/",
	"hw/mcu/stm",
	"hw/drivers/",
	"hw/",
	"project/",
	"targets/",
	"sys/",
}

type Project struct {
	// Name of this project
	name string

	// Base path of the project
	BasePath string

	packages interfaces.PackageList

	projState *ProjectState

	// Repositories configured on this project
	repos map[string]*repo.Repo

	localRepo *repo.Repo

	// Package search directories for this project
	packageSearchDirs []string

	v *viper.Viper
}

func InitProject(dir string) error {
	var err error

	globalProject, err = LoadProject(dir)
	if err != nil {
		return err
	}
	globalProject.LoadPackageList()

	return nil
}

func GetProject() *Project {
	if globalProject == nil {
		wd, err := os.Getwd()
		if err != nil {
			panic(err.Error())
		}
		err = InitProject(wd)
		if err != nil {
			panic(err.Error())
		}
	}
	return globalProject
}

func NewProject(dir string) (*Project, error) {
	proj := &Project{}

	if err := proj.Init(dir); err != nil {
		return nil, err
	}

	return proj, nil
}

func (proj *Project) Path() string {
	return proj.BasePath
}

func (proj *Project) Name() string {
	return proj.name
}

func (proj *Project) Repos() map[string]*repo.Repo {
	return proj.repos
}

func (proj *Project) FindRepo(rname string) *repo.Repo {
	r, _ := proj.repos[rname]
	return r
}

func (proj *Project) LocalRepo() *repo.Repo {
	return proj.localRepo
}

func (proj *Project) PackageSearchDirs() []string {
	return proj.packageSearchDirs
}

func (proj *Project) Install(upgrade bool, force bool) error {
	repoList := proj.Repos()
	for rname, r := range repoList {
		// Ignore the local repo on install
		if rname == repo.REPO_NAME_LOCAL {
			continue
		}
		// Download repository description.  Try and resolve this
		// repository out of the repo list (repodesc for each repo.)
		// If matching repository is found, then prompt for install and install
		// that repository.
		if err := r.DownloadDesc(); err != nil {
			return err
		}

		rdesc, err := r.ReadDesc()
		if err != nil {
			return err
		}

		vers := proj.projState.GetInstalledVersion(rname)
		if vers != nil {
			ok := rdesc.SatisfiesVersion(vers, r.VersionRequirements())
			if !ok && !upgrade {
				fmt.Printf("WARNING: Installed version %s of repository %s "+
					"does not match desired versions %s in project\n",
					vers, rname, r.VersionRequirementsString())
				continue
			}

			if !upgrade {
				cli.StatusMessage(cli.VERBOSITY_DEFAULT,
					fmt.Sprintf("Installed repository %s matches project requirements, "+
						"moving on to next repository.\n", r.Name()))
				continue
			} else {
				_, newVers, _ := rdesc.Match(r)
				if newVers == nil {
					return util.NewNewtError(fmt.Sprintf(
						"No matching version for repository %s\n", r.Name()))
				}
				if vers.CompareVersions(newVers, vers) != 0 ||
					vers.Stability() != newVers.Stability() {
					// Prompt for upgrade, unless force option present
					if !force {
						branch, newVers, _ := rdesc.Match(r)

						str := ""
						if newVers.Stability() != repo.VERSION_STABILITY_NONE {
							str += "(" + branch + ")"
						}

						fmt.Printf("Would you like to upgrade repository %s from %s to %s %s? [Yn] ",
							r.Name(), vers.String(), newVers.String(), str)

						line, more, err := bufio.NewReader(os.Stdin).ReadLine()
						if more || err != nil {
							return util.NewNewtError(fmt.Sprintf(
								"Couldn't read upgrade response: %s\n", err.Error()))
						}
						answer := strings.ToUpper(strings.Trim(string(line), " "))
						if answer == "N" || answer == "NO" {
							fmt.Printf("Users says don't upgrade, skipping upgrade of %s\n",
								r.Name())
							continue
						}

					}
				} else {
					fmt.Printf("Repository %s doesn't need to be upgraded, latest "+
						"version installed.\n", r.Name())
					continue
				}
			}
		}

		_, _, ok := rdesc.Match(r)
		if !ok {
			fmt.Printf("WARNING: No matching repository version found for repository "+
				"%s specified in project, skipping...\n", r.Name())
			continue
		}

		rvers, err := r.Install(rdesc, upgrade || force)
		if err != nil {
			return err
		}

		if rvers != nil {
			proj.projState.Replace(rname, rvers)
		}
	}

	if err := proj.projState.Save(); err != nil {
		return err
	}

	return nil
}

func (proj *Project) Upgrade(force bool) error {
	return proj.Install(true, force)
}

func (proj *Project) loadRepo(rname string, v *viper.Viper) error {
	varName := fmt.Sprintf("repository.%s", rname)

	repoVars := v.GetStringMapString(varName)

	if repoVars["type"] != "github" {
		return util.NewNewtError("Only github repositories are currently supported.")
	}
	rversreq := repoVars["vers"]

	dl := downloader.NewGithubDownloader()
	dl.User = repoVars["user"]
	dl.Repo = repoVars["repo"]

	r, err := repo.NewRepo(rname, rversreq, dl)
	if err != nil {
		return err
	}

	log.Printf("[VERBOSE] Loaded repository %s (type: %s, user: %s, repo: %s)", rname,
		repoVars["type"], repoVars["user"], repoVars["repo"])

	proj.repos[r.Name()] = r
	return nil
}

func (proj *Project) loadConfig() error {
	v, err := util.ReadConfig(proj.BasePath,
		strings.TrimSuffix(PROJECT_FILE_NAME, ".yml"))
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	// Store configuration object for access to future values,
	// this avoids keeping every string around as a project variable when
	// we need to process it later.
	proj.v = v

	proj.projState, err = LoadProjectState()
	if err != nil {
		return err
	}

	proj.name = v.GetString("project.name")

	// Local repository always included in initialization
	r, err := repo.NewLocalRepo()
	if err != nil {
		return err
	}
	proj.repos[r.Name()] = r
	proj.localRepo = r

	rstrs := v.GetStringSlice("project.repositories")
	for _, repoName := range rstrs {
		if err := proj.loadRepo(repoName, v); err != nil {
			return err
		}
	}

	pkgDirs := v.GetStringSlice("project.pkg_dirs")
	if len(pkgDirs) > 0 {
		proj.packageSearchDirs = append(proj.packageSearchDirs, pkgDirs...)
	}

	return nil
}

func (proj *Project) Init(dir string) error {
	proj.BasePath = dir

	// Only one project per system, when created, set it as the global project
	interfaces.SetProject(proj)

	proj.repos = map[string]*repo.Repo{}
	proj.packageSearchDirs = PackageSearchDirs

	// Load Project configuration
	if err := proj.loadConfig(); err != nil {
		return err
	}

	return nil
}

func (proj *Project) ResolveDependency(dep interfaces.DependencyInterface) interfaces.PackageInterface {
	for _, pkgList := range proj.packages {
		for _, pkg := range *pkgList {
			if dep.SatisfiesDependency(pkg) {
				return pkg
			}
		}
	}

	return nil
}

func findProjectDir(dir string) (string, error) {
	for {
		projFile := path.Clean(dir) + "/" + PROJECT_FILE_NAME

		log.Printf("[DEBUG] Searching for project file %s", projFile)
		if cli.NodeExist(projFile) {
			log.Printf("[INFO] Project file found at %s", projFile)
			break
		}

		// Move back one directory and continue searching
		dir = path.Clean(dir + "../../")
		if dir == "/" {
			return "", util.NewNewtError("No project file found!")
		}
	}

	return dir, nil
}

func (proj *Project) LoadPackageList() error {
	proj.packages = interfaces.PackageList{}

	// Go through a list of repositories, starting with local, and search for
	// packages / store them in the project package list.
	repos := proj.Repos()
	for name, repo := range repos {
		log.Printf("[VERBOSE] Loading packages in repository %s", repo.Path())
		list, err := pkg.ReadLocalPackages(repo, repo.Path(), proj.PackageSearchDirs())
		if err != nil {
			return err
		}

		proj.packages[name] = list
	}

	return nil
}

func (proj *Project) PackageList() interfaces.PackageList {
	return proj.packages
}

func (proj *Project) PackagesOfType(pkgType interfaces.PackageType) []interfaces.PackageInterface {
	matches := []interfaces.PackageInterface{}

	packs := proj.PackageList()
	for _, packHash := range packs {
		for _, pack := range *packHash {
			if pack.Type() == pkgType {
				matches = append(matches, pack)
			}
		}
	}

	return matches
}

func LoadProject(dir string) (*Project, error) {
	projDir, err := findProjectDir(dir)
	if err != nil {
		return nil, err
	}

	proj, err := NewProject(projDir)

	return proj, err
}
