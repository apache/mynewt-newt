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
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/util"
)

// Represents a supplied API.
type resolveApi struct {
	// The package which supplies the API.
	rpkg *ResolvePackage

	// The expression which enabled this API.
	expr string
}

// Represents a required API.
type resolveReqApi struct {
	// Whether the API requirement has been satisfied by a hard dependency.
	satisfied bool

	// The expression which enabled this API requirement.
	expr string
}

type Resolver struct {
	apis             map[string]resolveApi
	pkgMap           map[*pkg.LocalPackage]*ResolvePackage
	injectedSettings map[string]string
	flashMap         flash.FlashMap
	cfg              syscfg.Cfg

	// [api-name][api-supplier]
	apiConflicts map[string]map[*ResolvePackage]struct{}
}

type ResolveDep struct {
	// Package being depended on.
	Rpkg *ResolvePackage

	// Name of API that generated the dependency; "" if a hard dependency.
	Api string

	// Text of syscfg expression that generated this dependency; "" for none.
	Expr string
}

type ResolvePackage struct {
	Lpkg *pkg.LocalPackage
	Deps map[*ResolvePackage]*ResolveDep

	// Keeps track of API requirements and whether they are satisfied.
	reqApiMap map[string]resolveReqApi

	depsResolved  bool
	apisSatisfied bool
	refCount      int
}

type ResolveSet struct {
	// Parent resoluion.  Contains this ResolveSet.
	Res *Resolution

	// All seed pacakges and their dependencies.
	Rpkgs []*ResolvePackage
}

type ApiConflict struct {
	Api  string
	Pkgs []*ResolvePackage
}

