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

package target

import (
	"fmt"
	"strings"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
)

func varsFromChildDirs(key string, fullPath bool) ([]string, error) {
	valueSlice := []string{}

	repos := project.GetProject().Repos()
	searchDirs := project.GetProject().PackageSearchDirs()
	for _, r := range repos {
		for _, pkgDir := range searchDirs {
			pkgBaseDir := r.LocalPath + "/" + pkgDir
			values, err := cli.DescendantDirsOfParent(pkgBaseDir, key, fullPath)
			if err != nil {
				return nil, util.NewNewtError(err.Error())
			}

			for _, value := range values {
				if fullPath {
					value = strings.TrimPrefix(value, r.BasePath+"/")
				}
				if strings.HasPrefix(value, repo.REPOS_DIR+"/") {
					value = "$" + strings.TrimPrefix(value, repo.REPOS_DIR+"/")
				}
				valueSlice = append(valueSlice, value)
			}
		}
	}

	return cli.SortFields(valueSlice...), nil
}

var varsMap = map[string]func() ([]string, error){
	"arch": func() ([]string, error) {
		return varsFromChildDirs("arch", false)
	},

	"bsp": func() ([]string, error) {
		return varsFromChildDirs("bsp", true)
	},

	"compiler": func() ([]string, error) {
		return varsFromChildDirs("compiler", true)
	},

	"project": func() ([]string, error) {
		return varsFromChildDirs("project", true)
	},
}

// Returns a slice of valid values for the target variable with the specified
// name.  If an invalid target variable is specified, an error is returned.
func VarValues(varName string) ([]string, error) {
	fn := varsMap[varName]
	if fn == nil {
		err := util.NewNewtError(fmt.Sprintf("Unknown target variable: \"%s\"", varName))
		return nil, err
	}

	values, err := fn()
	if err != nil {
		return nil, err
	}

	return values, nil
}
