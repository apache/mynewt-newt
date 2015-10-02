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
	"bytes"
	"fmt"
	"github.com/spf13/viper"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Clutch struct {
	// Nestsitory associated with the Eggs
	Nest *Nest

	// List of packages for Nest
	Eggs map[string]*Egg

	EggShells map[string]*EggShell

	Name string

	LarvaFile string

	RemoteUrl string
}

// Allocate a new package manager structure, and initialize it.
func NewClutch(nest *Nest) (*Clutch, error) {
	clutch := &Clutch{
		Nest: nest,
	}
	err := clutch.Init()

	return clutch, err
}

func (clutch *Clutch) LoadConfigs(t *Target, force bool) error {
	for _, egg := range clutch.Eggs {
		if err := egg.LoadConfig(t, force); err != nil {
			return err
		}
	}
	return nil
}

func (clutch *Clutch) CheckEggDeps(egg *Egg,
	deps map[string]*DependencyRequirement,
	reqcap map[string]*DependencyRequirement,
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

		log.Printf("[DEBUG] Checking dependency %s for package %s", depReq,
			egg.Name)
		egg, ok := clutch.Eggs[depReq.Name]
		if !ok {
			return NewNewtError(
				fmt.Sprintf("No package dependency %s found for %s",
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

		if err := clutch.CheckEggDeps(clutch.Eggs[depReq.Name], deps,
			reqcap, caps); err != nil {
			return err
		}
	}

	return nil
}

func (clutch *Clutch) VerifyCaps(reqcaps map[string]*DependencyRequirement,
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

func (clutch *Clutch) CheckDeps() error {
	// Go through all the packages and check that their dependencies are satisfied
	for _, egg := range clutch.Eggs {
		deps := map[string]*DependencyRequirement{}
		reqcap := map[string]*DependencyRequirement{}
		caps := map[string]*DependencyRequirement{}

		if err := clutch.CheckEggDeps(egg, deps, reqcap, caps); err != nil {
			return err
		}
	}

	return nil
}

// Load an individual package specified by eggName into the package list for
// this repository
func (clutch *Clutch) loadEgg(eggDir string, eggPrefix string,
	eggName string) error {
	log.Println("[INFO] Loading Egg " + eggDir + "...")

	if clutch.Eggs == nil {
		clutch.Eggs = make(map[string]*Egg)
	}

	egg, err := NewEgg(clutch.Nest, eggDir)
	if err != nil {
		return nil
	}

	clutch.Eggs[eggPrefix+eggName] = egg

	return nil
}

func (clutch *Clutch) String() string {
	str := ""
	for eggName, _ := range clutch.Eggs {
		str += eggName + " "
	}
	return str
}

// Recursively load a package.  Given the baseDir of the packages (e.g. egg/ or
// hw/bsp), and the base package name.
func (clutch *Clutch) loadEggDir(baseDir string, eggPrefix string,
	eggName string) error {
	log.Printf("[DEBUG] Loading packages in %s, starting with package %s",
		baseDir, eggName)

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
			if err := clutch.loadEggDir(baseDir, eggPrefix,
				eggName+"/"+name); err != nil {
				return err
			}
		}
	}

	if NodeNotExist(baseDir + "/" + eggName + "/egg.yml") {
		return nil
	}

	return clutch.loadEgg(baseDir+"/"+eggName, eggPrefix, eggName)
}

// Load all the packages in the repository into the package structure
func (clutch *Clutch) loadEggs() error {
	nest := clutch.Nest

	// Multiple package directories to be searched
	searchDirs := []string{
		"libs/",
		"hw/bsp/",
		"hw/mcu/",
		"hw/mcu/stm",
		"hw/drivers/",
		"hw/",
	}

	for _, eggDir := range searchDirs {
		eggBaseDir := nest.BasePath + "/" + eggDir

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

			if err = clutch.loadEggDir(eggBaseDir, eggDir, name); err != nil {
				return err
			}
		}
	}

	return nil
}

// Initialize the package manager
func (clutch *Clutch) Init() error {
	err := clutch.loadEggs()
	return err
}

// Resolve the package specified by eggName into a package structure.
func (clutch *Clutch) ResolveEggName(eggName string) (*Egg, error) {
	egg, ok := clutch.Eggs[eggName]
	if !ok {
		return nil, NewNewtError(fmt.Sprintf("Invalid egg '%s' specified "+
			"(eggs = %s)", eggName, clutch))
	}
	return egg, nil
}

func (clutch *Clutch) ResolveEggShellName(eggName string) (*EggShell, error) {
	eggShell, ok := clutch.EggShells[eggName]
	if !ok {
		return nil, NewNewtError(fmt.Sprintf("Invalid egg '%s' specified "+
			"(eggs = %s)", eggName, clutch))
	}
	return eggShell, nil
}

func (clutch *Clutch) ResolveEggDir(eggDir string) (*Egg, error) {
	eggDir = filepath.Clean(eggDir)
	for name, egg := range clutch.Eggs {
		if filepath.Clean(egg.BasePath) == eggDir {
			return clutch.Eggs[name], nil
		}
	}
	return nil, NewNewtError(fmt.Sprintf("Cannot resolve package dir %s in "+
		"package manager", eggDir))
}

