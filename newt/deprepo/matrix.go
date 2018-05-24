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

// deprepo: Package for resolving repo dependencies.
package deprepo

import (
	"fmt"
	"strings"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

// Eliminates non-matching version numbers when applied to a matrix row.
type Filter struct {
	Name string
	Reqs []newtutil.RepoVersionReq
}

// Contains all versions of a single repo.  These version numbers are read from
// the repo's `repository.yml` file.  Only normalized versions are included.
type MatrixRow struct {
	// The name of the repo that the row corresponds to.
	RepoName string

	// All normalized versions of the repo.
	Vers []newtutil.RepoVersion

	// Indicates the version of this repo currently being evaluated for
	// conflicts.
	VerIdx int

	// All filters that have been applied to this row.  This is only used
	// during reporting.
	Filters []Filter
}

// Contains all versions of a set of repos.  Each row correponds to a single
// repo.  Each element within a row represents a single version of the repo.
//
// The Matrix type serves two purposes:
//
// 1. Simple lookup: Provides a convenient means of determining whether a
// specific version of a repo exists.
//
// 2. Requirements matching: The client can cycle through all combinations of
// repo versions via the `Increment()` function.  Each combination can be
// exported as a version map via the `CurVersions()` function.  By evaluating
// each version map against a set of requirements, the client can find the set
// of repo versions to upgrade to.
type Matrix struct {
	rows []MatrixRow
}

func (m *Matrix) String() string {
	lines := make([]string, len(m.rows))

	for i, row := range m.rows {
		line := fmt.Sprintf("%s:", row.RepoName)
		for _, v := range row.Vers {
			line += fmt.Sprintf(" %s", v.String())
		}

		lines[i] = line
	}

	return strings.Join(lines, "\n")
}

// Adjusts the matrix to point to the next possible set of repo versions.
//
// @return bool                 true if the matrix points to a new set;
//                              false if the matrix wrapped around to the first
//                                  set.
func (m *Matrix) Increment() bool {
	for i := range m.rows {
		row := &m.rows[i]

		row.VerIdx++
		if row.VerIdx < len(row.Vers) {
			return true
		}

		// No more versions left for this repo; proceed to next.
		row.VerIdx = 0
	}

	// All version combinations evaluated.
	return false
}

func (m *Matrix) findRowIdx(repoName string) int {
	for i, row := range m.rows {
		if row.RepoName == repoName {
			return i
		}
	}

	return -1
}

func (m *Matrix) FindRow(repoName string) *MatrixRow {
	idx := m.findRowIdx(repoName)
	if idx == -1 {
		return nil
	}
	return &m.rows[idx]
}

func (m *Matrix) AddRow(repoName string,
	vers []newtutil.RepoVersion) error {

	if m.findRowIdx(repoName) != -1 {
		return util.FmtNewtError("Duplicate repo \"%s\" in repo matrix",
			repoName)
	}

	m.rows = append(m.rows, MatrixRow{
		RepoName: repoName,
		Vers:     newtutil.SortedVersionsDesc(vers),
	})

	return nil
}

// Removes all non-matching versions of the specified repo from the matrix.
func (m *Matrix) ApplyFilter(repoName string, filter Filter) {
	rowIdx := m.findRowIdx(repoName)
	if rowIdx == -1 {
		return
	}
	row := &m.rows[rowIdx]

	goodVers := []newtutil.RepoVersion{}
	for _, v := range row.Vers {
		if v.SatisfiesAll(filter.Reqs) {
			goodVers = append(goodVers, v)
		}
	}

	row.Vers = goodVers
	row.Filters = append(row.Filters, filter)
}

// Constructs a version map from the matrix's current state.
func (m *Matrix) CurVersions() VersionMap {
	vm := make(VersionMap, len(m.rows))
	for _, row := range m.rows {
		if len(row.Vers) > 0 {
			vm[row.RepoName] = row.Vers[row.VerIdx]
		}
	}

	return vm
}
