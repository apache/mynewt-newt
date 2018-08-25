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
	"strconv"
	"strings"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
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
	KeyFile      string
	PkgProfiles  map[string]string

	// target.yml configuration structure
	TargetY ycfg.YCfg
}

func NewTarget(basePkg *pkg.LocalPackage) *Target {
	target := &Target{
		TargetY: ycfg.YCfg{},
	}
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
	yc, err := newtutil.ReadConfig(basePkg.BasePath(),
		strings.TrimSuffix(TARGET_FILENAME, ".yml"))
	if err != nil {
		return err
	}

	target.TargetY = yc

	target.BspName = yc.GetValString("target.bsp", nil)
	target.AppName = yc.GetValString("target.app", nil)
	target.LoaderName = yc.GetValString("target.loader", nil)

	target.BuildProfile = yc.GetValString("target.build_profile", nil)
	if target.BuildProfile == "" {
		target.BuildProfile = DEFAULT_BUILD_PROFILE
	}

	target.HeaderSize = DEFAULT_HEADER_SIZE
	if yc.GetValString("target.header_size", nil) != "" {
		hs, err := strconv.ParseUint(
			yc.GetValString("target.header_size", nil), 0, 32)
		if err == nil {
			target.HeaderSize = uint32(hs)
		}
	}

	target.KeyFile = yc.GetValString("target.key_file", nil)
	target.PkgProfiles = yc.GetValStringMapString(
		"target.package_profiles", nil)

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

func (target *Target) resolvePackageRepoAndName(repo *repo.Repo, name string) *pkg.LocalPackage {
	dep, err := pkg.NewDependency(repo, name)
	if err != nil {
		return nil
	}

	pack, ok := project.GetProject().ResolveDependency(dep).(*pkg.LocalPackage)
	if !ok {
		return nil
	}

	return pack
}

func (target *Target) resolvePackageName(name string) *pkg.LocalPackage {
	pack := target.resolvePackageYmlName(name)

	if pack == nil || pack.Type() != pkg.PACKAGE_TYPE_TRANSIENT {
		return pack
	}

	// We follow only one level of linking here to make things easier and assuming
	// nested linking means someone using really deprecated packages ;)

	pack = target.resolvePackageRepoAndName(pack.Repo().(*repo.Repo), pack.LinkedName())

	return pack
}

func (target *Target) resolvePackageYmlName(name string) *pkg.LocalPackage {
	return target.resolvePackageRepoAndName(target.basePkg.Repo().(*repo.Repo), name)
}

// Methods below resolve package by name and follow links to get proper package

func (target *Target) App() *pkg.LocalPackage {
	return target.resolvePackageName(target.AppName)
}

func (target *Target) Loader() *pkg.LocalPackage {
	return target.resolvePackageName(target.LoaderName)
}

func (target *Target) Bsp() *pkg.LocalPackage {
	return target.resolvePackageName(target.BspName)
}

// Methods below resolve package by name as stated in YML file (so do not follow links)
// e.g. to use as seed for dependencies calculation

func (target *Target) AppYml() *pkg.LocalPackage {
	return target.resolvePackageYmlName(target.AppName)
}

func (target *Target) LoaderYml() *pkg.LocalPackage {
	return target.resolvePackageYmlName(target.LoaderName)
}

func (target *Target) BspYml() *pkg.LocalPackage {
	return target.resolvePackageYmlName(target.BspName)
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

	s := newtutil.YCfgToYaml(t.TargetY)
	file.WriteString(s)

	if err := t.basePkg.SaveSyscfg(); err != nil {
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
