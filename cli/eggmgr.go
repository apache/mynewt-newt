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
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type EggMgr struct {
	// Repository associated with the Eggs
	Repo *Repo

	Target *Target

	// List of packages for Repo
	Eggs map[string]*Egg
}

// Allocate a new package manager structure, and initialize it.
func NewEggMgr(r *Repo, t *Target) (*EggMgr, error) {
	em := &EggMgr{
		Repo:   r,
		Target: t,
	}
	err := em.Init()

	return em, err
}

func (em *EggMgr) CheckEggDeps(egg *Egg,
	deps map[string]*DependencyRequirement, reqcap map[string]*DependencyRequirement,
	caps map[string]*DependencyRequirement) error {
	// if no dependencies, then everything is ok!
	if egg.Deps == nil || len(egg.Deps) == 0 {
		return nil
	}

	for _, depReq := range egg.Deps {
		// don't process this package if we've already processed it
		if _, ok := deps[depReq.String()]; ok {
			continue
		}

		log.Printf("[DEBUG] Checking dependency %s for package %s", depReq, egg.Name)
		egg, ok := em.Eggs[depReq.Name]
		if !ok {
			return NewNewtError(fmt.Sprintf("No package dependency %s found for %s",
				depReq.Name, egg.Name))
		}

		if ok := depReq.SatisfiesDependency(egg); !ok {
			return NewNewtError(fmt.Sprintf("Egg %s doesn't satisfy dependency %s",
				egg.Name, depReq))
		}

		// We've checked this dependency requirement, all is gute!
		deps[depReq.String()] = depReq
	}

	// Now go through and recurse through the sub-package dependencies
	for _, depReq := range egg.Deps {
		if _, ok := deps[depReq.String()]; ok {
			continue
		}

		if err := em.CheckEggDeps(em.Eggs[depReq.Name], deps, reqcap, caps); err != nil {
			return err
		}
	}

	return nil
}

func (em *EggMgr) VerifyCaps(reqcaps map[string]*DependencyRequirement,
	caps map[string]*DependencyRequirement) error {

	for name, rcap := range reqcaps {
		capability, ok := caps[name]
		if !ok {
			return NewNewtError(fmt.Sprintf("Required capability %s not found", name))
		}

		if err := rcap.SatisfiesCapability(capability); err != nil {
			return err
		}
	}

	return nil
}

func (em *EggMgr) CheckDeps() error {
	// Go through all the packages and check that their dependencies are satisfied
	for _, egg := range em.Eggs {
		deps := map[string]*DependencyRequirement{}
		reqcap := map[string]*DependencyRequirement{}
		caps := map[string]*DependencyRequirement{}

		if err := em.CheckEggDeps(egg, deps, reqcap, caps); err != nil {
			return err
		}
	}

	return nil
}

// Load an individual package specified by eggName into the package list for
// this repository
func (em *EggMgr) loadEgg(eggDir string) error {
	log.Println("[INFO] Loading Egg " + eggDir + "...")

	if em.Eggs == nil {
		em.Eggs = make(map[string]*Egg)
	}

	egg, err := NewEgg(em.Repo, em.Target, eggDir)
	if err != nil {
		return nil
	}

	em.Eggs[egg.FullName] = egg

	return nil
}

func (em *EggMgr) String() string {
	str := ""
	for eggName, _ := range em.Eggs {
		str += eggName + " "
	}
	return str
}

// Recursively load a package.  Given the baseDir of the packages (e.g. egg/ or
// hw/bsp), and the base package name.
func (em *EggMgr) loadEggDir(baseDir string, eggPrefix string, eggName string) error {
	log.Printf("[DEBUG] Loading packages in %s, starting with package %s", baseDir,
		eggName)

	// first recurse and load subpackages
	list, err := ioutil.ReadDir(baseDir + "/" + eggName)
	if err != nil {
		return NewNewtError(err.Error())
	}

	for _, ent := range list {
		if !ent.IsDir() {
			continue
		}

		name := ent.Name()

		if name == "src" || name == "include" || strings.HasPrefix(name, ".") ||
			name == "bin" {
			continue
		} else {
			if err := em.loadEggDir(baseDir, eggPrefix, eggName+"/"+name); err != nil {
				return err
			}
		}
	}

	if NodeNotExist(baseDir + "/" + eggName + "/egg.yml") {
		return nil
	}

	return em.loadEgg(baseDir + "/" + eggName)
}

