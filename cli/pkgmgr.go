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
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type PkgMgr struct {
	// Repository associated with the Packages
	Repo *Repo

	Target *Target

	// List of packages for Repo
	Packages map[string]*Package
}

// Allocate a new package manager structure, and initialize it.
func NewPkgMgr(r *Repo, t *Target) (*PkgMgr, error) {
	pm := &PkgMgr{
		Repo:   r,
		Target: t,
	}
	err := pm.Init()

	return pm, err
}

func (pm *PkgMgr) CheckPkgDeps(pkg *Package,
	deps map[string]*DependencyRequirement, reqcap map[string]*DependencyRequirement,
	caps map[string]*DependencyRequirement) error {
	// if no dependencies, then everything is ok!
	if pkg.Deps == nil || len(pkg.Deps) == 0 {
		return nil
	}

	for _, depReq := range pkg.Deps {
		// don't process this package if we've already processed it
		if _, ok := deps[depReq.String()]; ok {
			continue
		}

		log.Printf("[DEBUG] Checking dependency %s for package %s", depReq, pkg.Name)
		pkg, ok := pm.Packages[depReq.Name]
		if !ok {
			return NewStackError(fmt.Sprintf("No package dependency %s found for %s",
				depReq.Name, pkg.Name))
		}

		if ok := depReq.SatisfiesDependency(pkg); !ok {
			return NewStackError(fmt.Sprintf("Package %s doesn't satisfy dependency %s",
				pkg.Name, depReq))
		}

		// We've checked this dependency requirement, all is gute!
		deps[depReq.String()] = depReq
	}

	// Now go through and recurse through the sub-package dependencies
	for _, depReq := range pkg.Deps {
		if _, ok := deps[depReq.String()]; ok {
			continue
		}

		if err := pm.CheckPkgDeps(pm.Packages[depReq.Name], deps, reqcap, caps); err != nil {
			return err
		}
	}

	return nil
}

func (pm *PkgMgr) VerifyCaps(reqcaps map[string]*DependencyRequirement,
	caps map[string]*DependencyRequirement) error {

	for name, rcap := range reqcaps {
		capability, ok := caps[name]
		if !ok {
			return NewStackError(fmt.Sprintf("Required capability %s not found", name))
		}

		if err := rcap.SatisfiesCapability(capability); err != nil {
			return err
		}
	}

	return nil
}

func (pm *PkgMgr) CheckDeps() error {
	// Go through all the packages and check that their dependencies are satisfied
	for _, pkg := range pm.Packages {
		deps := map[string]*DependencyRequirement{}
		reqcap := map[string]*DependencyRequirement{}
		caps := map[string]*DependencyRequirement{}

		if err := pm.CheckPkgDeps(pkg, deps, reqcap, caps); err != nil {
			return err
		}
	}

	return nil
}

// Load an individual package specified by pkgName into the package list for
// this repository
func (pm *PkgMgr) loadPackage(pkgDir string) error {
	log.Println("[INFO] Loading Package " + pkgDir + "...")

	if pm.Packages == nil {
		pm.Packages = make(map[string]*Package)
	}

	pkg, err := NewPackage(pm.Repo, pm.Target, pkgDir)
	if err != nil {
		return nil
	}

	pm.Packages[pkg.FullName] = pkg

	return nil
}

func (pm *PkgMgr) String() string {
	str := ""
	for pkgName, _ := range pm.Packages {
		str += pkgName + " "
	}
	return str
}

// Recursively load a package.  Given the baseDir of the packages (e.g. pkg/ or
// hw/bsp), and the base package name.
func (pm *PkgMgr) loadPackageDir(baseDir string, pkgPrefix string, pkgName string) error {
	log.Printf("[DEBUG] Loading packages in %s, starting with package %s", baseDir, pkgName)

	// first recurse and load subpackages
	list, err := ioutil.ReadDir(baseDir + "/" + pkgName)
	if err != nil {
		return NewStackError(err.Error())
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
			if err := pm.loadPackageDir(baseDir, pkgPrefix, pkgName+"/"+name); err != nil {
				return err
			}
		}
	}

	if NodeNotExist(baseDir + "/" + pkgName + "/pkg.yml") {
		return nil
	}

	return pm.loadPackage(baseDir + "/" + pkgName)
}

