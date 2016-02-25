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
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"

	"github.com/spf13/viper"
)

type PkgList struct {
	// Repository associated with the Pkgs
	Repo *Repo

	// List of packages for Repo
	Pkgs map[string]*Pkg

	PkgDescs map[string]*PkgDesc

	Name string

	LarvaFile string

	RemoteUrl string

	Branch string
}

// Allocate a new package manager structure, and initialize it.
func NewPkgList(repo *Repo) (*PkgList, error) {
	pkgList := &PkgList{
		Repo: repo,
	}
	err := pkgList.Init()

	return pkgList, err
}

func (pkgList *PkgList) LoadConfigs(t *Target, force bool) error {
	for _, pkg := range pkgList.Pkgs {
		if err := pkg.LoadConfig(t, force); err != nil {
			return err
		}
	}
	return nil
}

func (pkgList *PkgList) CheckPkgDeps(pkg *Pkg,
	deps map[string]*DependencyRequirement,
	reqcap map[string]*DependencyRequirement,
	caps map[string]*DependencyRequirement,
	capPkgs map[string]string) error {

	for _, depReq := range pkg.Deps {
		// don't process this package if we've already processed it
		if _, ok := deps[depReq.String()]; ok {
			continue
		}

		pkgName := pkg.Name
		StatusMessage(VERBOSITY_VERBOSE,
			"Checking dependency %s for package %s\n", depReq.Name, pkgName)
		pkg, ok := pkgList.Pkgs[depReq.Name]
		if !ok {
			return NewNewtError(
				fmt.Sprintf("No package dependency %s found for %s",
					depReq.Name, pkgName))
		}

		if ok := depReq.SatisfiesDependency(pkg); !ok {
			return NewNewtError(fmt.Sprintf("Pkg %s doesn't satisfy dependency %s",
				pkg.Name, depReq))
		}

		// We've checked this dependency requirement, all is gute!
		deps[depReq.String()] = depReq
	}

	for _, reqCap := range pkg.ReqCapabilities {
		reqcap[reqCap.String()] = reqCap
	}

	for _, cap := range pkg.Capabilities {
		if caps[cap.String()] != nil && capPkgs[cap.String()] != pkg.FullName {
			return NewNewtError(fmt.Sprintf("Multiple pkgs with capability %s",
				cap.String()))
		}
		caps[cap.String()] = cap
		if capPkgs != nil {
			capPkgs[cap.String()] = pkg.FullName
		}
	}

	// Now go through and recurse through the sub-package dependencies
	for _, depReq := range pkg.Deps {
		if _, ok := deps[depReq.String()]; ok {
			continue
		}

		if err := pkgList.CheckPkgDeps(pkgList.Pkgs[depReq.Name], deps,
			reqcap, caps, capPkgs); err != nil {
			return err
		}
	}

	return nil
}

func (pkgList *PkgList) VerifyCaps(reqcaps map[string]*DependencyRequirement,
	caps map[string]*DependencyRequirement) error {

	for name, rcap := range reqcaps {
		capability, ok := caps[name]
		if !ok {
			return NewNewtError(fmt.Sprintf("Required capability %s not found",
				name))
		}

		if err := rcap.SatisfiesCapability(capability); err != nil {
			return err
		}
	}

	return nil
}

func (pkgList *PkgList) CheckDeps() error {
	// Go through all the packages and check that their dependencies are satisfied
	for _, pkg := range pkgList.Pkgs {
		deps := map[string]*DependencyRequirement{}
		reqcap := map[string]*DependencyRequirement{}
		caps := map[string]*DependencyRequirement{}

		if err := pkgList.CheckPkgDeps(pkg, deps, reqcap, caps, nil); err != nil {
			return err
		}
	}

	return nil
}

// Load an individual package specified by pkgName into the package list for
// this repository
func (pkgList *PkgList) loadPkg(pkgDir string, pkgPrefix string,
	pkgName string) error {
	StatusMessage(VERBOSITY_VERBOSE, "Loading Pkg "+pkgDir+"...\n")

	if pkgList.Pkgs == nil {
		pkgList.Pkgs = make(map[string]*Pkg)
	}

	pkg, err := NewPkg(pkgList.Repo, pkgDir)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Cannot load package %s, error %s",
			pkgDir, err.Error()))
	}

	pkgList.Pkgs[pkgPrefix+pkgName] = pkg

	return nil
}

