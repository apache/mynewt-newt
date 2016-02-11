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
	"crypto/sha1"
	"fmt"
	"io/ioutil"
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

type Pkg struct {
	// Base directory of the pkg
	BasePath string
	// Name of the pkg
	Name string
	// Full Name of the pkg include prefix dir
	FullName string
	// Repo this pkg belongs to
	Repo *Repo
	// Pkg version
	Version *Version
	// Type of pkg
	LinkerScript string

	// For BSP pkg, how to download
	DownloadScript string
	// For BSP pkg, how to start debugger and attach it to target board
	DebugScript string

	// Has the dependencies been loaded for this pkg
	DepLoaded bool

	// Has the configuration been loaded for this pkg
	CfgLoaded bool

	// Pkg sources
	Sources []string
	// Pkg include directories
	Includes []string

	// Pkg compiler flags
	Cflags string

	// Pkg linker flags
	Lflags string

	// Pkg assembler flags
	Aflags string

	// Whether or not this pkg is a BSP
	IsBsp bool

	// Capabilities that this pkg exports
	Capabilities []*DependencyRequirement

	// Capabilities that this pkg requires
	ReqCapabilities []*DependencyRequirement

	// Whether or not we've already compiled this pkg
	Built bool

	// Whether or not we've already cleaned this pkg
	Clean bool

	// Pkgs that this pkg depends on
	Deps []*DependencyRequirement
}

type PkgDesc struct {
	FullName string
	Version  *Version
	/* PkgList this pkgshell belongs to */
	PkgList *PkgList
	Hash    string
	Deps    []*DependencyRequirement
	Caps    []*DependencyRequirement
	ReqCaps []*DependencyRequirement
}

func NewPkgDesc(pkgList *PkgList) (*PkgDesc, error) {
	eShell := &PkgDesc{
		PkgList: pkgList,
	}

	return eShell, nil
}

