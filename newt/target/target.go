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

package target

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/yaml"
)

const TARGET_FILENAME string = "target.yml"
const TARGET_DEFAULT_DIR string = "targets"
const TARGET_KEYWORD_ALL string = "all"

var globalTargetMap map[string]*Target

type Target struct {
	basePkg *pkg.LocalPackage

	// XXX: Probably don't need the below four fields; they can just be
	// retrieved from the viper object.  Keep them here for now for easy
	// initializiation of dummy targets.
	BspName      string
	AppName      string
	Arch         string
	BuildProfile string

	// target.yml configuration structure
	Vars map[string]string
}

func NewTarget(basePkg *pkg.LocalPackage) *Target {
	target := &Target{}
	target.Init(basePkg)
	return target
}

func LoadTarget(basePkg *pkg.LocalPackage) (*Target, error) {
	target := NewTarget(basePkg)
	if err := target.Load(basePkg); err != nil {
		return nil, err
	}

	return target, nil
}

func (target *Target) Init(basePkg *pkg.LocalPackage) {
	target.basePkg = basePkg
}

func (target *Target) Load(basePkg *pkg.LocalPackage) error {
	v, err := util.ReadConfig(basePkg.BasePath(),
		strings.TrimSuffix(TARGET_FILENAME, ".yml"))
	if err != nil {
		return err
	}

	target.Vars = map[string]string{}

	settings := v.AllSettings()
	for k, v := range settings {
		target.Vars[k] = v.(string)
	}

	target.BspName = target.Vars["target.bsp"]
	target.AppName = target.Vars["target.app"]
	target.Arch = target.Vars["target.arch"]
	target.BuildProfile = target.Vars["target.build_profile"]

	return nil
}

func (target *Target) Package() *pkg.LocalPackage {
	return target.basePkg
}

func (target *Target) Name() string {
	return target.basePkg.Name()
}

func (target *Target) FullName() string {
	return target.basePkg.FullName()
}

func (target *Target) ShortName() string {
	return filepath.Base(target.Name())
}

func resolvePackageName(name string) *pkg.LocalPackage {
	dep, err := pkg.NewDependency(nil, name)
	if err != nil {
		return nil
	}

	pack, ok := project.GetProject().ResolveDependency(dep).(*pkg.LocalPackage)
	if !ok {
		return nil
	}

	return pack
}

func (target *Target) App() *pkg.LocalPackage {
	return resolvePackageName(target.AppName)
}

func (target *Target) Bsp() *pkg.LocalPackage {
	return resolvePackageName(target.BspName)
}

func (target *Target) BinBasePath() string {
	appPkg := target.App()
	if appPkg == nil {
		return ""
	}

	return appPkg.BasePath() + "/bin/" + target.Package().Name() + "/" +
		appPkg.Name()
}

func (target *Target) ElfPath() string {
	return target.BinBasePath() + ".elf"
}

func (target *Target) ImagePath() string {
	return target.BinBasePath() + ".img"
}

// Save the target's configuration elements
func (t *Target) Save() error {
	if err := t.basePkg.Save(); err != nil {
		return err
	}

	dirpath := t.basePkg.BasePath()
	filepath := dirpath + "/" + TARGET_FILENAME
	file, err := os.Create(filepath)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer file.Close()

	file.WriteString("### Target: " + t.basePkg.Name() + "\n")

	keys := []string{}
	for k, _ := range t.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		file.WriteString(k + ": " + yaml.EscapeString(t.Vars[k]) + "\n")
	}

	return nil
}

// Tells you if the target's directory contains extra user files (i.e., files
// other than pkg.yml).
func (t *Target) ContainsUserFiles() (bool, error) {
	contents, err := ioutil.ReadDir(t.basePkg.BasePath())
	if err != nil {
		return false, err
	}

	userFiles := false
	for _, node := range contents {
		name := node.Name()
		if name != "." && name != ".." &&
			name != pkg.PACKAGE_FILE_NAME && name != TARGET_FILENAME {

			userFiles = true
			break
		}
	}

	return userFiles, nil
}

func (t *Target) Delete() error {
	if err := os.RemoveAll(t.basePkg.BasePath()); err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}

func buildTargetMap() error {
	globalTargetMap = map[string]*Target{}

	packs := project.GetProject().PackagesOfType(pkg.PACKAGE_TYPE_TARGET)
	for _, packItf := range packs {
		pack := packItf.(*pkg.LocalPackage)
		target, err := LoadTarget(pack)
		if err != nil {
			nerr := err.(*util.NewtError)
			util.ErrorMessage(util.VERBOSITY_QUIET,
				"Warning: failed to load target \"%s\": %s\n", pack.Name,
				nerr.Text)
		} else {
			globalTargetMap[pack.FullName()] = target
		}
	}

	return nil
}

func GetTargets() map[string]*Target {
	if globalTargetMap == nil {
		err := buildTargetMap()
		if err != nil {
			panic(err.Error())
		}
	}

	return globalTargetMap
}

func ResolveTarget(name string) *Target {
	targetMap := GetTargets()

	// Check for fully-qualified name.
	if t := targetMap[name]; t != nil {
		return t
	}

	// Check the local "targets" directory.
	if t := targetMap[TARGET_DEFAULT_DIR+"/"+name]; t != nil {
		return t
	}

	// Check each repo alphabetically.
	fullNames := []string{}
	for fullName, _ := range targetMap {
		fullNames = append(fullNames, fullName)
	}
	for _, fullName := range util.SortFields(fullNames...) {
		if name == filepath.Base(fullName) {
			return targetMap[fullName]
		}
	}

	return nil
}

func ResolveTargetNames(names ...string) ([]*Target, error) {
	targets := []*Target{}

	for _, name := range names {
		t := ResolveTarget(name)
		if t == nil {
			return nil, util.NewNewtError("Could not resolve target name: " +
				name)
		}

		targets = append(targets, t)
	}

	return targets, nil
}

func ResolveNewTargetName(name string) (string, error) {
	repoName, pkgName, err := cli.ParsePackageString(name)
	if err != nil {
		return "", err
	}

	if repoName != "" {
		return "", util.NewNewtError("Target name cannot contain repo; " +
			"must be local")
	}

    if pkgName == TARGET_KEYWORD_ALL {
        return "", util.NewNewtError("Target name " + TARGET_KEYWORD_ALL +
            " is reserved")
    }

	// "Naked" target names translate to "targets/<name>".
	if !strings.Contains(pkgName, "/") {
		pkgName = TARGET_DEFAULT_DIR + "/" + pkgName
	}

	if GetTargets()[pkgName] != nil {
		return "", util.NewNewtError("Target already exists: " + pkgName)
	}

    return pkgName, nil
}
