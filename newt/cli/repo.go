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
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
)

type Repo struct {
	// Name of the Repo
	Name string

	// Path to the Repo Store
	StorePath string

	// Path to the Repo PkgLists
	PkgListPath string

	// Repo File
	RepoFile string

	// Base path of the repo
	BasePath string

	// Store of PkgLists
	PkgLists map[string]*PkgList

	AddlPackagePaths []string

	// Targets
	Targets map[string]map[string]string
}

// Create a new Repo object and initialize it
func NewRepo() (*Repo, error) {
	n := &Repo{}

	err := n.Init()
	if err != nil {
		return nil, err
	}

	return n, nil
}

// Create a Repo object constructed out of repo in given path
func NewRepoWithDir(srcDir string) (*Repo, error) {
	n := &Repo{}

	err := n.InitPath(srcDir)
	if err != nil {
		return nil, err
	}

	return n, nil
}

func CreateRepo(repoName string, destDir string, tadpoleUrl string) error {
	if tadpoleUrl == "" {
		tadpoleUrl = "https://git-wip-us.apache.org/repos/asf/incubator-mynewt-tadpole.git"
	}

	if NodeExist(destDir) {
		return NewNewtError(fmt.Sprintf("Directory %s already exists, "+
			" cannot create new newt repo", destDir))
	}

	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Downloading application skeleton from %s...",
		tadpoleUrl)
	if err := dl.DownloadFile(tadpoleUrl, "master", "/",
		destDir); err != nil {
		return err
	}
	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	// Overwrite app.yml
	contents := []byte(fmt.Sprintf("app.name: %s\n", repoName))
	if err := ioutil.WriteFile(destDir+"/app.yml",
		contents, 0644); err != nil {
		return NewNewtError(err.Error())
	}

	// DONE!

	return nil
}

// Get a temporary directory to stick stuff in
func (repo *Repo) GetTmpDir(dirName string, prefix string) (string, error) {
	tmpDir := dirName
	if NodeNotExist(tmpDir) {
		if err := os.MkdirAll(tmpDir, 0700); err != nil {
			return "", err
		}
	}

	name, err := ioutil.TempDir(tmpDir, prefix)
	if err != nil {
		return "", err
	}

	return name, nil
}

// Find the repo file.  Searches the current directory, and then recurses
// parent directories until it finds a file named app.yml
// if no repo file found in the directory heirarchy, an error is returned
func (repo *Repo) getRepoFile() (string, error) {
	rFile := ""

	curDir, err := os.Getwd()
	if err != nil {
		return rFile, NewNewtError(err.Error())
	}

	for {
		rFile = curDir + "/app.yml"
		log.Printf("[DEBUG] Searching for repo file at %s", rFile)
		if _, err := os.Stat(rFile); err == nil {
			log.Printf("[DEBUG] Found repo file at %s!", rFile)
			break
		}

		curDir = path.Clean(curDir + "../../")
		if curDir == "/" {
			rFile = ""
			err = NewNewtError("No repo file found!")
			break
		}
	}

	return rFile, err
}

func (repo *Repo) TargetDir(targetName string) string {
	return repo.BasePath + "/targets/" + targetName
}

// Loads a single target definition residing in the specified directory.  The
// path must contain a pkg.yml file at the top level to be a valid target
// definion.
func (repo *Repo) loadTarget(path string) error {
	v, err := ReadConfig(path, "pkg")
	if err != nil {
		return err
	}

	targetName := filepath.Base(path)
	targetMap, ok := repo.Targets[targetName]
	if !ok {
		targetMap = make(map[string]string)
		repo.Targets[targetName] = targetMap
	}
	settings := v.AllSettings()
	for k, v := range settings {
		targetMap[k] = v.(string)
	}

	return nil
}