// Load all the packages in the repository into the package structure
func (pm *PkgMgr) loadPackages() error {
	r := pm.Repo

	// Multiple package directories to be searched
	searchDirs := []string{"libs/", "hw/bsp/", "hw/mcu/", "hw/mcu/stm", "hw/drivers/", "hw/"}

	for _, pkgDir := range searchDirs {
		pkgBaseDir := r.BasePath + "/" + pkgDir

		if NodeNotExist(pkgBaseDir) {
			continue
		}

		pkgList, err := ioutil.ReadDir(pkgBaseDir)
		if err != nil {
			return NewStackError(err.Error())
		}

		for _, subPkgDir := range pkgList {
			name := subPkgDir.Name()
			if filepath.HasPrefix(name, ".") || filepath.HasPrefix(name, "..") {
				continue
			}

			if !subPkgDir.IsDir() {
				continue
			}

			if err = pm.loadPackageDir(pkgBaseDir, pkgDir, name); err != nil {
				return err
			}
		}
	}

	return nil
}

// Initialize the package manager
func (pm *PkgMgr) Init() error {
	err := pm.loadPackages()
	return err
}

// Resolve the package specified by pkgName into a package structure.
func (pm *PkgMgr) ResolvePkgName(pkgName string) (*Package, error) {
	pkg, ok := pm.Packages[pkgName]
	if !ok {
		return nil, NewStackError(fmt.Sprintf("Invalid package %s specified (pkgs = %s)",
			pkgName, pm))
	}
	return pkg, nil
}

func (pm *PkgMgr) ResolvePkgDir(pkgDir string) (*Package, error) {
	pkgDir = filepath.Clean(pkgDir)
	for name, pkg := range pm.Packages {
		if filepath.Clean(pkg.BasePath) == pkgDir {
			return pm.Packages[name], nil
		}
	}
	return nil, NewStackError(fmt.Sprintf("Cannot resolve package dir %s in package "+
		"manager", pkgDir))
}

func (pm *PkgMgr) VerifyPackage(pkgDir string) (*Package, error) {
	err := pm.loadPackage(pkgDir)
	if err != nil {
		return nil, err
	}

	pkg, err := pm.ResolvePkgDir(pkgDir)
	if err != nil {
		return nil, err
	}

	if err := pm.checkIncludes(pkg); err != nil {
		return nil, err
	}

	return pkg, nil
}

// Clean the build for the package specified by pkgName.   if cleanAll is
// specified, all architectures are cleaned.
func (pm *PkgMgr) BuildClean(t *Target, pkgName string, cleanAll bool) error {
	pkg, err := pm.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	tName := t.Name + "/"
	if cleanAll {
		tName = ""
	}

	if pkg.Clean {
		return nil
	}
	pkg.Clean = true

	for _, dep := range pkg.Deps {
		if err := pm.BuildClean(t, dep.Name, cleanAll); err != nil {
			return err
		}
	}

	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, []string{})
	if err != nil {
		return err
	}

	if NodeExist(pkg.BasePath + "/src/") {
		if err := c.RecursiveClean(pkg.BasePath+"/src/", tName); err != nil {
			return err
		}

		if err := os.RemoveAll(pkg.BasePath + "/bin/" + tName); err != nil {
			return NewStackError(err.Error())
		}
	}

	pkg.Clean = true

	return nil
}

func (pm *PkgMgr) GetPackageLib(t *Target, pkg *Package) string {
	libDir := pkg.BasePath + "/bin/" + t.Name + "/" +
		"lib" + filepath.Base(pkg.Name) + ".a"
	return libDir
}

