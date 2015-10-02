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
	"fmt"
	"github.com/termie/go-shutil"
	"log"
	"os"
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

type Egg struct {
	// Base directory of the egg
	BasePath string
	// Name of the egg
	Name string
	// Full Name of the egg include prefix dir
	FullName string
	// Nest this egg belongs to
	Nest *Nest
	// Egg version
	Version *Version
	// Type of egg
	LinkerScript string

	// Has the configuration been loaded for this egg
	CfgLoaded bool

	// Egg sources
	Sources []string
	// Egg include directories
	Includes []string

	// Egg compiler flags
	Cflags string

	// Egg linker flags
	Lflags string

	// Egg assembler flags
	Aflags string

	// Whether or not this egg is a BSP
	IsBsp bool

	// Capabilities that this egg exports
	Capabilities []*DependencyRequirement

	// Capabilities that this egg requires
	ReqCapabilities []*DependencyRequirement

	// Whether or not we've already compiled this egg
	Built bool

	// Whether or not we've already cleaned this egg
	Clean bool

	// Eggs that this egg depends on
	Deps []*DependencyRequirement
}

type EggShell struct {
	FullName string
	Version  *Version
	/* Clutch this eggshell belongs to */
	Clutch  *Clutch
	Deps    []*DependencyRequirement
	Caps    []*DependencyRequirement
	ReqCaps []*DependencyRequirement
}

func NewEggShell(clutch *Clutch) (*EggShell, error) {
	eShell := &EggShell{
		Clutch: clutch,
	}

	return eShell, nil
}

func (es *EggShell) serializeDepReq(name string,
	drList []*DependencyRequirement, indent string) string {
	drStr := ""
	if len(drList) > 0 {
		drStr += fmt.Sprintf("%s%s:\n", indent, name)
		for _, dr := range drList {
			drStr += fmt.Sprintf("%s    - %s\n", indent, dr)
		}
	}

	return drStr
}

func (es *EggShell) Serialize(indent string) string {
	esStr := fmt.Sprintf("%s%s:\n", indent, es.FullName)
	indent += "    "
	if es.Version == nil {
		es.Version = &Version{0, 0, 0}
	}
	esStr += fmt.Sprintf("%svers: %s\n", indent, es.Version)

	esStr += es.serializeDepReq("deps", es.Deps, indent)
	esStr += es.serializeDepReq("caps", es.Caps, indent)
	esStr += es.serializeDepReq("req_caps", es.ReqCaps, indent)

	return esStr
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
		return nil, NewNewtError(fmt.Sprintf("Invalid version string: %s", versStr))
	}

	if strings.Trim(parts[0], " ") == "" || strings.Trim(parts[0], " ") == "none" {
		return nil, nil
	}

	v := &Version{}

	// convert first string to an int
	if v.Major, err = strconv.ParseInt(parts[0], 0, 64); err != nil {
		return nil, NewNewtError(err.Error())
	}
	if len(parts) >= 2 {
		if v.Minor, err = strconv.ParseInt(parts[1], 0, 64); err != nil {
			return nil, NewNewtError(err.Error())
		}
	}
	if len(parts) == 3 {
		if v.Revision, err = strconv.ParseInt(parts[2], 0, 64); err != nil {
			return nil, NewNewtError(err.Error())
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

			if vm.Vers != nil {
				dr.VersMatches = append(dr.VersMatches, vm)
			}
		}
	} else {
		dr.VersMatches = make([]*VersMatch, 0)
		vm := &VersMatch{}
		vm.CompareType = "=="
		if vm.Vers, err = NewVersParseString(versStr); err != nil {
			return err
		}
		if vm.Vers != nil {
			dr.VersMatches = append(dr.VersMatches, vm)
		}
	}

	if len(dr.VersMatches) == 0 {
		dr.VersMatches = nil
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
	return fmt.Sprintf("%s@%s#%s", dr.Name, dr.VersMatchesString(), dr.Stability)
}

func (dr *DependencyRequirement) SatisfiesCapability(
	capability *DependencyRequirement) error {
	if dr.Name != capability.Name {
		return NewNewtError(fmt.Sprintf("Required capability name %s doesn't match "+
			"specified capability name %s", dr.Name, capability.Name))
	}

	for _, versMatch := range dr.VersMatches {
		if !versMatch.Vers.SatisfiesVersion(capability.VersMatches) {
			return NewNewtError(fmt.Sprintf("Capability %s doesn't satisfy version "+
				"requirement %s", capability, versMatch.Vers))
		}
	}

	return nil
}

// Check whether the passed in egg satisfies the current dependency requirement
func (dr *DependencyRequirement) SatisfiesDependency(egg *Egg) bool {
	if egg.FullName != dr.Name {
		return false
	}

	if egg.Version.SatisfiesVersion(dr.VersMatches) {
		return true
	}

	return false
}

