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

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
)

var NewtVersionStr string = "Apache Newt (incubating) version: 1.0.1-dev"
var NewtBlinkyTag string = "develop"
var NewtNumJobs int
var NewtForce bool

const NEWTRC_DIR string = ".newt"
const REPOS_FILENAME string = "repos.yml"

const CORE_REPO_NAME string = "apache-mynewt-core"
const ARDUINO_ZERO_REPO_NAME string = "mynewt_arduino_zero"

type RepoCommitEntry struct {
	Version     string
	Hash        string
	Description string
}

// A warning is displayed if newt requires a newer version of a repo.
var RepoMinCommits = map[string]*RepoCommitEntry{
	// Newt no longer cd's to a source directory when it compiles its contents.
	// Consequently, package include flags need to be relative to the project
	// directory, not the package source directory.
	CORE_REPO_NAME: &RepoCommitEntry{
		Version:     "develop",
		Hash:        "cd99344df197d5b9e372b93142184a39ec078f69",
		Description: "Include paths now relative to project base.",
	},
	ARDUINO_ZERO_REPO_NAME: &RepoCommitEntry{
		Version:     "develop",
		Hash:        "a6348961fef56dbfe09a1b9418d3add3ad22eaf2",
		Description: "Include paths now relative to project base.",
	},
}

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
	featureKeys := make([]string, 0, len(features))
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

func GetBoolFeaturesDflt(v *viper.Viper, features map[string]bool,
	key string, dflt bool) (bool, error) {

	s := GetStringFeatures(v, features, key)
	if s == "" {
		return dflt, nil
	}

	b, err := strconv.ParseBool(s)
	if err != nil {
		return dflt, util.FmtNewtError("invalid bool value for %s: %s",
			key, s)
	}

	return b, nil
}

func GetBoolFeatures(v *viper.Viper, features map[string]bool,
	key string) (bool, error) {

	return GetBoolFeaturesDflt(v, features, key, false)
}

func GetStringSliceFeatures(v *viper.Viper, features map[string]bool,
	key string) []string {

	vals := GetSliceFeatures(v, features, key)

	strVals := []string{}
	for _, v := range vals {
		subVals := cast.ToStringSlice(v)
		strVals = append(strVals, subVals...)
	}

	return strVals
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

func FindRepoDesignator(s string) (int, int) {
	start := strings.Index(s, "@")
	if start == -1 {
		return -1, -1
	}

	len := strings.Index(s[start:], "/")
	if len == -1 {
		return -1, -1
	}

	return start, len
}

func ReplaceRepoDesignators(s string) (string, bool) {
	start, len := FindRepoDesignator(s)
	if start == -1 {
		return s, false
	}
	repoName := s[start+1 : start+len]

	proj := interfaces.GetProject()
	repoPath := proj.FindRepoPath(repoName)
	if repoPath == "" {
		return s, false
	}

	// Trim common project base from repo path.
	relRepoPath := strings.TrimPrefix(repoPath, proj.Path()+"/")

	return s[:start] + relRepoPath + s[start+len:], true
}

func BuildPackageString(repoName string, pkgName string) string {
	if repoName != "" {
		return "@" + repoName + "/" + pkgName
	} else {
		return pkgName
	}
}

func GeneratedPreamble() string {
	return fmt.Sprintf("/**\n * This file was generated by %s\n */\n\n",
		NewtVersionStr)
}
