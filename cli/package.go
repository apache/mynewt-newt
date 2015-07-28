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
	"strconv"
	"strings"
)

type Version struct {
	Major    int64
	Minor    int64
	Revision int64
}

type Capability struct {
	Name string

	Vers *Version
}

type DependencyRequirement struct {
	CompareType string
	Name        string
	Stability   string
	MinVers     *Version
	MaxVers     *Version
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
	Version *Version
	// Type of package
	LinkerScript string

	// Package sources
	Sources []string
	// Package include directories
	Includes []string

	// Whether or not this package is a BSP
	IsBsp bool

	// Capabilities that this package exports
	Capabilities map[string]*Capability

	// Capabilities that this package requires
	ReqCapabilities map[string]*Capability

	// Whether or not we've already compiled this package
	Built bool

	// Packages that this package depends on
	Deps []*DependencyRequirement
}

func NewVersFromStr(versStr string) (*Version, error) {
	var err error

	parts := strings.Split(versStr, ".")
	if len(parts) > 3 {
		return nil, NewStackError(fmt.Sprintf("Invalid version string: %s", versStr))
	}

	if strings.Trim(parts[0], " ") == "" {
		return &Version{0, 0, 0}, nil
	}

	v := &Version{}

	// convert first string to an int
	if v.Major, err = strconv.ParseInt(parts[0], 0, 64); err != nil {
		return nil, NewStackError(err.Error())
	}
	if len(parts) >= 2 {
		if v.Minor, err = strconv.ParseInt(parts[1], 0, 64); err != nil {
			return nil, NewStackError(err.Error())
		}
	}
	if len(parts) == 3 {
		if v.Revision, err = strconv.ParseInt(parts[2], 0, 64); err != nil {
			return nil, NewStackError(err.Error())
		}
	}

	return v, nil
}

func (v *Version) SatisfiesVersion(minVers *Version, maxVers *Version) bool {
	return true
}

func (vers *Version) String() string {
	return fmt.Sprintf("%d.%d.%d", vers.Major, vers.Minor, vers.Revision)
}

func NewDependencyRequirementFromStr(depReq string) (*DependencyRequirement, error) {
	// Allocate dependency requirement
	dr := &DependencyRequirement{}
	// Split string into multiple parts, @#
	// first, get dependency name
	parts := strings.Split(depReq, "@")
	if len(parts) == 1 {
		parts = strings.Split(depReq, "#")
		dr.Name = parts[0]
		if len(parts) > 1 {
			dr.Stability = parts[1]
		} else {
			dr.Stability = "stable"
		}
		dr.CompareType = "-"
	} else if len(parts) == 2 {
		dr.Name = parts[0]
		verParts := strings.Split(parts[1], "#")

		if err := dr.SetVersStr(verParts[0]); err != nil {
			return nil, err
		}
		if len(verParts) == 2 {
			dr.Stability = verParts[1]
		} else {
			dr.Stability = "stable"
		}
	}

	return dr, nil
}

func (dr *DependencyRequirement) SetVersStr(versStr string) error {
	var err error
	// Set the dependency version from the string that
	// Options here are:
	//  - vers = x[.y[.z]]: just a version, requires that version be present
	//     - x = major version integer, string LATEST can be put as a placeholder for max
	//     - y = minor version integer, LATEST can be put as a placeholder for max
	//     - z = revision integer, LATEST can be put as placeholder for max
	//  - vers>=<=vers: Version between these two versions

	// OH, the humanity!  order matters here, as "<" and ">" will match for "<=" and ">="
	// and be processed incorrectly.
	splits := []string{"<=", ">=", ">", "<"}

	minVersStr := versStr
	maxVersStr := ""
	compareType := "=="
	for _, split := range splits {
		parts := strings.Split(versStr, split)
		if len(parts) == 2 {
			compareType = split
			switch split {
			case "<=":
				fallthrough
			case "<":
				minVersStr = parts[0]
				maxVersStr = parts[1]
			case ">=":
				fallthrough
			case ">":
				minVersStr = parts[1]
				maxVersStr = parts[0]
			}
			break
		}
	}

	dr.CompareType = compareType

	if dr.MinVers, err = NewVersFromStr(minVersStr); err != nil {
		return err
	}

	if dr.MaxVers, err = NewVersFromStr(maxVersStr); err != nil {
		return err
	}

	return nil
}

