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
	"log"
	"os"
)

// Structure representing a project
type Project struct {
	// Project name
	Name string

	// Base path of project
	BasePath string

	// Pkgs
	Pkgs []string

	// Capabilities
	Capabilities []string

	// Assembler compiler flags
	Aflags string

	// Compiler flags
	Cflags string

	// Linker flags
	Lflags string

	// The repository the project is located in
	Repo *Repo

	// The target associated with this project
	Target *Target
}

// Load and initialize a project specified by name
// repo & t are the repo and target to associate the project with
func LoadProject(repo *Repo, t *Target, name string) (*Project, error) {
	p := &Project{
		Name:   name,
		Repo:   repo,
		Target: t,
	}

	StatusMessage(VERBOSITY_VERBOSE,
		"Loading project %s for repo %s, target %s\n",
		name, repo.BasePath, t.Name)

	if err := p.Init(); err != nil {
		return nil, err
	} else {
		return p, nil
	}
}

// Get the packages associated with the project
func (p *Project) GetPkgs() []string {
	return p.Pkgs
}

// Load project configuration
func (p *Project) loadConfig() error {
	log.Printf("[DEBUG] Reading Project configuration for %s in %s",
		p.Name, p.BasePath)

	v, err := ReadConfig(p.BasePath, "pkg")
	if err != nil {
		return err
	}

	t := p.Target

	p.Pkgs = GetStringSliceIdentities(v, t.Identities, "pkg.deps")

	idents := GetStringSliceIdentities(v, t.Identities, "pkg.identities")
	for _, ident := range idents {
		t.Identities[ident] = p.Name
	}
	p.Capabilities = GetStringSliceIdentities(v, t.Identities, "pkg.caps")

	p.Cflags = GetStringIdentities(v, t.Identities, "pkg.cflags")
	p.Lflags = GetStringIdentities(v, t.Identities, "pkg.lflags")
	p.Aflags = GetStringIdentities(v, t.Identities, "pkg.aflags")

	return nil
}

