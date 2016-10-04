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

package resolve

import (
	"fmt"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/flash"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/util"
)

type Resolver struct {
	apis             map[string]*ResolvePackage
	pkgMap           map[*pkg.LocalPackage]*ResolvePackage
	injectedSettings map[string]string
	flashMap         flash.FlashMap
	cfg              syscfg.Cfg
}

type ResolvePackage struct {
	*pkg.LocalPackage

	// Keeps track of API requirements and whether they are satisfied.
	reqApiMap map[string]bool

	depsResolved  bool
	apisSatisfied bool
}

type CfgResolution struct {
	Cfg             syscfg.Cfg
	ApiMap          map[string]*pkg.LocalPackage
	UnsatisfiedApis map[string][]*pkg.LocalPackage
}

func newResolver() *Resolver {
	return &Resolver{
		apis:             map[string]*ResolvePackage{},
		pkgMap:           map[*pkg.LocalPackage]*ResolvePackage{},
		injectedSettings: map[string]string{},
		cfg:              syscfg.NewCfg(),
	}
}

func newResolvePkg(lpkg *pkg.LocalPackage) *ResolvePackage {
	return &ResolvePackage{
		LocalPackage: lpkg,
		reqApiMap:    map[string]bool{},
	}
}

func newCfgResolution() CfgResolution {
	return CfgResolution{
		Cfg:             syscfg.NewCfg(),
		ApiMap:          map[string]*pkg.LocalPackage{},
		UnsatisfiedApis: map[string][]*pkg.LocalPackage{},
	}
}

func (r *Resolver) lpkgSlice() []*pkg.LocalPackage {
	lpkgs := make([]*pkg.LocalPackage, len(r.pkgMap))

	i := 0
	for lpkg, _ := range r.pkgMap {
		lpkgs[i] = lpkg
		i++
	}

	return lpkgs
}

func (r *Resolver) apiSlice() []string {
	apis := make([]string, len(r.apis))

	i := 0
	for api, _ := range r.apis {
		apis[i] = api
		i++
	}

	return apis
}

func (r *Resolver) addPkg(lpkg *pkg.LocalPackage) *ResolvePackage {
	rpkg := newResolvePkg(lpkg)
	r.pkgMap[lpkg] = rpkg
	return rpkg
}

// @return bool                 true if this is a new API.
func (r *Resolver) addApi(apiString string, rpkg *ResolvePackage) bool {
	curRpkg := r.apis[apiString]
	if curRpkg == nil {
		r.apis[apiString] = rpkg
		return true
	} else {
		if curRpkg != rpkg {
			util.StatusMessage(util.VERBOSITY_QUIET,
				"Warning: API conflict: %s (%s <-> %s)\n", apiString,
				curRpkg.Name(), rpkg.Name())
		}
		return false
	}
}

// Searches for a package which can satisfy bpkg's API requirement.  If such a
// package is found, bpkg's API requirement is marked as satisfied, and the
// package is added to bpkg's dependency list.
func (r *Resolver) satisfyReqApi(rpkg *ResolvePackage, reqApi string) bool {
	depRpkg := r.apis[reqApi]
	if depRpkg == nil {
		// Insert nil to indicate an unsatisfied API.
		r.apis[reqApi] = nil
		return false
	}

	dep := &pkg.Dependency{
		Name: depRpkg.Name(),
		Repo: depRpkg.Repo().Name(),
	}
	rpkg.reqApiMap[reqApi] = true

	// This package now has a new unresolved dependency.
	rpkg.depsResolved = false

	log.Debugf("API requirement satisfied; pkg=%s API=(%s, %s)",
		rpkg.Name(), reqApi, dep.String())

	return true
}

// @return bool                 true if a new dependency was detected as a
//                                  result of satisfying an API for this
//                                  package.
func (r *Resolver) satisfyApis(rpkg *ResolvePackage) bool {
	// Assume all this package's APIs are satisfied and that no new
	// dependencies will be detected.
	rpkg.apisSatisfied = true
	newDeps := false

	features := r.cfg.FeaturesForLpkg(rpkg.LocalPackage)

	// Determine if any of the package's API requirements can now be satisfied.
	// If so, another full iteration is required.
	reqApis := newtutil.GetStringSliceFeatures(rpkg.PkgV, features,
		"pkg.req_apis")
	for _, reqApi := range reqApis {
		reqStatus := rpkg.reqApiMap[reqApi]
		if !reqStatus {
			apiSatisfied := r.satisfyReqApi(rpkg, reqApi)
			if apiSatisfied {
				// An API was satisfied; the package now has a new dependency
				// that needs to be resolved.
				newDeps = true
				reqStatus = true
			} else {
				rpkg.reqApiMap[reqApi] = false
			}
		}
		if reqStatus {
			rpkg.apisSatisfied = false
		}
	}

	return newDeps
}

