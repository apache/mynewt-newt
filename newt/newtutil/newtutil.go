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
	"os/user"
	"sort"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
)

var NewtVersionStr string = "Apache Newt (incubating) version: 0.9.0"
var NewtBlinkyTag string = "mynewt_0_9_0_tag"

const NEWTRC_DIR string = ".newt"
const REPOS_FILENAME string = "repos.yml"

// Contains general newt settings read from $HOME/.newt
var newtrc *viper.Viper

func readNewtrc() *viper.Viper {
	usr, err := user.Current()
	if err != nil {
		log.Warn("Failed to obtain user name")
		return viper.New()
	}

	dir := usr.HomeDir + "/" + NEWTRC_DIR
	v, err := util.ReadConfig(dir, strings.TrimSuffix(REPOS_FILENAME, ".yml"))
	if err != nil {
		log.Debugf("Failed to read %s/%s file", dir, REPOS_FILENAME)
		return viper.New()
	}

	return v
}

func Newtrc() *viper.Viper {
	if newtrc != nil {
		return newtrc
	}

	newtrc = readNewtrc()
	return newtrc
}

func GetSliceFeatures(v *viper.Viper, features map[string]bool,
	key string) []interface{} {

	val := v.Get(key)
	vals := []interface{}{val}

	// Process the features in alphabetical order to ensure consistent
	// results across repeated runs.
	var featureKeys []string
	for feature, _ := range features {
		featureKeys = append(featureKeys, feature)
	}
	sort.Strings(featureKeys)

	for _, feature := range featureKeys {
		overwriteVal := v.Get(key + "." + feature + ".OVERWRITE")
		if overwriteVal != nil {
			return []interface{}{overwriteVal}
		}

		appendVal := v.Get(key + "." + feature)
		if appendVal != nil {
			vals = append(vals, appendVal)
		}
	}

	return vals
}

func GetStringMapFeatures(v *viper.Viper, features map[string]bool,
	key string) map[string]interface{} {

	result := map[string]interface{}{}

	slice := GetSliceFeatures(v, features, key)
	for _, itf := range slice {
		sub := cast.ToStringMap(itf)
		for k, v := range sub {
			result[k] = v
		}
	}

	return result
}

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

func GetBoolFeatures(v *viper.Viper, features map[string]bool,
	key string) (bool, error) {

	s := GetStringFeatures(v, features, key)
	if s == "" {
		return false, nil
	}

	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, util.FmtNewtError("invalid bool value for %s: %s",
			key, s)
	}

	return b, nil
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
		overwriteVal := v.GetStringSlice(key + "." + item + ".OVERWRITE")
		if overwriteVal != nil {
			result = overwriteVal
			break
		}

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
