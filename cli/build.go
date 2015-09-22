/*
 Copyright 2015 Runtime Inc.
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
		return "", NewNewtError("Expected a BSP")
	}

	bspPackage, err := pm.ResolvePkgName(t.Bsp)
	if err != nil {
		return "", NewNewtError("No BSP package for " + t.Bsp + " exists")
	}

	// Add the BSP include directory to the global list of include paths.
	// All subsequent code that gets built can include BSP header files.
	*incls = append(*incls, bspPackage.BasePath+"/include/")

	if err = pm.Build(t, t.Bsp, *incls, libs); err != nil {
		return "", err
	}

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
func CreateCFlags(pm *PkgMgr, c *Compiler, t *Target, p *Package) string {
	cflags := c.Cflags + " " + p.Cflags + " " + t.Cflags

	// The 'test' identity causes the TEST symbol to be defined.  This allows
	// package code to behave differently in test builds.
	if t.HasIdentity("test") {
		cflags += " -DTEST"
	}

	cflags += " -DARCH=" + t.Arch

	// If a non-BSP package is being built, add the BSP's C flags to the list.
	// The BSP's compiler flags get exported to all packages.
	bspPackage, err := pm.ResolvePkgName(t.Bsp)
	if err == nil && bspPackage != p {
		cflags += " " + bspPackage.Cflags
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
			return NewNewtError(err.Error())
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
