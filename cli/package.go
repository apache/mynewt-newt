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
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type VersMatch struct {
	CompareType string
	Vers        *Version
}

type Version struct {
	Major    int64
	Minor    int64
	Revision int64
}

type DependencyRequirement struct {
	Name        string
	Stability   string
	VersMatches []*VersMatch
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

	// Package compiler flags
	Cflags string

	// Package linker flags
	Lflags string

	// Package assembler flags
	Aflags string

	// Whether or not this package is a BSP
	IsBsp bool

	// Capabilities that this package exports
	Capabilities []*DependencyRequirement

	// Capabilities that this package requires
	ReqCapabilities []*DependencyRequirement

	// Whether or not we've already compiled this package
	Built bool

	// Packages that this package depends on
	Deps []*DependencyRequirement
}

func (v *Version) compareVersions(vers1 *Version, vers2 *Version) int64 {
	log.Printf("[DEBUG] Comparing %s to %s (%d %d %d)", vers1, vers2,
		vers1.Major-vers2.Major, vers1.Minor-vers2.Minor,
		vers1.Revision-vers2.Revision)

	if r := vers1.Major - vers2.Major; r != 0 {
		return r
	}

	if r := vers1.Minor - vers2.Minor; r != 0 {
		return r
	}

	if r := vers1.Revision - vers2.Revision; r != 0 {
		return r
	}

	return 0
}

func (v *Version) SatisfiesVersion(versMatches []*VersMatch) bool {
	if versMatches == nil {
		return true
	}

	for _, match := range versMatches {
		r := v.compareVersions(match.Vers, v)
		switch match.CompareType {
		case "<":
			if r <= 0 {
				return false
			}
		case "<=":
			if r < 0 {
				return false
			}
		case ">":
			if r >= 0 {
				return false
			}
		case ">=":
			if r > 0 {
				return false
			}
		case "==":
			if r != 0 {
				return false
			}
		}
	}

	return true
}

func (vers *Version) String() string {
	return fmt.Sprintf("%d.%d.%d", vers.Major, vers.Minor, vers.Revision)
}