// Loads all target definitions rooted at the specified path.  Targets have the
// following file structure:
//     <path>/<target-name>/pkg.yml
func (repo *Repo) loadPath(path string) error {
	targetDirList, err := ioutil.ReadDir(path)
	if err != nil && !os.IsNotExist(err) {
		return NewNewtError(err.Error())
	}

	for _, node := range targetDirList {
		name := node.Name()
		if node.IsDir() &&
			!filepath.HasPrefix(name, ".") &&
			!filepath.HasPrefix(name, "..") {

			fullPath := path + name + "/"

			err = repo.loadTarget(fullPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Retrieves the key-value variable map for the target with the specified name.
// error is populated if variable doesn't exist
func (repo *Repo) GetTargetVars(name string) (map[string]string, error) {
	targetVars, ok := repo.Targets[name]
	if !ok {
		return nil, NewNewtError(fmt.Sprintf("Target not found: %s", name))
	}

	return targetVars, nil
}

func (repo *Repo) PkgPaths() []string {
	// Get additional package search directories from the repo
	// configuration. If empty, use the default set.

	searchDirs := []string{
		"app/",
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
		"sys/",
	}

	if len(repo.AddlPackagePaths) > 0 {
		searchDirs = append(searchDirs, repo.AddlPackagePaths...)
	}

	return searchDirs
}

// Load the repo configuration file
func (repo *Repo) loadConfig() error {
	v, err := ReadConfig(repo.BasePath, "app")
	if err != nil {
		return NewNewtError(err.Error())
	}

	repo.Name = v.GetString("app.name")
	if repo.Name == "" {
		return NewNewtError("Application file must specify application name")
	}

	repo.AddlPackagePaths = v.GetStringSlice("app.additional_package_paths")

	return nil
}

func (repo *Repo) LoadPkgLists() error {
	files, err := ioutil.ReadDir(repo.PkgListPath)
	if err != nil {
		return err
	}
	for _, fileInfo := range files {
		file := fileInfo.Name()
		if filepath.Ext(file) == ".yml" {
			name := file[:len(filepath.Base(file))-len(".yml")]
			log.Printf("[DEBUG] Loading PkgList %s", name)
			pkgList, err := NewPkgList(repo)
			if err != nil {
				return err
			}
			if err := pkgList.Load(name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (repo *Repo) InitPath(repoPath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return NewNewtError(err.Error())
	}

	if err = os.Chdir(repoPath); err != nil {
		return NewNewtError(err.Error())
	}

	log.Printf("[DEBUG] Searching for repository, starting in directory %s", cwd)

	if repo.RepoFile, err = repo.getRepoFile(); err != nil {
		return err
	}

	log.Printf("[DEBUG] Repo file found, directory %s, loading configuration...",
		repo.RepoFile)

	repo.BasePath = filepath.ToSlash(path.Dir(repo.RepoFile))

	if err = repo.loadConfig(); err != nil {
		return err
	}

	if err = os.Chdir(cwd); err != nil {
		return NewNewtError(err.Error())
	}
	return nil
}

// Initialze the repository
// returns a NewtError on failure, and nil on success
func (repo *Repo) Init() error {
	var err error

	cwd, err := os.Getwd()
	if err != nil {
		return NewNewtError(err.Error())
	}
	if err := repo.InitPath(cwd); err != nil {
		return err
	}

	log.Printf("[DEBUG] Configuration loaded")

	// Create Repo store directory
	repo.StorePath = repo.BasePath + "/.app/"
	if NodeNotExist(repo.StorePath) {
		if err := os.MkdirAll(repo.StorePath, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	// Load target YAML files.
	repo.Targets = make(map[string]map[string]string)
	if err := repo.loadPath(repo.BasePath + "/targets/"); err != nil {
		return err
	}

	// Load PkgLists for the current Repo
	repo.PkgListPath = repo.StorePath + "/pkg-lists/"
	if NodeNotExist(repo.PkgListPath) {
		if err := os.MkdirAll(repo.PkgListPath, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	repo.PkgLists = map[string]*PkgList{}

	if err := repo.LoadPkgLists(); err != nil {
		return err
	}

	return nil
}

func (repo *Repo) GetPkgLists() (map[string]*PkgList, error) {
	return repo.PkgLists, nil
}
