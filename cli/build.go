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
	"os"
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

func PkgIncludeDirs(pkg *Package, t *Target, srcDir string) []string {
	incls := pkg.Includes
	incls = append(incls, srcDir)
	incls = append(incls, srcDir+"/arch/"+t.Arch)

	if t.HasIdentity("test") {
		testSrcDir := srcDir + "/test"
		incls = append(incls, testSrcDir)
		incls = append(incls, testSrcDir+"/arch/"+t.Arch)
	}

	return incls
}

// Recursively compiles all the .c and .s files in the specified directory.
// Architecture-specific files are also compiled.
func BuildDir(srcDir string, c *Compiler, t *Target, ignDirs []string) error {

	var err error

	log.Printf("[DEBUG] compiling src in base directory: %s", srcDir)

	// First change into the package src directory, and build all the objects
	// there
	os.Chdir(srcDir)

	// Don't recurse into destination directories.
	ignDirs = append(ignDirs, "obj")
	ignDirs = append(ignDirs, "bin")

	// Ignore architecture-specific source files for now.  Use a temporary
	// string array here so that the "arch" director is not ignored in the
	// subsequent architecture-specific compile phase.
	baseIgnDirs := append(ignDirs, "arch")

	if err = c.RecursiveCompile("*.c", 0, baseIgnDirs); err != nil {
		return err
	}

	log.Printf("[DEBUG] compiling architecture specific src packages")

	archDir := srcDir + "/arch/" + t.Arch + "/"
	if NodeExist(archDir) {
		if err := os.Chdir(archDir); err != nil {
			return NewStackError(err.Error())
		}
		if err := c.RecursiveCompile("*.c", 0, ignDirs); err != nil {
			return err
		}

		// compile assembly sources in recursive compile as well
		if err = c.RecursiveCompile("*.s", 1, ignDirs); err != nil {
			return err
		}
	}

	return nil
}
