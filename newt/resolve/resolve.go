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

	"mynewt.apache.org/newt/newt/flashmap"
	"mynewt.apache.org/newt/newt/logcfg"
	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/sysdown"
	"mynewt.apache.org/newt/newt/sysinit"
	"mynewt.apache.org/newt/newt/ycfg"
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
	seedPkgs         []*pkg.LocalPackage
	injectedSettings map[string]string
	flashMap         flashmap.FlashMap
	cfg              syscfg.Cfg
	lcfg             logcfg.LCfg
	sysinitCfg       sysinit.SysinitCfg
	sysdownCfg       sysdown.SysdownCfg

	// [api-name][api-supplier]
	apiConflicts map[string]map[*ResolvePackage]struct{}
}

type ResolveDep struct {
	// Package being depended on.
	Rpkg *ResolvePackage

	// Name of API that generated the dependency; "" if a hard dependency.
	Api string

	// Set of syscfg expressions that generated this dependency.
	ExprMap map[string]struct{}
}

type ResolvePackage struct {
	Lpkg *pkg.LocalPackage
	Deps map[*ResolvePackage]*ResolveDep

	// Keeps track of API requirements and whether they are satisfied.
	reqApiMap map[string]resolveReqApi

	depsResolved bool

	// Tracks this package's dependents (things that depend on us).  If this
	// map becomes empty, this package can be deleted from the resolver.
	revDeps map[*ResolvePackage]struct{}
}

type ResolveSet struct {
	// Parent resoluion.  Contains this ResolveSet.
	Res *Resolution

	// All seed packages and their dependencies.
	Rpkgs []*ResolvePackage
}

type ApiConflict struct {
	Api  string
	Pkgs []*ResolvePackage
}

// The result of resolving a target's configuration, APIs, and dependencies.
type Resolution struct {
	Cfg             syscfg.Cfg
	LCfg            logcfg.LCfg
	SysinitCfg      sysinit.SysinitCfg
	SysdownCfg      sysdown.SysdownCfg
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
	flashMap flashmap.FlashMap) *Resolver {

	r := &Resolver{
		apis:             map[string]resolveApi{},
		pkgMap:           map[*pkg.LocalPackage]*ResolvePackage{},
		seedPkgs:         seedPkgs,
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
		revDeps:   map[*ResolvePackage]struct{}{},
	}
}

// Creates an expression string from all the conditionals associated with the
// dependency.  The resulting expression is the union of all sub expressions.
// For example:
//     pkg.deps.FOO:
//         - my_pkg
//     pkg.deps.BAR:
//         - my_pkg
//
// The expression string for the `my_pkg` dependency is:
//     (FOO) || (BAR)
func (rdep *ResolveDep) ExprString() string {
	// If there is an unconditional dependency, the conditional dependencies
	// can be ignored.
	if _, ok := rdep.ExprMap[""]; ok {
		return ""
	}

	exprs := make([]string, 0, len(rdep.ExprMap))
	for expr, _ := range rdep.ExprMap {
		exprs = append(exprs, expr)
	}

	// The union of one object is itself.
	if len(exprs) == 1 {
		return exprs[0]
	}

	// Sort all the subexpressions and OR them together.
	sort.Strings(exprs)
	s := ""
	for i, expr := range exprs {
		if i != 0 {
			s += fmt.Sprintf(" || ")
		}
		s += fmt.Sprintf("(%s)", expr)
	}
	return s
}

