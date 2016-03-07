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
	"strings"

	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
)

type Dependency struct {
	Name      string
	Repo      string
	Stability string
	VersReqs  []*VersionMatch
}

func (dep *Dependency) SetVersionReqs(versStr string) error {
	var err error

	dep.VersReqs, err = LoadVersionMatches(versStr)
	if err != nil {
		return err
	}

	return nil
}

func (dep *Dependency) GetVersionReqsString() string {
	if dep.VersReqs == nil {
		return "none"
	}

	str := ""
	for _, vreq := range dep.VersReqs {
		str += fmt.Sprintf("%s%s", vreq.CompareType, vreq.Vers)
	}

	return str
}

func (dep *Dependency) String() string {
	str := fmt.Sprintf("$%s/%s@%s#%s", dep.Repo, dep.Name,
		dep.GetVersionReqsString(), dep.Stability)
	return str
}

func (dep *Dependency) SatisfiesDependency(pkg Package) bool {
	if dep.Name != pkg.Name() {
		return false
	}

	if dep.Repo != pkg.Repo().Name() {
		return false
	}

	if !pkg.Vers().SatisfiesVersion(dep.VersReqs) {
		return false
	}

	return true
}

func (dep *Dependency) setRepoAndName(parentRepo *repo.Repo, str string) error {
	// First part is always repo/dependency name combination.
	// If repo is present, string will always begin with a $ sign
	// representing the repo name, followed by 'n' slashes.
	if strings.HasPrefix(str, "$") {
		nameParts := strings.SplitN(str[1:], "/", 2)
		if len(nameParts) == 1 {
			return util.NewNewtError(fmt.Sprintf(
				"Must specify both repo and package name, no package detected: %s",
				str))
		}
		dep.Repo = nameParts[0]
		dep.Name = nameParts[1]
	} else {
		if parentRepo != nil {
			dep.Repo = parentRepo.Name()
		} else {
			dep.Repo = repo.REPO_NAME_LOCAL
		}
		dep.Name = str
	}

	return nil
}

func (dep *Dependency) Init(parentRepo *repo.Repo, depStr string) error {
	// Split string into multiple parts
	// @ separates repo/name from version requirements
	// # separates repo/name or version requirements from stability level
	parts := strings.Split(depStr, "@")
	if len(parts) == 1 {
		// There is no @ sign, which means version requirements don't
		// exist.  Now check for stability level
		parts = strings.Split(depStr, "#")

		if err := dep.setRepoAndName(parentRepo, parts[0]); err != nil {
			return err
		}

		if len(parts) > 1 {
			dep.Stability = parts[1]
		} else {
			dep.Stability = PACKAGE_STABILITY_STABLE
		}
	} else if len(parts) == 2 {
		if err := dep.setRepoAndName(parentRepo, parts[0]); err != nil {
			return err
		}
		verParts := strings.Split(parts[1], "#")

		if err := dep.SetVersionReqs(verParts[0]); err != nil {
			return err
		}
		if len(verParts) == 2 {
			dep.Stability = verParts[1]
		} else {
			dep.Stability = PACKAGE_STABILITY_STABLE
		}
	}

	return nil
}

func NewDependency(parentRepo *repo.Repo, depStr string) (*Dependency, error) {
	// Allocate depedency
	dep := &Dependency{}

	if err := dep.Init(parentRepo, depStr); err != nil {
		return nil, err
	}

	return dep, nil
}
