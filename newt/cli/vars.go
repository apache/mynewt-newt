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
	"sort"
)

func varsFromChildDirs(key string) ([]string, error) {
	repo, err := NewRepo()
	if err != nil {
		return nil, err
	}

	valueMap := make(map[string]struct{})
	searchDirs := repo.PkgPaths()
	for _, pkgDir := range searchDirs {
		pkgBaseDir := repo.BasePath + "/" + pkgDir
		values, err := DescendantDirsOfParent(pkgBaseDir, key)
		if err != nil {
			return nil, NewNewtError(err.Error())
		}

		for _, value := range values {
			valueMap[value] = struct{}{}
		}
	}

	valueSlice := make([]string, 0, len(valueMap))
	for value, _ := range valueMap {
		valueSlice = append(valueSlice, value)
	}

	return valueSlice, nil
}

var varsMap = map[string]func() ([]string, error){
	"arch": func() ([]string, error) {
		return varsFromChildDirs("arch")
	},

	"bsp": func() ([]string, error) {
		return varsFromChildDirs("bsp")
	},

	"compiler": func() ([]string, error) {
		return varsFromChildDirs("compiler")
	},

	"project": func() ([]string, error) {
		return varsFromChildDirs("project")
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

	sort.Strings(values)
	return values, nil
}
