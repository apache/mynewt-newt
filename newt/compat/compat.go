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

package compat

import (
	"fmt"
	"math"
	"sort"

	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

type NewtCompatCode int

const (
	NEWT_COMPAT_GOOD NewtCompatCode = iota
	NEWT_COMPAT_WARN
	NEWT_COMPAT_ERROR
)

var NewtCompatCodeNames = map[NewtCompatCode]string{
	NEWT_COMPAT_GOOD:  "good",
	NEWT_COMPAT_WARN:  "warn",
	NEWT_COMPAT_ERROR: "error",
}

type NewtCompatEntry struct {
	code       NewtCompatCode
	minNewtVer newtutil.Version
}

// Sorted in ascending order by newt version number.
type NewtCompatTable []NewtCompatEntry

type NewtCompatMap map[newtutil.Version]NewtCompatTable

func newtCompatCodeToString(code NewtCompatCode) string {
	return NewtCompatCodeNames[code]
}

func newtCompatCodeFromString(codeStr string) (NewtCompatCode, error) {
	for c, s := range NewtCompatCodeNames {
		if codeStr == s {
			return c, nil
		}
	}

	return NewtCompatCode(0),
		util.FmtNewtError("Invalid newt compatibility code: %s", codeStr)
}

func parseNcEntry(verStr string, codeStr string) (NewtCompatEntry, error) {
	entry := NewtCompatEntry{}
	var err error

	entry.minNewtVer, err = newtutil.ParseVersion(verStr)
	if err != nil {
		return entry, err
	}

	entry.code, err = newtCompatCodeFromString(codeStr)
	if err != nil {
		return entry, err
	}

	return entry, nil
}

func ParseNcTable(strMap map[string]string) (NewtCompatTable, error) {
	tbl := NewtCompatTable{}

	for v, c := range strMap {
		entry, err := parseNcEntry(v, c)
		if err != nil {
			return tbl, err
		}

		tbl = append(tbl, entry)
	}

	sortEntries(tbl)

	return tbl, nil
}

func ReadNcMap(yc ycfg.YCfg) (NewtCompatMap, error) {
	mp := NewtCompatMap{}
	ncMap := yc.GetValStringMap("repo.newt_compatibility", nil)

	for k, v := range ncMap {
		repoVer, err := newtutil.ParseVersion(k)
		if err != nil {
			return nil, util.FmtNewtError("Newt compatibility table contains " +
				"invalid repo version \"%s\"")
		}

		if _, ok := mp[repoVer]; ok {
			return nil, util.FmtNewtError("Newt compatibility table contains "+
				"duplicate version specifier: %s", repoVer.String())
		}

		strMap := cast.ToStringMapString(v)
		tbl, err := ParseNcTable(strMap)
		if err != nil {
			return nil, err
		}

		mp[repoVer] = tbl
	}

	return mp, nil
}

func (tbl NewtCompatTable) matchIdx(newtVer newtutil.Version) int {
	// Iterate the table backwards.  The first entry whose version is less than
	// or equal to the specified version is the match.
	for i := 0; i < len(tbl); i++ {
		idx := len(tbl) - i - 1
		entry := &tbl[idx]
		cmp := newtutil.VerCmp(entry.minNewtVer, newtVer)
		if cmp <= 0 {
			return idx
		}
	}

	return -1
}

func (tbl NewtCompatTable) newIdxRange(i int, j int) []int {
	if i >= len(tbl) {
		return []int{j, i}
	}

	if j >= len(tbl) {
		return []int{i, j}
	}

	e1 := tbl[i]
	e2 := tbl[j]

	if newtutil.VerCmp(e1.minNewtVer, e2.minNewtVer) < 0 {
		return []int{i, j}
	} else {
		return []int{j, i}
	}
}

func (tbl NewtCompatTable) idxRangesWithCode(c NewtCompatCode) [][]int {
	ranges := [][]int{}

	curi := -1
	for i, e := range tbl {
		if curi == -1 {
			if e.code == c {
				curi = i
			}
		} else {
			if e.code != c {
				ranges = append(ranges, tbl.newIdxRange(curi, i))
				curi = -1
			}
		}
	}

	if curi != -1 {
		ranges = append(ranges, tbl.newIdxRange(curi, len(tbl)))
	}
	return ranges
}

func (tbl NewtCompatTable) minMaxTgtVers(goodRange []int) (
	newtutil.Version, newtutil.Version, newtutil.Version) {

	minVer := tbl[goodRange[0]].minNewtVer

	var maxVer newtutil.Version
	if goodRange[1] < len(tbl) {
		maxVer = tbl[goodRange[1]].minNewtVer
	} else {
		maxVer = newtutil.Version{math.MaxInt64, math.MaxInt64, math.MaxInt64}
	}

	targetVer := tbl[goodRange[1]-1].minNewtVer

	return minVer, maxVer, targetVer
}

// @return NewtCompatCode       The severity of the newt incompatibility
//         string               The warning or error message to display in case
//                                  of incompatibility.
func (tbl NewtCompatTable) CheckNewtVer(
	newtVer newtutil.Version) (NewtCompatCode, string) {

	var code NewtCompatCode
	idx := tbl.matchIdx(newtVer)
	if idx == -1 {
		// This version of newt is older than every entry in the table.
		code = NEWT_COMPAT_ERROR
	} else {
		code = tbl[idx].code
		if code == NEWT_COMPAT_GOOD {
			return NEWT_COMPAT_GOOD, ""
		}
	}

	goodRanges := tbl.idxRangesWithCode(NEWT_COMPAT_GOOD)
	for i := 0; i < len(goodRanges); i++ {
		minVer, maxVer, tgtVer := tbl.minMaxTgtVers(goodRanges[i])

		if newtutil.VerCmp(newtVer, minVer) < 0 {
			return code, fmt.Sprintf("Please upgrade your newt tool to "+
				"version %s", tgtVer.String())
		}

		if newtutil.VerCmp(newtVer, maxVer) >= 0 {
			return code, fmt.Sprintf("Please upgrade your project "+
				"or downgrade newt to %s", tgtVer.String())
		}
	}

	return code, ""
}

type entrySorter struct {
	entries []NewtCompatEntry
}

func (s entrySorter) Len() int {
	return len(s.entries)
}
func (s entrySorter) Swap(i, j int) {
	s.entries[i], s.entries[j] = s.entries[j], s.entries[i]
}
func (s entrySorter) Less(i, j int) bool {
	e1 := s.entries[i]
	e2 := s.entries[j]

	cmp := newtutil.VerCmp(e1.minNewtVer, e2.minNewtVer)
	if cmp < 0 {
		return true
	} else if cmp > 0 {
		return false
	}

	return false
}

func sortEntries(entries []NewtCompatEntry) {
	sorter := entrySorter{
		entries: entries,
	}

	sort.Sort(sorter)
}
