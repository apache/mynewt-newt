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

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
	"mynewt.apache.org/newt/yaml"
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
	repo        *repo.Repo
	name        string
	basePath    string
	packageType interfaces.PackageType

	// General information about the package
	desc *PackageDesc
	// Dependencies for this package
	deps []*Dependency
	// APIs that this package exports
	apis []string
	// APIs that this package requires
	reqApis []string

	// Pointer to pkg.yml configuration structure
	Viper *viper.Viper
}

func NewLocalPackage(r *repo.Repo, pkgDir string) *LocalPackage {
	pkg := &LocalPackage{
		desc: &PackageDesc{},
	}
	pkg.Init(r, pkgDir)
	return pkg
}

func (pkg *LocalPackage) Name() string {
	return pkg.name
}

func (pkg *LocalPackage) FullName() string {
	r := pkg.Repo()
	if r.IsLocal() {
		return pkg.Name()
	} else {
		return cli.BuildPackageString(r.Name(), pkg.Name())
	}
}

func (pkg *LocalPackage) BasePath() string {
	return pkg.basePath
}

func (pkg *LocalPackage) Type() interfaces.PackageType {
	return pkg.packageType
}

func (pkg *LocalPackage) Repo() interfaces.RepoInterface {
	return pkg.repo
}

func (pkg *LocalPackage) Desc() *PackageDesc {
	return pkg.desc
}

func (pkg *LocalPackage) SetName(name string) {
	pkg.name = name
}

func (pkg *LocalPackage) SetBasePath(basePath string) {
	pkg.basePath = basePath
}

func (pkg *LocalPackage) SetType(packageType interfaces.PackageType) {
	pkg.packageType = packageType
}

func (pkg *LocalPackage) SetDesc(desc *PackageDesc) {
	pkg.desc = desc
}

func (pkg *LocalPackage) SetRepo(r *repo.Repo) {
	pkg.repo = r
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

func (pkg *LocalPackage) AddApi(api string) {
	pkg.apis = append(pkg.apis, api)
}

func (pkg *LocalPackage) Apis() []string {
	return pkg.apis
}

func (pkg *LocalPackage) AddReqApi(api string) {
	pkg.reqApis = append(pkg.reqApis, api)
}

func (pkg *LocalPackage) ReqApis() []string {
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

// Load reads everything that isn't identity specific into the
// package
func (pkg *LocalPackage) Init(repo *repo.Repo, pkgDir string) {
	pkg.repo = repo
	pkg.basePath = filepath.Clean(pkgDir) + "/"
}

// Saves the package's pkg.yml file.
// NOTE: This does not save every field in the package.  Only the fields
// necessary for creating a new target get saved.
func (pkg *LocalPackage) Save() error {
	dirpath := pkg.BasePath()
	if err := os.MkdirAll(dirpath, 0755); err != nil {
		return util.NewNewtError(err.Error())
	}

	filepath := dirpath + "/" + PACKAGE_FILE_NAME
	file, err := os.Create(filepath)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer file.Close()

	file.WriteString("### Package: " + pkg.Name() + "\n")

	file.WriteString("pkg.name: " + yaml.EscapeString(pkg.Name()) + "\n")
	file.WriteString("pkg.type: " +
		yaml.EscapeString(PackageTypeNames[pkg.Type()]) + "\n")
	file.WriteString("pkg.description: " +
		yaml.EscapeString(pkg.Desc().Description) + "\n")
	file.WriteString("pkg.author: " +
		yaml.EscapeString(pkg.Desc().Author) + "\n")
	file.WriteString("pkg.homepage: " +
		yaml.EscapeString(pkg.Desc().Homepage) + "\n")
	file.WriteString("pkg.repository: " +
		yaml.EscapeString(pkg.Repo().Name()) + "\n")

	return nil
}

func (pkg *LocalPackage) Load() error {
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

	typeString := v.GetString("pkg.type")
	pkg.packageType = PACKAGE_TYPE_LIB
	for t, n := range PackageTypeNames {
		if typeString == n {
			pkg.packageType = t
			break
		}
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
	pkg.Init(repo, pkgDir)
	err := pkg.Load()
	return pkg, err
}

func LocalPackageSpecialName(dirName string) bool {
	_, ok := LocalPackageSpecialNames[dirName]
	return ok
}

func ReadLocalPackageRecursive(repo *repo.Repo,
	pkgList map[string]interfaces.PackageInterface, basePath string,
	pkgName string) error {

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

	if util.NodeNotExist(basePath + "/" + pkgName + "/" + PACKAGE_FILE_NAME) {
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
	searchPaths []string) (*map[string]interfaces.PackageInterface, error) {

	pkgList := map[string]interfaces.PackageInterface{}

	for _, path := range searchPaths {
		pkgDir := basePath + "/" + path

		if util.NodeNotExist(pkgDir) {
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
