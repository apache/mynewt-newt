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
	"fmt"
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

const TARGET_FILE_NAME string = "target.yml"

var globalTargetMap map[string]*Target

var targetSearchDirs = []string{
	"targets",
}

type Target struct {
	basePkg *pkg.LocalPackage

	// XXX: Probably don't need the below four fields; they can just be
	// retrieved from the viper object.  Keep them here for now for easy
	// initializiation of dummy targets.
	CompilerName string
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
		strings.TrimSuffix(TARGET_FILE_NAME, ".yml"))
	if err != nil {
		return err
	}

	target.Vars = map[string]string{}

	settings := v.AllSettings()
	for k, v := range settings {
		target.Vars[k] = v.(string)
	}

	target.CompilerName = target.Vars["target.compiler"]
	target.BspName = target.Vars["target.bsp"]
	target.AppName = target.Vars["target.app"]
	target.Arch = target.Vars["target.arch"]
	target.BuildProfile = target.Vars["target.build_profile"]

	// XXX: Verify required fields set?

	return nil
}

func (target *Target) Package() *pkg.LocalPackage {
	return target.basePkg
}

func (target *Target) Name() string {
	return target.basePkg.Name()
}

func (target *Target) ShortName() string {
	return filepath.Base(target.Name())
}

func (target *Target) App() *pkg.LocalPackage {
	dep, err := pkg.NewDependency(nil, target.AppName)
	if err != nil {
		fmt.Println("app name = %s\n", target.AppName)
		fmt.Println("dep is nil")
		return nil
	}

	appPkg := project.GetProject().ResolveDependency(dep)
	if appPkg == nil {
		fmt.Printf("app name = %s\n", target.AppName)
		return nil
	}

	return appPkg.(*pkg.LocalPackage)
}

func (target *Target) Bsp() *pkg.LocalPackage {
	dep, _ := pkg.NewDependency(nil, target.BspName)
	mypkg := project.GetProject().ResolveDependency(dep).(*pkg.LocalPackage)
	return mypkg
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
	filepath := dirpath + "/" + TARGET_FILE_NAME
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
			name != pkg.PACKAGE_FILE_NAME && name != TARGET_FILE_NAME {

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

	packs := project.GetProject().PackageList()
	for _, packHash := range packs {
		for name, pack := range *packHash {
			if pack.Type() == pkg.PACKAGE_TYPE_TARGET {
				target, err := LoadTarget(pack.(*pkg.LocalPackage))
				if err != nil {
					nerr := err.(*util.NewtError)
					cli.ErrorMessage(cli.VERBOSITY_QUIET,
						"Warning: failed to load target \"%s\": %s\n", name,
						nerr.Text)
				} else {
					globalTargetMap[name] = target
				}
			}
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

func ResolveTargetName(name string) *Target {
	targetMap := GetTargets()

	t := targetMap[name]
	for i := 0; t == nil && i < len(targetSearchDirs); i++ {
		guess := targetSearchDirs[i] + "/" + name
		t = targetMap[guess]
	}

	return t
}

func ResolveTargetNames(names ...string) ([]*Target, error) {
	targets := []*Target{}

	for _, name := range names {
		t := ResolveTargetName(name)
		if t == nil {
			return nil, util.NewNewtError("Could not resolve target name: " +
				name)
		}

		targets = append(targets, t)
	}

	return targets, nil
}
