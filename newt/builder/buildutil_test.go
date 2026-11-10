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
	"slices"
	"testing"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/resolve"
)

// newTestBuilder creates a builder with packages depending on each other as
// specified by the map: package name => names of its dependencies.
func newTestBuilder(deps map[string][]string) *Builder {
	b := &Builder{PkgMap: map[*resolve.ResolvePackage]*BuildPackage{}}

	rpkgs := map[string]*resolve.ResolvePackage{}
	for name := range deps {
		lpkg := pkg.NewLocalPackage(nil, name)
		lpkg.SetName(name)
		lpkg.SetType(pkg.PACKAGE_TYPE_LIB)
		rpkg := &resolve.ResolvePackage{
			Lpkg: lpkg,
			Deps: map[*resolve.ResolvePackage]*resolve.ResolveDep{},
		}
		rpkgs[name] = rpkg
		b.PkgMap[rpkg] = &BuildPackage{rpkg: rpkg}
	}

	for name, depNames := range deps {
		for _, d := range depNames {
			rpkgs[name].Deps[rpkgs[d]] = &resolve.ResolveDep{Rpkg: rpkgs[d]}
		}
	}

	return b
}

func bpkgNames(bpkgs []*BuildPackage) []string {
	names := []string{}
	for _, bpkg := range bpkgs {
		names = append(names, bpkg.rpkg.Lpkg.Name())
	}
	return names
}

func (b *Builder) testBpkg(name string) *BuildPackage {
	for _, bpkg := range b.PkgMap {
		if bpkg.rpkg.Lpkg.Name() == name {
			return bpkg
		}
	}
	return nil
}

// Each package follows its dependencies; otherwise packages are in
// alphabetical order, also in dependency cycles.
func TestDepSortedBuildPackages(t *testing.T) {
	b := newTestBuilder(map[string][]string{
		"app":   {"net", "os"},
		"bsp":   {"mcu"},
		"mcu":   {"os"},
		"net":   {"os", "util"},
		"os":    {},
		"util":  {},
		"cyc_a": {"cyc_b"},
		"cyc_b": {"cyc_a"},
	})

	exp := []string{"os", "util", "net", "app", "mcu", "bsp", "cyc_b",
		"cyc_a"}
	got := bpkgNames(b.depSortedBuildPackages())
	if !slices.Equal(got, exp) {
		t.Errorf("unexpected order:\nexpected %q\ngot      %q", exp, got)
	}

	tests := []struct {
		pkg string
		dep string
		exp bool
	}{
		{"app", "util", true},
		{"app", "net", true},
		{"util", "app", false},
		{"bsp", "net", false},
		{"os", "os", false},
		{"cyc_a", "cyc_b", true},
		{"cyc_b", "cyc_a", true},
		{"cyc_a", "os", false},
	}
	for _, tt := range tests {
		got := b.dependsOn(b.testBpkg(tt.pkg), b.testBpkg(tt.dep))
		if got != tt.exp {
			t.Errorf("dependsOn(%s, %s) = %v, expected %v", tt.pkg, tt.dep,
				got, tt.exp)
		}
	}
}
