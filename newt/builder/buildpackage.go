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
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

// Indicates whether a package's API dependency has been another package.
type reqApiStatus int

const (
	REQ_API_STATUS_UNSATISFIED reqApiStatus = iota
	REQ_API_STATUS_SATISFIED
)

type BuildPackage struct {
	*pkg.LocalPackage

	ci *toolchain.CompilerInfo

	SourceDirectories []string

	// Keeps track of API requirements and whether they are satisfied.
	reqApiMap map[string]reqApiStatus

	depsResolved  bool
	apisSatisfied bool
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
		dpkg := project.GetProject().ResolveDependency(dep).(*pkg.LocalPackage)
		if dpkg == nil {
			return util.NewNewtError("Cannot resolve dependency " + dep.String())
		}

		dbpkg := b.Packages[dpkg]
		if dbpkg == nil {
			return util.NewNewtError(fmt.Sprintf("Package not found (%s)",
				dpkg.Name()))
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
func (bpkg *BuildPackage) recursiveIncludePaths(b *Builder) ([]string, error) {
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

func (bpkg *BuildPackage) CompilerInfo(b *Builder) (*toolchain.CompilerInfo, error) {
	if !bpkg.depsResolved || !bpkg.apisSatisfied {
		return nil, util.NewNewtError("Package must be resolved before " +
			"compiler info is fetched; package=" + bpkg.Name())
	}

	// If this package's compiler info has already been generated, returned the
	// cached copy.
	if bpkg.ci != nil {
		return bpkg.ci, nil
	}

	ci := toolchain.NewCompilerInfo()
	ci.Cflags = newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(bpkg),
		"pkg.cflags")
	ci.Lflags = newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(bpkg),
		"pkg.lflags")
	ci.Aflags = newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(bpkg),
		"pkg.aflags")

	for fname, _ := range b.Features(bpkg) {
		ci.Cflags = append(ci.Cflags, fmt.Sprintf("-DFEATURE_%s", fname))
	}

	ci.IgnoreFiles = []*regexp.Regexp{}
	ignPats := newtutil.GetStringSliceFeatures(bpkg.Viper,
		b.Features(bpkg), "pkg.ign_files")
	for _, str := range ignPats {
		re, err := regexp.Compile(str)
		if err != nil {
			return nil, util.NewNewtError("Ignore files, unable to compile re: " +
				err.Error())
		}
		ci.IgnoreFiles = append(ci.IgnoreFiles, re)
	}

	ci.IgnoreDirs = []*regexp.Regexp{}
	ignPats = newtutil.GetStringSliceFeatures(bpkg.Viper,
		b.Features(bpkg), "pkg.ign_dirs")
	for _, str := range ignPats {
		re, err := regexp.Compile(str)
		if err != nil {
			return nil, util.NewNewtError("Ignore dirs, unable to compile re: " +
				err.Error())
		}
		ci.IgnoreDirs = append(ci.IgnoreDirs, re)
	}

	bpkg.SourceDirectories = newtutil.GetStringSliceFeatures(bpkg.Viper,
		b.Features(bpkg), "pkg.src_dirs")

	includePaths, err := bpkg.recursiveIncludePaths(b)
	if err != nil {
		return nil, err
	}
	ci.Includes = append(bpkg.privateIncludeDirs(b), includePaths...)
	bpkg.ci = ci

	return bpkg.ci, nil
}

func (bpkg *BuildPackage) loadFeatures(b *Builder) (map[string]bool, bool) {
	features := b.AllFeatures()

	foundNewFeature := false

	newFeatures := newtutil.GetStringSliceFeatures(bpkg.Viper, features,
		"pkg.features")
	for _, nfeature := range newFeatures {
		_, ok := features[nfeature]
		if !ok {
			b.AddFeature(nfeature)
			foundNewFeature = true
			log.Debugf("Detected new feature: %s (%s)", nfeature,
				bpkg.Name())
		}
	}

	return b.Features(bpkg), foundNewFeature
}

// Searches for a package which can satisfy bpkg's API requirement.  If such a
// package is found, bpkg's API requirement is marked as satisfied, and the
// package is added to bpkg's dependency list.
func (bpkg *BuildPackage) satisfyReqApi(b *Builder, reqApi string) bool {
	depBpkg := b.apis[reqApi]
	if depBpkg == nil {
		return false
	}

	dep := &pkg.Dependency{
		Name: depBpkg.Name(),
		Repo: depBpkg.Repo().Name(),
	}
	bpkg.AddDep(dep)
	bpkg.reqApiMap[reqApi] = REQ_API_STATUS_SATISFIED

	// This package now has a new unresolved dependency.
	bpkg.depsResolved = false

	log.Debugf("API requirement satisfied; pkg=%s API=(%s, %s)",
		bpkg.Name(), reqApi, dep.String())

	return true
}

