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

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/target"
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

func (b *Builder) HasPackage(pkg *pkg.LocalPackage) bool {
	_, ok := b.Packages[pkg]
	if !ok {
		return false
	} else {
		return true
	}
}

func (b *Builder) AddPackage(pkg *pkg.LocalPackage) {
	// Don't allow nil entries to the map
	if pkg == nil {
		return
	}

	b.Packages[pkg] = NewBuildPackage(pkg)
}

func (b *Builder) ResolvePackageDeps() error {
	reprocess := false
	for {
		if !reprocess {
			break
		}

		reprocess = false
		for _, bpkg := range b.Packages {
			loaded, err := bpkg.Load(b)
			if err != nil {
				return err
			}

			if !loaded {
				reprocess = true
			}
		}
	}

	return nil
}

func (b *Builder) Build() error {
	b.AddPackage(b.target.Bsp())
	b.AddPackage(b.target.App())
	b.AddPackage(b.target.Compiler())

	b.ResolvePackageDeps()

	for _, bpkg := range b.Packages {
		fmt.Printf("Package %s being built\n", bpkg.Name())
	}

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
