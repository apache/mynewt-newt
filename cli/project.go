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

// Structure representing a project
type Project struct {
	// Project name
	Name string

	// Base path of project
	BasePath string

	// Eggs
	Eggs []string

	// Capabilities
	Capabilities []string

	// Assembler compiler flags
	Aflags string

	// Compiler flags
	Cflags string

	// Linker flags
	Lflags string

	// The repository the project is located in
	Nest *Nest

	// The target associated with this project
	Target *Target
}

// Load and initialize a project specified by name
// nest & t are the nest and target to associate the project with
func LoadProject(nest *Nest, t *Target, name string) (*Project, error) {
	p := &Project{
		Name:   name,
		Nest:   nest,
		Target: t,
	}

	StatusMessage(VERBOSITY_VERBOSE,
		"Loading project %s for repo %s, target %s\n",
		name, nest.BasePath, t.Name)

	if err := p.Init(); err != nil {
		return nil, err
	} else {
		return p, nil
	}
}

// Get the packages associated with the project
func (p *Project) GetEggs() []string {
	return p.Eggs
}

// Load project configuration
func (p *Project) loadConfig() error {
	log.Printf("[DEBUG] Reading Project configuration for %s in %s",
		p.Name, p.BasePath)

	v, err := ReadConfig(p.BasePath, p.Name)
	if err != nil {
		return err
	}

	t := p.Target

	p.Eggs = GetStringSliceIdentities(v, t.Identities, "project.eggs")

	idents := GetStringSliceIdentities(v, t.Identities, "project.identities")
	for _, ident := range idents {
		t.Identities[ident] = p.Name
	}
	p.Capabilities = GetStringSliceIdentities(v, t.Identities, "project.caps")

	p.Cflags = GetStringIdentities(v, t.Identities, "project.cflags")
	p.Lflags = GetStringIdentities(v, t.Identities, "project.lflags")
	p.Aflags = GetStringIdentities(v, t.Identities, "project.aflags")

	return nil
}

// Clean the project build, and all packages that were built with the
// project, if cleanAll is true, then clean everything, not just the current
// architecture
func (p *Project) BuildClean(cleanAll bool) error {
	clutch, err := NewClutch(p.Nest)
	if err != nil {
		return err
	}

	// first, clean packages
	StatusMessage(VERBOSITY_VERBOSE,
		"Cleaning all the packages associated with project %s", p.Name)
	for _, eggName := range p.GetEggs() {
		err = clutch.BuildClean(p.Target, eggName, cleanAll)
		if err != nil {
			return err
		}
	}

	// clean the BSP, if it exists
	if p.Target.Bsp != "" {
		if err := clutch.BuildClean(p.Target, p.Target.Bsp, cleanAll); err != nil {
			return err
		}
	}

	c, err := NewCompiler(p.Target.GetCompiler(), p.Target.Cdef, p.Target.Name, []string{})
	if err != nil {
		return err
	}

	tName := p.Target.Name
	if cleanAll {
		tName = ""
	}

	if err := c.RecursiveClean(p.BasePath, tName); err != nil {
		return err
	}

	return nil
}

// Collect identities and capabilities that egg provides
func (p *Project) collectEggDeps(clutch *Clutch, egg *Egg,
	identities map[string]string,
	capabilities map[string]string) error {

	if egg.DepLoaded {
		return nil
	}

	StatusMessage(VERBOSITY_VERBOSE, "  Collecting egg %s dependencies\n", egg.Name)

	err := egg.LoadDeps(identities, capabilities)
	if err != nil {
		return err
	}

	for _, dep := range egg.Deps {
		StatusMessage(VERBOSITY_VERBOSE, "   Egg %s dependency %s\n", egg.Name,
			dep.Name)

		egg, err := clutch.ResolveEggName(dep.Name)
		if err != nil {
			return err
		}
		err = p.collectEggDeps(clutch, egg, identities, capabilities)
		if err != nil {
			return err
		}
	}
	return nil
}

// Collect all identities and capabilities that project has
func (p *Project) collectAllDeps(clutch *Clutch) (map[string]string, map[string]string,
	error) {

	eggList := p.GetEggs()
	if eggList == nil {
		return nil, nil, nil
	}

	StatusMessage(VERBOSITY_VERBOSE, " Collecting all project dependencies\n")

	t := p.Target

	eggList = append(eggList, t.Dependencies...)
	if t.Bsp != "" {
		eggList = append(eggList, t.Bsp)
	}
	identities := map[string]string{}
	capabilities := map[string]string{}

	for _, eggName := range eggList {
		if eggName == "" {
			continue
		}

		egg, err := clutch.ResolveEggName(eggName)
		if err != nil {
			return nil, nil, err
		}

		err = p.collectEggDeps(clutch, egg, identities,
			capabilities)
		if err != nil {
			return nil, nil, err
		}
	}
	return identities, capabilities, nil
}

