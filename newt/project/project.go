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
	"log"
	"os"
	"path"
	"strings"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/downloader"
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
	Name string

	// Base path of the project
	BasePath string

	packages map[string]*map[string]*pkg.LocalPackage

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

func (proj *Project) loadRepo(rname string, v *viper.Viper) error {
	//rType := v.GetString(rname + ".type")
	//if rType != "github" {
	//return util.NewNewtError(fmt.Sprintf("Unknown repo type %s, only github supported.", rType))
	//}

	user := v.GetString(rname + ".user")
	gitRepo := v.GetString(rname + ".repo")

	dl := downloader.NewGithubDownloader()
	dl.User = user
	dl.Repo = gitRepo

	r, err := repo.NewRepo(proj.Path(), rname, dl)
	if err != nil {
		return err
	}
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

	proj.Name = v.GetString("project.name")

	// Local repository always included in initialization
	r, err := repo.NewLocalRepo(proj.BasePath)
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

	proj.repos = map[string]*repo.Repo{}
	proj.packageSearchDirs = PackageSearchDirs

	// Load Project configuration
	if err := proj.loadConfig(); err != nil {
		return err
	}

	return nil
}

func (proj *Project) ResolveDependency(dep *pkg.Dependency) (*pkg.LocalPackage, error) {
	for _, pkgList := range proj.packages {
		for _, pkg := range *pkgList {
			if dep.SatisfiesDependency(pkg) {
				return pkg, nil
			}
		}
	}

	return nil, util.NewNewtError("Can't resolve dependency: " + dep.Name)
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
	proj.packages = map[string]*map[string]*pkg.LocalPackage{}

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

func (proj *Project) PackageList() map[string]*map[string]*pkg.LocalPackage {
	return proj.packages
}

func LoadProject(dir string) (*Project, error) {
	projDir, err := findProjectDir(dir)
	if err != nil {
		return nil, err
	}

	proj, err := NewProject(projDir)
	return proj, err
}