// The result of resolving a target's configuration, APIs, and dependencies.
type Resolution struct {
	Cfg             syscfg.Cfg
	ApiMap          map[string]*ResolvePackage
	UnsatisfiedApis map[string][]*ResolvePackage
	ApiConflicts    []ApiConflict

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
		apis:             map[string]resolveApi{},
		pkgMap:           map[*pkg.LocalPackage]*ResolvePackage{},
		injectedSettings: injectedSettings,
		flashMap:         flashMap,
		cfg:              syscfg.NewCfg(),
		apiConflicts:     map[string]map[*ResolvePackage]struct{}{},
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
		reqApiMap: map[string]resolveReqApi{},
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

// @return                      true if the package's dependency list was
//                                  modified.
func (rpkg *ResolvePackage) AddDep(
	apiPkg *ResolvePackage, api string, expr string) bool {

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
			Expr: expr,
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
	rpkg.refCount = 1
	return rpkg, true
}

// @return bool                 true if this is a new API.
func (r *Resolver) addApi(
	apiString string, rpkg *ResolvePackage, expr string) bool {

	api := r.apis[apiString]
	if api.rpkg == nil {
		r.apis[apiString] = resolveApi{
			rpkg: rpkg,
			expr: expr,
		}
		return true
	} else {
		if api.rpkg != rpkg {
			if r.apiConflicts[apiString] == nil {
				r.apiConflicts[apiString] = map[*ResolvePackage]struct{}{}
			}
			r.apiConflicts[apiString][api.rpkg] = struct{}{}
			r.apiConflicts[apiString][rpkg] = struct{}{}
		}
		return false
	}
}

func joinExprs(expr1 string, expr2 string) string {
	if expr1 == "" {
		return expr2
	}
	if expr2 == "" {
		return expr1
	}

	return expr1 + "," + expr2
}

// Searches for a package which can satisfy bpkg's API requirement.  If such a
// package is found, bpkg's API requirement is marked as satisfied, and the
// package is added to bpkg's dependency list.
//
// @return bool                 true if the API is now satisfied.
func (r *Resolver) satisfyApi(
	rpkg *ResolvePackage, reqApi string, expr string) bool {

	api := r.apis[reqApi]
	if api.rpkg == nil {
		// Insert null entry to indicate an unsatisfied API.
		r.apis[reqApi] = resolveApi{}
		return false
	}

	rpkg.reqApiMap[reqApi] = resolveReqApi{
		satisfied: true,
		expr:      expr,
	}

	// This package now has a new unresolved dependency.
	rpkg.depsResolved = false

	log.Debugf("API requirement satisfied; pkg=%s API=(%s, %s) expr=(%s)",
		rpkg.Lpkg.Name(), reqApi, api.rpkg.Lpkg.FullName(), expr)

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

	settings := r.cfg.SettingsForLpkg(rpkg.Lpkg)

	// Determine if any of the package's API requirements can now be satisfied.
	// If so, another full iteration is required.
	reqApiEntries := rpkg.Lpkg.PkgY.GetSlice("pkg.req_apis", settings)
	for _, entry := range reqApiEntries {
		apiStr, ok := entry.Value.(string)
		if ok && apiStr != "" {
			if !rpkg.reqApiMap[apiStr].satisfied {
				apiSatisfied := r.satisfyApi(rpkg, apiStr, entry.Expr)
				if apiSatisfied {
					// An API was satisfied; the package now has a new
					// dependency that needs to be resolved.
					newDeps = true
				} else {
					rpkg.reqApiMap[apiStr] = resolveReqApi{
						satisfied: false,
						expr:      "",
					}
					rpkg.apisSatisfied = false
				}
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
	settings := r.cfg.SettingsForLpkg(rpkg.Lpkg)

	changed := false

	depEntries := rpkg.Lpkg.PkgY.GetSlice("pkg.deps", settings)
	depender := rpkg.Lpkg.Name()

	seen := make(map[*ResolvePackage]struct{}, len(rpkg.Deps))

	for _, entry := range depEntries {
		depStr, ok := entry.Value.(string)
		if ok && depStr != "" {
			newDep, err := pkg.NewDependency(rpkg.Lpkg.Repo(), depStr)
			if err != nil {
				return false, err
			}

			lpkg, err := r.resolveDep(newDep, depender)
			if err != nil {
				return false, err
			}

			depRpkg, _ := r.addPkg(lpkg)
			if rpkg.AddDep(depRpkg, "", entry.Expr) {
				changed = true
			}
			seen[depRpkg] = struct{}{}
		}
	}

	for rpkg, _ := range rpkg.Deps {
		if _, ok := seen[rpkg]; !ok {
			delete(rpkg.Deps, rpkg)
			rpkg.refCount--
			changed = true
		}
	}

	// Determine if this package supports any APIs that we haven't seen
	// yet.  If so, another full iteration is required.
	apiEntries := rpkg.Lpkg.PkgY.GetSlice("pkg.apis", settings)
	for _, entry := range apiEntries {
		apiStr, ok := entry.Value.(string)
		if ok && apiStr != "" {
			if r.addApi(apiStr, rpkg, entry.Expr) {
				changed = true
			}
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

	// Determine which settings have been detected so far.  The feature map is
	// required for reloading syscfg, as settings may unlock additional
	// settings.
	settings := r.cfg.SettingValues()
	cfg, err := syscfg.Read(lpkgs, apis, r.injectedSettings, settings,
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

	// Remove orphaned packages.
	for lpkg, rpkg := range r.pkgMap {
		if rpkg.refCount == 0 {
			delete(r.pkgMap, lpkg)
		}
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
		for apiString, reqApi := range rpkg.reqApiMap {
			api := r.apis[apiString]
			if api.rpkg != nil {
				rpkg.AddDep(api.rpkg, apiString,
					joinExprs(api.expr, reqApi.expr))
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
	for name, api := range r.apis {
		if api.rpkg == nil {
			anyUnsatisfied = true
		} else {
			apiMap[name] = api.rpkg
		}
	}

	unsatisfied := map[string][]*ResolvePackage{}
	if anyUnsatisfied {
		for _, rpkg := range r.pkgMap {
			for name, reqApi := range rpkg.reqApiMap {
				if !reqApi.satisfied {
					slice := unsatisfied[name]
					slice = append(slice, rpkg)
					unsatisfied[name] = slice
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

	for api, m := range r.apiConflicts {
		c := ApiConflict{
			Api: api,
		}
		for rpkg, _ := range m {
			c.Pkgs = append(c.Pkgs, rpkg)
		}

		res.ApiConflicts = append(res.ApiConflicts, c)
	}

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

	str = strings.TrimSpace(str)
	if str != "" {
		str += "\n"
	}

	return str
}

func (res *Resolution) WarningText() string {
	text := ""

	for _, c := range res.ApiConflicts {
		text += fmt.Sprintf("Warning: API conflict: %s (", c.Api)
		for i, rpkg := range c.Pkgs {
			if i != 0 {
				text += " <-> "
			}
			text += rpkg.Lpkg.Name()
		}
	}

	return text + res.Cfg.WarningText()
}