// @return bool                 True if this this function changed the builder
//                                  state; another full iteration is required
//                                  in this case.
//         error                non-nil on failure.
func (r *Resolver) loadDepsForPkg(rpkg *ResolvePackage) (bool, error) {
	proj := project.GetProject()
	features := r.cfg.FeaturesForLpkg(rpkg.LocalPackage)

	changed := false

	newDeps := newtutil.GetStringSliceFeatures(rpkg.PkgV, features, "pkg.deps")
	for _, newDepStr := range newDeps {
		newDep, err := pkg.NewDependency(rpkg.Repo(), newDepStr)
		if err != nil {
			return false, err
		}

		lpkg, ok := proj.ResolveDependency(newDep).(*pkg.LocalPackage)
		if !ok {
			return false,
				util.FmtNewtError("Could not resolve package dependency: "+
					"%s; depender: %s", newDep.String(), rpkg.Name())
		}

		if r.pkgMap[lpkg] == nil {
			changed = true
			r.addPkg(lpkg)
		}
	}

	// Determine if this package supports any APIs that we haven't seen
	// yet.  If so, another full iteration is required.
	apis := newtutil.GetStringSliceFeatures(rpkg.PkgV, features, "pkg.apis")
	for _, api := range apis {
		newApi := r.addApi(api, rpkg)
		if newApi {
			changed = true
		}
	}

	return changed, nil
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
func (r *Resolver) resolvePkg(rpkg *ResolvePackage) (bool, error) {
	var err error
	newDeps := false

	if !rpkg.depsResolved {
		newDeps, err = r.loadDepsForPkg(rpkg)
		if err != nil {
			return false, err
		}

		rpkg.depsResolved = !newDeps
	}

	if !rpkg.apisSatisfied {
		newApiDep := r.satisfyApis(rpkg)
		if newApiDep {
			newDeps = true
		}
	}

	return newDeps, nil
}

// @return                      changed,err
func (r *Resolver) reloadCfg() (bool, error) {
	lpkgs := r.lpkgSlice()
	apis := r.apiSlice()

	// Determine which features have been detected so far.  The feature map is
	// required for reloading syscfg, as features may unlock additional
	// settings.
	features := r.cfg.Features()
	cfg, err := syscfg.Read(lpkgs, apis, r.injectedSettings, features,
		r.flashMap)
	if err != nil {
		return false, err
	}

	changed := false
	for k, v := range cfg.Settings {
		oldval, ok := r.cfg.Settings[k]
		if !ok || len(oldval.History) != len(v.History) {
			r.cfg = cfg
			changed = true
		}
	}

	return changed, nil
}

func (r *Resolver) loadDepsOnce() (bool, error) {
	// Circularly resolve dependencies, APIs, and required APIs until no new
	// ones exist.
	newDeps := false
	for {
		reprocess := false
		for _, rpkg := range r.pkgMap {
			newDeps, err := r.resolvePkg(rpkg)
			if err != nil {
				return false, err
			}

			if newDeps {
				// The new dependencies need to be processed.  Iterate again
				// after this iteration completes.
				reprocess = true
			}
		}

		if !reprocess {
			break
		}
	}

	return newDeps, nil
}

func (r *Resolver) loadDepsAndCfg() error {
	if _, err := r.loadDepsOnce(); err != nil {
		return err
	}

	for {
		cfgChanged, err := r.reloadCfg()
		if err != nil {
			return err
		}
		if cfgChanged {
			// A new supported feature was discovered.  It is impossible
			// to determine what new dependency and API requirements are
			// generated as a result.  All packages need to be
			// reprocessed.
			for _, rpkg := range r.pkgMap {
				rpkg.depsResolved = false
				rpkg.apisSatisfied = false
			}
		}

		newDeps, err := r.loadDepsOnce()
		if err != nil {
			return err
		}

		if !newDeps && !cfgChanged {
			break
		}
	}

	// Log the final syscfg.
	r.cfg.Log()

	return nil
}

func pkgSliceUnion(left []*pkg.LocalPackage,
	right []*pkg.LocalPackage) []*pkg.LocalPackage {

	set := make(map[*pkg.LocalPackage]struct{}, len(left)+len(right))
	for _, lpkg := range left {
		set[lpkg] = struct{}{}
	}
	for _, lpkg := range right {
		set[lpkg] = struct{}{}
	}

	result := []*pkg.LocalPackage{}
	for lpkg := range set {
		result = append(result, lpkg)
	}

	return result
}

func pkgSliceRemove(slice []*pkg.LocalPackage,
	toRemove *pkg.LocalPackage) []*pkg.LocalPackage {

	for i, lpkg := range slice {
		if lpkg == toRemove {
			return append(slice[:i], slice[i+1:]...)
		}
	}

	return slice
}

func ResolvePkgs(cfgResolution CfgResolution,
	seedPkgs []*pkg.LocalPackage) ([]*pkg.LocalPackage, error) {

	r := newResolver()
	r.cfg = cfgResolution.Cfg
	for _, lpkg := range seedPkgs {
		r.addPkg(lpkg)
	}

	if _, err := r.loadDepsOnce(); err != nil {
		return nil, err
	}

	// Satisfy API requirements.
	for _, rpkg := range r.pkgMap {
		for api, _ := range rpkg.reqApiMap {
			apiPkg := cfgResolution.ApiMap[api]
			if apiPkg == nil {
				return nil, util.FmtNewtError(
					"Unsatisfied API at unexpected time: %s", api)
			}

			r.addPkg(apiPkg)
		}
	}

	lpkgs := make([]*pkg.LocalPackage, len(r.pkgMap))
	i := 0
	for lpkg, _ := range r.pkgMap {
		lpkgs[i] = lpkg
		i++
	}

	return lpkgs, nil
}

func ResolveSplitPkgs(cfgResolution CfgResolution,
	appLoaderPkg *pkg.LocalPackage,
	appAppPkg *pkg.LocalPackage,
	bspPkg *pkg.LocalPackage,
	compilerPkg *pkg.LocalPackage,
	targetPkg *pkg.LocalPackage) (

	loaderPkgs []*pkg.LocalPackage,
	appPkgs []*pkg.LocalPackage,
	err error) {

	if appLoaderPkg != nil {
		loaderSeedPkgs := []*pkg.LocalPackage{
			appLoaderPkg, bspPkg, compilerPkg, targetPkg}
		loaderPkgs, err = ResolvePkgs(cfgResolution, loaderSeedPkgs)
		if err != nil {
			return
		}
	}

	appSeedPkgs := []*pkg.LocalPackage{
		appAppPkg, bspPkg, compilerPkg, targetPkg}
	appPkgs, err = ResolvePkgs(cfgResolution, appSeedPkgs)
	if err != nil {
		return
	}

	// The app image needs to have access to all the loader packages except
	// the loader app itself.  This is required because the app sysinit
	// function needs to initialize every package in the build.
	if appLoaderPkg != nil {
		appPkgs = pkgSliceUnion(appPkgs, loaderPkgs)
		appPkgs = pkgSliceRemove(appPkgs, appLoaderPkg)
	}

	return
}

func ResolveCfg(seedPkgs []*pkg.LocalPackage,
	injectedSettings map[string]string,
	flashMap flash.FlashMap) (CfgResolution, error) {

	resolution := newCfgResolution()

	r := newResolver()
	r.flashMap = flashMap
	if injectedSettings == nil {
		injectedSettings = map[string]string{}
	}
	r.injectedSettings = injectedSettings

	for _, lpkg := range seedPkgs {
		r.addPkg(lpkg)
	}

	if err := r.loadDepsAndCfg(); err != nil {
		return resolution, err
	}

	resolution.Cfg = r.cfg
	resolution.ApiMap = make(map[string]*pkg.LocalPackage, len(r.apis))
	anyUnsatisfied := false
	for api, rpkg := range r.apis {
		if rpkg == nil {
			anyUnsatisfied = true
		} else {
			resolution.ApiMap[api] = rpkg.LocalPackage
		}
	}

	if anyUnsatisfied {
		for lpkg, rpkg := range r.pkgMap {
			for api, satisfied := range rpkg.reqApiMap {
				if !satisfied {
					slice := resolution.UnsatisfiedApis[api]
					slice = append(slice, lpkg)
					resolution.UnsatisfiedApis[api] = slice
				}
			}
		}
	}

	return resolution, nil
}

func (cfgResolution *CfgResolution) ErrorText() string {
	str := ""

	if len(cfgResolution.UnsatisfiedApis) > 0 {
		apiNames := make([]string, 0, len(cfgResolution.UnsatisfiedApis))
		for api, _ := range cfgResolution.UnsatisfiedApis {
			apiNames = append(apiNames, api)
		}
		sort.Strings(apiNames)

		str += "Unsatisfied APIs detected:\n"
		for _, api := range apiNames {
			str += fmt.Sprintf("    * %s, required by: ", api)

			pkgs := cfgResolution.UnsatisfiedApis[api]
			pkgNames := make([]string, len(pkgs))
			for i, lpkg := range pkgs {
				pkgNames[i] = lpkg.Name()
			}
			sort.Strings(pkgNames)

			str += strings.Join(pkgNames, ", ")
			str += "\n"
		}
	}

	str += cfgResolution.Cfg.ErrorText()

	return strings.TrimSpace(str)
}

func (cfgResolution *CfgResolution) WarningText() string {
	return cfgResolution.Cfg.WarningText()
}