func (pkgList *PkgList) String() string {
	str := ""
	for pkgName, _ := range pkgList.Pkgs {
		str += pkgName + " "
	}
	return str
}

// Recursively load a package.  Given the baseDir of the packages (e.g. pkg/ or
// hw/bsp), and the base package name.
func (pkgList *PkgList) loadPkgDir(baseDir string, pkgPrefix string,
	pkgName string) error {
	log.Printf("[DEBUG] Loading pkgs in %s, starting with pkg %s",
		baseDir, pkgName)

	// first recurse and load subpackages
	list, err := ioutil.ReadDir(baseDir + "/" + pkgName)
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
			if err := pkgList.loadPkgDir(baseDir, pkgPrefix,
				pkgName+"/"+name); err != nil {
				return err
			}
		}
	}

	if NodeNotExist(baseDir + "/" + pkgName + "/pkg.yml") {
		return nil
	}

	return pkgList.loadPkg(baseDir+"/"+pkgName, pkgPrefix, pkgName)
}

// Load all the packages in the repository into the package structure
func (pkgList *PkgList) loadPkgs() error {
	repo := pkgList.Repo

	searchDirs := repo.PkgPaths()

	for _, pkgDir := range searchDirs {
		pkgBaseDir := repo.BasePath + "/" + pkgDir

		if NodeNotExist(pkgBaseDir) {
			continue
		}

		pkgDirList, err := ioutil.ReadDir(pkgBaseDir)
		if err != nil {
			return NewNewtError(err.Error())
		}

		for _, subPkgDir := range pkgDirList {
			name := subPkgDir.Name()
			if filepath.HasPrefix(name, ".") || filepath.HasPrefix(name, "..") {
				continue
			}

			if !subPkgDir.IsDir() {
				continue
			}

			if err = pkgList.loadPkgDir(pkgBaseDir, pkgDir, name); err != nil {
				return err
			}
		}
	}

	return nil
}

// Initialize the package manager
func (pkgList *PkgList) Init() error {
	if err := pkgList.loadPkgs(); err != nil {
		return err
	}

	pkgList.PkgDescs = map[string]*PkgDesc{}

	return nil
}

// Resolve the package specified by pkgName into a package structure.
func (pkgList *PkgList) ResolvePkgName(pkgName string) (*Pkg, error) {
	pkg, ok := pkgList.Pkgs[pkgName]
	if !ok {
		return nil, NewNewtError(fmt.Sprintf("Invalid pkg '%s' specified "+
			"(pkgs = %s)", pkgName, pkgList.Pkgs))
	}
	return pkg, nil
}

func (pkgList *PkgList) ResolvePkgDescName(pkgName string) (*PkgDesc, error) {
	pkgDesc, ok := pkgList.PkgDescs[pkgName]
	if !ok {
		return nil, NewNewtError(fmt.Sprintf("Invalid pkg '%s' specified "+
			"(pkgs = %s)", pkgName, pkgList))
	}
	return pkgDesc, nil
}

func (pkgList *PkgList) ResolvePkgDir(pkgDir string) (*Pkg, error) {
	pkgDir = filepath.Clean(pkgDir)
	for name, pkg := range pkgList.Pkgs {
		if filepath.Clean(pkg.BasePath) == pkgDir {
			return pkgList.Pkgs[name], nil
		}
	}
	return nil, NewNewtError(fmt.Sprintf("Cannot resolve package dir %s in "+
		"package manager", pkgDir))
}

// Clean the build for the package specified by pkgName.   if cleanAll is
// specified, all architectures are cleaned.
func (pkgList *PkgList) BuildClean(t *Target, pkgName string, cleanAll bool) error {
	pkg, err := pkgList.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	if err := pkg.LoadConfig(t, false); err != nil {
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
		if err := pkgList.BuildClean(t, dep.Name, cleanAll); err != nil {
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
			return NewNewtError(err.Error())
		}
	}

	pkg.Clean = true

	return nil
}

func (pkgList *PkgList) GetPkgLib(t *Target, pkg *Pkg) string {
	libDir := pkg.BasePath + "/bin/" + t.Name + "/" +
		"lib" + filepath.Base(pkg.Name) + ".a"
	return libDir
}

