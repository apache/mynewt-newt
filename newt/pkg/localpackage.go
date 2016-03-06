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

package pkg

import (
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"mynewt.apache.org/newt/viper"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
)

var PackageHashIgnoreDirs = map[string]bool{
	"obj": true,
	"bin": true,
	".":   true,
}

var LocalPackageSpecialNames = map[string]bool{
	"src":     true,
	"include": true,
	"bin":     true,
}

type LocalPackage struct {
	repo     *repo.Repo
	name     string
	basePath string

	// General information about the package
	desc *PackageDesc
	// Version information about this package
	vers *Version
	// Dependencies for this package
	deps []*Dependency
	// APIs that this package exports
	apis []*Dependency
	// APIs that this package requires
	reqApis []*Dependency

	// Pointer to pkg.yml configuration structure
	Viper *viper.Viper
}

func (pkg *LocalPackage) Name() string {
	return pkg.name
}

func (pkg *LocalPackage) Repo() *repo.Repo {
	return pkg.repo
}

func (pkg *LocalPackage) Hash() (string, error) {
	hash := sha1.New()

	err := filepath.Walk(pkg.basePath,
		func(path string, info os.FileInfo, err error) error {
			name := info.Name()
			if PackageHashIgnoreDirs[name] {
				return filepath.SkipDir
			}

			if info.IsDir() {
				// SHA the directory name into the hash
				hash.Write([]byte(name))
			} else {
				// SHA the file name & contents into the hash
				contents, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}
				hash.Write(contents)
			}
			return nil
		})
	if err != nil && err != filepath.SkipDir {
		return "", util.NewNewtError(err.Error())
	}

	hashStr := fmt.Sprintf("%x", hash.Sum(nil))

	return hashStr, nil
}

func (pkg *LocalPackage) Desc() *PackageDesc {
	return pkg.desc
}

func (pkg *LocalPackage) Vers() *Version {
	return pkg.vers
}

func (pkg *LocalPackage) HasDep(searchDep *Dependency) bool {
	for _, dep := range pkg.deps {
		if dep.String() == searchDep.String() {
			return true
		}
	}
	return false
}

func (pkg *LocalPackage) AddDep(dep *Dependency) {
	pkg.deps = append(pkg.deps, dep)
}

func (pkg *LocalPackage) Deps() []*Dependency {
	return pkg.deps
}

func (pkg *LocalPackage) AddApi(api *Dependency) {
	pkg.apis = append(pkg.apis, api)
}

func (pkg *LocalPackage) Apis() []*Dependency {
	return pkg.apis
}

func (pkg *LocalPackage) AddReqApi(api *Dependency) {
	pkg.reqApis = append(pkg.reqApis, api)
}

func (pkg *LocalPackage) ReqApis() []*Dependency {
	return pkg.reqApis
}

func (pkg *LocalPackage) readDesc(v *viper.Viper) (*PackageDesc, error) {
	pdesc := &PackageDesc{}

	pdesc.Author = v.GetString("pkg.author")
	pdesc.Homepage = v.GetString("pkg.homepage")
	pdesc.Description = v.GetString("pkg.description")
	pdesc.Keywords = v.GetStringSlice("pkg.keywords")

	return pdesc, nil
}

// Init reads everything that isn't identity specific into the
// package
func (pkg *LocalPackage) Init(repo *repo.Repo, pkgDir string) error {
	var err error

	pkg.repo = repo

	pkg.basePath = filepath.Clean(pkgDir) + "/"

	// Load configuration
	log.Printf("[DEBUG] Loading configuration for package %s", pkg.basePath)

	v, err := util.ReadConfig(pkg.basePath,
		strings.TrimSuffix(PACKAGE_FILE_NAME, ".yml"))
	if err != nil {
		return err
	}
	pkg.Viper = v

	// Set package name from the package
	pkg.name = v.GetString("pkg.name")

	switch v.GetString("pkg.type") {
	case PACKAGE_TYPE_STR_LIB:
		pkg.packageType = PACKAGE_TYPE_LIB

	case PACKAGE_TYPE_STR_BSP:
		pkg.packageType = PACKAGE_TYPE_BSP

	case PACKAGE_TYPE_STR_APP:
		pkg.packageType = PACKAGE_TYPE_APP

	case PACKAGE_TYPE_STR_TARGET:
		pkg.packageType = PACKAGE_TYPE_TARGET

	default:
		pkg.packageType = PACKAGE_TYPE_LIB
	}

	// Get the package version
	pkg.vers, err = LoadVersion(v.GetString("pkg.vers"))
	if err != nil {
		return err
	}

	// Read the package description from the file
	pkg.desc, err = pkg.readDesc(v)
	if err != nil {
		return err
	}

	return nil
}

func LoadLocalPackage(repo *repo.Repo, pkgDir string) (*LocalPackage, error) {
	pkg := &LocalPackage{}

	if err := pkg.Init(repo, pkgDir); err != nil {
		return nil, err
	}

	return pkg, nil
}

func LocalPackageSpecialName(dirName string) bool {
	_, ok := LocalPackageSpecialNames[dirName]
	return ok
}

func ReadLocalPackageRecursive(repo *repo.Repo, pkgList map[string]*LocalPackage,
	basePath string, pkgName string) error {

	dirList, err := ioutil.ReadDir(basePath + "/" + pkgName)
	if err != nil {
		return util.NewNewtError(err.Error())
	}

	for _, dirEnt := range dirList {
		if !dirEnt.IsDir() {
			continue
		}

		name := dirEnt.Name()
		if LocalPackageSpecialName(name) || strings.HasPrefix(name, ".") {
			continue
		}

		if err := ReadLocalPackageRecursive(repo, pkgList, basePath,
			pkgName+"/"+name); err != nil {
			return err
		}
	}

	if cli.NodeNotExist(basePath + "/" + pkgName + "/" + PACKAGE_FILE_NAME) {
		return nil
	}

	pkg, err := LoadLocalPackage(repo, basePath+"/"+pkgName)
	if err != nil {
		return err
	}
	pkgList[pkg.Name()] = pkg

	return nil
}

func ReadLocalPackages(repo *repo.Repo, basePath string,
	searchPaths []string) (*map[string]*LocalPackage, error) {

	pkgList := map[string]*LocalPackage{}

	for _, path := range searchPaths {
		pkgDir := basePath + "/" + path

		if cli.NodeNotExist(pkgDir) {
			continue
		}

		dirList, err := ioutil.ReadDir(pkgDir)
		if err != nil {
			return nil, util.NewNewtError(err.Error())
		}

		for _, subDir := range dirList {
			name := subDir.Name()
			if filepath.HasPrefix(name, ".") || filepath.HasPrefix(name, "..") {
				continue
			}

			if !subDir.IsDir() {
				continue
			}

			if err := ReadLocalPackageRecursive(repo, pkgList, pkgDir,
				name); err != nil {
				return nil, util.NewNewtError(err.Error())
			}
		}
	}

	return &pkgList, nil
}
