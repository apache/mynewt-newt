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
	"path/filepath"

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

	// Keeps track of API requirements and whether they are satisfied.
	reqApiMap map[string]reqApiStatus

	resolved bool
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
	if !bpkg.resolved {
		return nil, util.NewNewtError("Package must be resolved before " +
			"compiler info is fetched; package=" + bpkg.Name())
	}

	// If this package's compiler info has already been generated, returned the
	// cached copy.
	if bpkg.ci != nil {
		return bpkg.ci, nil
	}

	ci := toolchain.NewCompilerInfo()
	ci.Cflags = newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(),
		"pkg.cflags")
	ci.Lflags = newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(),
		"pkg.lflags")
	ci.Aflags = newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(),
		"pkg.aflags")

	includePaths, err := bpkg.recursiveIncludePaths(b)
	if err != nil {
		return nil, err
	}
	ci.Includes = append(bpkg.privateIncludeDirs(b), includePaths...)
	bpkg.ci = ci

	return bpkg.ci, nil
}

func (bpkg *BuildPackage) loadFeatures(b *Builder) (map[string]bool, bool) {
	features := b.Features()

	foundNewFeature := false

	newFeatures := newtutil.GetStringSliceFeatures(bpkg.Viper, features,
		"pkg.features")
	for _, nfeature := range newFeatures {
		_, ok := features[nfeature]
		if !ok {
			b.AddFeature(nfeature)
			foundNewFeature = true
		}
	}

	return b.Features(), foundNewFeature
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

		pkg := proj.ResolveDependency(newDep).(*pkg.LocalPackage)
		if pkg == nil {
			return false, util.NewNewtError("Cannot resolve dependency: " + newDep.String())
		}

		if pkg == nil {
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

		// Determine if this package supports any APIs that we haven't seen
		// yet.  If so, another full iteration is required.
		apis := newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(),
			"pkg.caps")
		for _, api := range apis {
			newApi := b.AddApi(api, bpkg)
			if newApi {
				changed = true
			}
		}

		// Determine if any of package's API requirements can now be satisfied.
		// If so, another full iteration is required.
		reqApis := newtutil.GetStringSliceFeatures(bpkg.Viper, b.Features(),
			"pkg.req_caps")
		for _, reqApi := range reqApis {
			reqStatus, ok := bpkg.reqApiMap[reqApi]
			if !ok {
				reqStatus = REQ_API_STATUS_UNSATISFIED
				bpkg.reqApiMap[reqApi] = reqStatus
				changed = true
			}

			if reqStatus == REQ_API_STATUS_UNSATISFIED {
				apiSatisfied := bpkg.satisfyReqApi(b, reqApi)
				if apiSatisfied {
					changed = true
				}
			}
		}
	}

	return changed, nil
}

func (bpkg *BuildPackage) publicIncludeDirs(b *Builder) []string {
	pkgBase := filepath.Base(bpkg.Name())

	return []string{
		bpkg.BasePath() + "/include",
		bpkg.BasePath() + "/include/" + pkgBase + "/arch/" + b.target.Arch,
	}
}

func (bpkg *BuildPackage) privateIncludeDirs(b *Builder) []string {
	srcDir := bpkg.BasePath() + "/src/"

	incls := []string{}
	incls = append(incls, srcDir)
	incls = append(incls, srcDir+"/arch/"+b.target.Arch)

	if b.Features()["test"] {
		testSrcDir := srcDir + "/test"
		incls = append(incls, testSrcDir)
		incls = append(incls, testSrcDir+"/arch/"+b.target.Arch)
	}

	return incls
}

// Attempts to resolve all of a build package's dependencies, identities, APIs,
// and required APIs.  This function should be called repeatedly until the
// package is fully resolved.
//
// @return bool                 true if the package is fully resolved;
//                              false if another call is required.
func (bpkg *BuildPackage) Resolve(b *Builder) (bool, error) {
	if bpkg.resolved {
		return true, nil
	}

	features, newFeatures := bpkg.loadFeatures(b)
	newDeps, err := bpkg.loadDeps(b, features)
	if err != nil {
		return false, err
	}

	if newFeatures || newDeps {
		return false, nil
	}

	bpkg.resolved = true

	return true, nil
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