// @param incls                 Extra include paths that get specified during
//                                  build; not modified by this function.
// @param libs                  List of libraries that have been built so far;
//                                  This function appends entries to this list.
func (pkgList *PkgList) buildDeps(pkg *Pkg, t *Target, incls *[]string,
	libs *[]string) error {

	StatusMessage(VERBOSITY_VERBOSE,
		"Building pkg dependencies for %s, target %s\n", pkg.Name, t.Name)

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
		dpkg, err := pkgList.ResolvePkgName(dep.Name)
		if err != nil {
			return err
		}

		// Build the package
		if err = pkgList.Build(t, dep.Name, *incls, libs); err != nil {
			return err
		}

		// After build, get dependency package includes.  Build function
		// generates all the package includes
		pkg.Includes = append(pkg.Includes, dpkg.Includes...)
		if lib := pkgList.GetPkgLib(t, dpkg); NodeExist(lib) {
			*libs = append(*libs, lib)
		}
	}

	// Add on dependency includes to package includes
	log.Printf("[DEBUG] Pkg dependencies for %s built, incls = %s",
		pkg.Name, pkg.Includes)

	return nil
}

// Build the package specified by pkgName
//
// @param incls            Extra include paths that get specified during
//                             build.  Note: passed by value.
// @param lib              List of libraries that have been built so far;
//                             This function appends entries to this list.
func (pkgList *PkgList) Build(t *Target, pkgName string, incls []string,
	libs *[]string) error {

	// Look up package structure
	pkg, err := pkgList.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	if err := pkg.LoadConfig(t, false); err != nil {
		return err
	}

	// already built the package, no need to rebuild.  This is to handle
	// recursive calls to Build()
	if pkg.Built {
		return nil
	}
	pkg.Built = true

	if err := pkgList.buildDeps(pkg, t, &incls, libs); err != nil {
		return err
	}

	StatusMessage(VERBOSITY_VERBOSE, "Building pkg %s for arch %s\n",
		pkgName, t.Arch)

	// NOTE: this assignment must happen after the call to buildDeps(), as
	// buildDeps() fills in the package includes.
	incls = append(incls, PkgIncludeDirs(pkg, t)...)
	log.Printf("[DEBUG] Pkg includes for %s are %s", pkgName, incls)

	srcDir := pkg.BasePath + "/src/"
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
	c.Cflags = CreateCflags(pkgList, c, t, pkg.Cflags)
	c.Lflags += " " + pkg.Lflags + " " + t.Lflags
	c.Aflags += " " + pkg.Aflags + " " + t.Aflags

	log.Printf("[DEBUG] compiling src pkgs in base pkg directory: %s", srcDir)

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
		return NewNewtError(err.Error())
	}

	binDir := pkg.BasePath + "/bin/" + t.Name + "/"

	if NodeNotExist(binDir) {
		if err := os.MkdirAll(binDir, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	if err = c.CompileArchive(pkgList.GetPkgLib(t, pkg), []string{}); err != nil {
		return err
	}

	return nil
}

// Check the include directories for the package, to make sure there are
// no conflicts in include paths for source code
func (pkgList *PkgList) checkIncludes(pkg *Pkg) error {
	incls, err := filepath.Glob(pkg.BasePath + "/include/*")
	if err != nil {
		return NewNewtError(err.Error())
	}

	// Append all the architecture specific directories
	archDir := pkg.BasePath + "/include/" + pkg.Name + "/arch/"
	dirs, err := ioutil.ReadDir(archDir)
	if err != nil {
		return NewNewtError(err.Error())
	}

	for _, dir := range dirs {
		if !dir.IsDir() {
			return NewNewtError(fmt.Sprintf(
				"Only directories are allowed in architecture dir: %s",
				archDir+dir.Name()))
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

		if filepath.Base(incl) != pkg.Name {
			if pkg.IsBsp && filepath.Base(incl) != "bsp" {
				bad = true
			}
		}

		if bad {
			return NewNewtError(fmt.Sprintf("File %s should not exist"+
				"in include directory, only file allowed in include "+
				"directory is a directory with the package name %s",
				incl, pkg.Name))
		}
	}

	return nil
}

// Clean the tests in the tests parameter, for the package identified by
// pkgName.  If cleanAll is set to true, all architectures will be removed.
func (pkgList *PkgList) TestClean(t *Target, pkgName string,
	cleanAll bool) error {
	pkg, err := pkgList.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	if err := pkg.LoadConfig(t, false); err != nil {
		return err
	}

	tName := t.Name + "/"
	if cleanAll {
		tName = ""
	}

	if err := os.RemoveAll(pkg.BasePath + "/src/test/bin/" + tName); err != nil {
		return NewNewtError(err.Error())
	}
	if err := os.RemoveAll(pkg.BasePath + "/src/test/obj/" + tName); err != nil {
		return NewNewtError(err.Error())
	}

	return nil
}

// Compile tests specified by the tests parameter.  The tests are linked
// to the package specified by the pkg parameter
func (pkgList *PkgList) linkTests(t *Target, pkg *Pkg,
	incls []string, libs *[]string) error {

	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, incls)
	if err != nil {
		return err
	}

	// Configure Lflags.  Since we are only linking, Cflags and Aflags are
	// unnecessary.
	c.Lflags += " " + pkg.Lflags + " " + t.Lflags

	testBinDir := pkg.BasePath + "/src/test/bin/" + t.Name + "/"
	binFile := testBinDir + pkg.TestBinName()
	options := map[string]bool{}

	// Determine if the test executable is already up to date.
	linkRequired, err := c.depTracker.LinkRequired(binFile, options, *libs)
	if err != nil {
		return err
	}

	// Build the test executable if necessary.
	if linkRequired {
		if NodeNotExist(testBinDir) {
			if err := os.MkdirAll(testBinDir, 0755); err != nil {
				return NewNewtError(err.Error())
			}
		}

		err = c.CompileBinary(binFile, options, *libs)
		if err != nil {
			return err
		}
	}

	return nil
}

// Run all the tests in the tests parameter.  pkg is the package to check for
// the tests.  exitOnFailure specifies whether to exit immediately when a
// test fails, or continue executing all tests.
func (pkgList *PkgList) runTests(t *Target, pkg *Pkg, exitOnFailure bool) error {
	StatusMessage(VERBOSITY_DEFAULT, "Testing pkg %s for arch %s\n",
		pkg.Name, t.Arch)

	if err := os.Chdir(pkg.BasePath + "/src/test/bin/" + t.Name +
		"/"); err != nil {
		return err
	}

	o, err := ShellCommand("./" + pkg.TestBinName())
	if err != nil {
		StatusMessage(VERBOSITY_DEFAULT, "%s", string(o))

		// Always terminate on test failure since only one test is being run.
		return NewtErrorNoTrace(fmt.Sprintf("Test %s failed",
			pkg.TestBinName()))
	} else {
		StatusMessage(VERBOSITY_VERBOSE, "%s", string(o))
		StatusMessage(VERBOSITY_DEFAULT, "Test %s ok!\n", pkg.TestBinName())
		return nil
	}
}

// Check to ensure tests exist.  Go through the array of tests specified by
// the tests parameter.  pkg is the package to check for these tests.
func (pkgList *PkgList) testsExist(pkg *Pkg) error {
	dirName := pkg.BasePath + "/src/test/"
	if NodeNotExist(dirName) {
		return NewNewtError("No test exists for package " + pkg.Name)
	}

	return nil
}

// Test the package identified by pkgName, by executing the tests specified.
// exitOnFailure signifies whether to stop the test program when one of them
// fails.
func (pkgList *PkgList) Test(t *Target, pkgName string,
	exitOnFailure bool) error {

	// A few identities are implicitly exported when the test command is used:
	// *    test:       ensures that the test code gets compiled.
	// *    selftest:   indicates that there is no project
	t.Identities["test"] = "test"
	t.Identities["selftest"] = "selftest"

	pkg, err := pkgList.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	if err := pkg.LoadConfig(t, false); err != nil {
		return err
	}

	// Make sure the test directories exist
	if err := pkgList.testsExist(pkg); err != nil {
		return err
	}

	// The pkg under test must be compiled with the PKG_TEST symbol defined so
	// that the appropriate main function gets built.
	pkg.Cflags += " -DPKG_TEST"

	incls := []string{}
	libs := []string{}

	// If there is a BSP:
	//     1. Calculate the include paths that it and its dependencies export.
	//        This set of include paths is accessible during all subsequent
	//        builds.
	//     2. Build the BSP package.
	if t.Bsp != "" {
		incls, err = BspIncludePaths(pkgList, t)
		if err != nil {
			return err
		}
		_, err = buildBsp(t, pkgList, &incls, &libs, nil)
		if err != nil {
			return err
		}
	}

	// Build the package under test.
	if err := pkgList.Build(t, pkgName, incls, &libs); err != nil {
		return err
	}
	lib := pkgList.GetPkgLib(t, pkg)
	if !NodeExist(lib) {
		return NewNewtError("Pkg " + pkgName + " did not produce binary")
	}
	libs = append(libs, lib)

	// Compile the package's test code.
	if err := pkgList.linkTests(t, pkg, incls, &libs); err != nil {
		return err
	}

	// Run the tests.
	if err := pkgList.runTests(t, pkg, exitOnFailure); err != nil {
		return err
	}

	return nil
}

func (cl *PkgList) LoadFromPkgList(local *PkgList) error {
	var err error
	for _, pkg := range local.Pkgs {
		if err := pkg.LoadConfig(nil, false); err != nil {
			return err
		}

		log.Printf("[DEBUG] Pkg %s loaded, putting it into pkgList %s",
			pkg.FullName, local.Name)

		pkgDesc := &PkgDesc{}
		pkgDesc.FullName = pkg.FullName
		pkgDesc.Deps = pkg.Deps
		pkgDesc.Caps = pkg.Capabilities
		pkgDesc.ReqCaps = pkg.ReqCapabilities
		pkgDesc.Version = pkg.Version
		pkgDesc.Hash, err = pkg.GetHash()
		if err != nil {
			return err
		}

		cl.PkgDescs[pkgDesc.FullName] = pkgDesc
	}

	return nil
}

func (cl *PkgList) Serialize() (string, error) {
	clStr := "name: " + cl.Name + "\n"
	clStr = clStr + "url: " + cl.RemoteUrl + "\n"
	clStr = clStr + "pkgs:\n"

	buf := bytes.Buffer{}

	indent := "    "
	for _, pkgDesc := range cl.PkgDescs {
		buf.WriteString(pkgDesc.Serialize(indent))
	}

	return clStr + buf.String(), nil
}

func (cl *PkgList) strSliceToDr(list []string) ([]*DependencyRequirement, error) {
	drList := []*DependencyRequirement{}

	for _, name := range list {
		req, err := NewDependencyRequirementParseString(name)
		if err != nil {
			return nil, err
		}
		drList = append(drList, req)
	}

	if len(drList) == 0 {
		return nil, nil
	} else {
		return drList, nil
	}
}

func (cl *PkgList) fileToPkgList(cfg *viper.Viper) (map[string]*PkgDesc,
	error) {
	pkgMap := cfg.GetStringMap("pkgs")

	pkgList := map[string]*PkgDesc{}

	for name, _ := range pkgMap {
		pkgDesc, err := NewPkgDesc(cl)
		if err != nil {
			return nil, err
		}
		pkgDesc.FullName = name

		pkgDef := cfg.GetStringMap("pkgs." + name)
		pkgDesc.Version, err = NewVersParseString(pkgDef["vers"].(string))
		if err != nil {
			return nil, err
		}

		pkgDesc.Deps, err = cl.strSliceToDr(
			cfg.GetStringSlice("pkgs." + name + ".deps"))
		if err != nil {
			return nil, err
		}

		pkgDesc.Caps, err = cl.strSliceToDr(
			cfg.GetStringSlice("pkgs." + name + ".caps"))
		if err != nil {
			return nil, err
		}

		pkgDesc.ReqCaps, err = cl.strSliceToDr(
			cfg.GetStringSlice("pkgs." + name + ".req_caps"))
		if err != nil {
			return nil, err
		}

		pkgList[name] = pkgDesc
	}

	return pkgList, nil
}

// Create the manifest file name, it's the manifest dir + manifest name +
// branch and a.yml extension
func (pkgList *PkgList) GetPkgListFile(name string, branch string) string {
	return name + "@" + branch
}

func (pkgList *PkgList) GetPkgListFullFile(name string, branch string) string {
	return pkgList.Repo.PkgListPath + pkgList.GetPkgListFile(name, branch) + ".yml"
}

func (pkgList *PkgList) Load(name string) error {
	cfg, err := ReadConfig(pkgList.Repo.PkgListPath, name)
	if err != nil {
		return err
	}

	pkgListName := name
	branchName := "master"

	parts := strings.Split(name, "@")
	if len(parts) == 2 {
		pkgListName = parts[0]
		branchName = parts[1]
	}

	if cfg.GetString("name") != pkgListName {
		return NewNewtError(
			fmt.Sprintf("Wrong name %s in remote larva file (expected %s)",
				cfg.GetString("name"), pkgListName))
	}

	pkgList.Name = cfg.GetString("name")
	pkgList.Branch = branchName
	pkgList.RemoteUrl = cfg.GetString("url")

	pkgList.PkgDescs, err = pkgList.fileToPkgList(cfg)
	if err != nil {
		return err
	}

	pkgList.Repo.PkgLists[name] = pkgList

	return nil
}

func (cl *PkgList) Install(name string, url string, branch string) error {
	pkgListFile := cl.GetPkgListFullFile(name, branch)

	// XXX: Should warn if file already exists, and require force option
	os.Remove(pkgListFile)

	// Download the manifest
	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Downloading pkg-list.yml from %s/"+
		"%s...", url, branch)

	if err := dl.DownloadFile(url, branch, "pkg-list.yml",
		pkgListFile); err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	// Load the manifest, and ensure that it is in the correct format
	StatusMessage(VERBOSITY_DEFAULT, "Verifying pkg-list.yml format...\n")
	if err := cl.Load(cl.GetPkgListFile(name, branch)); err != nil {
		os.Remove(pkgListFile)
		return err
	}
	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	return nil
}