// @param incls                 Extra include paths that get specified during
//                                  build; not modified by this function.
// @param libs                  List of libraries that have been built so far;
//                                  This function appends entries to this list.
func (pm *PkgMgr) buildDeps(pkg *Package, t *Target, incls *[]string,
	libs *[]string) error {

	log.Printf("[DEBUG] Building package dependencies for %s, target %s",
		pkg.Name, t.Name)

	var err error

	if pkg.Includes, err = pkg.GetIncludes(t); err != nil {
		return err
	}

	if incls == nil {
		incls = &[]string{}
	}
	if libs == nil {
		libs = &[]string{}
	}

	for _, dep := range pkg.Deps {
		if dep.Name == "" {
			break
		}

		log.Printf("[DEBUG] Loading package dependency: %s", dep.Name)
		// Get package structure
		dpkg, err := pm.ResolvePkgName(dep.Name)
		if err != nil {
			return err
		}

		// Build the package
		if err = pm.Build(t, dep.Name, *incls, libs); err != nil {
			return err
		}

		// After build, get dependency package includes.  Build function
		// generates all the package includes
		pkg.Includes = append(pkg.Includes, dpkg.Includes...)
		if lib := pm.GetPackageLib(t, dpkg); NodeExist(lib) {
			*libs = append(*libs, lib)
		}
	}

	// Add on dependency includes to package includes
	log.Printf("[DEBUG] Package dependencies for %s built, incls = %s",
		pkg.Name, pkg.Includes)

	return nil
}

// Build the package specified by pkgName
//
// @param incls                 Extra include paths that get specified during
//                                  build.  Note: passed by value.
// @param libs                  List of libraries that have been built so far;
//                                  This function appends entries to this list.
func (pm *PkgMgr) Build(t *Target, pkgName string, incls []string,
	libs *[]string) error {

	// Look up package structure
	pkg, err := pm.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	log.Println("[INFO] Building package " + pkgName + " for arch " + t.Arch)

	// already built the package, no need to rebuild.  This is to handle
	// recursive calls to Build()
	if pkg.Built {
		return nil
	}
	pkg.Built = true

	if err := pm.buildDeps(pkg, t, &incls, libs); err != nil {
		return err
	}

	srcDir := pkg.BasePath + "/src/"

	// NOTE: this assignment must happen after the call to buildDeps(), as
	// buildDeps() fills in the package includes.
	incls = append(incls, PkgIncludeDirs(pkg, t, srcDir)...)
	log.Printf("[DEBUG] Package includes for %s are %s", pkgName, incls)

	if NodeNotExist(srcDir) {
		// nothing to compile, return true!
		return nil
	}

	// Build the package designated by pkgName
	// Initialize a compiler
	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, incls)
	if err != nil {
		return err
	}
	// setup Cflags, Lflags and Aflags
	c.Cflags = CreateCFlags(c, t, pkg)
	c.Lflags += " " + pkg.Lflags + " " + t.Lflags
	c.Aflags += " " + pkg.Aflags + " " + t.Aflags

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
	if err := os.Chdir(pkg.BasePath + "/"); err != nil {
		return NewStackError(err.Error())
	}

	binDir := pkg.BasePath + "/bin/" + t.Name + "/"

	if NodeNotExist(binDir) {
		if err := os.MkdirAll(binDir, 0755); err != nil {
			return NewStackError(err.Error())
		}
	}

	if err = c.CompileArchive(pm.GetPackageLib(t, pkg), ""); err != nil {
		return err
	}

	return nil
}

// Check the include directories for the package, to make sure there are no conflicts in
// include paths for source code
func (pm *PkgMgr) checkIncludes(pkg *Package) error {
	incls, err := filepath.Glob(pkg.BasePath + "/include/*")
	if err != nil {
		return NewStackError(err.Error())
	}

	// Append all the architecture specific directories
	archDir := pkg.BasePath + "/include/" + pkg.Name + "/arch/"
	dirs, err := ioutil.ReadDir(archDir)
	if err != nil {
		return NewStackError(err.Error())
	}

	for _, dir := range dirs {
		if !dir.IsDir() {
			return NewStackError(fmt.Sprintf("Only directories are allowed in "+
				"architecture dir: %s", archDir+dir.Name()))
		}

		incls2, err := filepath.Glob(archDir + dir.Name() + "/*")
		if err != nil {
			return NewStackError(err.Error())
		}

		incls = append(incls, incls2...)
	}

	for _, incl := range incls {
		finfo, err := os.Stat(incl)
		if err != nil {
			return NewStackError(err.Error())
		}

		bad := false
		if !finfo.IsDir() {
			bad = true
		}

		if filepath.Base(incl) != pkg.Name {
			if pkg.IsBsp && filepath.Base(incl) != "bsp" {
				bad = true
			}
		}

		if bad {
			return NewStackError(fmt.Sprintf("File %s should not exist in include "+
				"directory, only file allowed in include directory is a directory with "+
				"the package name %s",
				incl, pkg.Name))
		}
	}

	return nil
}

