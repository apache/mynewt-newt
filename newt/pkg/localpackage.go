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
	"bytes"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
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

	// Package init function name and stage.  These are used to generate the
	// sysinit C file.
	init map[string]int

	// Extra package-specific settings that don't come from syscfg.  For
	// example, SELFTEST gets set when the newt test command is used.
	injectedSettings map[string]string

	// Settings read from pkg.yml.
	PkgV *viper.Viper

	// Settings read from syscfg.yml.
	SyscfgV *viper.Viper

	// Names of all source yml files; used to determine if rebuild required.
	cfgFilenames []string
}

func NewLocalPackage(r *repo.Repo, pkgDir string) *LocalPackage {
	pkg := &LocalPackage{
		desc:             &PackageDesc{},
		PkgV:             viper.New(),
		SyscfgV:          viper.New(),
		repo:             r,
		basePath:         filepath.ToSlash(filepath.Clean(pkgDir)),
		init:             map[string]int{},
		injectedSettings: map[string]string{},
	}
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
		return newtutil.BuildPackageString(r.Name(), pkg.Name())
	}
}

func (pkg *LocalPackage) BasePath() string {
	return pkg.basePath
}

func (pkg *LocalPackage) RelativePath() string {
	proj := interfaces.GetProject()
	return strings.TrimPrefix(pkg.BasePath(), proj.Path())
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
	// XXX: Also set "pkg.name" in viper object (possibly just remove cached
	// variable from code entirely).
}

func (pkg *LocalPackage) SetBasePath(basePath string) {
	pkg.basePath = filepath.ToSlash(filepath.Clean(basePath))
}

func (pkg *LocalPackage) SetType(packageType interfaces.PackageType) {
	pkg.packageType = packageType
	// XXX: Also set "pkg.type" in viper object (possibly just remove cached
	// variable from code entirely).
}

func (pkg *LocalPackage) SetDesc(desc *PackageDesc) {
	pkg.desc = desc
	// XXX: Also set desc fields in viper object (possibly just remove cached
	// variable from code entirely).
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

func (pkg *LocalPackage) CfgFilenames() []string {
	return pkg.cfgFilenames
}

func (pkg *LocalPackage) AddCfgFilename(cfgFilename string) {
	pkg.cfgFilenames = append(pkg.cfgFilenames, cfgFilename)
}

func (pkg *LocalPackage) readDesc(v *viper.Viper) (*PackageDesc, error) {
	pdesc := &PackageDesc{}

	pdesc.Author = v.GetString("pkg.author")
	pdesc.Homepage = v.GetString("pkg.homepage")
	pdesc.Description = v.GetString("pkg.description")
	pdesc.Keywords = v.GetStringSlice("pkg.keywords")

	return pdesc, nil
}

func (pkg *LocalPackage) sequenceString(key string) string {
	var buffer bytes.Buffer

	if pkg.PkgV != nil {
		for _, f := range pkg.PkgV.GetStringSlice(key) {
			buffer.WriteString("    - " + yaml.EscapeString(f) + "\n")
		}
	}

	if buffer.Len() == 0 {
		return ""
	} else {
		return key + ":\n" + buffer.String()
	}
}

func (lpkg *LocalPackage) SaveSyscfgVals() error {
	dirpath := lpkg.BasePath()
	if err := os.MkdirAll(dirpath, 0755); err != nil {
		return util.NewNewtError(err.Error())
	}

	filepath := dirpath + "/" + SYSCFG_YAML_FILENAME

	syscfgVals := lpkg.SyscfgV.GetStringMapString("syscfg.vals")
	if syscfgVals == nil || len(syscfgVals) == 0 {
		os.Remove(filepath)
		return nil
	}

	file, err := os.Create(filepath)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer file.Close()

	names := make([]string, 0, len(syscfgVals))
	for k, _ := range syscfgVals {
		names = append(names, k)
	}
	sort.Strings(names)

	fmt.Fprintf(file, "### Package: %s\n", lpkg.Name())
	fmt.Fprintf(file, "\n")
	fmt.Fprintf(file, "syscfg.vals:\n")
	for _, name := range names {
		fmt.Fprintf(file, "    %s: %s\n", name, yaml.EscapeString(syscfgVals[name]))
	}

	return nil
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

	// XXX: Just iterate viper object's settings rather than calling out
	// cached settings individually.
	file.WriteString("pkg.name: " + yaml.EscapeString(pkg.Name()) + "\n")
	file.WriteString("pkg.type: " +
		yaml.EscapeString(PackageTypeNames[pkg.Type()]) + "\n")
	file.WriteString("pkg.description: " +
		yaml.EscapeString(pkg.Desc().Description) + "\n")
	file.WriteString("pkg.author: " +
		yaml.EscapeString(pkg.Desc().Author) + "\n")
	file.WriteString("pkg.homepage: " +
		yaml.EscapeString(pkg.Desc().Homepage) + "\n")

	file.WriteString("\n")

	file.WriteString(pkg.sequenceString("pkg.aflags"))
	file.WriteString(pkg.sequenceString("pkg.cflags"))
	file.WriteString(pkg.sequenceString("pkg.lflags"))

	return nil
}

// Load reads everything that isn't identity specific into the
// package
func (pkg *LocalPackage) Load() error {
	// Load configuration
	log.Debugf("Loading configuration for package %s", pkg.basePath)

	var err error

	pkg.PkgV, err = util.ReadConfig(pkg.basePath,
		strings.TrimSuffix(PACKAGE_FILE_NAME, ".yml"))
	if err != nil {
		return err
	}
	pkg.AddCfgFilename(pkg.basePath + "/" + PACKAGE_FILE_NAME)

	// Set package name from the package
	pkg.name = pkg.PkgV.GetString("pkg.name")

	typeString := pkg.PkgV.GetString("pkg.type")
	pkg.packageType = PACKAGE_TYPE_LIB
	for t, n := range PackageTypeNames {
		if typeString == n {
			pkg.packageType = t
			break
		}
	}

	init := pkg.PkgV.GetStringMapString("pkg.init")
	for name, stageStr := range init {
		stage, err := strconv.ParseInt(stageStr, 10, 64)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Parsing pkg %s config: %s",
				pkg.FullName(), err.Error()))
		}
		pkg.init[name] = int(stage)
	}
	initFnName := pkg.PkgV.GetString("pkg.init_function")
	initStage := pkg.PkgV.GetInt("pkg.init_stage")

	if initFnName != "" {
		pkg.init[initFnName] = initStage
	}

	// Read the package description from the file
	pkg.desc, err = pkg.readDesc(pkg.PkgV)
	if err != nil {
		return err
	}

	// Load syscfg settings.
	if util.NodeExist(pkg.basePath + "/" + SYSCFG_YAML_FILENAME) {
		pkg.SyscfgV, err = util.ReadConfig(pkg.basePath,
			strings.TrimSuffix(SYSCFG_YAML_FILENAME, ".yml"))
		if err != nil {
			return err
		}
		pkg.AddCfgFilename(pkg.basePath + "/" + SYSCFG_YAML_FILENAME)
	}

	return nil
}

