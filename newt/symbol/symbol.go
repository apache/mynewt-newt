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

/* this file maintains a list of all the symbols from a */

package symbol

import (
	"fmt"
	"sort"
	"strings"

	"mynewt.apache.org/newt/util"
)

type SymbolInfo struct {
	Bpkg    string
	Name    string
	Code    string
	Section string
	Ext     string
	Size    int
	Loc     int
}

type SymbolMap map[string]SymbolInfo

func NewSymbolMap() *SymbolMap {
	val := &SymbolMap{}
	return val
}

func NewSymbolInfo() *SymbolInfo {
	val := &SymbolInfo{}
	return val
}

func NewElfSymbol(name string) *SymbolInfo {
	val := NewSymbolInfo()
	val.Name = name
	val.Ext = ".elf"
	return val
}

func (s *SymbolMap) Add(info SymbolInfo) {
	(*s)[info.Name] = info
}

func (s *SymbolMap) GlobalFunctionsOnly() *SymbolMap {
	s3 := NewSymbolMap()

	for _, info1 := range *s {
		if info1.IsFunction() && !info1.IsLocal() {
			s3.Add(info1)
		}
	}
	return s3
}

func (s *SymbolMap) GlobalDataOnly() *SymbolMap {
	s3 := NewSymbolMap()

	for _, info1 := range *s {
		if !info1.IsFunction() && !info1.IsLocal() {
			s3.Add(info1)
		}
	}
	return s3
}

func (s *SymbolMap) Packages() map[string]bool {
	pkg := make(map[string]bool)
	for _, info1 := range *s {
		pkg[info1.Bpkg] = true
	}
	return pkg
}

func IdenticalUnion(s1 *SymbolMap, s2 *SymbolMap, comparePkg bool,
	compareAddr bool) (error, *SymbolMap, *SymbolMap) {
	s3 := NewSymbolMap()
	s_no := NewSymbolMap()
	var err_str string
	var err error
	/* look through all symbols in S1 and if they are in s1,
	 * add to new map s3 */

	for name, info1 := range *s1 {
		if info2, ok := (*s2)[name]; ok {
			var pkg bool
			var addr bool

			if comparePkg {
				pkg = info1.Bpkg == info2.Bpkg
			} else {
				pkg = true
			}

			if compareAddr {
				addr = info1.Loc == info2.Loc
			} else {
				addr = true
			}

			/* compare to info 1 */
			if info1.Code == info2.Code &&
				info1.Size == info2.Size && pkg && addr {
				s3.Add(info1)
			} else if !info1.IsLocal() && !info1.IsFunction() {
				/* Here is an unusual case.  We have a global data
				 * symbol (bss or data) with the same name that is used
				 * in both apps.  If code is linked against both of these
				 * the code in the loader will call one while the code in
				 * the app will call the other.  If the intention was for
				 * these to be the same, then things are bad.  */
				if err_str == "" {
					err_str = "There are global symbols with the same name that " +
						"are access via the loader and split application.  These " +
						"symbols are either different sizes or from different " +
						"packages.  Reconcile this issue before buidling.  If the " +
						"symbols are intended to be shared by both, move the " +
						"symbol to a package that is shared by both apps. If " +
						"the symbols are distict (not shared), then make them " +
						"static or rename them so they do not conflict" +
						"\nNon Matching Symbols:\n"
				}

				err_str = err_str + fmt.Sprintf("%s-%s\n", info1.Sprintf(), info2.Sprintf())
			} else {
				info1.Name = info1.Name + "(app)"
				info2.Name = info2.Name + "(loader)"
				s_no.Add(info1)
				s_no.Add(info2)
			}
		}
	}

	if err_str != "" {
		err = util.NewNewtError(err_str)
	}
	return err, s3, s_no
}

type SymbolMapIterator func(s *SymbolInfo)

