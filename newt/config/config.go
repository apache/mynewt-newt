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

// The config package handles reading of newt YAML files.
package config

import (
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/yaml"
)

const (
	KEYWORD_IMPORT = "$import"
)

// keywordMap is a map of all supported keywords.  Config keywords always start
// with "$".
var keywordMap = map[string]struct{}{
	KEYWORD_IMPORT: struct{}{},
}

// FileEntry represents a single YAML file.  It does not contain import
// information.
type FileEntry struct {
	FileInfo *util.FileInfo
	Settings map[string]interface{}
}

func readSettings(path string) (map[string]interface{}, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, util.ChildNewtError(err)
	}

	settings := map[string]interface{}{}
	if err := yaml.Unmarshal(file, &settings); err != nil {
		return nil, util.FmtNewtError("Failure parsing \"%s\": %s",
			path, err.Error())
	}

	return settings, nil
}

func readFileEntry(path string, parent *util.FileInfo) (FileEntry, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return FileEntry{}, err
	}

	settings, err := readSettings(absPath)
	if err != nil {
		return FileEntry{}, err
	}

	return FileEntry{
		FileInfo: &util.FileInfo{
			Path:   absPath,
			Parent: parent,
		},
		Settings: settings,
	}, nil
}

func extractImports(settings map[string]interface{}) ([]string, error) {
	itf := settings[KEYWORD_IMPORT]
	if itf == nil {
		return nil, nil
	}

	strs, err := cast.ToStringSliceE(itf)
	if err != nil {
		return nil, util.FmtNewtError(
			"invalid %s section; must contain sequence of strings",
			KEYWORD_IMPORT)
	}

	return strs, nil
}

func (fe *FileEntry) warnUnrecognizedKeywords() {
	m := map[string]struct{}{}

	// Find all unrecognized entries starting with "$".
	for k, _ := range fe.Settings {
		if strings.HasPrefix(k, "$") {
			if _, ok := keywordMap[k]; !ok {
				m[k] = struct{}{}
			}
		}
	}

	if len(m) == 0 {
		return
	}

	keywords := make([]string, 0, len(m))
	for k, _ := range m {
		keywords = append(keywords, k)
	}
	sort.Strings(keywords)

	s := ""
	for _, k := range keywords {
		s += "\n    " + k
	}

	util.OneTimeWarning(
		"%s contains unrecognized keywords: %s\n"+
			"you may need to upgrade your version of newt.",
		fe.FileInfo.Path, s)
}

// readLineage reads a configuration file and all files it imports (directly
// or indirectly).  The resulting []FileEntry is sorted in the order the
// corresponding files were read.
func readLineage(path string) ([]FileEntry, error) {
	entries := []FileEntry{}
	seen := map[string]struct{}{}

	// Recursively process imports, accumulating file info in `entries`.
	var iter func(path string, parent *util.FileInfo) error
	iter = func(path string, parent *util.FileInfo) error {
		// Relative paths are relative to the project base.
		if !filepath.IsAbs(path) {
			proj := interfaces.GetProject()
			newPath, err := proj.ResolvePath(proj.Path(), path)
			if err != nil {
				return err
			}
			path = newPath
		}

		// Only operate on absolute paths to ensure each file has a unique ID.
		absPath, err := filepath.Abs(path)
		if err != nil {
			return parent.ErrTree(err)
		}

		// Don't process the same config file twice.
		if _, ok := seen[absPath]; ok {
			return nil
		}
		seen[absPath] = struct{}{}

		entry, err := readFileEntry(path, parent)
		if err != nil {
			return parent.ErrTree(err)
		}

		imports, err := extractImports(entry.Settings)
		if err != nil {
			return err
		}

		for _, imp := range imports {
			if err := iter(imp, entry.FileInfo); err != nil {
				return err
			}
		}

		// Only add the top-level entry now that the imports have been
		// processed.  This comes last so that it can override settings
		// specified by imported files.
		entries = append(entries, entry)

		return nil
	}

	// Recursively read imported configuration files.
	if err := iter(path, nil); err != nil {
		return nil, err
	}

	// Log the configuration files that were read.  If there are imports, log
	// it using a tree notation.
	if len(entries) == 1 {
		log.Debugf("Read config file: %s",
			newtutil.ProjRelPath(entries[0].FileInfo.Path))
	} else {
		tree, err := BuildTree(entries)
		if err != nil {
			return nil, err
		}
		log.Debugf("Read config files:\n%s", TreeString(tree))
	}

	return entries, nil
}

// ReadFile reads a YAML file, processes all its `$import` directives, and
// returns a populated YCfg tree.
func ReadFile(path string) (ycfg.YCfg, error) {
	yc := ycfg.NewYCfg(path)

	entries, err := readLineage(path)
	if err != nil {
		return yc, err
	}

	for _, e := range entries {
		for k, v := range e.Settings {
			if err := yc.MergeFromFile(k, v, e.FileInfo); err != nil {
				return yc, e.FileInfo.Parent.ErrTree(err)
			}
		}
	}

	return yc, nil
}
