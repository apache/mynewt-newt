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

package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The browser keeps its own settings, e.g. whether the boards pane is
// hidden, in ~/.newt/tui.yml.

// tuiConfigPath is where the browser's own settings are kept; replaced in
// tests.
var tuiConfigPath = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".newt", "tui.yml")
}

// savedSetting returns one of the browser's settings in tui.yml, or "".
func savedSetting(name string) string {
	path := tuiConfigPath()
	if path == "" || !fileExists(path) {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return ymlVar(string(data), name)
}

// ymlVar returns the value of a top-level "<variable>: <value>" line of a
// yml file, as setYmlVar writes it.
func ymlVar(yml string, variable string) string {
	for _, line := range strings.Split(yml, "\n") {
		rest, ok := strings.CutPrefix(line, variable+":")
		if !ok {
			continue
		}
		value := strings.TrimSpace(rest)
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
		return strings.Trim(value, "'")
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// saveSetting changes one of the browser's settings in tui.yml.
func saveSetting(name string, value string) error {
	path := tuiConfigPath()
	if path == "" {
		return errors.New("no home directory to save settings in")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	data = []byte(setYmlVar(string(data), name, value))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return nil
}