func NewVersParseString(versStr string) (*Version, error) {
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

//
// Set the version comparison constraints on a dependency requirement.
// The version string contains a list of version constraints in the following format:
//    - <comparison><version>
// Where <comparison> can be any one of the following comparison
//   operators: <=, <, >, >=, ==
// And <version> is specified in the form: X.Y.Z where X, Y and Z are all
// int64 types in decimal form
func (dr *DependencyRequirement) SetVersStr(versStr string) error {
	var err error

	re, err := regexp.Compile(`(<=|>=|==|>|<)([\d\.]+)`)
	if err != nil {
		return err
	}

	matches := re.FindAllStringSubmatch(versStr, -1)
	if matches != nil {
		dr.VersMatches = make([]*VersMatch, 0, len(matches))
		for _, match := range matches {
			vm := &VersMatch{}
			vm.CompareType = match[1]
			if vm.Vers, err = NewVersParseString(match[2]); err != nil {
				return err
			}
			dr.VersMatches = append(dr.VersMatches, vm)
		}
	} else {
		dr.VersMatches = make([]*VersMatch, 0)
		vm := &VersMatch{}
		vm.CompareType = "=="
		if vm.Vers, err = NewVersParseString(versStr); err != nil {
			return err
		}
		dr.VersMatches = append(dr.VersMatches, vm)
	}
	return nil
}

// Convert the array of version matches into a string for display
func (dr *DependencyRequirement) VersMatchesString() string {
	if dr.VersMatches != nil {
		str := ""
		for _, match := range dr.VersMatches {
			str += fmt.Sprintf("%s%s", match.CompareType, match.Vers)
		}
		return str
	} else {
		return "none"
	}
}

// Convert the dependency requirement to a string for display
func (dr *DependencyRequirement) String() string {
	return fmt.Sprintf("%s:%s:%s", dr.Name, dr.VersMatchesString(), dr.Stability)
}

// Check whether the passed in package satisfies the current dependency requirement
func (dr *DependencyRequirement) SatisfiesDependency(pkg *Package) bool {
	if pkg.FullName != dr.Name {
		return false
	}

	if pkg.Version.SatisfiesVersion(dr.VersMatches) {
		return true
	}

	return false
}

// Create a New DependencyRequirement structure from the contents of the depReq
// string that has been passed in as an argument.
func NewDependencyRequirementParseString(depReq string) (*DependencyRequirement, error) {
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

// Get a map of package capabilities.  The returned map contains the name of the
// capability, and its version as the key, and a pointer to the
// Capability structure associated with that name.
func (pkg *Package) GetCapabilities() ([]*DependencyRequirement, error) {
	return pkg.Capabilities, nil
}

// Return the package dependencies for this package.
func (pkg *Package) GetDependencies() ([]*DependencyRequirement, error) {
	return pkg.Deps, nil
}

// Load a package's configuration information from the package config
// file.
func (pkg *Package) GetIncludes(t *Target) ([]string, error) {
	// Return the include directories for just this package
	incls := []string{
		pkg.BasePath + "/include/",
		pkg.BasePath + "/include/" + pkg.Name + "/arch/" + t.Arch + "/",
	}

	return incls, nil
}

// Load capabilities from a string containing a list of capabilities.
// The capability format is expected to be one of:
//   name@version
//   name
// @param capList An array of capability strings
// @return On success error is nil, and a list of capabilities is returned, on failure
// error is non-nil
func (pkg *Package) loadCaps(capList []string) ([]*DependencyRequirement, error) {
	if len(capList) == 0 {
		return nil, nil
	}

	// Allocate an array of capabilities
	caps := make([]*DependencyRequirement, 0)

	log.Printf("[DEBUG] Loading capabilities %s", strings.Join(capList, " "))
	for _, capItem := range capList {
		dr, err := NewDependencyRequirementParseString(capItem)
		if err != nil {
			return nil, err
		}

		caps = append(caps, dr)
		log.Printf("[DEBUG] Appending new capability pkg: %s, cap:%s",
			pkg.Name, dr)
	}

	return caps, nil
}

// Load a package's configuration.  This allocates & initializes a fair number of
// the main data structures within the package.
func (pkg *Package) loadConfig(t *Target) error {
	log.Printf("[DEBUG] Loading configuration for pkg %s", pkg.BasePath)

	v, err := ReadConfig(pkg.BasePath, "pkg")
	if err != nil {
		return err
	}

	pkg.FullName = v.GetString("pkg.name")
	pkg.Name = filepath.Base(pkg.FullName)

	pkg.Version, err = NewVersParseString(v.GetString("pkg.vers"))
	if err != nil {
		return err
	}

	pkg.LinkerScript = GetStringIdentities(v, t, "pkg.linkerscript")

	pkg.Cflags = GetStringIdentities(v, t, "pkg.cflags")
	pkg.Lflags = GetStringIdentities(v, t, "pkg.lflags")
	pkg.Aflags = GetStringIdentities(v, t, "pkg.aflags")

	// Append all the identities that this package exposes to sub-packages
	if t != nil {
		idents := GetStringSliceIdentities(v, t, "pkg.identities")
		t.Identities = append(t.Identities, idents...)
	}

	// Load package dependencies
	depList := GetStringSliceIdentities(v, t, "pkg.deps")
	if len(depList) > 0 {
		pkg.Deps = make([]*DependencyRequirement, 0, len(depList))
		for _, depStr := range depList {
			log.Printf("[DEBUG] Loading depedency %s from package %s", depStr,
				pkg.FullName)
			dr, err := NewDependencyRequirementParseString(depStr)
			if err != nil {
				return err
			}

			pkg.Deps = append(pkg.Deps, dr)
		}
	}

	// Load the list of capabilities that this package exposes
	pkg.Capabilities, err = pkg.loadCaps(GetStringSliceIdentities(v, t, "pkg.caps"))
	if err != nil {
		return err
	}

	// Load the list of capabilities that this package requires
	pkg.ReqCapabilities, err = pkg.loadCaps(GetStringSliceIdentities(v, t, "pkg.req_caps"))
	if err != nil {
		return err
	}

	return nil
}

// Initialize a package: loads the package configuration, and sets up package data
// structures.  Should only be called from NewPackage
func (pkg *Package) Init(t *Target) error {
	log.Printf("[DEBUG] Initializing package %s in path %s", pkg.Name, pkg.BasePath)

	// Load package configuration file
	if err := pkg.loadConfig(t); err != nil {
		return err
	}

	return nil
}

// Allocate and initialize a new package, and return a fully initialized Package
//     structure.
// @param r The repository this package is located in
// @param basePath The path to this package, within the specified repository
// @return On success, error is nil, and a Package is returned.  on failure,
//         error is not nil.
func NewPackage(r *Repo, t *Target, basePath string) (*Package, error) {
	pkg := &Package{
		BasePath: basePath,
		Repo:     r,
	}

	if err := pkg.Init(t); err != nil {
		return nil, err
	}

	return pkg, nil
}
