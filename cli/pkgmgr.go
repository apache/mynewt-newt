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

	// List of packages for Repo
	Packages map[string]*Package
}

type Package struct {
	// Base directory of the package
	BasePath string
	// Name of the package
	Name string
	// Full Name of the package include prefix dir
	FullName string
	// Repository this package belongs to
	Repo *Repo
	// Package version
	Version string
	// Type of package
	LinkerScript string

	// Package sources
	Sources []string
	// Package include directories
	Includes []string

	// Whether or not this package is a BSP
	IsBsp bool

	// Whether or not we've already compiled this package
	Built bool

	// Packages that this package depends on
	Deps []string
}

// Allocate a new package manager structure, and initialize it.
func NewPkgMgr(r *Repo) (*PkgMgr, error) {
	pm := &PkgMgr{
		Repo: r,
	}
	err := pm.Init()

	return pm, err
}

// Load a package's configuration information from the package config
// file.
func (pkg *Package) loadConfig() error {
	log.Printf("[DEBUG] Loading configuration for pkg %s, in directory %s",
		filepath.Base(pkg.Name), pkg.BasePath)

	v, err := ReadConfig(pkg.BasePath, filepath.Base(pkg.Name))
	if err != nil {
		return err
	}

	if filepath.Base(pkg.Name) != v.GetString("pkg.name") {
		return NewStackError(
			"Package file in directory doesn't match directory name.")
	}

	pkg.Version = v.GetString("pkg.vers")
	pkg.LinkerScript = v.GetString("pkg.linkerscript")

	// Load package dependencies
	pkg.Deps = strings.Split(v.GetString("pkg.deps"), " ")

	return nil
}

// Check the include directories for the package, to make sure there are no conflicts in
// include paths for source code
func (pkg *Package) checkIncludes() error {
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
			return NewStackError(fmt.Sprintf("File %s should not exist in include directory, "+
				"only file allowed in include directory is a directory with the package name %s",
				incl, pkg.Name))
		}
	}

	return nil
}

func (pkg *Package) GetBuildIncludes(t *Target) ([]string, error) {
	log.Printf("[DEBUG] Checking package includes to ensure correctness")
	// Check to make sure no include files are in the /include/* directory for the
	// package
	if err := pkg.checkIncludes(); err != nil {
		return nil, err
	}

	// Return the include directories for just this package
	incls := []string{
		pkg.BasePath + "/include/",
		pkg.BasePath + "/include/" + pkg.Name + "/arch/" + t.Arch + "/",
	}

	return incls, nil
}

// Initialize a package
func (pkg *Package) Init() error {
	log.Printf("[DEBUG] Initializing package %s in path %s", pkg.Name, pkg.BasePath)

	// Load package configuration file
	if err := pkg.loadConfig(); err != nil {
		return err
	}

	return nil
}

// Load an individual package specified by pkgName into the package list for
// this repository
func (pm *PkgMgr) loadPackage(pkgBaseDir string, pkgPrefix string, pkgName string) error {
	log.Println("[INFO] Loading Package " + pkgName + "...")

	if pm.Packages == nil {
		pm.Packages = make(map[string]*Package)
	}

	pkgDir := pkgBaseDir + pkgName
	pkg := &Package{
		BasePath: pkgDir,
		Name:     pkgName,
		FullName: pkgPrefix + pkgName,
		Repo:     pm.Repo,
	}
	err := pkg.Init()
	if err != nil {
		return err
	}

	pm.Packages[pkg.FullName] = pkg

	return nil
}

// Recursively load a package.  Given the baseDir of the packages (e.g. pkg/ or hw/bsp), and
// the base package name.
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
			err := pm.loadPackageDir(baseDir, pkgPrefix, pkgName+"/"+name)
			if err != nil {
				return err
			}
		}
	}

	return pm.loadPackage(baseDir, pkgPrefix, pkgName)
}