func sprintfSi(si *SymbolInfo) string {
	str := fmt.Sprintf("  %32s(%4s) (%8s) -- (%12s) %5d (0x%08x) from %s\n",
		(*si).Name, (*si).Ext, (*si).Code, (*si).Section,
		(*si).Size, (*si).Loc, (*si).Bpkg)
	return str
}

func dumpSi(si *SymbolInfo) {
	fmt.Printf(sprintfSi(si))
}

func (si *SymbolInfo) Dump() {
	dumpSi(si)
}

func (si *SymbolInfo) Sprintf() string {
	return sprintfSi(si)
}

func (si *SymbolInfo) IsLocal() bool {
	val := (*si).Code[:1]

	if val == "l" {
		return true
	}
	return false
}

func (si *SymbolInfo) IsWeak() bool {
	val := (*si).Code[1:2]

	if val == "w" {
		return true
	}
	return false
}

func (si *SymbolInfo) IsDebug() bool {
	val := (*si).Code[5:6]

	if val == "d" {
		return true
	}
	return false
}

func (si *SymbolInfo) IsSection(section string) bool {
	val := (*si).Section
	return strings.HasPrefix(val, section)
}

func (si *SymbolInfo) IsFile() bool {
	val := (*si).Code[6:7]

	if val == "f" {
		return true
	}
	return false
}

func (si *SymbolInfo) IsFunction() bool {
	val := (*si).Code[6:7]

	if val == "F" {
		return true
	}
	return false
}

func (s *SymbolMap) FilterPkg(pname string) *SymbolMap {
	sm := NewSymbolMap()
	for _, info1 := range *s {
		if pname != "" && pname == info1.Bpkg {
			sm.Add(info1)
		}
	}
	return sm
}

func (s *SymbolMap) String(name string) string {
	// To store the keys in slice in sorted order
	var keys []string
	for k := range *s {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// To perform the opertion you want
	out := fmt.Sprintf("Dumping symbols in file: %s\n", name)
	for _, k := range keys {
		info1 := (*s)[k]
		out += info1.Sprintf()
	}
	return out
}

func (s *SymbolMap) Dump(name string) {

	// To store the keys in slice in sorted order
	var keys []string
	for k := range *s {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// To perform the opertion you want
	fmt.Printf("Dumping symbols in file: %s\n", name)
	for _, k := range keys {
		info1 := (*s)[k]
		info1.Dump()
	}
}

// Merge - merges given maps into 1 map
// values will be overridden by last matching key - value
func (s1 *SymbolMap) Merge(s2 *SymbolMap) (*SymbolMap, error) {

	for k, v := range *s2 {

		if val, ok := (*s1)[k]; ok {
			/* We already have this in the MAP */
			if val.IsWeak() && !v.IsWeak() {
				(*s1)[k] = v
			} else if v.IsWeak() && !val.IsWeak() {
				/* nothing to do here as this is OK not to replace */
			} else if v.IsLocal() && val.IsLocal() {
				/* two locals that must conflict with name */
				/* have to have separate instances of these */
				util.StatusMessage(util.VERBOSITY_VERBOSE,
					"Local Symbol Conflict: %s from packages %s and %s \n",
					v.Name, v.Bpkg, val.Bpkg)
				(*s2).Remove(k)
			} else {
				util.StatusMessage(util.VERBOSITY_QUIET,
					"Global Symbol Conflict: %s from packages %s and %s \n",
					v.Name, v.Bpkg, val.Bpkg)
				return nil, util.NewNewtError("Global Symbol Conflict")
			}
		} else {
			(*s1)[k] = v
		}

	}
	return s1, nil
}

func (s *SymbolMap) Remove(name string) {
	delete(*s, name)
}

func (s *SymbolMap) RemoveMap(subset *SymbolMap) {
	for name, _ := range *subset {
		(*s).Remove(name)
	}
}

/* Returns true if the symbol is present in the symbol map */
func (s *SymbolMap) Find(name string) (*SymbolInfo, bool) {
	val, ok := (*s)[name]
	return &val, ok
}
