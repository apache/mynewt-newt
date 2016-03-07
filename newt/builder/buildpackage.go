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
	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

type CompilerInfo struct {
	Includes []string
	Cflags   []string
	Lflags   []string
	Aflags   []string
}

type BuildPackage struct {
	*pkg.LocalPackage

	pkgCi  *CompilerInfo
	fullCi *CompilerInfo

	isBsp bool

	loaded bool
}

func (ci *CompilerInfo) AddCompilerInfo(newCi *CompilerInfo) {
	ci.Includes = append(ci.Includes, newCi.Includes...)
	ci.Cflags = append(ci.Cflags, newCi.Cflags...)
	ci.Lflags = append(ci.Lflags, newCi.Lflags...)
	ci.Aflags = append(ci.Aflags, newCi.Aflags...)
}

func NewCompilerInfo() *CompilerInfo {
	ci := &CompilerInfo{}
	ci.Includes = []string{}
	ci.Cflags = []string{}
	ci.Lflags = []string{}
	ci.Aflags = []string{}

	return ci
}

func (bpkg *BuildPackage) PackageCompilerInfo() *CompilerInfo {
	return bpkg.pkgCi
}

func (bpkg *BuildPackage) FullCompilerInfo(b *Builder) (*CompilerInfo, error) {
	if !bpkg.loaded {
		return nil, util.NewNewtError("Package must be loaded before Compiler info is fetched")
	}

	if bpkg.fullCi != nil {
		return bpkg.fullCi, nil
	}

	ci := NewCompilerInfo()
	ci.AddCompilerInfo(bpkg.pkgCi)

	// Go through every dependency, and add the compiler information for that
	// dependency
	for _, dep := range bpkg.Deps() {
		pkg, err := project.GetProject().ResolveDependency(dep)
		if err != nil {
			return nil, err
		}

		if pkg == nil {
			return nil, util.NewNewtError("Cannot resolve dep " + dep.String())
		}

		bpkg, ok := b.GetPackage(pkg)
		if !ok {
			return nil, util.NewNewtError("Unknown build info for package " + pkg.Name())
		}

		ci.AddCompilerInfo(bpkg.PackageCompilerInfo())
	}

	bpkg.fullCi = ci

	return bpkg.fullCi, nil
}

func (bpkg *BuildPackage) loadIdentities(b *Builder) (map[string]bool, bool) {
	idents := b.Identities()

	foundNewIdent := false

	newIdents := cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.identities")
	for _, nident := range newIdents {
		_, ok := idents[nident]
		if !ok {
			b.AddIdentity(nident)
			foundNewIdent = true
		}
	}

	if foundNewIdent {
		return b.Identities(), foundNewIdent
	} else {
		return idents, foundNewIdent
	}
}

func (bpkg *BuildPackage) loadDeps(b *Builder, idents map[string]bool) (bool, error) {
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
			return false, util.NewNewtError("Could not resolve package dependency " +
				newDep.String())
		}

		_, ok := b.GetPackage(pkg)
		if !ok {
			foundNewDep = true
			b.AddPackage(pkg)
		}

		if !bpkg.HasDep(newDep) {
			bpkg.AddDep(newDep)
		}
	}

	return foundNewDep, nil
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
	for _, apiStr := range apis {
		api, err := pkg.NewDependency(bpkg.Repo(), apiStr)
		if err != nil {
			return false, err
		}
		bpkg.AddApi(api)
	}

	reqApis := cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.req_caps")
	for _, apiStr := range reqApis {
		api, err := pkg.NewDependency(bpkg.Repo(), apiStr)
		if err != nil {
			return false, err
		}
		bpkg.AddReqApi(api)
	}

	ci := NewCompilerInfo()
	ci.Cflags = cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.cflags")
	ci.Lflags = cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.lflags")
	ci.Aflags = cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.aflags")
	ci.Includes = cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.includes")

	bpkg.pkgCi = ci

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
