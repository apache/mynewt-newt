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
)

type CompilerInfo struct {
	Includes []string
	Cflags   []string
	Lflags   []string
	Aflags   []string
}

type BuildPackage struct {
	*pkg.LocalPackage

	compilerInfo *CompilerInfo
	loaded       bool
}

func (bpkg *BuildPackage) LoadIdentities(b *Builder) (map[string]bool, bool) {
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

func (bpkg *BuildPackage) LoadDeps(b *Builder, idents map[string]bool) (bool, error) {
	proj := project.GetProject()

	foundNewDep := false

	newDeps := cli.GetStringSliceIdentities(bpkg.Viper, idents, "pkg.deps")
	for _, newDepStr := range newDeps {
		newDep, err := pkg.NewDependency(newDepStr)
		if err != nil {
			return false, err
		}

		pkg, err := proj.ResolveDependency(newDep)
		if err != nil {
			return false, err
		}

		if !b.HasPackage(pkg) {
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
	// Circularly resolve dependencies and identities until no more new
	// dependencies or identities exist.
	idents, newIdents := bpkg.LoadIdentities(b)
	newDeps, err := bpkg.LoadDeps(b, idents)
	if err != nil {
		return false, err
	}

	if newIdents || newDeps {
		return false, nil
	}

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