// Clean the tests in the tests parameter, for the package identified by
// pkgName.  If cleanAll is set to true, all architectures will be removed.
func (pm *PkgMgr) TestClean(t *Target, pkgName string,
	cleanAll bool) error {
	pkg, err := pm.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	tName := t.Name + "/"
	if cleanAll {
		tName = ""
	}

	if err := os.RemoveAll(pkg.BasePath + "/src/test/bin/" + tName); err != nil {
		return NewStackError(err.Error())
	}
	if err := os.RemoveAll(pkg.BasePath + "/src/test/obj/" + tName); err != nil {
		return NewStackError(err.Error())
	}

	return nil
}

// Compile tests specified by the tests parameter.  The tests are linked
// to the package specified by the pkg parameter
func (pm *PkgMgr) linkTests(t *Target, pkg *Package,
	incls []string, libs *[]string) error {

	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, incls)
	if err != nil {
		return err
	}

	// Configure Lflags.  Since we are only linking, Cflags and Aflags are
	// unnecessary.
	c.Lflags += " " + pkg.Lflags + " " + t.Lflags

	testBinDir := pkg.BasePath + "/src/test/bin/" + t.Name + "/"
	if NodeNotExist(testBinDir) {
		if err := os.MkdirAll(testBinDir, 0755); err != nil {
			return NewStackError(err.Error())
		}
	}

	if err := c.CompileBinary(testBinDir+pkg.TestBinName(), map[string]bool{},
		strings.Join(*libs, " ")); err != nil {

		return err
	}

	return nil
}

// Run all the tests in the tests parameter.  pkg is the package to check for
// the tests.  exitOnFailure specifies whether to exit immediately when a
// test fails, or continue executing all tests.
func (pm *PkgMgr) runTests(t *Target, pkg *Package, exitOnFailure bool) error {
	if err := os.Chdir(pkg.BasePath + "/src/test/bin/" + t.Name +
		"/"); err != nil {
		return err
	}
	o, err := ShellCommand("./" + pkg.TestBinName())
	if err != nil {
		log.Printf("[ERROR] Test %s failed, output: %s", pkg.TestBinName(),
			string(o))
		if exitOnFailure {
			return NewStackError("Unit tests failed to complete successfully.")
		}
	} else {
		log.Printf("[INFO] Test %s ok!", pkg.TestBinName())
	}

	return nil
}

// Check to ensure tests exist.  Go through the array of tests specified by
// the tests parameter.  pkg is the package to check for these tests.
func (pm *PkgMgr) testsExist(pkg *Package) error {
	dirName := pkg.BasePath + "/src/test/"
	if NodeNotExist(dirName) {
		return NewStackError("No test exists for package " + pkg.Name)
	}

	return nil
}

// Test the package identified by pkgName, by executing the tests specified.
// exitOnFailure signifies whether to stop the test program when one of them
// fails.
func (pm *PkgMgr) Test(t *Target, pkgName string, exitOnFailure bool) error {
	log.Printf("[INFO] Testing package %s for arch %s", pkgName, t.Arch)

	pkg, err := pm.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	// Make sure the test directories exist
	if err := pm.testsExist(pkg); err != nil {
		return err
	}

	incls := []string{}
	libs := []string{}

	// The 'test' identity is implicitly exported during a package test.
	t.Identities = append(t.Identities, "test")

	// Build the BSP first.  This populates the global set of include paths and
	// libraries that the test code needs.
	if t.Bsp != "" {
		_, err = buildBsp(t, pm, &incls, &libs)
		if err != nil {
			return err
		}
	}

	// Build the package under test.  This must be compiled with the PKG_TEST
	// symbol defined so that the appropriate main function gets built.
	pkg.Cflags += " -DPKG_TEST"
	if err := pm.Build(t, pkgName, incls, &libs); err != nil {
		return err
	}
	lib := pm.GetPackageLib(t, pkg)
	if !NodeExist(lib) {
		return NewStackError("Package " + pkgName + " did not produce binary")
	}
	libs = append(libs, lib)

	// Compile the package's test code.
	if err := pm.linkTests(t, pkg, incls, &libs); err != nil {
		return err
	}

	// Run the tests.
	if err := pm.runTests(t, pkg, exitOnFailure); err != nil {
		return err
	}

	return nil
}