func (dr *DependencyRequirement) String() string {
	return fmt.Sprintf("%s:%s<=%s", dr.Name, dr.MinVers, dr.MaxVers)
}

func (dr *DependencyRequirement) SatisfiesDependency(pkgName string, pkgVers *Version) bool {
	if pkgName == dr.Name && pkgVers.SatisfiesVersion(dr.MinVers, dr.MaxVers) {
		return true
	} else {
		return false
	}
}

func NewCapFromStr(capStr string) (*Capability, error) {
	// The capability can have 2 items
	// name@version
	c := &Capability{}

	parts := strings.Split(capStr, "@")
	c.Name = parts[0]
	if len(parts) == 2 {
		var err error
		if c.Vers, err = NewVersFromStr(parts[1]); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *Capability) SatisfiesCapability(caps map[string]*Capability) bool {
	key := fmt.Sprintf("%s:%s", c.Name, c.Vers)
	_, ok := caps[key]
	return ok
}

func (c *Capability) String() string {
	return fmt.Sprintf("%s:%s", c.Name, c.Vers)
}

func (pkg *Package) GetCapabilities() (map[string]*Capability, error) {
	return pkg.Capabilities, nil
}

func (pkg *Package) loadCaps(caps *map[string]*Capability, capList []string) error {
	// Allocate an array of capabilities
	caps = &map[string]*Capability{}

	if len(capList) == 0 {
		return nil
	}

	for _, capItem := range capList {
		c, err := NewCapFromStr(capItem)
		if err != nil {
			return err
		}

		(*caps)[c.String()] = c
		log.Printf("[DEBUG] Appending new capability pkg: %s, name: %s, vers: %s",
			pkg.Name, c.Name, c.Vers)
	}

	return nil
}

// Load a package's configuration information from the package config
// file.
func (pkg *Package) loadConfig() error {
	log.Printf("[DEBUG] Loading configuration for pkg %s", pkg.BasePath)

	v, err := ReadConfig(pkg.BasePath, "pkg")
	if err != nil {
		return err
	}

	pkg.FullName = v.GetString("pkg.name")
	pkg.Name = filepath.Base(pkg.FullName)

	pkg.Version, err = NewVersFromStr(v.GetString("pkg.vers"))
	if err != nil {
		return err
	}

	pkg.LinkerScript = v.GetString("pkg.linkerscript")

	// Load package dependencies
	depList := v.GetStringSlice("pkg.deps")
	if len(depList) > 0 {
		pkg.Deps = make([]*DependencyRequirement, len(depList), len(depList))
		for _, depStr := range depList {
			log.Printf("[DEBUG] Loading depedency %s from package %s", depStr, pkg.FullName)
			dr, err := NewDependencyRequirementFromStr(depStr)
			if err != nil {
				return err
			}

			pkg.Deps = append(pkg.Deps, dr)
		}
	}

	// Load package capabilities
	if err = pkg.loadCaps(&pkg.Capabilities,
		v.GetStringSlice("pkg.capabilities")); err != nil {
		return err
	}

	// Load required package capabilities
	if err = pkg.loadCaps(&pkg.ReqCapabilities,
		v.GetStringSlice("pkg.req_capabilities")); err != nil {
		return err
	}

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
			return NewStackError(fmt.Sprintf("File %s should not exist in include "+
				"directory, only file allowed in include directory is a directory with "+
				"the package name %s",
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
