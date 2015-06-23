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
	"errors"
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

	// Packages
	Packages []string

	// The repository the project is located in
	Repo *Repo

	// The target associated with this project
	Target *Target
}

// Load a project and return it
func LoadProject(r *Repo, t *Target, name string) (*Project, error) {
	p := &Project{
		Name:   name,
		Repo:   r,
		Target: t,
	}

	if err := p.Init(); err != nil {
		return nil, err
	} else {
		return p, nil
	}
}

func (p *Project) GetPackages() []string {
	return p.Packages
}

func (p *Project) loadConfig() error {
	v, err := ReadConfig(p.BasePath, p.Name)
	if err != nil {
		return err
	}

	p.Packages = strings.Split(v.GetString("project.pkgs"), " ")

	return nil
}

// Clean the project build, and all packages that were built with the
// project
func (p *Project) BuildClean(cleanAll bool) error {
	pm, err := NewPkgMgr(p.Repo, p.Target)
	if err != nil {
		return err
	}

	// first, clean packages
	for _, pkgName := range p.GetPackages() {
		err = pm.BuildClean(pkgName, cleanAll)
		if err != nil {
			return err
		}
	}

	// clean the bsp
	bspPath := p.Repo.BasePath + "/hw/bsp/" + p.Target.Bsp + "/"
	os.RemoveAll(bspPath + "/src/obj/" + p.Target.Arch + "/")
	if cleanAll {
		os.RemoveAll(bspPath + "/src/obj/")
	}

	// next, clean project
	os.RemoveAll(p.BasePath + "/src/obj/" + p.Target.Arch + "/")
	os.RemoveAll(p.BasePath + "/src/arch/" + p.Target.Arch + "/obj/")
	os.RemoveAll(p.BasePath + "/bin/" + p.Target.Arch + "/")
	if cleanAll {
		os.RemoveAll(p.BasePath + "/src/obj/")
		os.RemoveAll(p.BasePath + "/bin/")
	}

	return nil
}

func (p *Project) buildBsp(c *Compiler) error {
	bspDir := p.Repo.BasePath + "/hw/bsp/" + p.Target.Bsp + "/"

	if NodeExist(bspDir + "/" + p.Target.Bsp + ".ld") {
		c.LinkerScript = bspDir + "/" + p.Target.Bsp + ".ld"
	}

	os.Chdir(bspDir + "src/")

	if err := c.Compile("*.c"); err != nil {
		return err
	}

	if err := c.CompileAs("*.s"); err != nil {
		return err
	}

	return nil
}

// Build the project
func (p *Project) Build() error {
	pm, err := NewPkgMgr(p.Repo, p.Target)
	if err != nil {
		return err
	}

	// Save package includes
	incls := []string{}
	libs := []string{}

	// Build the packages associated with this project
	for _, pkgName := range p.GetPackages() {
		pkg, _ := pm.ResolvePkgName(pkgName)
		incls = append(incls, pkg.Includes...)
		libs = append(libs, pm.GetPackageLib(pkg))
		pm.Build(pkgName)
	}

	// Append project includes
	projIncls := []string{
		p.BasePath + "/include/",
		p.BasePath + "/arch/" + p.Target.Arch + "/include/",
		p.Repo.BasePath + "/hw/bsp/" + p.Target.Bsp + "/include/",
	}

	incls = append(incls, projIncls...)

	c, err := NewCompiler(p.Target.GetCompiler(), p.Target.Cdef, p.Target.Arch,
		incls)
	if err != nil {
		return err
	}

	// Build the configured BSP to start
	if p.Target.Bsp == "" {
		return errors.New("No BSP specified")
	}

	if err := p.buildBsp(c); err != nil {
		return err
	}

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
	binDir := p.BasePath + "/bin/" + p.Target.Arch + "/"
	if NodeNotExist(binDir) {
		os.MkdirAll(binDir, 0755)
	}
	log.Printf("[DEBUG] Compiling a binary %s from libs %s", binDir+p.Name,
		strings.Join(libs, " "))

	c.CompileElf(binDir+p.Name, map[string]bool{"mapFile": true, "listFile": true},
		strings.Join(libs, " "))

	return nil
}

// Initialize the project, and project definition
func (p *Project) Init() error {
	p.BasePath = p.Repo.BasePath + "/project/" + p.Name + "/"
	if NodeNotExist(p.BasePath) {
		return errors.New("Project directory does not exist")
	}

	if err := p.loadConfig(); err != nil {
		return err
	}
	return nil
}