// Load all the packages in the repository into the package structure
func (em *EggMgr) loadEggs() error {
	r := em.Repo

	// Multiple package directories to be searched
	searchDirs := []string{"libs/", "hw/bsp/", "hw/mcu/", "hw/mcu/stm", "hw/drivers/", "hw/"}

	for _, eggDir := range searchDirs {
		eggBaseDir := r.BasePath + "/" + eggDir

		if NodeNotExist(eggBaseDir) {
			continue
		}

		eggList, err := ioutil.ReadDir(eggBaseDir)
		if err != nil {
			return NewNewtError(err.Error())
		}

		for _, subEggDir := range eggList {
			name := subEggDir.Name()
			if filepath.HasPrefix(name, ".") || filepath.HasPrefix(name, "..") {
				continue
			}

			if !subEggDir.IsDir() {
				continue
			}

			if err = em.loadEggDir(eggBaseDir, eggDir, name); err != nil {
				return err
			}
		}
	}

	return nil
}

// Initialize the package manager
func (em *EggMgr) Init() error {
	err := em.loadEggs()
	return err
}

// Resolve the package specified by eggName into a package structure.
func (em *EggMgr) ResolveEggName(eggName string) (*Egg, error) {
	egg, ok := em.Eggs[eggName]
	if !ok {
		return nil, NewNewtError(fmt.Sprintf("Invalid package %s specified (eggs = %s)",
			eggName, em))
	}
	return egg, nil
}

func (em *EggMgr) ResolveEggDir(eggDir string) (*Egg, error) {
	eggDir = filepath.Clean(eggDir)
	for name, egg := range em.Eggs {
		if filepath.Clean(egg.BasePath) == eggDir {
			return em.Eggs[name], nil
		}
	}
	return nil, NewNewtError(fmt.Sprintf("Cannot resolve package dir %s in package "+
		"manager", eggDir))
}

func (em *EggMgr) VerifyEgg(eggDir string) (*Egg, error) {
	err := em.loadEgg(eggDir)
	if err != nil {
		return nil, err
	}

	egg, err := em.ResolveEggDir(eggDir)
	if err != nil {
		return nil, err
	}

	if err := em.checkIncludes(egg); err != nil {
		return nil, err
	}

	return egg, nil
}

// Clean the build for the package specified by eggName.   if cleanAll is
// specified, all architectures are cleaned.
func (em *EggMgr) BuildClean(t *Target, eggName string, cleanAll bool) error {
	egg, err := em.ResolveEggName(eggName)
	if err != nil {
		return err
	}

	tName := t.Name + "/"
	if cleanAll {
		tName = ""
	}

	if egg.Clean {
		return nil
	}
	egg.Clean = true

	for _, dep := range egg.Deps {
		if err := em.BuildClean(t, dep.Name, cleanAll); err != nil {
			return err
		}
	}

	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, []string{})
	if err != nil {
		return err
	}

	if NodeExist(egg.BasePath + "/src/") {
		if err := c.RecursiveClean(egg.BasePath+"/src/", tName); err != nil {
			return err
		}

		if err := os.RemoveAll(egg.BasePath + "/bin/" + tName); err != nil {
			return NewNewtError(err.Error())
		}
	}

	egg.Clean = true

	return nil
}

func (em *EggMgr) GetEggLib(t *Target, egg *Egg) string {
	libDir := egg.BasePath + "/bin/" + t.Name + "/" +
		"lib" + filepath.Base(egg.Name) + ".a"
	return libDir
}

// @param incls                 Extra include paths that get specified during
//                                  build; not modified by this function.
// @param libs                  List of libraries that have been built so far;
//                                  This function appends entries to this list.
func (em *EggMgr) buildDeps(egg *Egg, t *Target, incls *[]string,
	libs *[]string) error {

	log.Printf("[DEBUG] Building package dependencies for %s, target %s",
		egg.Name, t.Name)

	var err error

	if egg.Includes, err = egg.GetIncludes(t); err != nil {
		return err
	}

	if incls == nil {
		incls = &[]string{}
	}
	if libs == nil {
		libs = &[]string{}
	}

	for _, dep := range egg.Deps {
		if dep.Name == "" {
			break
		}

		log.Printf("[DEBUG] Loading package dependency: %s", dep.Name)
		// Get package structure
		degg, err := em.ResolveEggName(dep.Name)
		if err != nil {
			return err
		}

		// Build the package
		if err = em.Build(t, dep.Name, *incls, libs); err != nil {
			return err
		}

		// After build, get dependency package includes.  Build function
		// generates all the package includes
		egg.Includes = append(egg.Includes, degg.Includes...)
		if lib := em.GetEggLib(t, degg); NodeExist(lib) {
			*libs = append(*libs, lib)
		}
	}

	// Add on dependency includes to package includes
	log.Printf("[DEBUG] Egg dependencies for %s built, incls = %s",
		egg.Name, egg.Includes)

	return nil
}

