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

package builder

import (
	"os"
	"path/filepath"
	"regexp"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/resolve"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type BuildPackage struct {
	rpkg              *resolve.ResolvePackage
	SourceDirectories []string
	SourceFiles       []string
	ci                *toolchain.CompilerInfo
}

func NewBuildPackage(rpkg *resolve.ResolvePackage) *BuildPackage {
	bpkg := &BuildPackage{
		rpkg: rpkg,
	}

	return bpkg
}

// Recursively iterates through an pkg's dependencies, adding each pkg
// encountered to the supplied set.
func (bpkg *BuildPackage) collectDepsAux(b *Builder,
	set *map[*BuildPackage]bool) error {

	if (*set)[bpkg] {
		return nil
	}

	(*set)[bpkg] = true

	for _, dep := range bpkg.rpkg.Deps {
		dbpkg := b.PkgMap[dep.Rpkg]
		if dbpkg == nil {
			return util.FmtNewtError("Package not found %s; required by %s",
				dep.Rpkg.Lpkg.Name(), bpkg.rpkg.Lpkg.Name())
		}

		if err := dbpkg.collectDepsAux(b, set); err != nil {
			return err
		}
	}

	return nil
}

// Recursively iterates through an pkg's dependencies.  The resulting array
// contains a pointer to each encountered pkg.
func (bpkg *BuildPackage) collectDeps(b *Builder) ([]*BuildPackage, error) {
	set := map[*BuildPackage]bool{}

	err := bpkg.collectDepsAux(b, &set)
	if err != nil {
		return nil, err
	}

	arr := []*BuildPackage{}
	for p, _ := range set {
		arr = append(arr, p)
	}

	return arr, nil
}

// Calculates the include paths exported by the specified pkg and all of
// its recursive dependencies.
func (bpkg *BuildPackage) recursiveIncludePaths(
	b *Builder) ([]string, error) {

	deps, err := bpkg.collectDeps(b)
	if err != nil {
		return nil, err
	}

	incls := []string{}
	for _, p := range deps {
		incls = append(incls, p.publicIncludeDirs(b)...)
	}

	return incls, nil
}

// Replaces instances of "@<repo-name>" with repo paths.
func expandFlags(flags []string) {
	for i, f := range flags {
		newFlag, changed := newtutil.ReplaceRepoDesignators(f)
		if changed {
			flags[i] = newFlag
		}
	}
}

// Retrieves the build package's build profile override, as specified in its
// `pkg.yml` file.  If the package does not override the build profile, "" is
// returned.
func (bpkg *BuildPackage) BuildProfile(b *Builder) string {
	settings := b.cfg.AllSettingsForLpkg(bpkg.rpkg.Lpkg)

	profile, err := bpkg.rpkg.Lpkg.PkgY.GetValString("pkg.build_profile", settings)
	util.OneTimeWarningError(err)

	return profile
}

func (bpkg *BuildPackage) CompilerInfo(
	b *Builder) (*toolchain.CompilerInfo, error) {

	// If this package's compiler info has already been generated, return the
	// cached copy.
	if bpkg.ci != nil {
		return bpkg.ci, nil
	}

	ci := toolchain.NewCompilerInfo()
	settings := b.cfg.AllSettingsForLpkg(bpkg.rpkg.Lpkg)

	var err error

	// Read each set of flags and expand repo designators ("@<repo-name>") into
	// paths.
	ci.Cflags, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice("pkg.cflags", settings)
	util.OneTimeWarningError(err)
	expandFlags(ci.Cflags)

	ci.CXXflags, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice("pkg.cxxflags", settings)
	util.OneTimeWarningError(err)
	expandFlags(ci.CXXflags)

	ci.Lflags, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice("pkg.lflags", settings)
	util.OneTimeWarningError(err)
	expandFlags(ci.Lflags)

	ci.Aflags, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice("pkg.aflags", settings)
	util.OneTimeWarningError(err)
	expandFlags(ci.Aflags)

	// Package-specific injected settings get specified as C flags on the
	// command line.
	for _, k := range bpkg.rpkg.Lpkg.InjectedSettings().Names() {
		ci.Cflags = append(ci.Cflags, syscfg.FeatureToCflag(k))
	}

	ci.IgnoreFiles = []*regexp.Regexp{}

	ignPats, err := bpkg.rpkg.Lpkg.PkgY.GetValStringSlice(
		"pkg.ignore_files", settings)
	util.OneTimeWarningError(err)

	if len(ignPats) == 0 {
		ignPats, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice(
			"pkg.ign_files", settings)
		util.OneTimeWarningError(err)
	}

	for _, str := range ignPats {
		re, err := regexp.Compile(str)
		if err != nil {
			return nil, util.NewNewtError(
				"Ignore files, unable to compile re: " + err.Error())
		}
		ci.IgnoreFiles = append(ci.IgnoreFiles, re)
	}

	ci.IgnoreDirs = []*regexp.Regexp{}

	ignPats, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice(
		"pkg.ignore_dirs", settings)
	util.OneTimeWarningError(err)

	if len(ignPats) == 0 {
		ignPats, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice(
			"pkg.ign_dirs", settings)
		util.OneTimeWarningError(err)
	}

	for _, str := range ignPats {
		re, err := regexp.Compile(str)
		if err != nil {
			return nil, util.NewNewtError(
				"Ignore dirs, unable to compile re: " + err.Error())
		}
		ci.IgnoreDirs = append(ci.IgnoreDirs, re)
	}

	bpkg.SourceDirectories, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice(
		"pkg.source_dirs", settings)
	util.OneTimeWarningError(err)

	if len(bpkg.SourceDirectories) == 0 {
		bpkg.SourceDirectories, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice(
			"pkg.src_dirs", settings)
		util.OneTimeWarningError(err)
	}

	bpkg.SourceFiles, err = bpkg.rpkg.Lpkg.PkgY.GetValStringSlice(
		"pkg.source_files", settings)
	util.OneTimeWarningError(err)

	includePaths, err := bpkg.recursiveIncludePaths(b)
	if err != nil {
		return nil, err
	}

	ci.Includes = append(bpkg.privateIncludeDirs(b), includePaths...)
	bpkg.ci = ci

	return bpkg.ci, nil
}

