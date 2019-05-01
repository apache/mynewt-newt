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

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/newt/flashmap"
	"mynewt.apache.org/newt/newt/logcfg"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/sysdown"
	"mynewt.apache.org/newt/newt/sysinit"
	"mynewt.apache.org/newt/util"
)

// Represents a supplied API.
type resolveApi struct {
	// The package which supplies the API.
	rpkg *ResolvePackage

	// The expression which enabled this API.
	expr *parse.Node
}

// Represents a required API.
type resolveReqApi struct {
	// Whether the API requirement has been satisfied by a hard dependency.
	satisfied bool

	// The set of expressions which enabled this API requirement.  If any
	// expressions are true, the API requirement is enabled.
	exprs parse.ExprSet
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

	// Set of syscfg expressions that generated this dependency.
	Exprs parse.ExprSet

	// Represents the set of API requirements that this dependency satisfies.
	// The map key is the API name.
	ApiExprMap parse.ExprMap
}

type ResolvePackage struct {
	Lpkg *pkg.LocalPackage
	Deps map[*ResolvePackage]*ResolveDep

	Apis parse.ExprMap

	// Keeps track of API requirements and whether they are satisfied.
	reqApiMap map[string]resolveReqApi

	depsResolved bool

	// Tracks this package's dependents (things that depend on us).  If this
	// map becomes empty, this package can be deleted from the resolver.
	revDeps map[*ResolvePackage]struct{}
}