// Clean the project build, and all packages that were built with the
// project, if cleanAll is true, then clean everything, not just the current
// architecture
func (p *Project) BuildClean(cleanAll bool) error {
	pkgList, err := NewPkgList(p.Repo)
	if err != nil {
		return err
	}

	// first, clean packages
	StatusMessage(VERBOSITY_VERBOSE,
		"Cleaning all the packages associated with project %s", p.Name)
	for _, pkgName := range p.GetPkgs() {
		err = pkgList.BuildClean(p.Target, pkgName, cleanAll)
		if err != nil {
			return err
		}
	}

	// clean the BSP, if it exists
	if p.Target.Bsp != "" {
		if err := pkgList.BuildClean(p.Target, p.Target.Bsp, cleanAll); err != nil {
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

// Collect all identities and capabilities that project has
func (p *Project) collectAllDeps(pkgList *PkgList, identities map[string]string,
	capabilities map[string]string) error {

	pkgDepList := p.GetPkgs()
	if pkgDepList == nil {
		return nil
	}

	StatusMessage(VERBOSITY_VERBOSE, " Collecting all project dependencies\n")

	t := p.Target

	pkgDepList = append(pkgDepList, t.Dependencies...)
	if t.Bsp != "" {
		pkgDepList = append(pkgDepList, t.Bsp)
	}

	for _, pkgName := range pkgDepList {
		if pkgName == "" {
			continue
		}

		pkg, err := pkgList.ResolvePkgName(pkgName)
		if err != nil {
			return err
		}

		err = pkg.collectDependencies(pkgList, identities, capabilities)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Project) clearAllDeps(pkgList *PkgList) {
	pkgDepList := p.GetPkgs()
	if pkgDepList == nil {
		return
	}

	t := p.Target

	pkgDepList = append(pkgDepList, t.Dependencies...)
	if t.Bsp != "" {
		pkgDepList = append(pkgDepList, t.Bsp)
	}

	for _, pkgName := range pkgDepList {
		if pkgName == "" {
			continue
		}
		pkg, err := pkgList.ResolvePkgName(pkgName)
		if err != nil {
			return
		}
		pkg.clearDependencyMarker(pkgList)
	}
}

// Collect project identities and capabilities, and make target ready for
// building.
func (p *Project) collectDeps(pkgList *PkgList) error {

	identCount := 0
	capCount := 0

	t := p.Target

	StatusMessage(VERBOSITY_VERBOSE,
		"Collecting pkg dependencies for project %s\n", p.Name)

	// Need to do this multiple times, until there are no new identities,
	// capabilities which show up.
	identities := t.Identities
	capabilities := map[string]string{}
	for {
		err := p.collectAllDeps(pkgList, identities, capabilities)
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
		p.clearAllDeps(pkgList)
		identCount = newIdentCount
		capCount = newCapCount
	}

	return nil
}

// Build the packages that this project depends on
// pkgList is an initialized package manager, incls is an array of includes to
// append to (package includes get append as they are built)
// libs is an array of archive files to append to (package libraries get
// appended as they are built)
func (p *Project) buildDeps(pkgList *PkgList, incls *[]string,
	libs *[]string) (map[string]string, error) {
	pkgDepList := p.GetPkgs()
	if pkgDepList == nil {
		return nil, nil
	}

	StatusMessage(VERBOSITY_VERBOSE,
		"Building pkg dependencies for project %s\n", p.Name)

	t := p.Target

	// Append project variables to target variables, so that all package builds
	// inherit from them
	pkgDepList = append(pkgDepList, t.Dependencies...)
	t.Capabilities = append(t.Capabilities, p.Capabilities...)
	t.Cflags += " " + p.Cflags
	t.Lflags += " " + p.Lflags
	t.Aflags += " " + p.Aflags

	deps := map[string]*DependencyRequirement{}
	reqcaps := map[string]*DependencyRequirement{}
	caps := map[string]*DependencyRequirement{}
	capPkgs := map[string]string{}

	// inherit project capabilities, mark these capabilities as supported.
	for _, cName := range t.Capabilities {
		dr, err := NewDependencyRequirementParseString(cName)
		if err != nil {
			return nil, err
		}

		caps[dr.String()] = dr
	}

	for _, pkgName := range pkgDepList {
		if pkgName == "" {
			continue
		}

		pkg, err := pkgList.ResolvePkgName(pkgName)
		if err != nil {
			return nil, err
		}

		if err := pkgList.CheckPkgDeps(pkg, deps, reqcaps, caps, capPkgs); err != nil {
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
		if capPkgs[dname] != "" {
			StatusMessage(VERBOSITY_VERBOSE,
				"- %s\n", capPkgs[dname])
		} else {
			StatusMessage(VERBOSITY_VERBOSE, "\n")
		}
	}

	// After processing all the dependencies, verify that the package's
	// capability requirements are satisfied as well
	if err := pkgList.VerifyCaps(reqcaps, caps); err != nil {
		return nil, err
	}

	// now go through and build everything
	for _, pkgName := range pkgDepList {
		if pkgName == "" {
			continue
		}

		pkg, err := pkgList.ResolvePkgName(pkgName)
		if err != nil {
			return nil, err
		}

		if err = pkgList.Build(p.Target, pkgName, *incls, libs); err != nil {
			return nil, err
		}

		// Don't fail if package did not produce a library file; some packages
		// are header-only.
		if lib := pkgList.GetPkgLib(p.Target, pkg); NodeExist(lib) {
			*libs = append(*libs, lib)
		}

		*incls = append(*incls, pkg.Includes...)
	}

	return capPkgs, nil
}

// Build the BSP for this project.
// The BSP is specified by the Target attached to the project.
// pkgList is an initialized pkg mgr, containing all the packages
// incls and libs are pointers to an array of includes and libraries, when buildBsp()
// builds the BSP, it appends the include directories for the BSP, and the archive file
// to these variables.
func (p *Project) buildBsp(pkgList *PkgList, incls *[]string,
	libs *[]string, capPkgs map[string]string) (string, error) {

	StatusMessage(VERBOSITY_VERBOSE, "Building BSP %s for project %s\n",
		p.Target.Bsp, p.Name)

	if p.Target.Bsp == "" {
		return "", NewNewtError("Must specify a BSP to build project")
	}

	return buildBsp(p.Target, pkgList, incls, libs, capPkgs)
}

// Build the project
func (p *Project) Build() error {
	pkgList, err := NewPkgList(p.Repo)
	if err != nil {
		return err
	}

	// Load the configuration for this target
	if err := pkgList.LoadConfigs(nil, false); err != nil {
		return err
	}

	incls := []string{}
	libs := []string{}
	linkerScript := ""

	// Collect target identities, libraries to include
	err = p.collectDeps(pkgList)
	if err != nil {
		return err
	}

	// If there is a BSP:
	//     1. Calculate the include paths that it and its dependencies export.
	//        This set of include paths is accessible during all subsequent
	//        builds.
	//     2. Build the BSP package.
	if p.Target.Bsp != "" {
		incls, err = BspIncludePaths(pkgList, p.Target)
		if err != nil {
			return err
		}
	}

	// Build the project dependencies.
	capPkgs, err := p.buildDeps(pkgList, &incls, &libs)
	if err != nil {
		return err
	}

	if p.Target.Bsp != "" {
		linkerScript, err = p.buildBsp(pkgList, &incls, &libs, capPkgs)
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
	c.Cflags = CreateCflags(pkgList, c, p.Target, p.Cflags)

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

	// Mark package for this project as built
	projectPkg, err := pkgList.ResolvePkgName("project/" + p.Name)
	if projectPkg != nil {
		projectPkg.Built = true
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
	for _, pkg := range pkgList.Pkgs {
		if pkg.Built == true {
			builtPkg, err := NewBuiltPkg(pkg)
			if err != nil {
				return err
			}
			BuiltPkgs = append(BuiltPkgs, builtPkg)
		}
	}
	return nil
}

// Initialize the project, and project definition
func (p *Project) Init() error {
	p.BasePath = p.Repo.BasePath + "/apps/" + p.Name + "/"
	if NodeNotExist(p.BasePath) {
		return NewNewtError("apps directory does not exist")
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
