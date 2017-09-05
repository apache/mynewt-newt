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
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/yaml"
)

const TARGET_FILENAME string = "target.yml"
const DEFAULT_BUILD_PROFILE string = "default"
const DEFAULT_HEADER_SIZE uint32 = 0x20

var globalTargetMap map[string]*Target

type Target struct {
	basePkg *pkg.LocalPackage

	BspName      string
	AppName      string
	LoaderName   string
	BuildProfile string
	HeaderSize   uint32

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
		var ok bool
		target.Vars[k], ok = v.(string)
		if !ok {
			target.Vars[k] = strconv.Itoa(v.(int))
		}
	}

	target.BspName = target.Vars["target.bsp"]
	target.AppName = target.Vars["target.app"]
	target.LoaderName = target.Vars["target.loader"]

	target.BuildProfile = target.Vars["target.build_profile"]
	if target.BuildProfile == "" {
		target.BuildProfile = DEFAULT_BUILD_PROFILE
	}

	target.HeaderSize = DEFAULT_HEADER_SIZE
	if target.Vars["target.header_size"] != "" {
		hs, err := strconv.ParseUint(target.Vars["target.header_size"], 0, 32)
		if err == nil {
			target.HeaderSize = uint32(hs)
		}
	}

	// Note: App not required in the case of unit tests.

	// Remember the name of the configuration file so that it can be specified
	// as a dependency to the compiler.
	target.basePkg.AddCfgFilename(basePkg.BasePath() + TARGET_FILENAME)

	return nil
}

func (target *Target) Validate(appRequired bool) error {
	if target.BspName == "" {
		return util.NewNewtError("Target does not specify a BSP package " +
			"(target.bsp)")
	}
	bsp := target.resolvePackageName(target.BspName)
	if bsp == nil {
		return util.FmtNewtError("Could not resolve BSP package: %s",
			target.BspName)
	}

	if bsp.Type() != pkg.PACKAGE_TYPE_BSP {
		return util.FmtNewtError("bsp package (%s) is not of "+
			"type bsp; type is: %s\n", bsp.Name(),
			pkg.PackageTypeNames[bsp.Type()])
	}

	if appRequired {
		if target.AppName == "" {
			return util.NewNewtError("Target does not specify an app " +
				"package (target.app)")
		}
		app := target.resolvePackageName(target.AppName)
		if app == nil {
			return util.FmtNewtError("Could not resolve app package: %s",
				target.AppName)
		}

		if app.Type() != pkg.PACKAGE_TYPE_APP {
			return util.FmtNewtError("target.app package (%s) is not of "+
				"type app; type is: %s\n", app.Name(),
				pkg.PackageTypeNames[app.Type()])
		}

		if target.LoaderName != "" {
			loader := target.resolvePackageName(target.LoaderName)
			if loader == nil {
				return util.FmtNewtError(
					"Could not resolve loader package: %s", target.LoaderName)
			}

			if loader.Type() != pkg.PACKAGE_TYPE_APP {
				return util.FmtNewtError(
					"target.loader package (%s) is not of type app; type "+
						"is: %s\n", loader.Name(),
					pkg.PackageTypeNames[loader.Type()])
			}
		}
	}

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

func (target *Target) Clone(newRepo *repo.Repo, newName string) *Target {
	// Clone the target.
	newTarget := *target
	newTarget.basePkg = target.basePkg.Clone(newRepo, newName)

	// Insert the clone into the global target map.
	GetTargets()[newTarget.FullName()] = &newTarget

	return &newTarget
}

func (target *Target) resolvePackageName(name string) *pkg.LocalPackage {
	dep, err := pkg.NewDependency(target.basePkg.Repo(), name)
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
	return target.resolvePackageName(target.AppName)
}

func (target *Target) Loader() *pkg.LocalPackage {
	return target.resolvePackageName(target.LoaderName)
}

func (target *Target) Bsp() *pkg.LocalPackage {
	return target.resolvePackageName(target.BspName)
}

func (target *Target) BinBasePath() string {
	appPkg := target.App()
	if appPkg == nil {
		return ""
	}

	return appPkg.BasePath() + "/bin/" + target.Name() + "/" +
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

	file.WriteString("### Target: " + t.Name() + "\n")

	keys := []string{}
	for k, _ := range t.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		file.WriteString(k + ": " + yaml.EscapeString(t.Vars[k]) + "\n")
	}

	if err := t.basePkg.SaveSyscfgVals(); err != nil {
		return err
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
				"Warning: failed to load target \"%s\": %s\n", pack.Name(),
				nerr.Text)
		} else {
			globalTargetMap[pack.FullName()] = target
		}
	}

	return nil
}

func ResetTargets() {
	globalTargetMap = nil
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
