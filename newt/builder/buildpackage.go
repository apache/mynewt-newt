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

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type BuildPackage struct {
	*pkg.LocalPackage

	fullCi *toolchain.CompilerInfo

	isBsp bool

	loaded bool
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
		dpkg, err := project.GetProject().ResolveDependency(dep)
		if err != nil {
			return err
		}

		dbpkg := b.Packages[dpkg]
		if dbpkg == nil {
			return util.NewNewtError(fmt.Sprintf("Package not found (%s)",
				dpkg.Name()))
		}

		err = dbpkg.collectDepsAux(b, set)
		if err != nil {
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

func (bpkg *BuildPackage) FullCompilerInfo(b *Builder) (*toolchain.CompilerInfo, error) {
	if !bpkg.loaded {
		return nil, util.NewNewtError("Package must be loaded before Compiler info is fetched")
	}

	if bpkg.fullCi != nil {
		return bpkg.fullCi, nil
	}

	ci := toolchain.NewCompilerInfo()
	ci.Cflags = cli.GetStringSliceIdentities(bpkg.Viper, b.Identities(),
		"pkg.cflags")
	ci.Lflags = cli.GetStringSliceIdentities(bpkg.Viper, b.Identities(),
		"pkg.lflags")
	ci.Aflags = cli.GetStringSliceIdentities(bpkg.Viper, b.Identities(),
		"pkg.aflags")

	includePaths, err := bpkg.recursiveIncludePaths(b)
	if err != nil {
		return nil, err
	}
	ci.Includes = append(bpkg.privateIncludeDirs(b), includePaths...)
	bpkg.fullCi = ci

	return bpkg.fullCi, nil
}

func (bpkg *BuildPackage) loadIdentities(b *Builder) (map[string]bool, bool) {
	idents := b.Identities()

	foundNewIdent := false

	newIdents := cli.GetStringSliceIdentities(bpkg.Viper, idents,
		"pkg.identities")
	for _, nident := range newIdents {
		_, ok := idents[nident]
		if !ok {
			b.AddIdentity(nident)
			foundNewIdent = true
		}
	}

	if foundNewIdent {
		return b.Identities(), true
	} else {
		return idents, false
	}
}

func (bpkg *BuildPackage) loadDeps(b *Builder,
	idents map[string]bool) (bool, error) {

	proj := project.GetProject()

	foundNewDep := false

	newDeps := cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.deps")
	for _, newDepStr := range newDeps {
		newDep, err := pkg.NewDependency(bpkg.Repo(), newDepStr)
		if err != nil {
			return false, err
		}

		pkg, err := proj.ResolveDependency(newDep)
		if err != nil {
			return false, err
		}

		if pkg == nil {
			return false,
				util.NewNewtError("Could not resolve package dependency " +
					newDep.String())
		}

		if b.Packages[pkg] == nil {
			foundNewDep = true
			b.AddPackage(pkg)
		}

		if !bpkg.HasDep(newDep) {
			foundNewDep = true
			bpkg.AddDep(newDep)
		}
	}

	return foundNewDep, nil
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

	if cli.CheckBoolMap(b.Identities(), "test") {
		testSrcDir := srcDir + "/test"
		incls = append(incls, testSrcDir)
		incls = append(incls, testSrcDir+"/arch/"+b.target.Arch)
	}

	return incls
}

func (bpkg *BuildPackage) Load(b *Builder) (bool, error) {
	if bpkg.loaded {
		return true, nil
	}

	// Circularly resolve dependencies and identities until no more new
	// dependencies or identities exist.
	idents, newIdents := bpkg.loadIdentities(b)
	newDeps, err := bpkg.loadDeps(b, idents)
	if err != nil {
		return false, err
	}

	if newIdents || newDeps {
		return false, nil
	}

	// Now, load the rest of the package, this should happen only once.
	apis := cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.caps")
	for _, api := range apis {
		bpkg.AddApi(api)
	}

	reqApis := cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.req_caps")
	for _, reqApi := range reqApis {
		bpkg.AddReqApi(reqApi)
	}

	bpkg.loaded = true

	return true, nil
}

func (bp *BuildPackage) Init(pkg *pkg.LocalPackage) {
	bp.LocalPackage = pkg
}

func NewBuildPackage(pkg *pkg.LocalPackage) *BuildPackage {
	bpkg := &BuildPackage{}
	bpkg.Init(pkg)

	return bpkg
}
