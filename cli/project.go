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
	"strings"
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

	log.Printf("[DEBUG] Loading project %s for repo %s, target %s",
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

	p.Eggs = GetStringSliceIdentities(v, t, "project.eggs")

	idents := GetStringSliceIdentities(v, t, "project.identities")
	t.Identities = append(t.Identities, idents...)

	p.Capabilities = GetStringSliceIdentities(v, t, "project.caps")

	p.Cflags = GetStringIdentities(v, t, "project.cflags")
	p.Lflags = GetStringIdentities(v, t, "project.lflags")
	p.Aflags = GetStringIdentities(v, t, "project.aflags")

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
	log.Printf("[DEBUG] Cleaning all the packages associated with project %s",
		p.Name)
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

// Build the packages that this project depends on
// clutch is an initialized package manager, incls is an array of includes to
// append to (package includes get append as they are built)
// libs is an array of archive files to append to (package libraries get
// appended as they are built)
func (p *Project) buildDeps(clutch *Clutch, incls *[]string, libs *[]string) error {
	eggList := p.GetEggs()
	if eggList == nil {
		return nil
	}

	log.Printf("[INFO] Building package dependencies for project %s", p.Name)

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

	// inherit project capabilities, mark these capabilities as supported.
	for _, cName := range t.Capabilities {
		dr, err := NewDependencyRequirementParseString(cName)
		if err != nil {
			return err
		}

		caps[dr.String()] = dr
	}

	for _, eggName := range eggList {
		if eggName == "" {
			continue
		}

		egg, err := clutch.ResolveEggName(eggName)
		if err != nil {
			return err
		}

		if err := clutch.CheckEggDeps(egg, deps, reqcaps, caps); err != nil {
			return err
		}
	}

	// After processing all the dependencies, verify that the package's capability
	// requirements are satisfies as well
	if err := clutch.VerifyCaps(reqcaps, caps); err != nil {
		return err
	}

	// now go through and build everything
	for _, eggName := range eggList {
		if eggName == "" {
			continue
		}

		egg, err := clutch.ResolveEggName(eggName)
		if err != nil {
			return err
		}

		if err = clutch.Build(p.Target, eggName, *incls, libs); err != nil {
			return err
		}

		// Don't fail if package did not produce a library file; some packages
		// are header-only.
		if lib := clutch.GetEggLib(p.Target, egg); NodeExist(lib) {
			*libs = append(*libs, lib)
		}

		*incls = append(*incls, egg.Includes...)
	}

	return nil
}

// Build the BSP for this project.
// The BSP is specified by the Target attached to the project.
// clutch is an initialized egg mgr, containing all the packages
// incls and libs are pointers to an array of includes and libraries, when buildBsp()
// builds the BSP, it appends the include directories for the BSP, and the archive file
// to these variables.
func (p *Project) buildBsp(clutch *Clutch, incls *[]string,
	libs *[]string) (string, error) {

	log.Printf("[INFO] Building BSP %s for Project %s", p.Target.Bsp, p.Name)

	if p.Target.Bsp == "" {
		return "", NewNewtError("Must specify a BSP to build project")
	}

	return buildBsp(p.Target, clutch, incls, libs)
}

// Build the project
func (p *Project) Build() error {
	log.Printf("[INFO] Building project %s", p.Name)

	clutch, err := NewClutch(p.Nest)
	if err != nil {
		return err
	}

	// Load the configuration for this target
	if err := clutch.LoadConfigs(p.Target, false); err != nil {
		return err
	}

	incls := []string{}
	libs := []string{}
	linkerScript := ""

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
		linkerScript, err = p.buildBsp(clutch, &incls, &libs)
		if err != nil {
			return err
		}
	}

	// Build the project dependencies.
	if err := p.buildDeps(clutch, &incls, &libs); err != nil {
		return err
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

	// Create binaries in the project bin/ directory, under:
	// bin/<arch>/
	binDir := p.BasePath + "/bin/" + p.Target.Name + "/"
	if NodeNotExist(binDir) {
		os.MkdirAll(binDir, 0755)
	}
	log.Printf("[DEBUG] Compiling a binary %s from libs %s", binDir+p.Name,
		strings.Join(libs, " "))

	options := map[string]bool{"mapFile": c.ldMapFile,
		"listFile": true, "binFile": true}
	err = c.CompileElf(binDir+p.Name, options, strings.Join(libs, " "))
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
