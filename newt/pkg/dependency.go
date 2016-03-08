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

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
)

type Dependency struct {
	Name string
	Repo string
}

func (dep *Dependency) String() string {
	str := fmt.Sprintf("@%s/%s", dep.Repo, dep.Name)
	return str
}

func (dep *Dependency) SatisfiesDependency(pkg interfaces.PackageInterface) bool {
	if dep.Name != pkg.Name() {
		return false
	}

	if dep.Repo != pkg.Repo().Name() {
		return false
	}

	return true
}

func (dep *Dependency) setRepoAndName(parentRepo interfaces.RepoInterface, str string) error {
	// First part is always repo/dependency name combination.
	// If repo is present, string will always begin with a $ sign
	// representing the repo name, followed by 'n' slashes.
	if strings.HasPrefix(str, "@") {
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

func (dep *Dependency) Init(parentRepo interfaces.RepoInterface, depStr string) error {
	if err := dep.setRepoAndName(parentRepo, depStr); err != nil {
		return err
	}

	return nil
}

func NewDependency(parentRepo interfaces.RepoInterface, depStr string) (*Dependency, error) {
	dep := &Dependency{}

	if err := dep.Init(parentRepo, depStr); err != nil {
		return nil, err
	}

	return dep, nil
}
