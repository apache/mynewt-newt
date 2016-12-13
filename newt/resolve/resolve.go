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

// The result of resolving a target's configuration, APIs, and dependencies.
type Resolution struct {
	Cfg             syscfg.Cfg
	ApiMap          map[string]*pkg.LocalPackage
	UnsatisfiedApis map[string][]*pkg.LocalPackage

	LoaderPkgs []*pkg.LocalPackage
	AppPkgs    []*pkg.LocalPackage
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

func newResolvePkg(lpkg *pkg.LocalPackage) *ResolvePackage {
	return &ResolvePackage{
		LocalPackage: lpkg,
		reqApiMap:    map[string]bool{},
	}
}

func (r *Resolver) lpkgSlice() []*pkg.LocalPackage {
	lpkgs := make([]*pkg.LocalPackage, len(r.pkgMap))

	i := 0
	for _, rpkg := range r.pkgMap {
		lpkgs[i] = rpkg.LocalPackage
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

// @return bool					true if this is a new package.
func (r *Resolver) addPkg(lpkg *pkg.LocalPackage) bool {
	if rpkg := r.pkgMap[lpkg]; rpkg != nil {
		return false
	}

	r.pkgMap[lpkg] = newResolvePkg(lpkg)
	return true
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
		rpkg.Name(), reqApi, depRpkg.FullName())

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
					"%s; depender: %s", newDep.String(), rpkg.FullName())
		}

		if r.addPkg(lpkg) {
			changed = true
		}

		if rpkg.AddDep(newDep) {
			changed = true
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

func (r *Resolver) resolveDeps() ([]*pkg.LocalPackage, error) {
	if _, err := r.resolveDepsOnce(); err != nil {
		return nil, err
	}

	// Satisfy API requirements.
	if err := r.resolveApiDeps(); err != nil {
		return nil, err
	}

	lpkgs := r.lpkgSlice()
	return lpkgs, nil
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
// corresponding dependecy to each pacakge whic requires the API.
func (r *Resolver) resolveApiDeps() error {
	for _, rpkg := range r.pkgMap {
		for api, _ := range rpkg.reqApiMap {
			apiPkg := r.apis[api]
			if apiPkg == nil {
				return util.FmtNewtError(
					"Unsatisfied API at unexpected time: %s", api)
			}

			rpkg.AddDep(&pkg.Dependency{
				Name: apiPkg.Name(),
				Repo: apiPkg.Repo().Name(),
			})
		}
	}

	return nil
}

func (r *Resolver) apiResolution() (
	map[string]*pkg.LocalPackage,
	map[string][]*pkg.LocalPackage) {

	apiMap := make(map[string]*pkg.LocalPackage, len(r.apis))
	anyUnsatisfied := false
	for api, rpkg := range r.apis {
		if rpkg == nil {
			anyUnsatisfied = true
		} else {
			apiMap[api] = rpkg.LocalPackage
		}
	}

	unsatisfied := map[string][]*pkg.LocalPackage{}
	if anyUnsatisfied {
		for _, rpkg := range r.pkgMap {
			for api, satisfied := range rpkg.reqApiMap {
				if !satisfied {
					slice := unsatisfied[api]
					slice = append(slice, rpkg.LocalPackage)
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

	res := &Resolution{}
	res.Cfg = r.cfg
	if err := r.resolveApiDeps(); err != nil {
		return nil, err
	}

	// If there is no loader, then the set of all packages is just the app
	// packages.  We already resolved the necessary dependency information when
	// syscfg was calculated above.
	if loaderSeeds == nil {
		res.AppPkgs = r.lpkgSlice()
		return res, nil
	}

	// Otherwise, we need to resolve dependencies separately for:
	// 1. The set of loader pacakges, and
	// 2. The set of app pacakges.
	//
	// These need to be resolved separately so that it is possible later to
	// determine which packages need to be shared between loader and app.

	// Resolve loader dependencies.
	r = newResolver(loaderSeeds, injectedSettings, flashMap)
	r.cfg = res.Cfg

	var err error

	res.LoaderPkgs, err = r.resolveDeps()
	if err != nil {
		return nil, err
	}

	// Resolve app dependencies.
	r = newResolver(appSeeds, injectedSettings, flashMap)
	r.cfg = res.Cfg

	res.AppPkgs, err = r.resolveDeps()
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

			pkgs := res.UnsatisfiedApis[api]
			pkgNames := make([]string, len(pkgs))
			for i, lpkg := range pkgs {
				pkgNames[i] = lpkg.Name()
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
