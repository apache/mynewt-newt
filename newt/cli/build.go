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

package cli

import (
	"os"
	"sort"
	"strings"
)

// Recursively iterates through an pkg's dependencies, adding each pkg
// encountered to the supplied set.
func collectDepsAux(pkgList *PkgList, pkg *Pkg, set *map[*Pkg]bool) error {
	if (*set)[pkg] {
		return nil
	}

	(*set)[pkg] = true

	for _, dep := range pkg.Deps {
		if dep.Name == "" {
			break
		}

		// Get pkg structure
		dpkg, err := pkgList.ResolvePkgName(dep.Name)
		if err != nil {
			return err
		}

		collectDepsAux(pkgList, dpkg, set)
	}

	return nil
}

// Recursively iterates through an pkg's dependencies.  The resulting array
// contains a pointer to each encountered pkg.
func collectDeps(pkgList *PkgList, pkg *Pkg) ([]*Pkg, error) {
	set := map[*Pkg]bool{}

	err := collectDepsAux(pkgList, pkg, &set)
	if err != nil {
		return nil, err
	}

	arr := []*Pkg{}
	for p, _ := range set {
		arr = append(arr, p)
	}

	return arr, nil
}

// Calculates the include paths exported by the specified pkg and all of
// its recursive dependencies.
func recursiveIncludePaths(pkgList *PkgList, pkg *Pkg,
	t *Target) ([]string, error) {

	deps, err := collectDeps(pkgList, pkg)
	if err != nil {
		return nil, err
	}

	incls := []string{}
	for _, p := range deps {
		pkgIncls, err := p.GetIncludes(t)
		if err != nil {
			return nil, err
		}
		incls = append(incls, pkgIncls...)
	}

	return incls, nil
}

// Calculates the include paths exported by the specified target's BSP and all
// of its recursive dependencies.
func BspIncludePaths(pkgList *PkgList, t *Target) ([]string, error) {
	if t.Bsp == "" {
		return nil, NewNewtError("Expected a BSP")
	}

	bspPkg, err := pkgList.ResolvePkgName(t.Bsp)
	if err != nil {
		return nil, NewNewtError("No BSP pkg for " + t.Bsp + " exists")
	}

	return recursiveIncludePaths(pkgList, bspPkg, t)
}

func buildBsp(t *Target, pkgList *PkgList, incls *[]string,
	libs *[]string, capPkgs map[string]string) (string, error) {

	if t.Bsp == "" {
		return "", NewNewtError("Expected a BSP")
	}

	bspPkg, err := pkgList.ResolvePkgName(t.Bsp)
	if err != nil {
		return "", NewNewtError("No BSP pkg for " + t.Bsp + " exists")
	}

	if err = pkgList.Build(t, t.Bsp, *incls, libs); err != nil {
		return "", err
	}

	// A BSP doesn't have to contain source; don't fail if no library was
	// built.
	if lib := pkgList.GetPkgLib(t, bspPkg); NodeExist(lib) {
		*libs = append(*libs, lib)
	}

	var linkerScript string
	if bspPkg.LinkerScript != "" {
		linkerScript = bspPkg.BasePath + "/" + bspPkg.LinkerScript
	} else {
		linkerScript = ""
	}

	return linkerScript, nil
}

// Creates the set of compiler flags that should be specified when building a
// particular target-entity pair.  The "entity" is what is being built; either
// an pkg or a project.
func CreateCflags(pkgList *PkgList, c *Compiler, t *Target,
	entityCflags string) string {

	cflagsSlice := []string{}
	cflagsSlice = append(cflagsSlice, strings.Fields(c.Cflags)...)
	cflagsSlice = append(cflagsSlice, strings.Fields(entityCflags)...)
	cflagsSlice = append(cflagsSlice, strings.Fields(t.Cflags)...)

	// The 'test' identity causes the TEST symbol to be defined.  This allows
	// pkg code to behave differently in test builds.
	if t.HasIdentity("test") {
		cflagsSlice = append(cflagsSlice, "-DTEST")
	}

	cflagsSlice = append(cflagsSlice, "-DARCH_"+t.Arch)

	// If a non-BSP pkg is being built, add the BSP's C flags to the list.
	// The BSP's compiler flags get exported to all pkgs.
	bspPkg, err := pkgList.ResolvePkgName(t.Bsp)
	if err == nil && bspPkg.Cflags != entityCflags {
		cflagsSlice = append(cflagsSlice, strings.Fields(bspPkg.Cflags)...)
	}

	cflagsSlice = UniqueStrings(cflagsSlice)
	sort.Strings(cflagsSlice)
	return strings.Join(cflagsSlice, " ")
}

func CreateLflags(c *Compiler, t *Target, entityLflags string) string {
	fields := SortFields(c.Lflags, entityLflags, t.Lflags)
	return strings.Join(fields, " ")
}

func CreateAflags(c *Compiler, t *Target, entityAflags string) string {
	fields := SortFields(c.Aflags, entityAflags, t.Aflags)
	return strings.Join(fields, " ")
}

func PkgIncludeDirs(pkg *Pkg, t *Target) []string {
	srcDir := pkg.BasePath + "/src/"

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

	StatusMessage(VERBOSITY_VERBOSE, "compiling src in base directory: %s\n",
		srcDir)

	// First change into the pkg src directory, and build all the objects
	// there
	os.Chdir(srcDir)

	// Don't recurse into destination directories.
	ignDirs = append(ignDirs, "obj")
	ignDirs = append(ignDirs, "bin")

	// Ignore architecture-specific source files for now.  Use a temporary
	// string array here so that the "arch" directory is not ignored in the
	// subsequent architecture-specific compile phase.
	baseIgnDirs := append(ignDirs, "arch")

	if err = c.RecursiveCompile("*.c", 0, baseIgnDirs); err != nil {
		return err
	}

	archDir := srcDir + "/arch/" + t.Arch + "/"
	StatusMessage(VERBOSITY_VERBOSE,
		"compiling architecture specific src pkgs in directory: %s\n",
		archDir)

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