// Clear the fact that dependencies have been checked
func (p *Project) clearEggDeps(clutch *Clutch, egg *Egg) {

	if egg.DepLoaded == false {
		return
	}
	egg.DepLoaded = false

	for _, dep := range egg.Deps {
		egg, err := clutch.ResolveEggName(dep.Name)
		if err == nil {
			p.clearEggDeps(clutch, egg)
		}
	}
}

func (p *Project) clearAllDeps(clutch *Clutch) {
	eggList := p.GetEggs()
	if eggList == nil {
		return
	}

	t := p.Target

	eggList = append(eggList, t.Dependencies...)
	if t.Bsp != "" {
		eggList = append(eggList, t.Bsp)
	}

	for _, eggName := range eggList {
		if eggName == "" {
			continue
		}
		egg, err := clutch.ResolveEggName(eggName)
		if err != nil {
			return
		}
		p.clearEggDeps(clutch, egg)
	}
}

// Collect project identities and capabilities, and make target ready for
// building.
func (p *Project) collectDeps(clutch *Clutch) error {

	identCount := 0
	capCount := 0

	StatusMessage(VERBOSITY_VERBOSE,
		"Collecting egg dependencies for project %s\n", p.Name)

	// Need to do this multiple times, until there are no new identities,
	// capabilities which show up.
	identities := map[string]string{}
	for {
		identities, capabilities, err := p.collectAllDeps(clutch)
		if err != nil {
			return err
		}
		newIdentCount := len(identities)
		newCapCount := len(capabilities)
		StatusMessage(VERBOSITY_VERBOSE, "Collected idents %d caps %d\n",
			newIdentCount, newCapCount)
		if identCount == newIdentCount && capCount == newCapCount {
			break
		}
		p.clearAllDeps(clutch)
		identCount = newIdentCount
		capCount = newCapCount
	}

	t := p.Target

	for ident, name := range identities {
		t.Identities[ident] = name
	}
	return nil
}

// Build the packages that this project depends on
// clutch is an initialized package manager, incls is an array of includes to
// append to (package includes get append as they are built)
// libs is an array of archive files to append to (package libraries get
// appended as they are built)
func (p *Project) buildDeps(clutch *Clutch, incls *[]string,
	libs *[]string) (map[string]string, error) {
	eggList := p.GetEggs()
	if eggList == nil {
		return nil, nil
	}

	StatusMessage(VERBOSITY_VERBOSE,
		"Building egg dependencies for project %s\n", p.Name)

	t := p.Target

	// Append project variables to target variables, so that all package builds
	// inherit from them
	eggList = append(eggList, t.Dependencies...)
	t.Capabilities = append(t.Capabilities, p.Capabilities...)
	t.Cflags += " " + p.Cflags
	t.Lflags += " " + p.Lflags
	t.Aflags += " " + p.Aflags

	deps := map[string]*DependencyRequirement{}
	reqcaps := map[string]*DependencyRequirement{}
	caps := map[string]*DependencyRequirement{}
	capEggs := map[string]string{}

	// inherit project capabilities, mark these capabilities as supported.
	for _, cName := range t.Capabilities {
		dr, err := NewDependencyRequirementParseString(cName)
		if err != nil {
			return nil, err
		}

		caps[dr.String()] = dr
	}

	for _, eggName := range eggList {
		if eggName == "" {
			continue
		}

		egg, err := clutch.ResolveEggName(eggName)
		if err != nil {
			return nil, err
		}

		if err := clutch.CheckEggDeps(egg, deps, reqcaps, caps, capEggs); err != nil {
			return nil, err
		}
	}

	StatusMessage(VERBOSITY_VERBOSE,
		"Reporting required capabilities for project %s\n", p.Name)
	for dname, dep := range reqcaps {
		StatusMessage(VERBOSITY_VERBOSE,
			"	%s - %s\n", dname, dep.Name)
	}
	StatusMessage(VERBOSITY_VERBOSE,
		"Reporting actual capabilities for project %s\n", p.Name)
	for dname, dep := range caps {
		StatusMessage(VERBOSITY_VERBOSE,
			"	%s - %s ", dname, dep.Name)
		if capEggs[dname] != "" {
			StatusMessage(VERBOSITY_VERBOSE,
				"- %s\n", capEggs[dname])
		} else {
			StatusMessage(VERBOSITY_VERBOSE, "\n")
		}
	}

	// After processing all the dependencies, verify that the package's
	// capability requirements are satisfied as well
	if err := clutch.VerifyCaps(reqcaps, caps); err != nil {
		return nil, err
	}

	// now go through and build everything
	for _, eggName := range eggList {
		if eggName == "" {
			continue
		}

		egg, err := clutch.ResolveEggName(eggName)
		if err != nil {
			return nil, err
		}

		if err = clutch.Build(p.Target, eggName, *incls, libs, capEggs); err != nil {
			return nil, err
		}

		// Don't fail if package did not produce a library file; some packages
		// are header-only.
		if lib := clutch.GetEggLib(p.Target, egg); NodeExist(lib) {
			*libs = append(*libs, lib)
		}

		*incls = append(*incls, egg.Includes...)
	}

	return capEggs, nil
}

