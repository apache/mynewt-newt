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

type ResolveDep struct {
	// Package being depended on.
	Rpkg *ResolvePackage

	// Name of API that generated the dependency; "" if a hard dependency.
	Api string

	// XXX: slice of syscfg settings that generated this dependency.
}

type ResolvePackage struct {
	Lpkg *pkg.LocalPackage
	Deps map[*ResolvePackage]*ResolveDep

	// Keeps track of API requirements and whether they are satisfied.
	reqApiMap map[string]bool

	depsResolved  bool
	apisSatisfied bool
}

type ResolveSet struct {
	// Parent resoluion.  Contains this ResolveSet.
	Res *Resolution

	// All seed pacakges and their dependencies.
	Rpkgs []*ResolvePackage
}

// The result of resolving a target's configuration, APIs, and dependencies.
type Resolution struct {
	Cfg             syscfg.Cfg
	ApiMap          map[string]*ResolvePackage
	UnsatisfiedApis map[string][]*ResolvePackage

	LpkgRpkgMap map[*pkg.LocalPackage]*ResolvePackage

	// Contains all dependencies; union of loader and app.
	MasterSet *ResolveSet

	LoaderSet *ResolveSet
	AppSet    *ResolveSet
}

func newResolver(
	seedPkgs []*pkg.LocalPackage,
	injectedSettings map[string]string,
	flashMap flash.FlashMap) *Resolver {

	r := &Resolver{
		apis:             map[string]*ResolvePackage{},
		pkgMap:           map[*pkg.LocalPackage]*ResolvePackage{},
		injectedSettings: injectedSettings,
		flashMap:         flashMap,
		cfg:              syscfg.NewCfg(),
	}

	if injectedSettings == nil {
		r.injectedSettings = map[string]string{}
	}

	for _, lpkg := range seedPkgs {
		r.addPkg(lpkg)
	}

	return r
}

func newResolution() *Resolution {
	r := &Resolution{
		ApiMap:          map[string]*ResolvePackage{},
		UnsatisfiedApis: map[string][]*ResolvePackage{},
	}

	r.MasterSet = &ResolveSet{Res: r}
	r.LoaderSet = &ResolveSet{Res: r}
	r.AppSet = &ResolveSet{Res: r}

	return r
}

func NewResolvePkg(lpkg *pkg.LocalPackage) *ResolvePackage {
	return &ResolvePackage{
		Lpkg:      lpkg,
		reqApiMap: map[string]bool{},
		Deps:      map[*ResolvePackage]*ResolveDep{},
	}
}

func (r *Resolver) resolveDep(dep *pkg.Dependency, depender string) (*pkg.LocalPackage, error) {
	proj := project.GetProject()

	if proj.ResolveDependency(dep) == nil {
		return nil, util.FmtNewtError("Could not resolve package dependency: "+
			"%s; depender: %s", dep.String(), depender)
	}
	lpkg := proj.ResolveDependency(dep).(*pkg.LocalPackage)

	return lpkg, nil
}

// @return                      true if rhe package's dependency list was
//                                  modified.
func (rpkg *ResolvePackage) AddDep(apiPkg *ResolvePackage, api string) bool {
	if dep := rpkg.Deps[apiPkg]; dep != nil {
		if dep.Api != "" && api == "" {
			dep.Api = api
			return true
		} else {
			return false
		}
	} else {
		rpkg.Deps[apiPkg] = &ResolveDep{
			Rpkg: apiPkg,
			Api:  api,
		}
		return true
	}
}

func (r *Resolver) rpkgSlice() []*ResolvePackage {
	rpkgs := make([]*ResolvePackage, len(r.pkgMap))

	i := 0
	for _, rpkg := range r.pkgMap {
		rpkgs[i] = rpkg
		i++
	}

	return rpkgs
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

// @return ResolvePackage		The rpkg corresponding to the specified lpkg.
//                                  This is a new package if a package was
//                                  added; old if it was already present.
//         bool					true if this is a new package.
func (r *Resolver) addPkg(lpkg *pkg.LocalPackage) (*ResolvePackage, bool) {
	if rpkg := r.pkgMap[lpkg]; rpkg != nil {
		return rpkg, false
	}

	rpkg := NewResolvePkg(lpkg)
	r.pkgMap[lpkg] = rpkg
	return rpkg, true
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
				curRpkg.Lpkg.Name(), rpkg.Lpkg.Name())
		}
		return false
	}
}