func (pkgList *PkgList) InstallPkg(pkgName string, branch string,
	downloaded []*RemoteRepo) ([]*RemoteRepo, error) {
	log.Print("[VERBOSE] Looking for ", pkgName)
	pkg, err := pkgList.ResolvePkgName(pkgName)
	if err == nil {
		log.Printf("[VERBOSE] ", pkgName, " installed already")
		return downloaded, nil
	}
	repo := pkgList.Repo
	for _, remoteRepo := range downloaded {
		pkg, err = remoteRepo.ResolvePkgName(pkgName)
		if err == nil {
			log.Print("[VERBOSE] ", pkgName, " present in downloaded pkgList ",
				remoteRepo.Name)

			err = remoteRepo.fetchPkg(pkgName, repo.BasePath)
			if err != nil {
				return downloaded, err
			}

			// update local pkgList
			err = pkgList.loadPkgDir(repo.BasePath, "", pkgName)
			if err != nil {
				return downloaded, err
			}

			deps, err := pkg.GetDependencies()
			if err != nil {
				return downloaded, err
			}
			for _, dep := range deps {
				log.Print("[VERBOSE] ", pkgName, " checking dependency ",
					dep.Name)
				depBranch := dep.BranchName()
				downloaded, err = pkgList.InstallPkg(dep.Name, depBranch,
					downloaded)
				if err != nil {
					return downloaded, err
				}
			}
			return downloaded, nil
		}
	}

	// Not in downloaded pkgLists
	pkgLists, err := repo.GetPkgLists()
	if err != nil {
		return downloaded, err
	}
	for _, remotePkgList := range pkgLists {
		pkgDesc, err := remotePkgList.ResolvePkgDescName(pkgName)
		if err == nil {
			log.Print("[VERBOSE] ", pkgName, " present in remote pkgList ",
				remotePkgList.Name, remotePkgList.Branch)
			if branch == "" {
				branch = remotePkgList.Branch
			}
			remoteRepo, err := NewRemoteRepo(remotePkgList, branch)
			if err != nil {
				return downloaded, err
			}
			downloaded = append(downloaded, remoteRepo)
			return pkgList.InstallPkg(pkgDesc.FullName, branch, downloaded)
		}
	}

	return downloaded, NewNewtError(fmt.Sprintf("No package %s found\n", pkgName))
}
