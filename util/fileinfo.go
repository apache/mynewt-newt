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

package util

import (
	"fmt"
)

// FileInfo represents a configuration source.  It is intended to help the user
// understand how the system acquired its configuration, and to aid in tracking
// down errors in configuration files.
type FileInfo struct {
	Path   string    // Path of configuration file.
	Parent *FileInfo // File that imported this configuration file, if any.
}

// ImportString creates a string describing the import hierarchy of the given
// FileInfo.  It should be called on the *parent* of the file of interest.
func (fi *FileInfo) ImportString() string {
	s := ""

	first := true
	for fi != nil {
		if !first {
			s += ", "
		}
		first = false

		s += fmt.Sprintf("imported from %s", fi.Path)
		fi = fi.Parent
	}

	return s
}

// ErrTree decorates the given error with a description of the configuration
// file's import hierarchy.  If a configuration error is encountered, this
// function should be called on the configuration file's *parent*.
func (fi *FileInfo) ErrTree(err error) error {
	if fi == nil {
		return err
	}

	return FmtNewtError("%s - %s", err.Error(), fi.ImportString())
}
