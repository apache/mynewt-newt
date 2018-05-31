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
	"fmt"
	"path/filepath"
	"strconv"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/pkg"
)

func MfgBinDir(mfgPkgName string) string {
	return builder.BinRoot() + "/" + mfgPkgName
}

func MfgBootDir(mfgPkgName string) string {
	return MfgBinDir(mfgPkgName) + "/bootloader"
}

func MfgBootBinPath(mfgPkgName string, appName string) string {
	return MfgBootDir(mfgPkgName) + "/" + appName + ".elf.bin"
}

func MfgBootElfPath(mfgPkgName string, appName string) string {
	return MfgBootDir(mfgPkgName) + "/" + appName + ".elf"
}

func MfgBootManifestPath(mfgPkgName string, appName string) string {
	return MfgBootDir(mfgPkgName) + "/manifest.json"
}

// Image indices start at 0.
func MfgImageBinDir(mfgPkgName string, imageIdx int) string {
	return MfgBinDir(mfgPkgName) + "/image" + strconv.Itoa(imageIdx)
}

func MfgImageImgPath(mfgPkgName string, imageIdx int,
	appName string) string {

	return MfgImageBinDir(mfgPkgName, imageIdx) + "/" + appName + ".img"
}

func MfgImageElfPath(mfgPkgName string, imageIdx int,
	appName string) string {

	return MfgImageBinDir(mfgPkgName, imageIdx) + "/" + appName + ".elf"
}

func MfgImageManifestPath(mfgPkgName string, imageIdx int) string {
	return MfgImageBinDir(mfgPkgName, imageIdx) + "/manifest.json"
}

func MfgSectionBinDir(mfgPkgName string) string {
	return MfgBinDir(mfgPkgName) + "/sections"
}

func MfgSectionBinPath(mfgPkgName string, sectionNum int) string {
	return fmt.Sprintf("%s/%s-s%d.bin", MfgSectionBinDir(mfgPkgName),
		filepath.Base(mfgPkgName), sectionNum)
}

func MfgSectionHexPath(mfgPkgName string, sectionNum int) string {
	return fmt.Sprintf("%s/%s-s%d.hex", MfgSectionBinDir(mfgPkgName),
		filepath.Base(mfgPkgName), sectionNum)
}

func MfgManifestPath(mfgPkgName string) string {
	return MfgBinDir(mfgPkgName) + "/manifest.json"
}

func (mi *MfgImage) ManifestPath() string {
	return MfgManifestPath(mi.basePkg.Name())
}

func (mi *MfgImage) BootBinPath() string {
	if mi.boot == nil {
		return ""
	}

	return MfgBootBinPath(mi.basePkg.Name(),
		pkg.ShortName(mi.boot.App()))
}

func (mi *MfgImage) BootElfPath() string {
	if mi.boot == nil {
		return ""
	}

	return MfgBootElfPath(mi.basePkg.Name(), pkg.ShortName(mi.boot.App()))
}

func (mi *MfgImage) BootManifestPath() string {
	if mi.boot == nil {
		return ""
	}

	return MfgBootManifestPath(mi.basePkg.Name(),
		pkg.ShortName(mi.boot.App()))
}

func (mi *MfgImage) AppImgPath(imageIdx int) string {
	app, _ := mi.imgApps(imageIdx)
	if app == nil {
		return ""
	}

	return MfgImageImgPath(mi.basePkg.Name(), imageIdx, pkg.ShortName(app))
}

func (mi *MfgImage) AppElfPath(imageIdx int) string {
	app, _ := mi.imgApps(imageIdx)
	if app == nil {
		return ""
	}

	return MfgImageElfPath(mi.basePkg.Name(), imageIdx, pkg.ShortName(app))
}

func (mi *MfgImage) LoaderImgPath(imageIdx int) string {
	_, loader := mi.imgApps(imageIdx)
	if loader == nil {
		return ""
	}

	return MfgImageImgPath(mi.basePkg.Name(), imageIdx, pkg.ShortName(loader))
}

func (mi *MfgImage) LoaderElfPath(imageIdx int) string {
	_, loader := mi.imgApps(imageIdx)
	if loader == nil {
		return ""
	}

	return MfgImageElfPath(mi.basePkg.Name(), imageIdx, pkg.ShortName(loader))
}

func (mi *MfgImage) ImageManifestPath(imageIdx int) string {
	if imageIdx >= len(mi.images) {
		return ""
	}

	return MfgImageManifestPath(mi.basePkg.Name(), imageIdx)
}

func (mi *MfgImage) SectionBinPaths() []string {
	sectionIds := mi.sectionIds()

	paths := make([]string, len(sectionIds))
	for i, sectionId := range sectionIds {
		paths[i] = MfgSectionBinPath(mi.basePkg.Name(), sectionId)
	}
	return paths
}

func (mi *MfgImage) SectionHexPaths() []string {
	sectionIds := mi.sectionIds()

	paths := make([]string, len(sectionIds))
	for i, sectionId := range sectionIds {
		paths[i] = MfgSectionHexPath(mi.basePkg.Name(), sectionId)
	}
	return paths
}