// Create a New DependencyRequirement structure from the contents of the depReq
// string that has been passed in as an argument.
func NewDependencyRequirementParseString(depReq string) (*DependencyRequirement,
	error) {
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

// Get a map of egg capabilities.  The returned map contains the name of the
// capability, and its version as the key, and a pointer to the
// Capability structure associated with that name.
func (egg *Egg) GetCapabilities() ([]*DependencyRequirement, error) {
	return egg.Capabilities, nil
}

// Return the egg dependencies for this egg.
func (egg *Egg) GetDependencies() ([]*DependencyRequirement, error) {
	return egg.Deps, nil
}

func (eggShell *EggShell) GetCapabilities() ([]*DependencyRequirement, error) {
	return eggShell.Caps, nil
}

// Return the egg dependencies for this eggShell.
func (eggShell *EggShell) GetDependencies() ([]*DependencyRequirement, error) {
	return eggShell.Deps, nil
}

// Load a egg's configuration information from the egg config
// file.
func (egg *Egg) GetIncludes(t *Target) ([]string, error) {
	// Return the include directories for just this egg
	incls := []string{
		egg.BasePath + "/include/",
		egg.BasePath + "/include/" + egg.Name + "/arch/" + t.Arch + "/",
	}

	return incls, nil
}

// Load capabilities from a string containing a list of capabilities.
// The capability format is expected to be one of:
//   name@version
//   name
// @param capList An array of capability strings
// @return On success error is nil, and a list of capabilities is returned,
// on failure error is non-nil
func (egg *Egg) loadCaps(capList []string) ([]*DependencyRequirement, error) {
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
		log.Printf("[DEBUG] Appending new capability egg: %s, cap:%s",
			egg.Name, dr)
	}

	return caps, nil
}

// Load a egg's configuration.  This allocates & initializes a fair number of
// the main data structures within the egg.
func (egg *Egg) LoadConfig(t *Target, force bool) error {
	if egg.CfgLoaded && !force {
		return nil
	}

	log.Printf("[DEBUG] Loading configuration for egg %s", egg.BasePath)

	v, err := ReadConfig(egg.BasePath, "egg")
	if err != nil {
		return err
	}

	egg.FullName = v.GetString("egg.name")
	egg.Name = filepath.Base(egg.FullName)

	egg.Version, err = NewVersParseString(v.GetString("egg.vers"))
	if err != nil {
		return err
	}

	egg.LinkerScript = GetStringIdentities(v, t, "egg.linkerscript")

	egg.Cflags = GetStringIdentities(v, t, "egg.cflags")
	egg.Lflags = GetStringIdentities(v, t, "egg.lflags")
	egg.Aflags = GetStringIdentities(v, t, "egg.aflags")

	// Append all the identities that this egg exposes to sub-eggs
	if t != nil {
		idents := GetStringSliceIdentities(v, t, "egg.identities")
		t.Identities = append(t.Identities, idents...)
	}

	// Load egg dependencies
	depList := GetStringSliceIdentities(v, t, "egg.deps")
	if len(depList) > 0 {
		egg.Deps = make([]*DependencyRequirement, 0, len(depList))
		for _, depStr := range depList {
			log.Printf("[DEBUG] Loading depedency %s from egg %s", depStr,
				egg.FullName)
			dr, err := NewDependencyRequirementParseString(depStr)
			if err != nil {
				return err
			}

			egg.Deps = append(egg.Deps, dr)
		}
	}

	// Load the list of capabilities that this egg exposes
	egg.Capabilities, err = egg.loadCaps(GetStringSliceIdentities(v, t,
		"egg.caps"))
	if err != nil {
		return err
	}

	// Load the list of capabilities that this egg requires
	egg.ReqCapabilities, err = egg.loadCaps(GetStringSliceIdentities(v, t,
		"egg.req_caps"))
	if err != nil {
		return err
	}

	egg.CfgLoaded = true

	return nil
}

// Initialize a egg: loads the egg configuration, and sets up egg data
// structures.  Should only be called from NewEgg
func (egg *Egg) Init() error {
	return nil
}

// Allocate and initialize a new egg, and return a fully initialized Egg
//     structure.
// @param nest The Nest this egg is located in
// @param basePath The path to this egg, within the specified nest
// @return On success, error is nil, and a Egg is returned.  on failure,
//         error is not nil.
func NewEgg(nest *Nest, basePath string) (*Egg, error) {
	egg := &Egg{
		BasePath: basePath,
		Nest:     nest,
	}

	if err := egg.Init(); err != nil {
		return nil, err
	}

	return egg, nil
}

func (egg *Egg) TestBinName() string {
	return "test_" + egg.Name
}

/*
 * Download egg from a clutch and stick it to nest.
 */
func (eggShell *EggShell) Download() error {

	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	url := eggShell.Clutch.RemoteUrl

	StatusMessage(VERBOSITY_DEFAULT, "Downloading %s from %s/"+
		"master...", eggShell.Clutch.Name, url)

	dir, err := dl.GetRepo(url, "master")
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	StatusMessage(VERBOSITY_DEFAULT, "Moving %s\n", eggShell.FullName)

	dstdir := filepath.Join(eggShell.Clutch.Nest.BasePath, eggShell.FullName)

	err = os.RemoveAll(dstdir)
	if err != nil {
		return err
	}

	err = shutil.CopyTree(filepath.Join(dir, eggShell.FullName), dstdir, nil)
	if err != nil {
		return err
	}

	err = os.RemoveAll(dir)

	return err
}

func (egg *Egg) Remove() error {
	return os.RemoveAll(egg.BasePath)
}
