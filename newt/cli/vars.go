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

package cli

import (
	"fmt"
	"strings"
)

func varsFromChildDirs(key string, fullPath bool) ([]string, error) {
	repo, err := NewRepo()
	if err != nil {
		return nil, err
	}

	valueSlice := []string{}
	searchDirs := repo.PkgPaths()
	for _, pkgDir := range searchDirs {
		pkgBaseDir := repo.BasePath + "/" + pkgDir
		values, err := DescendantDirsOfParent(pkgBaseDir, key, fullPath)
		if err != nil {
			return nil, NewNewtError(err.Error())
		}

		for _, value := range values {
			if fullPath {
				value = strings.TrimPrefix(value, repo.BasePath+"/")
			}
			valueSlice = append(valueSlice, value)
		}
	}

	return SortFields(valueSlice...), nil
}

var varsMap = map[string]func() ([]string, error){
	"arch": func() ([]string, error) {
		return varsFromChildDirs("arch", false)
	},

	"bsp": func() ([]string, error) {
		return varsFromChildDirs("bsp", true)
	},

	"compiler": func() ([]string, error) {
		return varsFromChildDirs("compiler", false)
	},

	"apps": func() ([]string, error) {
		return varsFromChildDirs("apps", false)
	},
}

// Returns a slice of valid values for the target variable with the specified
// name.  If an invalid target variable is specified, an error is returned.
func VarValues(varName string) ([]string, error) {
	fn := varsMap[varName]
	if fn == nil {
		err := NewNewtError(fmt.Sprintf("Unknown target variable: \"%s\"", varName))
		return nil, err
	}

	values, err := fn()
	if err != nil {
		return nil, err
	}

	return values, nil
}
