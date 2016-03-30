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

// XXX: This should be moved to the cli package.

package target

import (
	"fmt"
	"path/filepath"
	"strings"

	. "mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
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
			pkgBaseDir := r.Path() + "/" + pkgDir
			values, err := util.DescendantDirsOfParent(pkgBaseDir, key,
				fullPath)
			if err != nil {
				return nil, util.NewNewtError(err.Error())
			}

			for _, value := range values {
				if fullPath {
					value = strings.TrimPrefix(value,
						project.GetProject().Path()+"/")
				}
				if strings.HasPrefix(value, repo.REPOS_DIR+"/") {
					parts := strings.SplitN(value, "/", 2)
					if len(parts) > 1 {
						value = newtutil.BuildPackageString(parts[0], parts[1])
					}
				}
				valueSlice = append(valueSlice, value)
			}
		}
	}

	return util.SortFields(valueSlice...), nil
}

func varsFromPackageType(pt PackageType, fullPath bool) ([]string, error) {
	values := []string{}

	packs := project.GetProject().PackagesOfType(pt)
	for _, pack := range packs {
		value := pack.FullName()
		if !fullPath {
			value = filepath.Base(value)
		}

		values = append(values, value)
	}

	return values, nil
}

var varsMap = map[string]func() ([]string, error){
	"target.bsp": func() ([]string, error) {
		return varsFromPackageType(pkg.PACKAGE_TYPE_BSP, true)
	},

	"target.app": func() ([]string, error) {
		return varsFromPackageType(pkg.PACKAGE_TYPE_APP, true)
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