// Searches for a package which can satisfy bpkg's API requirement.  If such a
// package is found, bpkg's API requirement is marked as satisfied, and the
// package is added to bpkg's dependency list.
//
// @return bool                 true if the API is now satisfied.
func (r *Resolver) satisfyApi(rpkg *ResolvePackage, reqApi string) bool {
	depRpkg := r.apis[reqApi]
	if depRpkg == nil {
		// Insert nil to indicate an unsatisfied API.
		r.apis[reqApi] = nil
		return false
	}

	rpkg.reqApiMap[reqApi] = true

	// This package now has a new unresolved dependency.
	rpkg.depsResolved = false

	log.Debugf("API requirement satisfied; pkg=%s API=(%s, %s)",
		rpkg.Lpkg.Name(), reqApi, depRpkg.Lpkg.FullName())

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

	features := r.cfg.FeaturesForLpkg(rpkg.Lpkg)

	// Determine if any of the package's API requirements can now be satisfied.
	// If so, another full iteration is required.
	reqApis := newtutil.GetStringSliceFeatures(rpkg.Lpkg.PkgV, features,
		"pkg.req_apis")
	for _, reqApi := range reqApis {
		reqStatus := rpkg.reqApiMap[reqApi]
		if !reqStatus {
			apiSatisfied := r.satisfyApi(rpkg, reqApi)
			if apiSatisfied {
				// An API was satisfied; the package now has a new dependency
				// that needs to be resolved.
				newDeps = true
				reqStatus = true
			} else {
				rpkg.reqApiMap[reqApi] = false
				rpkg.apisSatisfied = false
			}
		}
	}

	return newDeps
}

// @return bool                 True if this this function changed the resolver
//                                  state; another full iteration is required
//                                  in this case.
//         error                non-nil on failure.
func (r *Resolver) loadDepsForPkg(rpkg *ResolvePackage) (bool, error) {
	features := r.cfg.FeaturesForLpkg(rpkg.Lpkg)

	changed := false
	newDeps := newtutil.GetStringSliceFeatures(rpkg.Lpkg.PkgV, features,
		"pkg.deps")
	depender := rpkg.Lpkg.Name()
	for _, newDepStr := range newDeps {
		newDep, err := pkg.NewDependency(rpkg.Lpkg.Repo(), newDepStr)
		if err != nil {
			return false, err
		}

		lpkg, err := r.resolveDep(newDep, depender)
		if err != nil {
			return false, err
		}

		depRpkg, _ := r.addPkg(lpkg)
		if rpkg.AddDep(depRpkg, "") {
			changed = true
		}
	}

	// Determine if this package supports any APIs that we haven't seen
	// yet.  If so, another full iteration is required.
	apis := newtutil.GetStringSliceFeatures(rpkg.Lpkg.PkgV, features,
		"pkg.apis")
	for _, api := range apis {
		if r.addApi(api, rpkg) {
			changed = true
		}
	}

	return changed, nil
}

// Attempts to resolve all of a build package's dependencies, APIs, and
// required APIs.  This function should be called repeatedly until the package
// is fully resolved.
//
// If a dependency is resolved by this function, the new dependency needs to be
// processed.  The caller should attempt to resolve all packages again.
//
// @return bool                 true if >=1 dependencies were resolved.
//         error                non-nil on failure.
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
	lpkgs := RpkgSliceToLpkgSlice(r.rpkgSlice())
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

	// Determine if any settings have changed.
	for k, v := range cfg.Settings {
		oldval, ok := r.cfg.Settings[k]
		if !ok || len(oldval.History) != len(v.History) {
			r.cfg = cfg
			return true, nil
		}
	}

	return false, nil
}