func (r *Resolver) resolveDep(dep *pkg.Dependency,
	depender string) (*pkg.LocalPackage, error) {

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
	depPkg *ResolvePackage, api string, expr string) bool {

	norm, err := parse.NormalizeExpr(expr)
	if err != nil {
		panic("invalid expression, should have been caught earlier: " +
			err.Error())
	}

	if dep := rpkg.Deps[depPkg]; dep != nil {
		// This package already depends on dep.  If the conditional expression
		// is new, or if the API string is different, then the existing
		// dependency needs to be updated with the new information.  Otherwise,
		// ignore the duplicate.

		// Determine if this is a new conditional expression.
		oldExpr := dep.ExprString()

		changed := false
		if _, ok := dep.ExprMap[norm]; !ok {
			dep.ExprMap[norm] = struct{}{}
			merged := dep.ExprString()

			log.Debugf("Package %s has conflicting dependencies on %s: "+
				"old=`%s` new=`%s`; merging them into a single conditional: "+
				"`%s`",
				rpkg.Lpkg.FullName(), dep.Rpkg.Lpkg.FullName(),
				oldExpr, expr, merged)
			changed = true
		}

		if dep.Api != "" && api == "" {
			dep.Api = api
			changed = true
		}

		return changed
	} else {
		rpkg.Deps[depPkg] = &ResolveDep{
			Rpkg: depPkg,
			Api:  api,
			ExprMap: map[string]struct{}{
				norm: struct{}{},
			},
		}
	}

	// If this dependency came from an API requirement, record that the API
	// requirement is now satisfied.
	if api != "" {
		apiReq := rpkg.reqApiMap[api]
		apiReq.expr = expr
		apiReq.satisfied = true
		rpkg.reqApiMap[api] = apiReq
	}

	depPkg.revDeps[rpkg] = struct{}{}

	return true
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

func (r *Resolver) sortedRpkgs() []*ResolvePackage {
	rpkgs := make([]*ResolvePackage, 0, len(r.pkgMap))
	for _, rpkg := range r.pkgMap {
		rpkgs = append(rpkgs, rpkg)
	}

	SortResolvePkgs(rpkgs)
	return rpkgs
}

// Selects the final API suppliers among all packages implementing APIs.  The
// result gets written to the resolver's `apis` map.  If more than one package
// implements the same API, an API conflict error is recorded.
func (r *Resolver) selectApiSuppliers() {
	apiMap := map[string][]resolveApi{}

	for _, rpkg := range r.sortedRpkgs() {
		settings := r.cfg.AllSettingsForLpkg(rpkg.Lpkg)
		apiStrings := rpkg.Lpkg.PkgY.GetSlice("pkg.apis", settings)
		for _, entry := range apiStrings {
			apiStr, ok := entry.Value.(string)
			if ok && apiStr != "" {
				apiMap[apiStr] = append(apiMap[apiStr], resolveApi{
					rpkg: rpkg,
					expr: entry.Expr,
				})
			}
		}
	}

	apiNames := make([]string, 0, len(apiMap))
	for name, _ := range apiMap {
		apiNames = append(apiNames, name)
	}
	sort.Strings(apiNames)

	for _, name := range apiNames {
		apis := apiMap[name]
		for _, api := range apis {
			old := r.apis[name]
			if old.rpkg != nil {
				if r.apiConflicts[name] == nil {
					r.apiConflicts[name] = map[*ResolvePackage]struct{}{}
				}
				r.apiConflicts[name][api.rpkg] = struct{}{}
				r.apiConflicts[name][old.rpkg] = struct{}{}
			} else {
				r.apis[name] = api
			}
		}
	}
}

// Populates the specified package's set of API requirements.
func (r *Resolver) calcApiReqsFor(rpkg *ResolvePackage) {
	settings := r.cfg.AllSettingsForLpkg(rpkg.Lpkg)

	reqApiEntries := rpkg.Lpkg.PkgY.GetSlice("pkg.req_apis", settings)
	for _, entry := range reqApiEntries {
		apiStr, ok := entry.Value.(string)
		if ok && apiStr != "" {
			rpkg.reqApiMap[apiStr] = resolveReqApi{
				satisfied: false,
				expr:      "",
			}
		}
	}
}

// Populates all packages' API requirements sets.
func (r *Resolver) calcApiReqs() {
	for _, rpkg := range r.pkgMap {
		r.calcApiReqsFor(rpkg)
	}
}

// Completely removes a package from the resolver.  This is used to prune
// packages when newly-discovered syscfg values nullify dependencies.
func (r *Resolver) deletePkg(rpkg *ResolvePackage) error {
	delete(r.pkgMap, rpkg.Lpkg)

	// Delete the package from syscfg.
	r.cfg.DeletePkg(rpkg.Lpkg)

	// Remove all dependencies on the deleted package.
	for revdep, _ := range rpkg.revDeps {
		delete(revdep.Deps, rpkg)
	}

	// Remove all reverse dependencies pointing to the deleted package.  If the
	// deleted package is the only depender for any other packages (i.e., if
	// any of its dependencies have only one reverse dependency),
	// delete them as well.
	for dep, _ := range rpkg.Deps {
		if len(dep.revDeps) == 0 {
			return util.FmtNewtError(
				"package %s unexpectedly has 0 reverse dependencies",
				dep.Lpkg.FullName())
		}
		delete(dep.revDeps, rpkg)
		if len(dep.revDeps) == 0 {
			if err := r.deletePkg(dep); err != nil {
				return err
			}
		}
	}

	return nil
}

// @return bool                 True if this this function changed the resolver
//                                  state; another full iteration is required
//                                  in this case.
//         error                non-nil on failure.
func (r *Resolver) loadDepsForPkg(rpkg *ResolvePackage) (bool, error) {
	settings := r.cfg.AllSettingsForLpkg(rpkg.Lpkg)

	changed := false

	var depEntries []ycfg.YCfgEntry

	if rpkg.Lpkg.Type() == pkg.PACKAGE_TYPE_TRANSIENT {
		depEntries = rpkg.Lpkg.PkgY.GetSlice("pkg.link", nil)
	} else {
		depEntries = rpkg.Lpkg.PkgY.GetSlice("pkg.deps", settings)
	}
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

	// This iteration may have deleted some dependency relationships (e.g., if
	// a new syscfg setting was discovered which causes this package's
	// dependency list to be overwritten).  Detect and delete these
	// relationships.
	for rdep, _ := range rpkg.Deps {
		if _, ok := seen[rdep]; !ok {
			delete(rpkg.Deps, rdep)
			delete(rdep.revDeps, rpkg)
			changed = true

			// If we just deleted the last reference to a package, remove the
			// package entirely from the resolver and syscfg.
			if len(rdep.revDeps) == 0 {
				if err := r.deletePkg(rdep); err != nil {
					return true, err
				}
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

	cfg.ResolveValueRefs()

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

// @return bool                 True if any packages were pruned, false
//                                  otherwise.
// @return err                  Error
func (r *Resolver) pruneOrphans() (bool, error) {
	seenMap := map[*ResolvePackage]struct{}{}

	// This function traverses the specified package's dependency list,
	// recording each visited packges in `seenMap`.
	var visit func(rpkg *ResolvePackage)
	visit = func(rpkg *ResolvePackage) {
		if _, ok := seenMap[rpkg]; ok {
			return
		}

		seenMap[rpkg] = struct{}{}
		for dep, _ := range rpkg.Deps {
			visit(dep)
		}
	}

	// Starting from each seed package, recursively traverse the package's
	// dependency list, keeping track of which packages were visited.
	for _, lpkg := range r.seedPkgs {
		rpkg := r.pkgMap[lpkg]
		if rpkg == nil {
			panic("Resolver lacks mapping for seed package " + lpkg.FullName())
		}

		visit(rpkg)
	}

	// Any non-visited packages in the resolver are orphans and can be removed.
	anyPruned := false
	for _, rpkg := range r.pkgMap {
		if _, ok := seenMap[rpkg]; !ok {
			anyPruned = true
			if err := r.deletePkg(rpkg); err != nil {
				return false, err
			}
		}
	}

	return anyPruned, nil
}

func (r *Resolver) resolveHardDepsOnce() (bool, error) {
	// Circularly resolve dependencies, APIs, and required APIs until no new
	// ones exist.
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

	if reprocess {
		return true, nil
	}

	// Now prune orphan packages.
	anyPruned, err := r.pruneOrphans()
	if err != nil {
		return false, err
	}

	return anyPruned, nil
}

// Fully resolves all hard dependencies (i.e., packages listed in `pkg.deps`;
// not API dependencies).
func (r *Resolver) resolveHardDeps() error {
	for {
		reprocess, err := r.resolveHardDepsOnce()
		if err != nil {
			return err
		}

		if !reprocess {
			return nil
		}
	}
}

// Given a fully calculated syscfg and API map, resolves package dependencies
// by populating the resolver's package map.  This function should only be
// called if the resolver's syscfg (`cfg`) member is assigned.  This only
// happens for split images when the individual loader and app components are
// resolved separately, after the master syscfg and API map have been
// calculated.
func (r *Resolver) resolveDeps() ([]*ResolvePackage, error) {
	if err := r.resolveHardDeps(); err != nil {
		return nil, err
	}

	// Now that the final set of packages is known, determine which ones
	// satisfy each required API.
	r.selectApiSuppliers()

	// Determine which packages have API requirements.
	r.calcApiReqs()

	// Satisfy API requirements.
	if err := r.resolveApiDeps(); err != nil {
		return nil, err
	}

	rpkgs := r.rpkgSlice()
	return rpkgs, nil
}

// Performs a set of resolution actions:
// 1. Calculates the system configuration (syscfg).
// 2. Determines which packages satisfy which API requirements.
// 3. Resolves package dependencies by populating the resolver's package map.
func (r *Resolver) resolveDepsAndCfg() error {
	if err := r.resolveHardDeps(); err != nil {
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
			}
		}

		if err := r.resolveHardDeps(); err != nil {
			return err
		}

		if !cfgChanged {
			break
		}
	}

	// Now that the final set of packages is known, determine which ones
	// satisfy each required API.
	r.selectApiSuppliers()

	// Determine which packages have API requirements.
	r.calcApiReqs()

	// Satisfy API requirements.
	if err := r.resolveApiDeps(); err != nil {
		return err
	}

	lpkgs := RpkgSliceToLpkgSlice(r.rpkgSlice())
	r.lcfg = logcfg.Read(lpkgs, &r.cfg)
	r.sysinitCfg = sysinit.Read(lpkgs, &r.cfg)
	r.sysdownCfg = sysdown.Read(lpkgs, &r.cfg)

	// Log the final syscfg.
	r.cfg.Log()

	return nil
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

// Transforms each package's required APIs to hard dependencies.  That is, this
// function determines which package supplies each required API, and adds the
// corresponding dependecy to each package which requires the API.
func (r *Resolver) resolveApiDeps() error {
	for _, rpkg := range r.pkgMap {
		for apiString, reqApi := range rpkg.reqApiMap {
			// Determine which package satisfies this API requirement.
			api, ok := r.apis[apiString]

			// If there is a package that supports the requested API, add a
			// hard dependency to the package.  Otherwise, record an
			// unsatisfied API requirement with an empty API struct.
			if ok && api.rpkg != nil {
				rpkg.AddDep(api.rpkg, apiString,
					joinExprs(api.expr, reqApi.expr))
			} else if !ok {
				r.apis[apiString] = resolveApi{}
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
	flashMap flashmap.FlashMap) (*Resolution, error) {

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
	res.LCfg = r.lcfg
	res.SysinitCfg = r.sysinitCfg
	res.SysdownCfg = r.sysdownCfg

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

	// We have now resolved all packages so go through them and emit warning
	// when using link packages
	for _, rpkg := range res.MasterSet.Rpkgs {
		if rpkg.Lpkg.Type() != pkg.PACKAGE_TYPE_TRANSIENT {
			continue
		}

		log.Warnf("Transient package %s used, update configuration "+
			"to use linked package instead (%s)",
			rpkg.Lpkg.FullName(), rpkg.Lpkg.LinkedName())
	}

	// If there is no loader, then the set of all packages is just the app
	// packages.  We already resolved the necessary dependency information when
	// syscfg was calculated above.
	if loaderSeeds == nil {
		res.AppSet.Rpkgs = r.rpkgSlice()
		res.LoaderSet = nil
		res.Cfg.DetectErrors(flashMap)
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

	res.Cfg.DetectErrors(flashMap)

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
	str += res.LCfg.ErrorText()
	str += res.SysinitCfg.ErrorText()
	str += res.SysdownCfg.ErrorText()

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
		text += ")\n"
	}

	return text + res.Cfg.WarningText()
}

func (res *Resolution) DeprecatedWarning() []string {
	return res.Cfg.DeprecatedWarning()
}
