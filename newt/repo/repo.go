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

package repo

import (
	"fmt"
	"path/filepath"

	"mynewt.apache.org/newt/viper"
)

const REPO_NAME_LOCAL = "local"

const REPOS_DIR = "repos"

type Repo struct {
	Name      string
	Url       string
	LocalPath string
	BasePath  string
}

func (r *Repo) Init(basePath string, repoName string, v *viper.Viper) error {
	r.Name = repoName
	r.BasePath = basePath

	if r.Name == REPO_NAME_LOCAL {
		r.LocalPath = filepath.Clean(basePath)
	} else {
		r.LocalPath = filepath.Clean(basePath + "/" + REPOS_DIR + "/" +
			repoName)
	}

	if v != nil {
		r.Url = v.GetString(fmt.Sprintf("%s.url", repoName))
	}

	return nil
}

func NewRepo(basePath string, repoName string,
	v *viper.Viper) (*Repo, error) {
	r := &Repo{}

	if err := r.Init(basePath, repoName, v); err != nil {
		return nil, err
	}

	return r, nil
}

func NewLocalRepo(basePath string) (*Repo, error) {
	r := &Repo{}

	if err := r.Init(basePath, REPO_NAME_LOCAL, nil); err != nil {
		return nil, err
	}

	return r, nil
}
