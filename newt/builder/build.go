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
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

type Builder struct {
	Packages   map[*pkg.LocalPackage]*BuildPackage
	identities map[string]bool

	target *target.Target
}

func (b *Builder) Identities() map[string]bool {
	return b.identities
}

func (b *Builder) AddIdentity(identity string) {
	b.identities[identity] = true
}

func (b *Builder) GetPackage(pkg *pkg.LocalPackage) (*BuildPackage, bool) {
	if pkg == nil {
		panic("Package should not equal NIL")
	}

	bpkg, ok := b.Packages[pkg]
	if !ok {
		return nil, false
	} else {
		return bpkg, true
	}
}

func (b *Builder) AddPackage(pkg *pkg.LocalPackage) {
	// Don't allow nil entries to the map
	if pkg == nil {
		panic("Cannot add nil package builder map")
	}

	b.Packages[pkg] = NewBuildPackage(pkg)
}

func (b *Builder) LoadDeps() error {
	for {
		reprocess := false
		for _, bpkg := range b.Packages {
			loaded, err := bpkg.Load(b)
			if err != nil {
				return err
			}

			if !loaded {
				reprocess = true
			}
		}

		if !reprocess {
			break
		}
	}

	return nil
}

func (b *Builder) Build() error {
	b.AddPackage(b.target.Bsp())
	b.AddPackage(b.target.App())
	b.AddPackage(b.target.Compiler())

	if err := b.LoadDeps(); err != nil {
		return err
	}

	bspPackage, ok := b.GetPackage(b.target.Bsp())
	if !ok {
		return util.NewNewtError("BSP package not found!")
	}

	baseCi := NewCompilerInfo()
	baseCi.AddCompilerInfo(bspPackage.PackageCompilerInfo())

	// Loop through all packages and build them

	// Now, get the App, and link the application

	return nil
}

func (b *Builder) Init(target *target.Target) error {
	b.target = target

	b.Packages = map[*pkg.LocalPackage]*BuildPackage{}
	b.identities = map[string]bool{}

	return nil
}

func NewBuilder(target *target.Target) (*Builder, error) {
	b := &Builder{}

	if err := b.Init(target); err != nil {
		return nil, err
	}

	return b, nil
}
