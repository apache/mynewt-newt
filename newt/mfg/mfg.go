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

package mfg

import (
	"sort"

	"mynewt.apache.org/newt/newt/image"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
)

type MfgRawEntry struct {
	device   int
	offset   int
	filename string
	data     []byte
}

// A chunk of data in the manufacturing image.  Can be a firmware image or a
// raw entry (contents of a data file).
type mfgPart struct {
	device int
	offset int
	data   []byte
	name   string
}

type MfgImage struct {
	basePkg *pkg.LocalPackage

	bsp      *pkg.BspPackage
	compiler *toolchain.Compiler

	boot       *target.Target
	images     []*target.Target
	rawEntries []MfgRawEntry

	version image.ImageVersion
}

func (mi *MfgImage) SetVersion(ver image.ImageVersion) {
	mi.version = ver
}

func (mi *MfgImage) imgApps(imageIdx int) (
	app *pkg.LocalPackage, loader *pkg.LocalPackage) {

	if imageIdx >= len(mi.images) {
		return
	}

	t := mi.images[imageIdx]
	app = t.App()
	loader = t.Loader()
	return
}

func (mi *MfgImage) sectionIds() []int {
	idMap := map[int]struct{}{}

	// The bootloader and images always go in section 0.
	idMap[0] = struct{}{}

	for _, entry := range mi.rawEntries {
		idMap[entry.device] = struct{}{}
	}

	ids := make([]int, 0, len(idMap))
	for id, _ := range idMap {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	return ids
}

func (mi *MfgImage) NumImages() int {
	return len(mi.images)
}
