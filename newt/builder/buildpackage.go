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

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type BuildPackage struct {
	*pkg.LocalPackage

	ci                *toolchain.CompilerInfo
	SourceDirectories []string
}

// Recursively iterates through an pkg's dependencies, adding each pkg
// encountered to the supplied set.
func (bpkg *BuildPackage) collectDepsAux(b *Builder,
	set *map[*BuildPackage]bool) error {

	if (*set)[bpkg] {
		return nil
	}

	(*set)[bpkg] = true

	for _, dep := range bpkg.Deps() {
		if dep.Name == "" {
			break
		}

		// Get pkg structure
		p := project.GetProject().ResolveDependency(dep)
		if p == nil {
			return util.FmtNewtError("Cannot resolve dependency %+v", dep)
		}
		dpkg := p.(*pkg.LocalPackage)

		dbpkg := b.PkgMap[dpkg]
		if dbpkg == nil {
			return util.FmtNewtError("Package not found %s; required by %s",
				dpkg.Name(), bpkg.Name())
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
		incls = append(incls, p.publicIncludeDirs(b.targetBuilder.bspPkg)...)
	}

	return incls, nil
}

func (bpkg *BuildPackage) CompilerInfo(
	b *Builder) (*toolchain.CompilerInfo, error) {

	// If this package's compiler info has already been generated, returned the
	// cached copy.
	if bpkg.ci != nil {
		return bpkg.ci, nil
	}

	ci := toolchain.NewCompilerInfo()
	features := b.cfg.FeaturesForLpkg(bpkg.LocalPackage)
	ci.Cflags = newtutil.GetStringSliceFeatures(bpkg.PkgV, features,
		"pkg.cflags")
	ci.Lflags = newtutil.GetStringSliceFeatures(bpkg.PkgV, features,
		"pkg.lflags")
	ci.Aflags = newtutil.GetStringSliceFeatures(bpkg.PkgV, features,
		"pkg.aflags")

	// Package-specific injected settings get specified as C flags on the
	// command line.
	for k, _ := range bpkg.InjectedSettings() {
		ci.Cflags = append(ci.Cflags, syscfg.FeatureToCflag(k))
	}

	ci.IgnoreFiles = []*regexp.Regexp{}
	ignPats := newtutil.GetStringSliceFeatures(bpkg.PkgV,
		features, "pkg.ign_files")
	for _, str := range ignPats {
		re, err := regexp.Compile(str)
		if err != nil {
			return nil, util.NewNewtError(
				"Ignore files, unable to compile re: " + err.Error())
		}
		ci.IgnoreFiles = append(ci.IgnoreFiles, re)
	}

	ci.IgnoreDirs = []*regexp.Regexp{}
	ignPats = newtutil.GetStringSliceFeatures(bpkg.PkgV,
		features, "pkg.ign_dirs")
	for _, str := range ignPats {
		re, err := regexp.Compile(str)
		if err != nil {
			return nil, util.NewNewtError(
				"Ignore dirs, unable to compile re: " + err.Error())
		}
		ci.IgnoreDirs = append(ci.IgnoreDirs, re)
	}

	bpkg.SourceDirectories = newtutil.GetStringSliceFeatures(bpkg.PkgV,
		features, "pkg.src_dirs")

	includePaths, err := bpkg.recursiveIncludePaths(b)
	if err != nil {
		return nil, err
	}

	ci.Includes = append(bpkg.privateIncludeDirs(b), includePaths...)
	bpkg.ci = ci

	return bpkg.ci, nil
}

// Searches for a package which can satisfy bpkg's API requirement.  If such a
// package is found, bpkg's API requirement is marked as satisfied, and the
// package is added to bpkg's dependency list.
func (bpkg *BuildPackage) satisfyReqApi(
	reqApi string, apiPkg *BuildPackage) error {

	dep := &pkg.Dependency{
		Name: apiPkg.Name(),
		Repo: apiPkg.Repo().Name(),
	}
	bpkg.AddDep(dep)

	log.Debugf("API requirement satisfied; pkg=%s API=(%s, %s)",
		bpkg.Name(), reqApi, dep.String())

	return nil
}

func (bpkg *BuildPackage) findSdkIncludes() []string {
	sdkDir := bpkg.BasePath() + "/src/ext/"

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

func (bpkg *BuildPackage) publicIncludeDirs(bspPkg *pkg.BspPackage) []string {
	pkgBase := filepath.Base(bpkg.Name())
	bp := bpkg.BasePath()

	incls := []string{
		bp + "/include",
		bp + "/include/" + pkgBase + "/arch/" + bspPkg.Arch,
	}

	if bpkg.Type() == pkg.PACKAGE_TYPE_SDK {
		incls = append(incls, bspPkg.BasePath()+"/include/bsp/")

		sdkIncls := bpkg.findSdkIncludes()
		incls = append(incls, sdkIncls...)
	}

	return incls
}

func (bpkg *BuildPackage) privateIncludeDirs(b *Builder) []string {
	srcDir := bpkg.BasePath() + "/src/"

	incls := []string{}
	incls = append(incls, srcDir)
	incls = append(incls, srcDir+"/arch/"+b.targetBuilder.bspPkg.Arch)

	switch bpkg.Type() {
	case pkg.PACKAGE_TYPE_SDK:
		// If pkgType == SDK, include all the items in "ext" directly into the
		// include path
		incls = append(incls, b.bspPkg.BasePath()+"/include/bsp/")

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

// Resolves all of a build package's dependencies and API requirements.
func (bpkg *BuildPackage) resolveDeps(
	cfg syscfg.Cfg, apiMap map[string]*BuildPackage) error {

	features := cfg.FeaturesForLpkg(bpkg.LocalPackage)

	// Match each required API with the package which implements it.
	reqApis := newtutil.GetStringSliceFeatures(bpkg.PkgV, features,
		"pkg.req_apis")
	for _, reqApi := range reqApis {
		if err := bpkg.satisfyReqApi(reqApi, apiMap[reqApi]); err != nil {
			return err
		}
	}

	proj := project.GetProject()
	newDeps := newtutil.GetStringSliceFeatures(bpkg.PkgV, features,
		"pkg.deps")
	for _, newDepStr := range newDeps {
		newDep, err := pkg.NewDependency(bpkg.Repo(), newDepStr)
		if err != nil {
			return err
		}

		_, ok := proj.ResolveDependency(newDep).(*pkg.LocalPackage)
		if !ok {
			return util.NewNewtError("Could not resolve package dependency " +
				newDep.String())
		}

		if !bpkg.HasDep(newDep) {
			bpkg.AddDep(newDep)
		}
	}

	return nil
}

func NewBuildPackage(pkg *pkg.LocalPackage) *BuildPackage {
	bpkg := &BuildPackage{
		LocalPackage: pkg,
	}

	return bpkg
}