// @return bool                 True if this this function changed the builder
//                                  state; another full iteration is required
//                                  in this case.
//         error                non-nil on failure.
func (bpkg *BuildPackage) loadDeps(b *Builder,
	features map[string]bool) (bool, error) {

	proj := project.GetProject()

	changed := false

	newDeps := newtutil.GetStringSliceFeatures(bpkg.Viper, features, "pkg.deps")
	for _, newDepStr := range newDeps {
		newDep, err := pkg.NewDependency(bpkg.Repo(), newDepStr)
		if err != nil {
			return false, err
		}

		pkg, ok := proj.ResolveDependency(newDep).(*pkg.LocalPackage)
		if !ok {
			return false,
				util.NewNewtError("Could not resolve package dependency " +
					newDep.String())
		}

		if b.Packages[pkg] == nil {
			changed = true
			b.AddPackage(pkg)
		}

		if !bpkg.HasDep(newDep) {
			changed = true
			bpkg.AddDep(newDep)
		}
	}

	// Determine if this package supports any APIs that we haven't seen
	// yet.  If so, another full iteration is required.
	apis := newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(bpkg),
		"pkg.apis")
	for _, api := range apis {
		newApi := b.AddApi(api, bpkg)
		if newApi {
			changed = true
		}
	}

	return changed, nil
}

// @return bool                 true if a new dependency was detected as a
//                                  result of satisfying an API for this
//                                  package.
func (bpkg *BuildPackage) satisfyApis(b *Builder) bool {
	// Assume all this package's APIs are satisfied and that no new
	// dependencies will be detected.
	bpkg.apisSatisfied = true
	newDeps := false

	// Determine if any of the package's API requirements can now be satisfied.
	// If so, another full iteration is required.
	reqApis := newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(bpkg),
		"pkg.req_apis")
	for _, reqApi := range reqApis {
		reqStatus, ok := bpkg.reqApiMap[reqApi]
		if !ok {
			reqStatus = REQ_API_STATUS_UNSATISFIED
			bpkg.reqApiMap[reqApi] = reqStatus
		}

		if reqStatus == REQ_API_STATUS_UNSATISFIED {
			apiSatisfied := bpkg.satisfyReqApi(b, reqApi)
			if apiSatisfied {
				// An API was satisfied; the package now has a new dependency
				// that needs to be resolved.
				newDeps = true
				reqStatus = REQ_API_STATUS_SATISFIED
			}
		}
		if reqStatus == REQ_API_STATUS_UNSATISFIED {
			bpkg.apisSatisfied = false
		}
	}

	return newDeps
}

func (bpkg *BuildPackage) findSdkIncludes() []string {
	sdkDir := bpkg.BasePath() + "/src/ext/"

	sdkPathList := []string{}
	err := filepath.Walk(sdkDir, func(path string, info os.FileInfo, err error) error {
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

func (bpkg *BuildPackage) publicIncludeDirs(b *Builder) []string {
	pkgBase := filepath.Base(bpkg.Name())
	bp := bpkg.BasePath()

	incls := []string{
		bp + "/include",
		bp + "/include/" + pkgBase + "/arch/" + b.target.Bsp.Arch,
	}

	if bpkg.Type() == pkg.PACKAGE_TYPE_SDK {
		incls = append(incls, b.target.Bsp.BasePath()+"/include/bsp/")

		sdkIncls := bpkg.findSdkIncludes()
		incls = append(incls, sdkIncls...)
	}

	return incls
}

func (bpkg *BuildPackage) privateIncludeDirs(b *Builder) []string {
	srcDir := bpkg.BasePath() + "/src/"

	incls := []string{}
	incls = append(incls, srcDir)
	incls = append(incls, srcDir+"/arch/"+b.target.Bsp.Arch)

	if b.Features(bpkg)["TEST"] {
		testSrcDir := srcDir + "/test"
		incls = append(incls, testSrcDir)
		incls = append(incls, testSrcDir+"/arch/"+b.target.Bsp.Arch)
	}

	// If pkgType == SDK, include all the items in "ext" directly into the
	// include path
	if bpkg.Type() == pkg.PACKAGE_TYPE_SDK {
		incls = append(incls, b.target.Bsp.BasePath()+"/include/bsp/")

		sdkIncls := bpkg.findSdkIncludes()
		incls = append(incls, sdkIncls...)
	}

	return incls
}

// Attempts to resolve all of a build package's dependencies, identities, APIs,
// and required APIs.  This function should be called repeatedly until the
// package is fully resolved.
//
// If a dependency is resolved by this function, the new dependency needs to be
// processed.  The caller should attempt to resolve all packages again.
//
// If a new supported feature is detected by this function, all pacakges need
// to be reprocessed from scratch.  The caller should set all packages'
// depsResolved and apisSatisfied variables to false and attempt to resolve
// everything again.
//
// @return bool                 true if >=1 dependencies were resolved
//         bool                 true if >=1 new features were detected
func (bpkg *BuildPackage) Resolve(b *Builder) (bool, bool, error) {
	var err error
	newDeps := false
	newFeatures := false

	if !bpkg.depsResolved {
		var features map[string]bool

		features, newFeatures = bpkg.loadFeatures(b)
		newDeps, err = bpkg.loadDeps(b, features)
		if err != nil {
			return false, false, err
		}

		bpkg.depsResolved = !newFeatures && !newDeps

	}

	if !bpkg.apisSatisfied {
		newApiDep := bpkg.satisfyApis(b)
		if newApiDep {
			newDeps = true
		}
	}

	return newDeps, newFeatures, nil
}

func (bp *BuildPackage) Init(pkg *pkg.LocalPackage) {
	bp.LocalPackage = pkg
	bp.reqApiMap = map[string]reqApiStatus{}
}

func NewBuildPackage(pkg *pkg.LocalPackage) *BuildPackage {
	bpkg := &BuildPackage{}
	bpkg.Init(pkg)

	return bpkg
}
