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

package sysdown

import (
	"testing"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/syscfg"
)

func sysdownPkg(t *testing.T, dir string, downs map[string]string) *pkg.LocalPackage {
	t.Helper()

	m := map[interface{}]interface{}{}
	for name, stg := range downs {
		m[name] = stg
	}

	lpkg := pkg.NewLocalPackage(nil, dir)
	if err := lpkg.PkgY.Replace("pkg.down", m); err != nil {
		t.Fatal(err)
	}

	return lpkg
}

// Two packages defining the same sysdown function are reported as a
// conflict.
func TestSysdownConflict(t *testing.T) {
	cfg := syscfg.NewCfg()
	lpkgs := []*pkg.LocalPackage{
		sysdownPkg(t, "a", map[string]string{"foo_down": "100"}),
		sysdownPkg(t, "b", map[string]string{"foo_down": "200"}),
	}

	scfg := Read(lpkgs, &cfg)

	if len(scfg.Conflicts["foo_down"]) != 2 {
		t.Errorf("expected conflict for foo_down, got %v", scfg.Conflicts)
	}
}

// Invalid sysdown entries are reported and not added as stage functions.
func TestSysdownInvalid(t *testing.T) {
	cfg := syscfg.NewCfg()
	lpkgs := []*pkg.LocalPackage{
		sysdownPkg(t, "a", map[string]string{
			"bad1_down": "xyz",
			"bad2_down": "abc",
			"ok_down":   "100",
		}),
	}

	scfg := Read(lpkgs, &cfg)

	if len(scfg.InvalidSettings) != 2 {
		t.Errorf("expected 2 invalid settings, got %v", scfg.InvalidSettings)
	}
	if len(scfg.Conflicts) != 0 {
		t.Errorf("unexpected conflicts: %v", scfg.Conflicts)
	}
	if len(scfg.StageFuncs) != 1 || scfg.StageFuncs[0].Name != "ok_down" {
		t.Errorf("expected only ok_down stage function, got %v",
			scfg.StageFuncs)
	}
	for _, sf := range scfg.StageFuncs {
		if sf.Pkg == nil {
			t.Errorf("stage function %s without package", sf.Name)
		}
	}
}