type ResolveSet struct {
	// Parent resolution.  Contains this ResolveSet.
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

// useMasterPkgs replaces a resolve set's packages with their equivalents from
// the master set.  This function is necessary to ensure only a single copy of
// each package exists among all resolve sets in a split build.
func (rs *ResolveSet) useMasterPkgs() error {
	for i, rdst := range rs.Rpkgs {
		rsrc := rs.Res.LpkgRpkgMap[rdst.Lpkg]
		if rsrc == nil {
			return util.FmtNewtError(
				"cannot use master packages in resolve set; "+
					"package \"%s\" missing",
				rdst.Lpkg.FullName())
		}

		rs.Rpkgs[i] = rsrc
	}

	return nil
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
	depPkg *ResolvePackage, expr *parse.Node) bool {

	exprString := expr.String()

	var changed bool
	var dep *ResolveDep
	if dep = rpkg.Deps[depPkg]; dep != nil {
		// This package already depends on dep.  If the conditional expression
		// is new, then the existing dependency needs to be updated with the
		// new information.  Otherwise, ignore the duplicate.

		if _, ok := dep.Exprs[exprString]; !ok {
			changed = true
		}
	} else {
		// New dependency.
		dep = &ResolveDep{
			Rpkg: depPkg,
		}

		rpkg.Deps[depPkg] = dep
		depPkg.revDeps[rpkg] = struct{}{}
		changed = true
	}

	if dep.Exprs == nil {
		dep.Exprs = parse.ExprSet{}
	}
	dep.Exprs[exprString] = expr

	return changed
}

func (rpkg *ResolvePackage) AddApiDep(
	depPkg *ResolvePackage, api string, exprs []*parse.Node) {

	// Satisfy the API dependency.
	rpkg.reqApiMap[api] = resolveReqApi{
		satisfied: true,
		exprs:     parse.NewExprSet(exprs),
	}

	// Add a reverse dependency to the API-supplier.
	dep := rpkg.Deps[depPkg]
	if dep == nil {
		dep = &ResolveDep{
			Rpkg: depPkg,
		}
		rpkg.Deps[depPkg] = dep
	}
	if dep.ApiExprMap == nil {
		dep.ApiExprMap = parse.ExprMap{}
	}
	dep.ApiExprMap.Add(api, exprs)
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

func (r *Resolver) fillApisFor(rpkg *ResolvePackage) error {
	settings := r.cfg.AllSettingsForLpkg(rpkg.Lpkg)

	em, err := readExprMap(rpkg.Lpkg.PkgY, "pkg.apis", settings)
	if err != nil {
		return err
	}

	rpkg.Apis = em
	return nil
}

// Selects the final API suppliers among all packages implementing APIs.  The
// result gets written to the resolver's `apis` map.  If more than one package
// implements the same API, an API conflict error is recorded.
func (r *Resolver) selectApiSuppliers() {
	apiMap := map[string][]resolveApi{}

	// Fill each package's list of supplied APIs.
	for _, rpkg := range r.sortedRpkgs() {
		r.fillApisFor(rpkg)
		for apiName, exprSet := range rpkg.Apis {
			apiMap[apiName] = append(apiMap[apiName], resolveApi{
				rpkg: rpkg,
				expr: exprSet.Disjunction(),
			})
		}
	}

	// Detect API conflicts and determine which packages supply which APIs.
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
func (r *Resolver) calcApiReqsFor(rpkg *ResolvePackage) error {
	settings := r.cfg.AllSettingsForLpkg(rpkg.Lpkg)

	em, err := readExprMap(rpkg.Lpkg.PkgY, "pkg.req_apis", settings)
	if err != nil {
		return err
	}

	for api, es := range em {
		rpkg.reqApiMap[api] = resolveReqApi{
			satisfied: false,
			exprs:     es,
		}
	}

	return nil
}

// Populates all packages' API requirements sets.
func (r *Resolver) calcApiReqs() error {
	for _, rpkg := range r.pkgMap {
		if err := r.calcApiReqsFor(rpkg); err != nil {
			return err
		}
	}

	return nil
}

// Completely removes a package from the resolver.  This is used to prune
// packages when newly-discovered syscfg values nullify dependencies.
func (r *Resolver) deletePkg(rpkg *ResolvePackage) error {
	i := 0
	for i < len(r.seedPkgs) {
		lpkg := r.seedPkgs[i]
		if lpkg == rpkg.Lpkg {
			fmt.Printf("DELETING SEED: %s (%p)\n", lpkg.FullName(), lpkg)
			r.seedPkgs = append(r.seedPkgs[:i], r.seedPkgs[i+1:]...)
		} else {
			i++
		}
	}
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

	var depEm map[*parse.Node][]string
	var err error

	if rpkg.Lpkg.Type() == pkg.PACKAGE_TYPE_TRANSIENT {
		depEm, err = getExprMapStringSlice(rpkg.Lpkg.PkgY, "pkg.link", nil)
	} else {
		depEm, err = getExprMapStringSlice(rpkg.Lpkg.PkgY, "pkg.deps",
			settings)
	}
	if err != nil {
		return false, err
	}

	depender := rpkg.Lpkg.Name()

	oldDeps := rpkg.Deps
	rpkg.Deps = make(map[*ResolvePackage]*ResolveDep, len(oldDeps))
	for expr, depNames := range depEm {
		for _, depName := range depNames {
			newDep, err := pkg.NewDependency(rpkg.Lpkg.Repo(), depName)
			if err != nil {
				return false, err
			}

			lpkg, err := r.resolveDep(newDep, depender)
			if err != nil {
				return false, err
			}

			depRpkg, _ := r.addPkg(lpkg)
			rpkg.AddDep(depRpkg, expr)
		}
	}

	// This iteration may have deleted some dependency relationships (e.g., if
	// a new syscfg setting was discovered which causes this package's
	// dependency list to be overwritten).  Detect and delete these
	// relationships.
	for rdep, _ := range oldDeps {
		if _, ok := rpkg.Deps[rdep]; !ok {
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

	for rdep, _ := range rpkg.Deps {
		if _, ok := oldDeps[rdep]; !ok {
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

	// Determine if any new settings have been added or if any existing
	// settings have changed.
	for k, v := range cfg.Settings {
		oldval, ok := r.cfg.Settings[k]
		if !ok || len(oldval.History) != len(v.History) {
			r.cfg = cfg
			return true, nil
		}
	}

	// Determine if any existing settings have been removed.
	for k, _ := range r.cfg.Settings {
		if _, ok := cfg.Settings[k]; !ok {
			r.cfg = cfg
			return true, nil
		}
	}

	return false, nil
}

// traceToSeed determines if the specified package can be reached from a seed
// package via a traversal of the dependency graph.  The supplied settings are
// used to determine the validity of dependencies in the graph.
func (rpkg *ResolvePackage) traceToSeed(
	settings map[string]string) (bool, error) {

	seen := map[*ResolvePackage]struct{}{}

	// A nested function is used here so that the `seen` map can be used across
	// multiple invocations.
	var iter func(cur *ResolvePackage) (bool, error)
	iter = func(cur *ResolvePackage) (bool, error) {
		// Don't process the same package twice.
		if _, ok := seen[cur]; ok {
			return false, nil
		}
		seen[cur] = struct{}{}

		// A type greater than "library" is a seed package.
		if cur.Lpkg.Type() > pkg.PACKAGE_TYPE_LIB {
			return true, nil
		}

		// Repeat the trace recursively for each depending package.
		for depender, _ := range cur.revDeps {
			rdep := depender.Deps[cur]

			// Only process this reverse dependency if it is valid given the
			// specified syscfg.
			expr := rdep.Exprs.Disjunction()
			depValid, err := parse.Eval(expr, settings)
			if err != nil {
				return false, err
			}

			if depValid {
				traced, err := iter(depender)
				if err != nil {
					return false, err
				}
				if traced {
					// Depender can be traced to a seed package.
					return true, nil
				}
			}
		}

		// All dependencies processed without reaching a seed package.
		return false, nil
	}

	return iter(rpkg)
}

// detectImposter returns true if the package is an imposter.  A package is an
// imposter if it is in the dependency graph by virtue of its own syscfg
// defines and overrides.  For example, say we have a package `foo`:
//
//     pkg.name: foo
//     syscfg.defs:
//         FOO_SETTING:
//		       value: 1
//
// Then we have a BSP package:
//
//     pkg.name: my_bsp
//     pkg.deps.FOO_SETTING:
//         - foo
//
// If this is the only dependency on `foo`, then `foo` is an imposter.  It
// should be removed from the graph, and its syscfg defines and overrides
// should be deleted.
//
// Because the syscfg state changes as newt discovers new dependencies, it is
// possible for imposters to end up in the graph.
func (r *Resolver) detectImposter(rpkg *ResolvePackage) (bool, error) {
	// Calculate a new syscfg instance, pretending the specified package
	// doesn't exist.
	settings := make(map[string]syscfg.CfgEntry, len(r.cfg.Settings))
	for k, src := range r.cfg.Settings {
		// Copy the source entry in full, then check its history for the
		// potential imposter.
		dst := src
		dst.History = make([]syscfg.CfgPoint, len(src.History))
		copy(dst.History, src.History)

		for i, _ := range dst.History {
			if dst.History[i].Source == rpkg.Lpkg {
				if i == 0 {
					// This setting is defined by the package; remove it
					// entirely.
					dst.History = nil
				} else {
					// Remove the package's override.
					dst.History = append(dst.History[:i], dst.History[i+1:]...)
				}
				break
			}
		}

		// Retain the setting if it wasn't defined by the potential imposter.
		if len(dst.History) > 0 {
			settings[k] = dst
		}
	}

	// See if the package can still be traced to a seed package when the
	// modified settings are used.
	found, err := rpkg.traceToSeed(syscfg.SettingValues(settings))
	if err != nil {
		return false, err
	}

	return !found, nil
}

// detectImpostersWorker reads packages from a channel and determines if they
// are imposters.  It is meant to be run in parallel via multiple go routines.
//
// See detectImposter() for a definition of "imposter package".
func (r *Resolver) detectImpostersWorker(
	jobsCh <-chan *ResolvePackage,
	stopCh chan struct{},
	resultsCh chan<- *ResolvePackage,
	errCh chan<- error,
) {

	// Repeatedly process jobs until any of:
	// 1. Stop signal from another go routine.
	// 2. Error encountered.
	// 3. No more jobs.
	for {
		select {
		case <-stopCh:
			// Re-enqueue the stop signal for the other go routines.
			stopCh <- struct{}{}

			// Completed without error.
			errCh <- nil
			return

		case rpkg := <-jobsCh:
			isImposter, err := r.detectImposter(rpkg)
			if err != nil {
				stopCh <- struct{}{}
				errCh <- err
				return
			}

			if isImposter {
				// Signal that this package can be pruned.
				resultsCh <- rpkg
			}

		default:
			// No more jobs to process.
			errCh <- nil
			return
		}
	}
}

// pruneImposters identifies and deletes imposters contained by the resolver.
// This function should be called repeatedly until no more imposters are
// identified.  It returns true if any imposters were found and deleted.
func (r *Resolver) pruneImposters() (bool, error) {
	jobsCh := make(chan *ResolvePackage, len(r.pkgMap))
	defer close(jobsCh)

	stopCh := make(chan struct{}, newtutil.NewtNumJobs)
	defer close(stopCh)

	resultsCh := make(chan *ResolvePackage, len(r.pkgMap))
	defer close(resultsCh)

	errCh := make(chan error, newtutil.NewtNumJobs)
	defer close(errCh)

	seedMap := map[*pkg.LocalPackage]struct{}{}
	for _, lpkg := range r.seedPkgs {
		seedMap[lpkg] = struct{}{}
	}

	// Enqueue all packages to the jobs channel.
	for _, rpkg := range r.pkgMap {
		if _, ok := seedMap[rpkg.Lpkg]; !ok {
			jobsCh <- rpkg
		}
	}

	// Iterate through all packages with a collection of go routines.
	for i := 0; i < newtutil.NewtNumJobs; i++ {
		go r.detectImpostersWorker(jobsCh, stopCh, resultsCh, errCh)
	}

	// Collect errors from each routine.  Abort on first error.
	for i := 0; i < newtutil.NewtNumJobs; i++ {
		if err := <-errCh; err != nil {
			return false, err
		}
	}

	// Delete all imposter packages.
	anyPruned := false
	for {
		select {
		case rpkg := <-resultsCh:
			// This package may have already been deleted indirectly via a
			// prior delete.  If it has no more reverse dependencies, it is
			// already invalid.
			if len(rpkg.revDeps) > 0 {
				if err := r.deletePkg(rpkg); err != nil {
					return false, err
				}
				anyPruned = true
			}

		default:
			return anyPruned, nil
		}
	}
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
			panic(fmt.Sprintf("Resolver lacks mapping for seed package %s (%p)", lpkg.FullName(), lpkg))
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

	// Prune orphan packages.
	anyPruned, err := r.pruneOrphans()
	if err != nil {
		return false, err
	}
	if anyPruned {
		return true, nil
	}

	// Prune imposter packages.
	anyPruned, err = r.pruneImposters()
	if err != nil {
		return false, err
	}
	if anyPruned {
		return true, nil
	}

	return false, nil
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
	if err := r.calcApiReqs(); err != nil {
		return nil, err
	}

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
	if err := r.calcApiReqs(); err != nil {
		return err
	}

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
				rpkg.AddApiDep(api.rpkg, apiString, reqApi.exprs.Exprs())
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

	for _, slice := range unsatisfied {
		SortResolvePkgs(slice)
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
	res.ApiMap, res.UnsatisfiedApis = r.apiResolution()

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
	// 1. The set of loader packages, and
	// 2. The set of app packages.
	//
	// These need to be resolved separately so that it is possible later to
	// determine which packages need to be shared between loader and app.

	// It is OK if the app requires an API that is supplied by the loader.
	// Ensure each set of packages has access to the API-providers.
	for _, rpkg := range res.ApiMap {
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
	if err := res.LoaderSet.useMasterPkgs(); err != nil {
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
	if err := res.AppSet.useMasterPkgs(); err != nil {
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
