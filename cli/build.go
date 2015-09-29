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

// Recursively iterates through a package's dependencies, adding each package
// encountered to the supplied set.
func collectDepsAux(em *EggMgr, egg *Egg, set *map[*Egg]bool) error {
	if (*set)[egg] {
		return nil
	}

	(*set)[egg] = true

	for _, dep := range egg.Deps {
		if dep.Name == "" {
			break
		}

		// Get package structure
		degg, err := em.ResolveEggName(dep.Name)
		if err != nil {
			return err
		}

		collectDepsAux(em, degg, set)
	}

	return nil
}

// Recursively iterates through a package's dependencies.  The resulting array
// contains a pointer to each encountered package.
func collectDeps(em *EggMgr, egg *Egg) ([]*Egg, error) {
	set := map[*Egg]bool{}

	err := collectDepsAux(em, egg, &set)
	if err != nil {
		return nil, err
	}

	arr := []*Egg{}
	for p, _ := range set {
		arr = append(arr, p)
	}

	return arr, nil
}

// Calculates the include paths exported by the specified package and all of
// its recursive dependencies.
func recursiveIncludePaths(em *EggMgr, egg *Egg,
	t *Target) ([]string, error) {

	deps, err := collectDeps(em, egg)
	if err != nil {
		return nil, err
	}

	incls := []string{}
	for _, p := range deps {
		eggIncls, err := p.GetIncludes(t)
		if err != nil {
			return nil, err
		}
		incls = append(incls, eggIncls...)
	}

	return incls, nil
}

// Calculates the include paths exported by the specified target's BSP and all
// of its recursive dependencies.
func BspIncludePaths(em *EggMgr, t *Target) ([]string, error) {
	if t.Bsp == "" {
		return nil, NewNewtError("Expected a BSP")
	}

	bspEgg, err := em.ResolveEggName(t.Bsp)
	if err != nil {
		return nil, NewNewtError("No BSP package for " + t.Bsp + " exists")
	}

	return recursiveIncludePaths(em, bspEgg, t)
}

func buildBsp(t *Target, em *EggMgr, incls *[]string,
	libs *[]string) (string, error) {

	if t.Bsp == "" {
		return "", NewNewtError("Expected a BSP")
	}

	bspEgg, err := em.ResolveEggName(t.Bsp)
	if err != nil {
		return "", NewNewtError("No BSP package for " + t.Bsp + " exists")
	}

	if err = em.Build(t, t.Bsp, *incls, libs); err != nil {
		return "", err
	}

	// A BSP doesn't have to contain source; don't fail if no library was
	// built.
	if lib := em.GetEggLib(t, bspEgg); NodeExist(lib) {
		*libs = append(*libs, lib)
	}

	var linkerScript string
	if bspEgg.LinkerScript != "" {
		linkerScript = bspEgg.BasePath + "/" + bspEgg.LinkerScript
	} else {
		linkerScript = ""
	}

	return linkerScript, nil
}

// Creates the set of compiler flags that should be specified when building a
// particular target-entity pair.  The "entity" is what is being built; either
// a package or a project.
func CreateCflags(em *EggMgr, c *Compiler, t *Target,
	entityCflags string) string {

	cflags := c.Cflags + " " + entityCflags + " " + t.Cflags

	// The 'test' identity causes the TEST symbol to be defined.  This allows
	// package code to behave differently in test builds.
	if t.HasIdentity("test") {
		cflags += " -DTEST"
	}

	cflags += " -DARCH=" + t.Arch

	// If a non-BSP package is being built, add the BSP's C flags to the list.
	// The BSP's compiler flags get exported to all packages.
	bspEgg, err := em.ResolveEggName(t.Bsp)
	if err == nil && bspEgg.Cflags != entityCflags {
		cflags += " " + bspEgg.Cflags
	}

	return cflags
}

func EggIncludeDirs(egg *Egg, t *Target) []string {
	srcDir := egg.BasePath + "/src/"

	incls := egg.Includes
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
