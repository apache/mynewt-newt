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

package builder

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
)

func BinRoot() string {
	return project.GetProject().Path() + "/bin"
}

func TargetBinDir(target *target.Target) string {
	return BinRoot() + "/" + target.Name()
}

func GeneratedBaseDir(targetName string) string {
	return BinRoot() + "/" + targetName + "/generated"
}

func GeneratedSrcDir(targetName string) string {
	return GeneratedBaseDir(targetName) + "/src"
}

func GeneratedIncludeDir(targetName string) string {
	return GeneratedBaseDir(targetName) + "/include"
}

func GeneratedBinDir(targetName string) string {
	return GeneratedBaseDir(targetName) + "/bin"
}

func SysinitArchivePath(targetName string) string {
	return GeneratedBinDir(targetName) + "/sysinit.a"
}

func (b *Builder) BinDir() string {
	return BinRoot() + "/" + b.targetPkg.Name() + "/" + b.buildName
}

func (b *Builder) FileBinDir(pkgName string) string {
	return b.BinDir() + "/" + pkgName
}

func (b *Builder) PkgBinDir(bpkg *BuildPackage) string {
	switch bpkg.Type() {
	case pkg.PACKAGE_TYPE_GENERATED:
		return GeneratedBinDir(b.targetPkg.Name())
	default:
		return b.FileBinDir(bpkg.Name())
	}
}

// Generates the path+filename of the specified package's .a file.
func (b *Builder) ArchivePath(bpkg *BuildPackage) string {
	return b.PkgBinDir(bpkg) + "/" + filepath.Base(bpkg.Name()) + ".a"
}

func (b *Builder) AppTempElfPath() string {
	return b.PkgBinDir(b.appPkg) + "/" + filepath.Base(b.appPkg.Name()) +
		"_tmp.elf"
}

func (b *Builder) AppElfPath() string {
	return b.PkgBinDir(b.appPkg) + "/" + filepath.Base(b.appPkg.Name()) +
		".elf"
}

func (b *Builder) AppLinkerElfPath() string {
	return b.PkgBinDir(b.appPkg) + "/" + filepath.Base(b.appPkg.Name()) +
		"linker.elf"
}

func (b *Builder) AppImgPath() string {
	return b.PkgBinDir(b.appPkg) + "/" + filepath.Base(b.appPkg.Name()) +
		".img"
}

func (b *Builder) AppBinPath() string {
	return b.AppElfPath() + ".bin"
}

func (b *Builder) AppPath() string {
	return b.PkgBinDir(b.appPkg) + "/"
}

func (b *Builder) ManifestPath() string {
	return b.AppPath() + "/manifest.json"
}

func (b *Builder) AppBinBasePath() string {
	return b.PkgBinDir(b.appPkg) + "/" + filepath.Base(b.appPkg.Name())
}

func MfgBinDir(mfgPkgName string) string {
	return BinRoot() + "/" + mfgPkgName
}

func MfgBinPath(mfgPkgName string) string {
	return MfgBinDir(mfgPkgName) + "/" + filepath.Base(mfgPkgName) + ".mfg"
}

func MfgSectionPath(mfgPkgName string, sectionNum int) string {
	return fmt.Sprintf("%s/%s-s%d.bin", MfgBinDir(mfgPkgName),
		filepath.Base(mfgPkgName), sectionNum)
}

func TestTargetName(testPkgName string) string {
	return strings.Replace(testPkgName, "/", "_", -1)
}

func (b *Builder) TestExePath(bpkg *BuildPackage) string {
	return b.PkgBinDir(bpkg) + "/" + TestTargetName(bpkg.Name())
}

func (b *Builder) FeatureString() string {
	var buffer bytes.Buffer

	featureMap := b.cfg.Features()
	featureSlice := make([]string, 0, len(featureMap))
	for k, _ := range featureMap {
		featureSlice = append(featureSlice, k)
	}
	sort.Strings(featureSlice)

	for i, feature := range featureSlice {
		if i != 0 {
			buffer.WriteString(" ")
		}

		buffer.WriteString(feature)
	}
	return buffer.String()
}

type bpkgSorter struct {
	bpkgs []*BuildPackage
}

func (b bpkgSorter) Len() int {
	return len(b.bpkgs)
}
func (b bpkgSorter) Swap(i, j int) {
	b.bpkgs[i], b.bpkgs[j] = b.bpkgs[j], b.bpkgs[i]
}
func (b bpkgSorter) Less(i, j int) bool {
	return b.bpkgs[i].Name() < b.bpkgs[j].Name()
}

func (b *Builder) sortedBuildPackages() []*BuildPackage {
	sorter := bpkgSorter{
		bpkgs: make([]*BuildPackage, 0, len(b.PkgMap)),
	}

	for _, bpkg := range b.PkgMap {
		sorter.bpkgs = append(sorter.bpkgs, bpkg)
	}

	sort.Sort(sorter)
	return sorter.bpkgs
}

func (b *Builder) sortedLocalPackages() []*pkg.LocalPackage {
	bpkgs := b.sortedBuildPackages()

	lpkgs := make([]*pkg.LocalPackage, len(bpkgs), len(bpkgs))
	for i, bpkg := range bpkgs {
		lpkgs[i] = bpkg.LocalPackage
	}

	return lpkgs
}

func (b *Builder) logDepInfo() {
	// Log feature set.
	log.Debugf("Feature set: [" + b.FeatureString() + "]")

	// Log API set.
	apis := make([]string, 0, len(b.apiMap))
	for api, _ := range b.apiMap {
		apis = append(apis, api)
	}
	sort.Strings(apis)

	log.Debugf("API set:")
	for _, api := range apis {
		bpkg := b.apiMap[api]
		log.Debugf("    * " + api + " (" + bpkg.FullName() + ")")
	}

	// Log dependency graph.
	bpkgSorter := bpkgSorter{
		bpkgs: make([]*BuildPackage, 0, len(b.PkgMap)),
	}
	for _, bpkg := range b.PkgMap {
		bpkgSorter.bpkgs = append(bpkgSorter.bpkgs, bpkg)
	}
	sort.Sort(bpkgSorter)

	log.Debugf("Dependency graph:")
	var buffer bytes.Buffer
	for _, bpkg := range bpkgSorter.bpkgs {
		buffer.Reset()
		for i, dep := range bpkg.Deps() {
			if i != 0 {
				buffer.WriteString(" ")
			}
			buffer.WriteString(dep.String())
		}
		log.Debugf("    * " + bpkg.Name() + " [" +
			buffer.String() + "]")
	}
}