// Build the BSP for this project.
// The BSP is specified by the Target attached to the project.
// clutch is an initialized egg mgr, containing all the packages
// incls and libs are pointers to an array of includes and libraries, when buildBsp()
// builds the BSP, it appends the include directories for the BSP, and the archive file
// to these variables.
func (p *Project) buildBsp(clutch *Clutch, incls *[]string,
	libs *[]string, capEggs map[string]string) (string, error) {

	StatusMessage(VERBOSITY_VERBOSE, "Building BSP %s for project %s\n",
		p.Target.Bsp, p.Name)

	if p.Target.Bsp == "" {
		return "", NewNewtError("Must specify a BSP to build project")
	}

	return buildBsp(p.Target, clutch, incls, libs, capEggs)
}

// Build the project
func (p *Project) Build() error {
	clutch, err := NewClutch(p.Nest)
	if err != nil {
		return err
	}

	// Load the configuration for this target
	if err := clutch.LoadConfigs(nil, false); err != nil {
		return err
	}

	incls := []string{}
	libs := []string{}
	linkerScript := ""

	// Collect target identities, libraries to include
	err = p.collectDeps(clutch)
	if err != nil {
		return err
	}

	// If there is a BSP:
	//     1. Calculate the include paths that it and its dependencies export.
	//        This set of include paths is accessible during all subsequent
	//        builds.
	//     2. Build the BSP package.
	if p.Target.Bsp != "" {
		incls, err = BspIncludePaths(clutch, p.Target)
		if err != nil {
			return err
		}
	}

	// Build the project dependencies.
	capEggs, err := p.buildDeps(clutch, &incls, &libs)
	if err != nil {
		return err
	}

	if p.Target.Bsp != "" {
		linkerScript, err = p.buildBsp(clutch, &incls, &libs, capEggs)
		if err != nil {
			return err
		}
	}

	// Append project includes
	projIncls := []string{
		p.BasePath + "/include/",
		p.BasePath + "/arch/" + p.Target.Arch + "/include/",
	}

	incls = append(incls, projIncls...)

	c, err := NewCompiler(p.Target.GetCompiler(), p.Target.Cdef, p.Target.Name,
		incls)
	if err != nil {
		return err
	}

	c.LinkerScript = linkerScript

	// Add target C flags
	c.Cflags = CreateCflags(clutch, c, p.Target, p.Cflags)

	os.Chdir(p.BasePath + "/src/")
	if err = c.Compile("*.c"); err != nil {
		return err
	}

	if !NodeNotExist(p.BasePath + "/src/arch/" + p.Target.Arch + "/") {
		os.Chdir(p.BasePath + "/src/arch/" + p.Target.Arch + "/")
		if err = c.Compile("*.c"); err != nil {
			return err
		}
	}

	StatusMessage(VERBOSITY_DEFAULT, "Building project %s\n", p.Name)

	// Create binaries in the project bin/ directory, under:
	// bin/<arch>/
	binDir := p.BinPath()
	if NodeNotExist(binDir) {
		os.MkdirAll(binDir, 0755)
	}

	options := map[string]bool{"mapFile": c.ldMapFile,
		"listFile": true, "binFile": true}
	err = c.CompileElf(binDir+p.Name, options, libs)
	if err != nil {
		return err
	}

	return nil
}

// Initialize the project, and project definition
func (p *Project) Init() error {
	p.BasePath = p.Nest.BasePath + "/project/" + p.Name + "/"
	if NodeNotExist(p.BasePath) {
		return NewNewtError("Project directory does not exist")
	}

	if err := p.loadConfig(); err != nil {
		return err
	}
	return nil
}

// Return path to target binary
func (p *Project) BinPath() string {
	return p.BasePath + "/bin/" + p.Target.Name + "/"
}
