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
	"path/filepath"
	"sort"
	"strings"

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

func varsFromPackageType(
	pt interfaces.PackageType, fullPath bool) ([]string, error) {

	values := []string{}

	packs := project.GetProject().PackagesOfType(pt)
	for _, pack := range packs {
		value := pack.FullName()
		if !fullPath {
			value = filepath.Base(value)
		}

		values = append(values, value)
	}

	sort.Strings(values)

	return values, nil
}

func settingValues(settingName string) ([]string, error) {
	settingMap := map[string]struct{}{}

	packs := project.GetProject().PackagesOfType(-1)
	for _, pack := range packs {
		settings :=
			pack.(*pkg.LocalPackage).PkgY.GetValStringSlice(settingName, nil)

		for _, setting := range settings {
			settingMap[setting] = struct{}{}
		}
	}

	values := make([]string, 0, len(settingMap))
	for f, _ := range settingMap {
		values = append(values, f)
	}
	sort.Strings(values)

	return values, nil
}

func buildProfileValues() ([]string, error) {
	profileMap := map[string]struct{}{}

	packs := project.GetProject().PackagesOfType(pkg.PACKAGE_TYPE_COMPILER)
	for _, pack := range packs {
		v, err := newtutil.ReadConfig(pack.(*pkg.LocalPackage).BasePath(),
			"compiler")
		if err != nil {
			return nil, err
		}

		settingMap := v.AllSettings()
		for k, _ := range settingMap {
			if strings.HasPrefix(k, "compiler.flags") {
				fields := strings.Split(k, ".")
				if len(fields) >= 3 {
					profileMap[fields[2]] = struct{}{}
				}
			}
		}
	}

	values := make([]string, 0, len(profileMap))
	for k, _ := range profileMap {
		values = append(values, k)
	}

	sort.Strings(values)

	return values, nil
}

var varsMap = map[string]func() ([]string, error){
	// Package names.
	"app": func() ([]string, error) {
		return varsFromPackageType(pkg.PACKAGE_TYPE_APP, true)
	},
	"bsp": func() ([]string, error) {
		return varsFromPackageType(pkg.PACKAGE_TYPE_BSP, true)
	},
	"compiler": func() ([]string, error) {
		return varsFromPackageType(pkg.PACKAGE_TYPE_COMPILER, true)
	},
	"lib": func() ([]string, error) {
		return varsFromPackageType(pkg.PACKAGE_TYPE_LIB, true)
	},
	"sdk": func() ([]string, error) {
		return varsFromPackageType(pkg.PACKAGE_TYPE_SDK, true)
	},
	"target": func() ([]string, error) {
		return varsFromPackageType(pkg.PACKAGE_TYPE_TARGET, true)
	},

	// Package settings.
	"api": func() ([]string, error) {
		return settingValues("pkg.apis")
	},

	// Target settings.
	"build_profile": func() ([]string, error) {
		return buildProfileValues()
	},
}

// Returns a slice of valid values for the target variable with the specified
// name.  If an invalid target variable is specified, an error is returned.
func VarValues(varName string) ([]string, error) {
	_, err := project.TryGetProject()
	if err != nil {
		return nil, err
	}

	fn := varsMap[varName]
	if fn == nil {
		err := util.NewNewtError(fmt.Sprintf("Unknown setting name: \"%s\"",
			varName))
		return nil, err
	}

	values, err := fn()
	if err != nil {
		return nil, err
	}

	return values, nil
}

func VarTypes() []string {
	types := make([]string, 0, len(varsMap))

	for k, _ := range varsMap {
		types = append(types, k)
	}

	sort.Strings(types)
	return types
}
