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

func (p *Project) buildDeps(pm *PkgMgr, incls *[]string, libs *[]string) error {
	pkgList := p.GetPackages()
	if pkgList == nil {
		return nil
	}

	for _, pkgName := range pkgList {
		pkg, err := pm.ResolvePkgName(pkgName)
		if err != nil {
			return err
		}

		if lib := pm.GetPackageLib(pkg); NodeExist(lib) {
			*libs = append(*libs, lib)
		}

		if err = pm.Build(pkgName); err != nil {
			return err
		}

		*incls = append(*incls, pkg.Includes...)
	}

	return nil
}

func (p *Project) buildBsp(pm *PkgMgr, incls *[]string, libs *[]string) (string, error) {
	if p.Target.Bsp == "" {
		return "", NewStackError("Must specify a BSP to build project")
	}

	bspPackage, err := pm.ResolvePkgName(p.Target.Bsp)
	if err != nil {
		return "", NewStackError("No BSP package for " + p.Target.Bsp + " exists")
	}

	if err = pm.Build(p.Target.Bsp); err != nil {
		return "", err
	}

	*libs = append(*libs, pm.GetPackageLib(bspPackage))
	*incls = append(*incls, bspPackage.Includes...)

	linkerScript := bspPackage.BasePath + "/" + bspPackage.LinkerScript

	return linkerScript, nil
}

// Build the project
func (p *Project) Build() error {
	pm, err := NewPkgMgr(p.Repo, p.Target)
	if err != nil {
		return err
	}

	incls := []string{}
	libs := []string{}

	if err := p.buildDeps(pm, &incls, &libs); err != nil {
		return err
	}

	linkerScript, err := p.buildBsp(pm, &incls, &libs)
	if err != nil {
		return err
	}

	// Append project includes
	projIncls := []string{
		p.BasePath + "/include/",
		p.BasePath + "/arch/" + p.Target.Arch + "/include/",
	}

	incls = append(incls, projIncls...)

	c, err := NewCompiler(p.Target.GetCompiler(), p.Target.Cdef, p.Target.Arch,
		incls)
	if err != nil {
		return err
	}

	c.LinkerScript = linkerScript

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
		return NewStackError("Project directory does not exist")
	}

	if err := p.loadConfig(); err != nil {
		return err
	}
	return nil
}