// Load all the packages in the repository into the package structure
func (pm *PkgMgr) loadPackages() error {
	r := pm.Repo

	// Multiple package directories to be searched
	searchDirs := []string{"libs/", "hw/bsp/", "hw/mcu/", "hw/drivers/", "hw/hal/"}

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
		return nil, NewStackError("Invalid package " + pkgName + " specified")
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

	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, []string{})
	if err != nil {
		return err
	}

	if err := c.RecursiveClean(pkg.BasePath+"/src/", tName); err != nil {
		return err
	}
	if err := os.RemoveAll(pkg.BasePath + "/bin/" + tName); err != nil {
		return NewStackError(err.Error())
	}

	return nil
}

func (pm *PkgMgr) GetPackageLib(t *Target, pkg *Package) string {
	libDir := pkg.BasePath + "/bin/" + t.Name + "/" +
		"lib" + filepath.Base(pkg.Name) + ".a"
	return libDir
}

// Build the package specified by pkgName
func (pm *PkgMgr) Build(t *Target, pkgName string) error {
	log.Println("[INFO] Building package " + pkgName + " for arch " + t.Arch)
	// Look up package structure
	pkg, err := pm.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	// already built the package, no need to rebuild.  This is to handle
	// recursive calls to Build()
	if pkg.Built {
		return nil
	}
	pkg.Built = true

	incls, err := pkg.GetBuildIncludes(t)
	if err != nil {
		return err
	}

	// Go through the package dependencies, load & build those packages
	log.Printf("[DEBUG] Loading includes for %s\n", pkg.Name)
	for _, name := range pkg.Deps {
		if name == "" {
			break
		}

		log.Printf("[DEBUG] Loading package dependency: %s", name)
		// Get package structure
		dpkg, err := pm.ResolvePkgName(name)
		if err != nil {
			return err
		}

		// Build the package
		err = pm.Build(t, name)
		if err != nil {
			return err
		}

		// After build, get dependency package includes.  Build function generates all
		// the package includes
		incls = append(incls, dpkg.Includes...)
	}

	// Add on dependency includes to package includes
	log.Printf("[DEBUG] Adding includes for package %s (old: %s) (new: %s)",
		pkg.Name, pkg.Includes, incls)
	pkg.Includes = incls

	// Build the package designated by pkgName
	// Initialize a compiler
	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, incls)
	if err != nil {
		return err
	}

	if NodeNotExist(pkg.BasePath + "/src/") {
		// nothing to compile, return true!
		return nil
	}

	log.Printf("[DEBUG] compiling src packages in base package directories: %s",
		pkg.BasePath+"/src/")

	// First change into the package src directory, and build all the objects there
	os.Chdir(pkg.BasePath + "/src/")
	if err = c.RecursiveCompile("*.c", 0, []string{"arch"}); err != nil {
		return err
	}

	log.Printf("[DEBUG] compiling architecture specific src packages")

	if NodeExist(pkg.BasePath + "/src/arch/" + t.Arch + "/") {
		if err := os.Chdir(pkg.BasePath + "/src/arch/" + t.Arch + "/"); err != nil {
			return NewStackError(err.Error())
		}
		if err := c.RecursiveCompile("*.c", 0, nil); err != nil {
			return err
		}

		// compile assembly sources in recursive compile as well
		if err = c.RecursiveCompile("*.s", 1, nil); err != nil {
			return err
		}
	}

	// Link everything into a static library, which can be linked with a main
	// program
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

// Get the tests for the package specified by pkgName.
func (pm *PkgMgr) GetPackageTests(pkgName string) ([]string, error) {
	pkg, err := pm.ResolvePkgName(pkgName)
	if err != nil {
		return nil, err
	}

	tests, err := filepath.Glob(pkg.BasePath + "/src/test/*")
	if err != nil {
		return nil, NewStackError(err.Error())
	}

	for key, val := range tests {
		tests[key] = filepath.Base(val)
	}
	return tests, nil
}

