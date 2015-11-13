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

	Branch string
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
	caps map[string]*DependencyRequirement,
	capEggs map[string]string) error {

	for _, depReq := range egg.Deps {
		// don't process this package if we've already processed it
		if _, ok := deps[depReq.String()]; ok {
			continue
		}

		eggName := egg.Name
		StatusMessage(VERBOSITY_VERBOSE,
			"Checking dependency %s for package %s\n", depReq.Name, eggName)
		egg, ok := clutch.Eggs[depReq.Name]
		if !ok {
			return NewNewtError(
				fmt.Sprintf("No package dependency %s found for %s",
					depReq.Name, eggName))
		}

		if ok := depReq.SatisfiesDependency(egg); !ok {
			return NewNewtError(fmt.Sprintf("Egg %s doesn't satisfy dependency %s",
				egg.Name, depReq))
		}

		// We've checked this dependency requirement, all is gute!
		deps[depReq.String()] = depReq
	}

	for _, reqCap := range egg.ReqCapabilities {
		reqcap[reqCap.String()] = reqCap
	}

	for _, cap := range egg.Capabilities {
		if caps[cap.String()] != nil && capEggs[cap.String()] != egg.FullName {
			return NewNewtError(fmt.Sprintf("Multiple eggs with capability %s",
				cap.String()))
		}
		caps[cap.String()] = cap
		if capEggs != nil {
			capEggs[cap.String()] = egg.FullName
		}
	}

	// Now go through and recurse through the sub-package dependencies
	for _, depReq := range egg.Deps {
		if _, ok := deps[depReq.String()]; ok {
			continue
		}

		if err := clutch.CheckEggDeps(clutch.Eggs[depReq.Name], deps,
			reqcap, caps, capEggs); err != nil {
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

		if err := clutch.CheckEggDeps(egg, deps, reqcap, caps, nil); err != nil {
			return err
		}
	}

	return nil
}

// Load an individual package specified by eggName into the package list for
// this repository
func (clutch *Clutch) loadEgg(eggDir string, eggPrefix string,
	eggName string) error {
	StatusMessage(VERBOSITY_VERBOSE, "Loading Egg "+eggDir+"...\n")

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
	log.Printf("[DEBUG] Loading eggs in %s, starting with egg %s",
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
		"compiler/",
		"libs/",
		"net/",
		"hw/bsp/",
		"hw/mcu/",
		"hw/mcu/stm",
		"hw/drivers/",
		"hw/",
		"project/",
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
	if err := clutch.loadEggs(); err != nil {
		return err
	}

	clutch.EggShells = map[string]*EggShell{}

	return nil
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

	StatusMessage(VERBOSITY_VERBOSE,
		"Building egg dependencies for %s, target %s\n", egg.Name, t.Name)

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

	if err := egg.LoadConfig(t, false); err != nil {
		return err
	}

	// already built the package, no need to rebuild.  This is to handle
	// recursive calls to Build()
	if egg.Built {
		return nil
	}
	egg.Built = true

	if err := clutch.buildDeps(egg, t, &incls, libs); err != nil {
		return err
	}

	StatusMessage(VERBOSITY_VERBOSE, "Building egg %s for arch %s\n",
		eggName, t.Arch)

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

	log.Printf("[DEBUG] compiling src eggs in base egg directory: %s", srcDir)

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
	StatusMessage(VERBOSITY_DEFAULT, "Testing egg %s for arch %s\n",
		egg.Name, t.Arch)

	if err := os.Chdir(egg.BasePath + "/src/test/bin/" + t.Name +
		"/"); err != nil {
		return err
	}

	o, err := ShellCommand("./" + egg.TestBinName())
	if err != nil {
		StatusMessage(VERBOSITY_DEFAULT, "%s", string(o))

		// Always terminate on test failure since only one test is being run.
		return NewtErrorNoTrace(fmt.Sprintf("Test %s failed",
			egg.TestBinName()))
	} else {
		StatusMessage(VERBOSITY_VERBOSE, "%s", string(o))
		StatusMessage(VERBOSITY_DEFAULT, "Test %s ok!\n", egg.TestBinName())
		return nil
	}
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

	// The 'test' identity is implicitly exported during a package test.
	t.Identities["test"] = "test";

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

	// The egg under test must be compiled with the PKG_TEST symbol defined so
	// that the appropriate main function gets built.
	egg.Cflags += " -DPKG_TEST"

	incls := []string{}
	libs := []string{}

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
		_, err = buildBsp(t, clutch, &incls, &libs, nil)
		if err != nil {
			return err
		}
	}

	// Build the package under test.
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
	var err error
	for _, egg := range local.Eggs {
		if err := egg.LoadConfig(nil, false); err != nil {
			return err
		}

		log.Printf("[DEBUG] Egg %s loaded, putting it into clutch %s",
			egg.FullName, local.Name)

		eggShell := &EggShell{}
		eggShell.FullName = egg.FullName
		eggShell.Deps = egg.Deps
		eggShell.Caps = egg.Capabilities
		eggShell.ReqCaps = egg.ReqCapabilities
		eggShell.Version = egg.Version
		eggShell.Hash, err = egg.GetHash()
		if err != nil {
			return err
		}

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
		eggShell, err := NewEggShell(cl)
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

// Create the manifest file name, it's the manifest dir + manifest name +
// branch and a.yml extension
func (clutch *Clutch) GetClutchFile(name string, branch string) string {
	return name + "@" + branch
}

func (clutch *Clutch) GetClutchFullFile(name string, branch string) string {
	return clutch.Nest.ClutchPath + clutch.GetClutchFile(name, branch) + ".yml"
}

func (clutch *Clutch) Load(name string) error {
	cfg, err := ReadConfig(clutch.Nest.ClutchPath, name)
	if err != nil {
		return err
	}

	clutchName := name
	branchName := "master"

	parts := strings.Split(name, "@")
	if len(parts) == 2 {
		clutchName = parts[0]
		branchName = parts[1]
	}

	if cfg.GetString("name") != clutchName {
		return NewNewtError(
			fmt.Sprintf("Wrong name %s in remote larva file (expected %s)",
				cfg.GetString("name"), clutchName))
	}

	clutch.Name = cfg.GetString("name")
	clutch.Branch = branchName
	clutch.RemoteUrl = cfg.GetString("url")

	clutch.EggShells, err = clutch.fileToEggList(cfg)
	if err != nil {
		return err
	}

	clutch.Nest.Clutches[name] = clutch

	return nil
}

func (cl *Clutch) Install(name string, url string, branch string) error {
	clutchFile := cl.GetClutchFullFile(name, branch)

	// XXX: Should warn if file already exists, and require force option
	os.Remove(clutchFile)

	// Download the manifest
	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Downloading clutch.yml from %s/"+
		"%s...", url, branch)

	if err := dl.DownloadFile(url, branch, "clutch.yml",
		clutchFile); err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	// Load the manifest, and ensure that it is in the correct format
	StatusMessage(VERBOSITY_DEFAULT, "Verifying clutch.yml format...\n")
	if err := cl.Load(cl.GetClutchFile(name, branch)); err != nil {
		os.Remove(clutchFile)
		return err
	}
	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	return nil
}

func (clutch *Clutch) InstallEgg(eggName string, branch string,
	downloaded []*RemoteNest) ([]*RemoteNest, error) {
	log.Print("[VERBOSE] Looking for ", eggName)
	egg, err := clutch.ResolveEggName(eggName)
	if err == nil {
		log.Printf("[VERBOSE] ", eggName, " installed already")
		return downloaded, nil
	}
	nest := clutch.Nest
	for _, remoteNest := range downloaded {
		egg, err = remoteNest.ResolveEggName(eggName)
		if err == nil {
			log.Print("[VERBOSE] ", eggName, " present in downloaded clutch ",
				remoteNest.Name)

			err = remoteNest.fetchEgg(eggName, nest.BasePath)
			if err != nil {
				return downloaded, err
			}

			// update local clutch
			err = clutch.loadEggDir(nest.BasePath, "", eggName)
			if err != nil {
				return downloaded, err
			}

			deps, err := egg.GetDependencies()
			if err != nil {
				return downloaded, err
			}
			for _, dep := range deps {
				log.Print("[VERBOSE] ", eggName, " checking dependency ",
					dep.Name)
				depBranch := dep.BranchName()
				downloaded, err = clutch.InstallEgg(dep.Name, depBranch,
					downloaded)
				if err != nil {
					return downloaded, err
				}
			}
			return downloaded, nil
		}
	}

	// Not in downloaded clutches
	clutches, err := nest.GetClutches()
	if err != nil {
		return downloaded, err
	}
	for _, remoteClutch := range clutches {
		eggShell, err := remoteClutch.ResolveEggShellName(eggName)
		if err == nil {
			log.Print("[VERBOSE] ", eggName, " present in remote clutch ",
				remoteClutch.Name, remoteClutch.Branch)
			if branch == "" {
				branch = remoteClutch.Branch
			}
			remoteNest, err := NewRemoteNest(remoteClutch, branch)
			if err != nil {
				return downloaded, err
			}
			downloaded = append(downloaded, remoteNest)
			return clutch.InstallEgg(eggShell.FullName, branch, downloaded)
		}
	}

	return downloaded, NewNewtError(fmt.Sprintf("No package %s found\n", eggName))
}