func (r *Resolver) resolveDepsOnce() (bool, error) {
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

func (r *Resolver) resolveDeps() ([]*ResolvePackage, error) {
	if _, err := r.resolveDepsOnce(); err != nil {
		return nil, err
	}

	// Satisfy API requirements.
	if err := r.resolveApiDeps(); err != nil {
		return nil, err
	}

	rpkgs := r.rpkgSlice()
	return rpkgs, nil
}

func (r *Resolver) resolveDepsAndCfg() error {
	if _, err := r.resolveDepsOnce(); err != nil {
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

		newDeps, err := r.resolveDepsOnce()
		if err != nil {
			return err
		}

		if !newDeps && !cfgChanged {
			break
		}
	}

	// Satisfy API requirements.
	if err := r.resolveApiDeps(); err != nil {
		return err
	}

	// Log the final syscfg.
	r.cfg.Log()

	return nil
}

// Transforms each package's required APIs to hard dependencies.  That is, this
// function determines which package supplies each required API, and adds the
// corresponding dependecy to each package which requires the API.
func (r *Resolver) resolveApiDeps() error {
	for _, rpkg := range r.pkgMap {
		for api, _ := range rpkg.reqApiMap {
			apiPkg := r.apis[api]
			if apiPkg != nil {
				rpkg.AddDep(apiPkg, api)
			}
		}
	}

	return nil
}

func (r *Resolver) apiResolution() (
	map[string]*ResolvePackage,
	map[string][]*ResolvePackage) {

	apiMap := make(map[string]*ResolvePackage, len(r.apis))
	anyUnsatisfied := false
	for api, rpkg := range r.apis {
		if rpkg == nil {
			anyUnsatisfied = true
		} else {
			apiMap[api] = rpkg
		}
	}

	unsatisfied := map[string][]*ResolvePackage{}
	if anyUnsatisfied {
		for _, rpkg := range r.pkgMap {
			for api, satisfied := range rpkg.reqApiMap {
				if !satisfied {
					slice := unsatisfied[api]
					slice = append(slice, rpkg)
					unsatisfied[api] = slice
				}
			}
		}
	}

	return apiMap, unsatisfied
}

func ResolveFull(
	loaderSeeds []*pkg.LocalPackage,
	appSeeds []*pkg.LocalPackage,
	injectedSettings map[string]string,
	flashMap flash.FlashMap) (*Resolution, error) {

	// First, calculate syscfg and determine which package provides each
	// required API.  Syscfg and APIs are project-wide; that is, they are
	// calculated across the aggregate of all app packages and loader packages
	// (if any).  The dependency graph for the entire set of packages gets
	// calculated here as a byproduct.

	allSeeds := append(loaderSeeds, appSeeds...)
	r := newResolver(allSeeds, injectedSettings, flashMap)

	if err := r.resolveDepsAndCfg(); err != nil {
		return nil, err
	}

	res := newResolution()
	res.Cfg = r.cfg
	if err := r.resolveApiDeps(); err != nil {
		return nil, err
	}

	// Determine which package satisfies each API and which APIs are
	// unsatisfied.
	apiMap := map[string]*ResolvePackage{}
	apiMap, res.UnsatisfiedApis = r.apiResolution()

	res.LpkgRpkgMap = r.pkgMap

	res.MasterSet.Rpkgs = r.rpkgSlice()

	// If there is no loader, then the set of all packages is just the app
	// packages.  We already resolved the necessary dependency information when
	// syscfg was calculated above.
	if loaderSeeds == nil {
		res.AppSet.Rpkgs = r.rpkgSlice()
		res.LoaderSet = nil
		return res, nil
	}

	// Otherwise, we need to resolve dependencies separately for:
	// 1. The set of loader pacakges, and
	// 2. The set of app packages.
	//
	// These need to be resolved separately so that it is possible later to
	// determine which packages need to be shared between loader and app.

	// It is OK if the app requires an API that is supplied by the loader.
	// Ensure each set of packages has access to the API-providers.
	for _, rpkg := range apiMap {
		loaderSeeds = append(loaderSeeds, rpkg.Lpkg)
		appSeeds = append(appSeeds, rpkg.Lpkg)
	}

	// Resolve loader dependencies.
	r = newResolver(loaderSeeds, injectedSettings, flashMap)
	r.cfg = res.Cfg

	var err error

	res.LoaderSet.Rpkgs, err = r.resolveDeps()
	if err != nil {
		return nil, err
	}

	// Resolve app dependencies.  The app automtically gets all the packages
	// from the loader except for the loader-app-package.
	for _, rpkg := range res.LoaderSet.Rpkgs {
		if rpkg.Lpkg.Type() != pkg.PACKAGE_TYPE_APP {
			appSeeds = append(appSeeds, rpkg.Lpkg)
		}
	}

	r = newResolver(appSeeds, injectedSettings, flashMap)
	r.cfg = res.Cfg

	res.AppSet.Rpkgs, err = r.resolveDeps()
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (res *Resolution) ErrorText() string {
	str := ""

	if len(res.UnsatisfiedApis) > 0 {
		apiNames := make([]string, 0, len(res.UnsatisfiedApis))
		for api, _ := range res.UnsatisfiedApis {
			apiNames = append(apiNames, api)
		}
		sort.Strings(apiNames)

		str += "Unsatisfied APIs detected:\n"
		for _, api := range apiNames {
			str += fmt.Sprintf("    * %s, required by: ", api)

			rpkgs := res.UnsatisfiedApis[api]
			pkgNames := make([]string, len(rpkgs))
			for i, rpkg := range rpkgs {
				pkgNames[i] = rpkg.Lpkg.Name()
			}
			sort.Strings(pkgNames)

			str += strings.Join(pkgNames, ", ")
			str += "\n"
		}
	}

	str += res.Cfg.ErrorText()

	return strings.TrimSpace(str)
}

func (res *Resolution) WarningText() string {
	return res.Cfg.WarningText()
}
