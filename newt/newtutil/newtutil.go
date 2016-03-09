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

package newtutil

import (
	"fmt"
	"sort"
	"strings"

	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
)

func GetStringFeatures(v *viper.Viper, features map[string]bool,
	key string) string {
	val := v.GetString(key)

	// Process the features in alphabetical order to ensure consistent
	// results across repeated runs.
	var featureKeys []string
	for feature, _ := range features {
		featureKeys = append(featureKeys, feature)
	}
	sort.Strings(featureKeys)

	for _, feature := range featureKeys {
		overwriteVal := v.GetString(key + "." + feature + ".OVERWRITE")
		if overwriteVal != "" {
			val = strings.Trim(overwriteVal, "\n")
			break
		}

		appendVal := v.GetString(key + "." + feature)
		if appendVal != "" {
			val += " " + strings.Trim(appendVal, "\n")
		}
	}
	return strings.TrimSpace(val)
}

func GetStringSliceFeatures(v *viper.Viper, features map[string]bool,
	key string) []string {

	val := v.GetStringSlice(key)

	// string empty items
	result := []string{}
	for _, item := range val {
		if item == "" || item == " " {
			continue
		}
		result = append(result, item)
	}

	for item, _ := range features {
		result = append(result, v.GetStringSlice(key+"."+item)...)
	}

	return result
}

// Parses a string of the following form:
//     [@repo]<path/to/package>
//
// @return string               repo name ("" if no repo)
//         string               package name
//         error                if invalid package string
func ParsePackageString(pkgStr string) (string, string, error) {
	if strings.HasPrefix(pkgStr, "@") {
		nameParts := strings.SplitN(pkgStr[1:], "/", 2)
		if len(nameParts) == 1 {
			return "", "", util.NewNewtError(fmt.Sprintf("Invalid package "+
				"string; contains repo but no package name: %s", pkgStr))
		} else {
			return nameParts[0], nameParts[1], nil
		}
	} else {
		return "", pkgStr, nil
	}
}

func BuildPackageString(repoName string, pkgName string) string {
	if repoName != "" {
		return "@" + repoName + "/" + pkgName
	} else {
		return pkgName
	}
}