// Build the package specified by eggName
//
// @param incls                 Extra include paths that get specified during
//                                  build.  Note: passed by value.
// @param libs                  List of libraries that have been built so far;
//                                  This function appends entries to this list.
func (em *EggMgr) Build(t *Target, eggName string, incls []string,
	libs *[]string) error {

	// Look up package structure
	egg, err := em.ResolveEggName(eggName)
	if err != nil {
		return err
	}

	log.Println("[INFO] Building package " + eggName + " for arch " + t.Arch)

	// already built the package, no need to rebuild.  This is to handle
	// recursive calls to Build()
	if egg.Built {
		return nil
	}
	egg.Built = true

	if err := em.buildDeps(egg, t, &incls, libs); err != nil {
		return err
	}

	// NOTE: this assignment must happen after the call to buildDeps(), as
	// buildDeps() fills in the package includes.
	incls = append(incls, EggIncludeDirs(egg, t)...)
	log.Printf("[DEBUG] Egg includes for %s are %s", eggName, incls)

	srcDir := egg.BasePath + "/src/"
	if NodeNotExist(srcDir) {
		// nothing to compile, return true!
		return nil
	}

	// Build the package designated by eggName
	// Initialize a compiler
	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, incls)
	if err != nil {
		return err
	}
	// setup Cflags, Lflags and Aflags
	c.Cflags = CreateCflags(em, c, t, egg.Cflags)
	c.Lflags += " " + egg.Lflags + " " + t.Lflags
	c.Aflags += " " + egg.Aflags + " " + t.Aflags

	log.Printf("[DEBUG] compiling src packages in base package directories: %s",
		srcDir)

	// For now, ignore test code.  Tests get built later if the test identity
	// is in effect.
	ignDirs := []string{"test"}

	if err = BuildDir(srcDir, c, t, ignDirs); err != nil {
		return err
	}

	// Now build the test code if requested.
	if t.HasIdentity("test") {
		testSrcDir := srcDir + "/test"
		if err = BuildDir(testSrcDir, c, t, ignDirs); err != nil {
			return err
		}
	}

	// Archive everything into a static library, which can be linked with a
	// main program
	if err := os.Chdir(egg.BasePath + "/"); err != nil {
		return NewNewtError(err.Error())
	}

	binDir := egg.BasePath + "/bin/" + t.Name + "/"

	if NodeNotExist(binDir) {
		if err := os.MkdirAll(binDir, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	if err = c.CompileArchive(em.GetEggLib(t, egg), ""); err != nil {
		return err
	}

	return nil
}

// Check the include directories for the package, to make sure there are no conflicts in
// include paths for source code
func (em *EggMgr) checkIncludes(egg *Egg) error {
	incls, err := filepath.Glob(egg.BasePath + "/include/*")
	if err != nil {
		return NewNewtError(err.Error())
	}

	// Append all the architecture specific directories
	archDir := egg.BasePath + "/include/" + egg.Name + "/arch/"
	dirs, err := ioutil.ReadDir(archDir)
	if err != nil {
		return NewNewtError(err.Error())
	}

	for _, dir := range dirs {
		if !dir.IsDir() {
			return NewNewtError(fmt.Sprintf("Only directories are allowed in "+
				"architecture dir: %s", archDir+dir.Name()))
		}

		incls2, err := filepath.Glob(archDir + dir.Name() + "/*")
		if err != nil {
			return NewNewtError(err.Error())
		}

		incls = append(incls, incls2...)
	}

	for _, incl := range incls {
		finfo, err := os.Stat(incl)
		if err != nil {
			return NewNewtError(err.Error())
		}

		bad := false
		if !finfo.IsDir() {
			bad = true
		}

		if filepath.Base(incl) != egg.Name {
			if egg.IsBsp && filepath.Base(incl) != "bsp" {
				bad = true
			}
		}

		if bad {
			return NewNewtError(fmt.Sprintf("File %s should not exist in include "+
				"directory, only file allowed in include directory is a directory with "+
				"the package name %s",
				incl, egg.Name))
		}
	}

	return nil
}

// Clean the tests in the tests parameter, for the package identified by
// eggName.  If cleanAll is set to true, all architectures will be removed.
func (em *EggMgr) TestClean(t *Target, eggName string,
	cleanAll bool) error {
	egg, err := em.ResolveEggName(eggName)
	if err != nil {
		return err
	}

	tName := t.Name + "/"
	if cleanAll {
		tName = ""
	}

	if err := os.RemoveAll(egg.BasePath + "/src/test/bin/" + tName); err != nil {
		return NewNewtError(err.Error())
	}
	if err := os.RemoveAll(egg.BasePath + "/src/test/obj/" + tName); err != nil {
		return NewNewtError(err.Error())
	}

	return nil
}

// Compile tests specified by the tests parameter.  The tests are linked
// to the package specified by the egg parameter
func (em *EggMgr) linkTests(t *Target, egg *Egg,
	incls []string, libs *[]string) error {

	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, incls)
	if err != nil {
		return err
	}

	// Configure Lflags.  Since we are only linking, Cflags and Aflags are
	// unnecessary.
	c.Lflags += " " + egg.Lflags + " " + t.Lflags

	testBinDir := egg.BasePath + "/src/test/bin/" + t.Name + "/"
	if NodeNotExist(testBinDir) {
		if err := os.MkdirAll(testBinDir, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	if err := c.CompileBinary(testBinDir+egg.TestBinName(), map[string]bool{},
		strings.Join(*libs, " ")); err != nil {

		return err
	}

	return nil
}

// Run all the tests in the tests parameter.  egg is the package to check for
// the tests.  exitOnFailure specifies whether to exit immediately when a
// test fails, or continue executing all tests.
func (em *EggMgr) runTests(t *Target, egg *Egg, exitOnFailure bool) error {
	if err := os.Chdir(egg.BasePath + "/src/test/bin/" + t.Name +
		"/"); err != nil {
		return err
	}
	o, err := ShellCommand("./" + egg.TestBinName())
	if err != nil {
		log.Printf("[ERROR] Test %s failed, output: %s", egg.TestBinName(),
			string(o))
		if exitOnFailure {
			return NewNewtError("Unit tests failed to complete successfully.")
		}
	} else {
		log.Printf("[INFO] Test %s ok!", egg.TestBinName())
	}

	return nil
}

// Check to ensure tests exist.  Go through the array of tests specified by
// the tests parameter.  egg is the package to check for these tests.
func (em *EggMgr) testsExist(egg *Egg) error {
	dirName := egg.BasePath + "/src/test/"
	if NodeNotExist(dirName) {
		return NewNewtError("No test exists for package " + egg.Name)
	}

	return nil
}

// Test the package identified by eggName, by executing the tests specified.
// exitOnFailure signifies whether to stop the test program when one of them
// fails.
func (em *EggMgr) Test(t *Target, eggName string, exitOnFailure bool) error {
	log.Printf("[INFO] Testing package %s for arch %s", eggName, t.Arch)

	egg, err := em.ResolveEggName(eggName)
	if err != nil {
		return err
	}

	// Make sure the test directories exist
	if err := em.testsExist(egg); err != nil {
		return err
	}

	incls := []string{}
	libs := []string{}

	// The 'test' identity is implicitly exported during a package test.
	t.Identities = append(t.Identities, "test")

	// If there is a BSP:
	//     1. Calculate the include paths that it and its dependencies export.
	//        This set of include paths is accessible during all subsequent
	//        builds.
	//     2. Build the BSP package.
	if t.Bsp != "" {
		incls, err = BspIncludePaths(em, t)
		if err != nil {
			return err
		}
		_, err = buildBsp(t, em, &incls, &libs)
		if err != nil {
			return err
		}
	}

	// Build the package under test.  This must be compiled with the PKG_TEST
	// symbol defined so that the appropriate main function gets built.
	egg.Cflags += " -DPKG_TEST"
	if err := em.Build(t, eggName, incls, &libs); err != nil {
		return err
	}
	lib := em.GetEggLib(t, egg)
	if !NodeExist(lib) {
		return NewNewtError("Egg " + eggName + " did not produce binary")
	}
	libs = append(libs, lib)

	// Compile the package's test code.
	if err := em.linkTests(t, egg, incls, &libs); err != nil {
		return err
	}

	// Run the tests.
	if err := em.runTests(t, egg, exitOnFailure); err != nil {
		return err
	}

	return nil
}
