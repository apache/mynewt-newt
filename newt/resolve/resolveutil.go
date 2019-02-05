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

package resolve

import (
	"sort"

	"mynewt.apache.org/newt/newt/pkg"
)

type rpkgSorter struct {
	pkgs []*ResolvePackage
}

func (s rpkgSorter) Len() int {
	return len(s.pkgs)
}
func (s rpkgSorter) Swap(i, j int) {
	s.pkgs[i], s.pkgs[j] = s.pkgs[j], s.pkgs[i]
}
func (s rpkgSorter) Less(i, j int) bool {
	return s.pkgs[i].Lpkg.FullName() < s.pkgs[j].Lpkg.FullName()
}

func SortResolvePkgs(pkgs []*ResolvePackage) {
	sorter := rpkgSorter{pkgs}
	sort.Sort(sorter)
}

func RpkgSliceToLpkgSlice(rpkgs []*ResolvePackage) []*pkg.LocalPackage {
	lpkgs := make([]*pkg.LocalPackage, len(rpkgs))

	i := 0
	for _, rpkg := range rpkgs {
		lpkgs[i] = rpkg.Lpkg
		i++
	}

	return lpkgs
}