func (bpkg *BuildPackage) findSdkIncludes() []string {
	sdkDir := bpkg.rpkg.Lpkg.BasePath() + "/src/ext/"

	if _, err := os.Stat(sdkDir); err != nil {
		return []string{}
	}

	sdkPathList := []string{}
	err := filepath.Walk(sdkDir,
		func(path string, info os.FileInfo, err error) error {
			if !info.IsDir() {
				return nil
			}

			sdkPathList = append(sdkPathList, path)
			return nil
		})
	if err != nil {
		return []string{}
	}

	return sdkPathList
}

func addIncludeDir(incls *[]string, inc string) bool {
	if _, err := os.Stat(inc); err == nil {
		*incls = append(*incls, inc)

		return true
	}

	return false
}

func (bpkg *BuildPackage) publicIncludeDirs(b *Builder) []string {
	bspPkg := b.targetBuilder.bspPkg
	pkgBase := filepath.Base(bpkg.rpkg.Lpkg.Name())
	bp := bpkg.rpkg.Lpkg.BasePath()

	incls := []string{}
	if addIncludeDir(&incls, bp+"/include") {
		addIncludeDir(&incls, bp+"/include/"+pkgBase+"/arch/"+bspPkg.Arch)
	}

	if bpkg.rpkg.Lpkg.Type() == pkg.PACKAGE_TYPE_SDK {
		incls = append(incls, bspPkg.BasePath()+"/include/bsp/")

		sdkIncls := bpkg.findSdkIncludes()
		incls = append(incls, sdkIncls...)

		settings := b.cfg.AllSettingsForLpkg(bpkg.rpkg.Lpkg)

		inclDirs, err := bpkg.rpkg.Lpkg.PkgY.GetValStringSlice(
			"pkg.include_dirs", settings)
		util.OneTimeWarningError(err)

		for _, dir := range inclDirs {
			repo, path, err := newtutil.ParsePackageString(dir)

			if err != nil {
				util.OneTimeWarningError(err)
			}

			if repo != "" {
				incls = append(incls, "repos/"+repo+"/"+path)
			} else {
				incls = append(incls, bp+"/"+dir)
			}
		}
	}

	return incls
}

func (bpkg *BuildPackage) privateIncludeDirs(b *Builder) []string {
	srcDir := bpkg.rpkg.Lpkg.BasePath() + "/src/"

	incls := []string{}
	if addIncludeDir(&incls, srcDir) {
		addIncludeDir(&incls, srcDir+"/arch/"+b.targetBuilder.bspPkg.Arch)
	}

	switch bpkg.rpkg.Lpkg.Type() {
	case pkg.PACKAGE_TYPE_SDK:
		// If pkgType == SDK, include all the items in "ext" directly into the
		// include path
		incls = append(incls, b.bspPkg.rpkg.Lpkg.BasePath()+"/include/bsp/")

		sdkIncls := bpkg.findSdkIncludes()
		incls = append(incls, sdkIncls...)

	case pkg.PACKAGE_TYPE_UNITTEST:
		// A unittest package gets access to its parent package's private
		// includes.
		parentPkg := b.testOwner(bpkg)
		if parentPkg != nil {
			parentIncls := parentPkg.privateIncludeDirs(b)
			incls = append(incls, parentIncls...)
		}

	default:
	}

	return incls
}