// Clean the tests in the tests parameter, for the package identified by
// pkgName.  If cleanAll is set to true, all architectures will be removed.
func (pm *PkgMgr) TestClean(t *Target, pkgName string, tests []string,
	cleanAll bool) error {
	pkg, err := pm.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	tName := t.Name + "/"
	if cleanAll {
		tName = ""
	}

	for _, test := range tests {
		if err := os.RemoveAll(pkg.BasePath + "/src/test/" + test + "/bin/" + tName); err != nil {
			return NewStackError(err.Error())
		}
		if err := os.RemoveAll(pkg.BasePath + "/src/test/" + test + "/obj/" + tName); err != nil {
			return NewStackError(err.Error())
		}
	}

	return nil
}

// Compile tests specified by the tests parameter.  The tests are linked
// to the package specified by the pkg parameter
func (pm *PkgMgr) compileTests(t *Target, pkg *Package, tests []string) error {
	// Now, go and build the individual tests, and link them.
	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, pkg.Includes)
	if err != nil {
		return err
	}

	for _, test := range tests {
		if err := os.Chdir(pkg.BasePath + "/src/test/" + test + "/"); err != nil {
			return NewStackError(err.Error())
		}

		if err = c.Compile("*.c"); err != nil {
			return err
		}

		testBinDir := pkg.BasePath + "/src/test/" + test + "/bin/" +
			t.Name + "/"
		if NodeNotExist(testBinDir) {
			if err := os.MkdirAll(testBinDir, 0755); err != nil {
				return NewStackError(err.Error())
			}
		}
		if err := c.CompileBinary(testBinDir+test, map[string]bool{},
			pkg.BasePath+"/bin/"+t.Name+"/lib"+pkg.Name+".a"); err != nil {
			return err
		}
	}
	return nil
}

// Run all the tests in the tests parameter.  pkg is the package to check for
// the tests.  exitOnFailure specifies whether to exit immediately when a
// test fails, or continue executing all tests.
func (pm *PkgMgr) runTests(t *Target, pkg *Package, exitOnFailure bool,
	tests []string) error {
	// go and run all the tests
	for _, test := range tests {
		if err := os.Chdir(pkg.BasePath + "/src/test/" + test + "/bin/" + t.Name +
			"/"); err != nil {
			return err
		}
		o, err := ShellCommand("./" + test)
		if err != nil {
			log.Printf("[ERROR] Test %s failed, output: %s", test, string(o))
			if exitOnFailure {
				return NewStackError("Unit tests failed to complete successfully.")
			}
		} else {
			log.Printf("[INFO] Test %s ok!", test)
		}
	}
	return nil
}

// Check to ensure tests exist.  Go through the array of tests specified by
// the tests parameter.  pkg is the package to check for these tests.
func (pm *PkgMgr) testsExist(pkg *Package, tests []string) error {
	for _, test := range tests {
		dirName := pkg.BasePath + "/src/test/" + test + "/"
		if NodeNotExist(dirName) {
			return NewStackError("No test exists for " + test)
		}
	}

	return nil
}

// Test the package identified by pkgName, by executing the tests specified.
// exitOnFailure signifies whether to stop the test program when one of them
// fails.
func (pm *PkgMgr) Test(t *Target, pkgName string, exitOnFailure bool, tests []string) error {
	log.Printf("[INFO] Testing package %s for arch %s", pkgName, t.Arch)

	pkg, err := pm.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	// Make sure the test directories exist
	if err := pm.testsExist(pkg, tests); err != nil {
		return err
	}

	// Build the package first
	if err := pm.Build(t, pkgName); err != nil {
		return err
	}

	// compile all the tests first.  want to catch compile errors on all tests
	// before we run the actual tests.
	if err := pm.compileTests(t, pkg, tests); err != nil {
		return err
	}

	if err := pm.runTests(t, pkg, exitOnFailure, tests); err != nil {
		return err
	}

	return nil
}
