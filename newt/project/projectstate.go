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

package project

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
)

const PROJECT_STATE_FILE = "project.state"

type ProjectState struct {
	installedRepos map[string]*repo.Version
}

func (ps *ProjectState) GetInstalledVersion(rname string) *repo.Version {
	v, _ := ps.installedRepos[rname]
	return v
}

func (ps *ProjectState) Replace(rname string, rvers *repo.Version) {
	ps.installedRepos[rname] = rvers
}

func (ps *ProjectState) StateFile() string {
	return interfaces.GetProject().Path() + "/" + PROJECT_STATE_FILE
}

func (ps *ProjectState) Save() error {
	file, err := os.Create(ps.StateFile())
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer file.Close()

	for k, v := range ps.installedRepos {
		str := ""
		if v.Tag() == "" {
			str = fmt.Sprintf("%s,%d.%d.%d\n", k, v.Major(), v.Minor(), v.Revision())
		} else {
			str = fmt.Sprintf("%s,%s-tag\n", k, v.Tag())
		}
		file.WriteString(str)
	}

	return nil
}

func (ps *ProjectState) Init() error {
	ps.installedRepos = map[string]*repo.Version{}

	path := ps.StateFile()

	// Read project state file.  If doesn't exist, it will be empty until somebody
	// installs a repo
	if util.NodeNotExist(path) {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.Split(scanner.Text(), ",")
		if len(line) != 2 {
			return util.NewNewtError(fmt.Sprintf(
				"Invalid format for line in project.state file: %s\n", line))
		}

		repoName := line[0]
		repoVers, err := repo.LoadVersion(line[1])
		if err != nil {
			return err
		}

		ps.installedRepos[repoName] = repoVers
	}
	return nil
}

func LoadProjectState() (*ProjectState, error) {
	ps := &ProjectState{}
	if err := ps.Init(); err != nil {
		return nil, err
	}
	return ps, nil
}
