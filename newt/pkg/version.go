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

package pkg

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"mynewt.apache.org/newt/util"
)

const VERSION_FORMAT = "%d.%d.%d"

type VersionMatch struct {
	CompareType string
	Vers        *Version
}

type Version struct {
	Major    int64
	Minor    int64
	Revision int64
}

func (v *Version) compareVersions(vers1 *Version, vers2 *Version) int64 {
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

func (v *Version) SatisfiesVersion(versMatches []*VersionMatch) bool {
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
	return fmt.Sprintf(VERSION_FORMAT, vers.Major, vers.Minor, vers.Revision)
}

func LoadVersion(versStr string) (*Version, error) {
	var err error

	parts := strings.Split(versStr, ".")
	if len(parts) > 3 {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid version string: %s", versStr))
	}

	if strings.Trim(parts[0], " ") == "" || strings.Trim(parts[0], " ") == "none" {
		return nil, nil
	}

	vers := &Version{}

	// convert first string to an int
	if vers.Major, err = strconv.ParseInt(parts[0], 0, 64); err != nil {
		return nil, util.NewNewtError(err.Error())
	}
	if len(parts) >= 2 {
		if vers.Minor, err = strconv.ParseInt(parts[1], 0, 64); err != nil {
			return nil, util.NewNewtError(err.Error())
		}
	}
	if len(parts) == 3 {
		if vers.Revision, err = strconv.ParseInt(parts[2], 0, 64); err != nil {
			return nil, util.NewNewtError(err.Error())
		}
	}

	return vers, nil
}

// Parse a set of version string constraints on a dependency.
// This function
// The version string contains a list of version constraints in the following format:
//    - <comparison><version>
// Where <comparison> can be any one of the following comparison
//   operators: <=, <, >, >=, ==
// And <version> is specified in the form: X.Y.Z where X, Y and Z are all
// int64 types in decimal form
func LoadVersionMatches(versStr string) ([]*VersionMatch, error) {
	var err error

	versMatches := []*VersionMatch{}

	re, err := regexp.Compile(`(<=|>=|==|>|<)([\d\.]+)`)
	if err != nil {
		return nil, err
	}

	matches := re.FindAllStringSubmatch(versStr, -1)
	if matches != nil {
		for _, match := range matches {
			vm := &VersionMatch{}
			vm.CompareType = match[1]
			if vm.Vers, err = LoadVersion(match[2]); err != nil {
				return nil, err
			}

			if vm.Vers != nil {
				versMatches = append(versMatches, vm)
			}
		}
	} else {
		vm := &VersionMatch{}
		vm.CompareType = "=="
		if vm.Vers, err = LoadVersion(versStr); err != nil {
			return nil, err
		}

		if vm.Vers != nil {
			versMatches = append(versMatches, vm)
		}
	}

	if len(versMatches) == 0 {
		versMatches = nil
	}

	return versMatches, nil
}
