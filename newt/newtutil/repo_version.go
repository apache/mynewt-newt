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

package newtutil

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mynewt.apache.org/newt/util"

	log "github.com/Sirupsen/logrus"
)

const (
	VERSION_STABILITY_NONE   = ""
	VERSION_STABILITY_STABLE = "stable"
	VERSION_STABILITY_DEV    = "dev"
	VERSION_STABILITY_LATEST = "latest"

	// "commit" is not actually a stability, but it takes the place of one in
	// the repo version notation.  The "commit" string indicates a commit hash,
	// tag, or branch, rather than a version specifier.
	VERSION_STABILITY_COMMIT = "commit"
)

// Represents an unspecified part in a version.  For example, in "1-latest",
// the minor and revision parts are floating.
const VERSION_FLOATING = -1

type RepoVersionReq struct {
	CompareType string
	Ver         RepoVersion
}

type RepoVersion struct {
	Major     int64
	Minor     int64
	Revision  int64
	Stability string
	Commit    string
}

func (v *RepoVersion) IsNormalized() bool {
	return v.Stability == VERSION_STABILITY_NONE
}

func (vm *RepoVersionReq) String() string {
	return vm.CompareType + vm.Ver.String()
}

func CompareRepoVersions(v1 RepoVersion, v2 RepoVersion) int64 {
	if r := v1.Major - v2.Major; r != 0 {
		return r
	}

	if r := v1.Minor - v2.Minor; r != 0 {
		return r
	}

	if r := v1.Revision - v2.Revision; r != 0 {
		return r
	}

	return 0
}

func (v *RepoVersion) Satisfies(verReq RepoVersionReq) bool {
	if verReq.Ver.Commit != "" && verReq.CompareType != "==" {
		log.Warningf("RepoVersion comparison with a tag %s %s %s",
			verReq.Ver, verReq.CompareType, v)
	}
	r := CompareRepoVersions(verReq.Ver, *v)
	switch verReq.CompareType {
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

	if verReq.Ver.Stability != v.Stability {
		return false
	}

	return true
}

func (v *RepoVersion) SatisfiesAll(verReqs []RepoVersionReq) bool {
	for _, r := range verReqs {
		if !v.Satisfies(r) {
			return false
		}
	}

	return true
}

func (ver *RepoVersion) String() string {
	s := fmt.Sprintf("%d", ver.Major)
	if ver.Minor != VERSION_FLOATING {
		s += fmt.Sprintf(".%d", ver.Minor)
	}
	if ver.Revision != VERSION_FLOATING {
		s += fmt.Sprintf(".%d", ver.Revision)
	}

	if ver.Stability != VERSION_STABILITY_NONE {
		s += fmt.Sprintf("-%s", ver.Stability)
	}

	if ver.Commit != "" {
		s += fmt.Sprintf("/%s", ver.Commit)
	}

	return s
}

func (ver *RepoVersion) ToNuVersion() Version {
	return Version{
		Major:    ver.Major,
		Minor:    ver.Minor,
		Revision: ver.Revision,
	}
}

func ParseRepoVersion(verStr string) (RepoVersion, error) {
	var err error

	stability := VERSION_STABILITY_NONE
	base := verStr

	dashIdx := strings.LastIndex(verStr, "-")
	if dashIdx != -1 {
		stability = strings.TrimSpace(verStr[dashIdx+1:])
		base = strings.TrimSpace(verStr[:dashIdx])

		switch stability {
		case VERSION_STABILITY_COMMIT:
			return RepoVersion{Commit: strings.TrimSpace(base)}, nil

		case VERSION_STABILITY_STABLE:
		case VERSION_STABILITY_DEV:
		case VERSION_STABILITY_LATEST:

		default:
			return RepoVersion{}, util.FmtNewtError(
				"Unknown stability (%s) in version %s", stability, verStr)
		}
	}

	parts := strings.Split(base, ".")
	if len(parts) > 3 {
		return RepoVersion{},
			util.FmtNewtError("Invalid version string: %s", verStr)
	}

	if len(parts) != 3 && stability == VERSION_STABILITY_NONE {
		return RepoVersion{},
			util.FmtNewtError("Invalid version string: %s", verStr)
	}

	// Assume no parts of the version are specified.
	ver := RepoVersion{
		Major:     VERSION_FLOATING,
		Minor:     VERSION_FLOATING,
		Revision:  VERSION_FLOATING,
		Stability: stability,
	}

	// Convert each dot-delimited part to an integer.
	if ver.Major, err = strconv.ParseInt(parts[0], 10, 64); err != nil {
		return RepoVersion{}, util.NewNewtError(err.Error())
	}
	if len(parts) >= 2 {
		if ver.Minor, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
			return RepoVersion{}, util.NewNewtError(err.Error())
		}
	}
	if len(parts) == 3 {
		if ver.Revision, err = strconv.ParseInt(parts[2], 10, 64); err != nil {
			return RepoVersion{}, util.NewNewtError(err.Error())
		}
	}

	return ver, nil
}

// Parse a set of version string constraints on a dependency.
// This function
// The version string contains a list of version constraints in the following format:
//    - <comparison><version>
// Where <comparison> can be any one of the following comparison
//   operators: <=, <, >, >=, ==
// And <version> is specified in the form: X.Y.Z where X, Y and Z are all
// int64 types in decimal form
func ParseRepoVersionReqs(versStr string) ([]RepoVersionReq, error) {
	var err error

	verReqs := []RepoVersionReq{}

	re, err := regexp.Compile(`(<=|>=|==|>|<)([\d\.]+)`)
	if err != nil {
		return nil, err
	}

	matches := re.FindAllStringSubmatch(versStr, -1)
	if matches != nil {
		for _, match := range matches {
			vm := RepoVersionReq{}
			vm.CompareType = match[1]
			if vm.Ver, err = ParseRepoVersion(match[2]); err != nil {
				return nil, err
			}

			verReqs = append(verReqs, vm)
		}
	} else {
		vm := RepoVersionReq{}
		vm.CompareType = "=="
		if vm.Ver, err = ParseRepoVersion(versStr); err != nil {
			return nil, err
		}

		verReqs = append(verReqs, vm)
	}

	if len(verReqs) == 0 {
		verReqs = nil
	}

	return verReqs, nil
}

func RepoVerReqsString(verReqs []RepoVersionReq) string {
	s := ""
	for i, r := range verReqs {
		if i != 0 {
			s += " "
		}
		s += r.String()
	}

	return s
}

type verSorter struct {
	vers []RepoVersion
}

func (v verSorter) Len() int {
	return len(v.vers)
}
func (v verSorter) Swap(i, j int) {
	v.vers[i], v.vers[j] = v.vers[j], v.vers[i]
}
func (v verSorter) Less(i, j int) bool {
	a := v.vers[i]
	b := v.vers[j]

	return CompareRepoVersions(a, b) < 0
}

func SortVersions(vers []RepoVersion) {
	sorter := verSorter{
		vers: vers,
	}

	sort.Sort(sorter)
}

func SortedVersions(vers []RepoVersion) []RepoVersion {
	clone := make([]RepoVersion, len(vers))
	copy(clone, vers)

	SortVersions(clone)
	return clone
}

func SortedVersionsDesc(vers []RepoVersion) []RepoVersion {
	slice := SortedVersions(vers)
	size := len(slice)
	for i := 0; i < size/2; i++ {
		j := size - 1 - i
		slice[i], slice[j] = slice[j], slice[i]
	}

	return slice
}