func (es *PkgDesc) serializeDepReq(name string,
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

func (es *PkgDesc) Serialize(indent string) string {
	esStr := fmt.Sprintf("%s%s:\n", indent, es.FullName)
	indent += "    "
	if es.Version == nil {
		es.Version = &Version{0, 0, 0}
	}
	esStr += fmt.Sprintf("%svers: %s\n", indent, es.Version)
	esStr += fmt.Sprintf("%shash: %s\n", indent, es.Hash)
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

// Check whether the passed in pkg satisfies the current dependency requirement
func (dr *DependencyRequirement) SatisfiesDependency(pkg *Pkg) bool {
	if pkg.FullName != dr.Name {
		return false
	}

	if pkg.Version.SatisfiesVersion(dr.VersMatches) {
		return true
	}

	return false
}

// Convert the dependency requirement to branch name to look for
func (dr *DependencyRequirement) BranchName() string {
	if dr.Stability != "stable" {
		// XXX should compare to latest
		return dr.Stability
	}
	for _, versMatch := range dr.VersMatches {
		if versMatch.CompareType == "==" || versMatch.CompareType == "<=" {
			if versMatch.Vers.Minor == 0 && versMatch.Vers.Revision == 0 {
				return fmt.Sprintf("%d", versMatch.Vers.Major)
			} else if versMatch.Vers.Revision == 0 {
				return fmt.Sprintf("%d.%d", versMatch.Vers.Major,
					versMatch.Vers.Minor)
			} else {
				return fmt.Sprintf("%d.%d.%d", versMatch.Vers.Major,
					versMatch.Vers.Minor, versMatch.Vers.Revision)
			}
		}
		// XXX What to do with other version comparisons?
	}
	return "master"
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

// Get a map of pkg capabilities.  The returned map contains the name of the
// capability, and its version as the key, and a pointer to the
// Capability structure associated with that name.
func (pkg *Pkg) GetCapabilities() ([]*DependencyRequirement, error) {
	return pkg.Capabilities, nil
}

// Return the pkg dependencies for this pkg.
func (pkg *Pkg) GetDependencies() ([]*DependencyRequirement, error) {
	return pkg.Deps, nil
}

func (pkg *Pkg) GetReqCapabilities() ([]*DependencyRequirement, error) {
	return pkg.ReqCapabilities, nil
}

func (pkgDesc *PkgDesc) GetCapabilities() ([]*DependencyRequirement, error) {
	return pkgDesc.Caps, nil
}

// Return the pkg dependencies for this pkgDesc.
func (pkgDesc *PkgDesc) GetDependencies() ([]*DependencyRequirement, error) {
	return pkgDesc.Deps, nil
}

func (pkgDesc *PkgDesc) GetReqCapabilities() ([]*DependencyRequirement, error) {
	return pkgDesc.ReqCaps, nil
}

// Load a pkg's configuration information from the pkg config
// file.
func (pkg *Pkg) GetIncludes(t *Target) ([]string, error) {
	// Return the include directories for just this pkg
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
// @return On success error is nil, and a list of capabilities is returned,
// on failure error is non-nil
func (pkg *Pkg) loadCaps(capList []string) ([]*DependencyRequirement, error) {
	if len(capList) == 0 {
		return nil, nil
	}

	// Allocate an array of capabilities
	caps := make([]*DependencyRequirement, 0)

	StatusMessage(VERBOSITY_VERBOSE, "Loading capabilities %s\n",
		strings.Join(capList, " "))
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

// Create a dependency requirement out of an pkg
//
func (pkg *Pkg) MakeDependency() (*DependencyRequirement, error) {
	return NewDependencyRequirementParseString(pkg.FullName)
}

// Generate a hash of the contents of an pkg.  This function recursively
// processes the contents of a directory, ignoring hidden files and the
// bin and obj directories.  It returns a hash of all the files, their
// contents.
func (pkg *Pkg) GetHash() (string, error) {
	hash := sha1.New()

	err := filepath.Walk(pkg.BasePath,
		func(path string, info os.FileInfo, err error) error {
			name := info.Name()
			if name == "bin" || name == "obj" || name[0] == '.' {
				return filepath.SkipDir
			}

			if info.IsDir() {
				// SHA the directory name into the hash
				hash.Write([]byte(name))
			} else {
				// SHA the file name & contents into the hash
				contents, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}
				hash.Write(contents)
			}
			return nil
		})
	if err != nil && err != filepath.SkipDir {
		return "", NewNewtError(err.Error())
	}

	hashStr := fmt.Sprintf("%x", hash.Sum(nil))

	return hashStr, nil
}

// Load pkg's configuration, and collect required capabilities, dependencies, and
// identities it provides, so we'll have this available when target is being built.
func (pkg *Pkg) LoadDependencies(identities map[string]string,
	capabilities map[string]string) error {

	if pkg.DepLoaded {
		return nil
	}

	log.Printf("[DEBUG] Loading dependencies for pkg %s", pkg.BasePath)

	v, err := ReadConfig(pkg.BasePath, "pkg")
	if err != nil {
		return err
	}

	// Append all identities that this pkg exposes.
	idents := GetStringSliceIdentities(v, identities, "pkg.identities")

	// Add these to project identities
	for _, item := range idents {
		StatusMessage(VERBOSITY_VERBOSE, "    Adding identity %s - %s\n", item,
			pkg.FullName)
		identities[item] = pkg.FullName
	}

	// Load the list of capabilities that this pkg exposes
	pkg.Capabilities, err = pkg.loadCaps(GetStringSliceIdentities(v, identities,
		"pkg.caps"))
	if err != nil {
		return err
	}

	// Add these to project capabilities
	for _, cap := range pkg.Capabilities {
		if capabilities[cap.String()] != "" &&
			capabilities[cap.String()] != pkg.FullName {

			return NewNewtError(fmt.Sprintf("Multiple pkgs with "+
				"capability %s (%s and %s)",
				cap.String(), capabilities[cap.String()], pkg.FullName))
		}
		capabilities[cap.String()] = pkg.FullName
		StatusMessage(VERBOSITY_VERBOSE, "    Adding capability %s - %s\n",
			cap.String(), pkg.FullName)
	}

	// Load the list of capabilities that this pkg requires
	pkg.ReqCapabilities, err = pkg.loadCaps(GetStringSliceIdentities(v, identities,
		"pkg.req_caps"))
	if err != nil {
		return err
	}

	// Load pkg dependencies
	depList := GetStringSliceIdentities(v, identities, "pkg.deps")
	if len(depList) > 0 {
		pkg.Deps = make([]*DependencyRequirement, 0, len(depList))
		for _, depStr := range depList {
			log.Printf("[DEBUG] Loading dependency %s from pkg %s", depStr,
				pkg.FullName)
			dr, err := NewDependencyRequirementParseString(depStr)
			if err != nil {
				return err
			}

			pkg.Deps = append(pkg.Deps, dr)
		}
	}
	for _, cap := range pkg.ReqCapabilities {
		pkgName := capabilities[cap.String()]
		if pkgName == "" {
			continue
		}
		dr, err := NewDependencyRequirementParseString(pkgName)
		if err != nil {
			return err
		}
		pkg.Deps = append(pkg.Deps, dr)
	}

	pkgName := identities["LIBC"]
	if pkgName != "" {
		dr, err := NewDependencyRequirementParseString(pkgName)
		if err != nil {
			return err
		}
		pkg.Deps = append(pkg.Deps, dr)
	}

	// Check these as well
	pkg.LinkerScript = GetStringIdentities(v, identities, "pkg.linkerscript")
	pkg.DownloadScript = GetStringIdentities(v, identities, "pkg.downloadscript")
	pkg.DebugScript = GetStringIdentities(v, identities, "pkg.debugscript")

	pkg.Cflags = ""
	cflags := GetStringSliceIdentities(v, identities, "pkg.cflags")
	for _, name := range cflags {
		pkg.Cflags += " " + name
	}
	pkg.Lflags = GetStringIdentities(v, identities, "pkg.lflags")
	pkg.Aflags = GetStringIdentities(v, identities, "pkg.aflags")

	pkg.DepLoaded = true

	return nil
}

// Collect identities and capabilities that pkg and it's dependencies provide
func (pkg *Pkg) collectDependencies(pkgList *PkgList,
	identities map[string]string,
	capabilities map[string]string) error {

	if pkg.DepLoaded {
		return nil
	}

	StatusMessage(VERBOSITY_VERBOSE, "  Collecting pkg %s dependencies\n", pkg.Name)

	err := pkg.LoadDependencies(identities, capabilities)
	if err != nil {
		return err
	}

	for _, dep := range pkg.Deps {
		pkg, err := pkgList.ResolvePkgName(dep.Name)
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

// Clear the var which says that dependencies have been checked for this pkg and
// it's dependencies
func (pkg *Pkg) clearDependencyMarker(pkgList *PkgList) {

	if pkg.DepLoaded == false {
		return
	}
	pkg.DepLoaded = false

	for _, dep := range pkg.Deps {
		pkg, err := pkgList.ResolvePkgName(dep.Name)
		if err == nil {
			pkg.clearDependencyMarker(pkgList)
		}
	}
}

// Load a pkg's configuration.  This allocates & initializes a fair number of
// the main data structures within the pkg.
func (pkg *Pkg) LoadConfig(t *Target, force bool) error {
	if pkg.CfgLoaded && !force {
		return nil
	}

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

	// Append all the identities that this pkg exposes to sub-pkgs.  This must
	// be done before the remainder of the settings, as some settings depend on
	// identity.
	identities := map[string]string{}
	if t != nil {
		identities = t.Identities
		idents := GetStringSliceIdentities(v, identities, "pkg.identities")
		for _, item := range idents {
			identities[item] = pkg.FullName
		}
	}

	pkg.LinkerScript = GetStringIdentities(v, identities, "pkg.linkerscript")
	pkg.DownloadScript = GetStringIdentities(v, identities, "pkg.downloadscript")
	pkg.DebugScript = GetStringIdentities(v, identities, "pkg.debugscript")

	pkg.Cflags += GetStringIdentities(v, identities, "pkg.cflags")
	pkg.Lflags += GetStringIdentities(v, identities, "pkg.lflags")
	pkg.Aflags += GetStringIdentities(v, identities, "pkg.aflags")

	// Load pkg dependencies
	depList := GetStringSliceIdentities(v, identities, "pkg.deps")
	if len(depList) > 0 {
		pkg.Deps = make([]*DependencyRequirement, 0, len(depList))
		for _, depStr := range depList {
			log.Printf("[DEBUG] Loading dependency %s from pkg %s", depStr,
				pkg.FullName)
			dr, err := NewDependencyRequirementParseString(depStr)
			if err != nil {
				return err
			}

			pkg.Deps = append(pkg.Deps, dr)
		}
	}

	// Load the list of capabilities that this pkg exposes
	pkg.Capabilities, err = pkg.loadCaps(GetStringSliceIdentities(v, identities,
		"pkg.caps"))
	if err != nil {
		return err
	}

	// Load the list of capabilities that this pkg requires
	pkg.ReqCapabilities, err = pkg.loadCaps(GetStringSliceIdentities(v, identities,
		"pkg.req_caps"))
	if err != nil {
		return err
	}
	if len(pkg.ReqCapabilities) > 0 {
		for _, reqStr := range pkg.ReqCapabilities {
			log.Printf("[DEBUG] Loading reqCap %s from pkg %s", reqStr,
				pkg.FullName)
		}
	}
	pkg.CfgLoaded = true

	return nil
}

// Initialize a pkg: loads the pkg configuration, and sets up pkg data
// structures.  Should only be called from NewPkg
func (pkg *Pkg) Init() error {
	return nil
}

// Allocate and initialize a new pkg, and return a fully initialized Pkg
//     structure.
// @param repo The Repo this pkg is located in
// @param basePath The path to this pkg, within the specified repo
// @return On success, error is nil, and a Pkg is returned.  on failure,
//         error is not nil.
func NewPkg(repo *Repo, basePath string) (*Pkg, error) {
	pkg := &Pkg{
		BasePath: basePath,
		Repo:     repo,
	}

	if err := pkg.Init(); err != nil {
		return nil, err
	}

	return pkg, nil
}

func (pkg *Pkg) TestBinName() string {
	return "test_" + pkg.Name
}

/*
 * Download pkg from a pkgList and stick it to repo.
 */
func (pkgDesc *PkgDesc) Install(pkgMgr *PkgList, branch string) error {
	downloaded, err := pkgMgr.InstallPkg(pkgDesc.FullName, branch, nil)
	for _, remoteRepo := range downloaded {
		remoteRepo.Remove()
	}
	return err
}

func (pkg *Pkg) Remove() error {
	return os.RemoveAll(pkg.BasePath)
}