// Clean the build for the package specified by eggName.   if cleanAll is
// specified, all architectures are cleaned.
func (clutch *Clutch) BuildClean(t *Target, eggName string, cleanAll bool) error {
	egg, err := clutch.ResolveEggName(eggName)
	if err != nil {
		return err
	}

	if err := egg.LoadConfig(t, false); err != nil {
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
		if err := clutch.BuildClean(t, dep.Name, cleanAll); err != nil {
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

func (clutch *Clutch) GetEggLib(t *Target, egg *Egg) string {
	libDir := egg.BasePath + "/bin/" + t.Name + "/" +
		"lib" + filepath.Base(egg.Name) + ".a"
	return libDir
}

// @param incls                 Extra include paths that get specified during
//                                  build; not modified by this function.
// @param libs                  List of libraries that have been built so far;
//                                  This function appends entries to this list.
func (clutch *Clutch) buildDeps(egg *Egg, t *Target, incls *[]string,
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
		degg, err := clutch.ResolveEggName(dep.Name)
		if err != nil {
			return err
		}

		// Build the package
		if err = clutch.Build(t, dep.Name, *incls, libs); err != nil {
			return err
		}

		// After build, get dependency package includes.  Build function
		// generates all the package includes
		egg.Includes = append(egg.Includes, degg.Includes...)
		if lib := clutch.GetEggLib(t, degg); NodeExist(lib) {
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
// @param incls            Extra include paths that get specified during
//                             build.  Note: passed by value.
// @param lib              List of libraries that have been built so far;
//                             This function appends entries to this list.
func (clutch *Clutch) Build(t *Target, eggName string, incls []string,
	libs *[]string) error {

	// Look up package structure
	egg, err := clutch.ResolveEggName(eggName)
	if err != nil {
		return err
	}
	//XXX: This overwrites the PKG_TEST define
	if err := egg.LoadConfig(t, false); err != nil {
		return err
	}

	log.Println("[INFO] Building package " + eggName + " for arch " + t.Arch)

	// already built the package, no need to rebuild.  This is to handle
	// recursive calls to Build()
	if egg.Built {
		return nil
	}
	egg.Built = true

	StatusMessage(VERBOSITY_DEFAULT, "Building egg %s\n", eggName)

	if err := clutch.buildDeps(egg, t, &incls, libs); err != nil {
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
	c.Cflags = CreateCflags(clutch, c, t, egg.Cflags)
	c.Lflags += " " + egg.Lflags + " " + t.Lflags
	c.Aflags += " " + egg.Aflags + " " + t.Aflags

	log.Printf("[DEBUG] compiling src packages in base package "+
		"directories: %s", srcDir)

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

	if err = c.CompileArchive(clutch.GetEggLib(t, egg), []string{}); err != nil {
		return err
	}

	return nil
}

// Check the include directories for the package, to make sure there are
// no conflicts in include paths for source code
func (clutch *Clutch) checkIncludes(egg *Egg) error {
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

		if filepath.Base(incl) != egg.Name {
			if egg.IsBsp && filepath.Base(incl) != "bsp" {
				bad = true
			}
		}

		if bad {
			return NewNewtError(fmt.Sprintf("File %s should not exist"+
				"in include directory, only file allowed in include "+
				"directory is a directory with the package name %s",
				incl, egg.Name))
		}
	}

	return nil
}

// Clean the tests in the tests parameter, for the package identified by
// eggName.  If cleanAll is set to true, all architectures will be removed.
func (clutch *Clutch) TestClean(t *Target, eggName string,
	cleanAll bool) error {
	egg, err := clutch.ResolveEggName(eggName)
	if err != nil {
		return err
	}

	if err := egg.LoadConfig(t, false); err != nil {
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
func (clutch *Clutch) linkTests(t *Target, egg *Egg,
	incls []string, libs *[]string) error {

	c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, incls)
	if err != nil {
		return err
	}

	// Configure Lflags.  Since we are only linking, Cflags and Aflags are
	// unnecessary.
	c.Lflags += " " + egg.Lflags + " " + t.Lflags

	testBinDir := egg.BasePath + "/src/test/bin/" + t.Name + "/"
	binFile := testBinDir + egg.TestBinName()
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

// Run all the tests in the tests parameter.  egg is the package to check for
// the tests.  exitOnFailure specifies whether to exit immediately when a
// test fails, or continue executing all tests.
func (clutch *Clutch) runTests(t *Target, egg *Egg, exitOnFailure bool) error {
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
func (clutch *Clutch) testsExist(egg *Egg) error {
	dirName := egg.BasePath + "/src/test/"
	if NodeNotExist(dirName) {
		return NewNewtError("No test exists for package " + egg.Name)
	}

	return nil
}

// Test the package identified by eggName, by executing the tests specified.
// exitOnFailure signifies whether to stop the test program when one of them
// fails.
func (clutch *Clutch) Test(t *Target, eggName string,
	exitOnFailure bool) error {
	log.Printf("[INFO] Testing egg %s for arch %s", eggName, t.Arch)

	egg, err := clutch.ResolveEggName(eggName)
	if err != nil {
		return err
	}

	if err := egg.LoadConfig(t, false); err != nil {
		return err
	}

	// Make sure the test directories exist
	if err := clutch.testsExist(egg); err != nil {
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
		incls, err = BspIncludePaths(clutch, t)
		if err != nil {
			return err
		}
		_, err = buildBsp(t, clutch, &incls, &libs)
		if err != nil {
			return err
		}
	}

	// Build the package under test.  This must be compiled with the PKG_TEST
	// symbol defined so that the appropriate main function gets built.
	egg.Cflags += " -DPKG_TEST"
	if err := clutch.Build(t, eggName, incls, &libs); err != nil {
		return err
	}
	lib := clutch.GetEggLib(t, egg)
	if !NodeExist(lib) {
		return NewNewtError("Egg " + eggName + " did not produce binary")
	}
	libs = append(libs, lib)

	// Compile the package's test code.
	if err := clutch.linkTests(t, egg, incls, &libs); err != nil {
		return err
	}

	// Run the tests.
	if err := clutch.runTests(t, egg, exitOnFailure); err != nil {
		return err
	}

	return nil
}

func (cl *Clutch) LoadFromClutch(local *Clutch) error {
	for _, egg := range local.Eggs {
		if err := egg.LoadConfig(nil, false); err != nil {
			return err
		}

		eggShell := &EggShell{}
		eggShell.FullName = egg.FullName
		eggShell.Deps = egg.Deps
		eggShell.Caps = egg.Capabilities
		eggShell.ReqCaps = egg.ReqCapabilities
		eggShell.Version = egg.Version

		cl.EggShells[eggShell.FullName] = eggShell
	}

	return nil
}

func (cl *Clutch) Serialize() (string, error) {
	clStr := "name: " + cl.Name + "\n"
	clStr = clStr + "url: " + cl.RemoteUrl + "\n"
	clStr = clStr + "eggs:\n"

	buf := bytes.Buffer{}

	indent := "    "
	for _, eggShell := range cl.EggShells {
		buf.WriteString(eggShell.Serialize(indent))
	}

	return clStr + buf.String(), nil
}

func (cl *Clutch) strSliceToDr(list []string) ([]*DependencyRequirement, error) {
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

func (cl *Clutch) fileToEggList(cfg *viper.Viper) (map[string]*EggShell,
	error) {
	eggMap := cfg.GetStringMap("eggs")

	eggList := map[string]*EggShell{}

	for name, _ := range eggMap {
		eggShell, err := NewEggShell()
		if err != nil {
			return nil, err
		}
		eggShell.FullName = name

		eggDef := cfg.GetStringMap("eggs." + name)
		eggShell.Version, err = NewVersParseString(eggDef["vers"].(string))
		if err != nil {
			return nil, err
		}

		eggShell.Deps, err = cl.strSliceToDr(
			cfg.GetStringSlice("eggs." + name + ".deps"))
		if err != nil {
			return nil, err
		}

		eggShell.Caps, err = cl.strSliceToDr(
			cfg.GetStringSlice("eggs." + name + ".caps"))
		if err != nil {
			return nil, err
		}

		eggShell.ReqCaps, err = cl.strSliceToDr(
			cfg.GetStringSlice("eggs." + name + ".req_caps"))
		if err != nil {
			return nil, err
		}

		eggList[name] = eggShell
	}

	return eggList, nil
}

// Create the manifest file name, it's the manifest dir + manifest name and a
// .yml extension
func (clutch *Clutch) GetClutchFile(name string) string {
	return clutch.Nest.ClutchPath + name + ".yml"
}

func (clutch *Clutch) Load(name string) error {
	cfg, err := ReadConfig(clutch.Nest.ClutchPath, name)
	if err != nil {
		return nil
	}

	if cfg.GetString("name") != name {
		return NewNewtError(
			fmt.Sprintf("Wrong name %s in remote larva file (expected %s)",
				cfg.GetString("name"), name))
	}

	clutch.Name = cfg.GetString("name")
	clutch.RemoteUrl = cfg.GetString("url")

	clutch.EggShells, err = clutch.fileToEggList(cfg)
	if err != nil {
		return err
	}

	clutch.Nest.Clutches[name] = clutch

	return nil
}

func (cl *Clutch) Install(name string, url string) error {
	clutchFile := cl.GetClutchFile(name)

	// XXX: Should warn if file already exists, and require force option
	os.Remove(clutchFile)

	// Download the manifest
	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Downloading clutch.yml from %s/"+
		"master...", url)

	if err := dl.DownloadFile(url, "master", "clutch.yml",
		clutchFile); err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	// Load the manifest, and ensure that it is in the correct format
	StatusMessage(VERBOSITY_DEFAULT, "Verifying clutch.yml format...")
	if err := cl.Load(name); err != nil {
		return err
	}
	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	return nil
}
