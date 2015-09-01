/*
 Copyright 2015 Stack Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cli

import (
	"log"
)

func buildBsp(t *Target, pm *PkgMgr, incls *[]string,
	libs *[]string) (string, error) {

	if t.Bsp == "" {
		return "", NewStackError("Expected a BSP")
	}

	bspPackage, err := pm.ResolvePkgName(t.Bsp)
	if err != nil {
		return "", NewStackError("No BSP package for " + t.Bsp + " exists")
	}

	if err = pm.Build(t, t.Bsp, nil, libs); err != nil {
		return "", err
	}

	*incls = append(*incls, bspPackage.Includes...)

	// A BSP doesn't have to contain source; don't fail if no library was
	// built.
	if lib := pm.GetPackageLib(t, bspPackage); NodeExist(lib) {
		*libs = append(*libs, lib)
	}

	var linkerScript string
	log.Printf("[INFO] bspPackage.LinkerScript=>%s<", bspPackage.LinkerScript)
	if bspPackage.LinkerScript != "" {
		linkerScript = bspPackage.BasePath + "/" + bspPackage.LinkerScript
	} else {
		linkerScript = ""
	}

	return linkerScript, nil
}

// Creates the set of compiler flags that should be specified when building a
// particular target-package pair.
func CreateCFlags(c *Compiler, t *Target, p *Package) string {
	cflags := c.Cflags + " " + p.Cflags + " " + t.Cflags

	// The 'test' identity causes the TEST symbol to be defined.  This allows
	// package code to behave differently in test builds.
	if t.HasIdentity("test") {
		cflags += " -DTEST"
	}

	return cflags
}