func (pkg *LocalPackage) Init() map[string]int {
	return pkg.init
}

func (pkg *LocalPackage) InjectedSettings() map[string]string {
	return pkg.injectedSettings
}

func (pkg *LocalPackage) Clone(newRepo *repo.Repo,
	newName string) *LocalPackage {

	// XXX: Validate name.

	// Copy the package.
	newPkg := *pkg
	newPkg.repo = newRepo
	newPkg.name = newName
	newPkg.basePath = newRepo.Path() + "/" + newPkg.name

	// Insert the clone into the global package map.
	proj := interfaces.GetProject()
	pMap := proj.PackageList()

	(*pMap[newRepo.Name()])[newPkg.name] = &newPkg

	return &newPkg
}

func LoadLocalPackage(repo *repo.Repo, pkgDir string) (*LocalPackage, error) {
	pkg := NewLocalPackage(repo, pkgDir)
	err := pkg.Load()
	return pkg, err
}

func LocalPackageSpecialName(dirName string) bool {
	_, ok := LocalPackageSpecialNames[dirName]
	return ok
}

func ReadLocalPackageRecursive(repo *repo.Repo,
	pkgList map[string]interfaces.PackageInterface, basePath string,
	pkgName string, searchedMap map[string]struct{}) ([]string, error) {

	var warnings []string

	dirList, err := repo.FilteredSearchList(pkgName, searchedMap)
	if err != nil {
		return warnings, util.NewNewtError(err.Error())
	}

	for _, name := range dirList {
		if LocalPackageSpecialName(name) || strings.HasPrefix(name, ".") {
			continue
		}

		subWarnings, err := ReadLocalPackageRecursive(repo, pkgList,
			basePath, filepath.Join(pkgName, name), searchedMap)
		warnings = append(warnings, subWarnings...)
		if err != nil {
			return warnings, err
		}
	}

	if util.NodeNotExist(filepath.Join(basePath, pkgName,
		PACKAGE_FILE_NAME)) {

		return warnings, nil
	}

	pkg, err := LoadLocalPackage(repo, filepath.Join(basePath, pkgName))
	if err != nil {
		warnings = append(warnings, err.Error())
		return warnings, nil
	}

	if oldPkg, ok := pkgList[pkg.Name()]; ok {
		oldlPkg := oldPkg.(*LocalPackage)
		warnings = append(warnings,
			fmt.Sprintf("Multiple packages with same pkg.name=%s "+
				"in repo %s; path1=%s path2=%s", oldlPkg.Name(), repo.Name(),
				oldlPkg.BasePath(), pkg.BasePath()))

		return warnings, nil
	}

	pkgList[pkg.Name()] = pkg

	return warnings, nil
}

func ReadLocalPackages(repo *repo.Repo, basePath string) (
	*map[string]interfaces.PackageInterface, []string, error) {

	pkgMap := &map[string]interfaces.PackageInterface{}

	// Keep track of which directories we have traversed.  Prevent infinite
	// loops caused by symlink cycles by not inspecting the same directory
	// twice.
	searchedMap := map[string]struct{}{}

	warnings, err := ReadLocalPackageRecursive(repo, *pkgMap,
		basePath, "", searchedMap)

	return pkgMap, warnings, err
}
