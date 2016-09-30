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
	"path/filepath"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

func BinRoot() string {
	return project.GetProject().Path() + "/bin"
}

func TargetBinDir(target *target.Target) string {
	return BinRoot() + "/" + target.Name()
}

func (b *Builder) BinDir() string {
	return BinRoot() + "/" + b.target.target.Name() + "/" + b.buildName
}

func (b *Builder) PkgBinDir(pkgName string) string {
	return b.BinDir() + "/" + pkgName
}

// Generates the path+filename of the specified package's .a file.
func (b *Builder) ArchivePath(pkgName string) string {
	return b.PkgBinDir(pkgName) + "/" + filepath.Base(pkgName) + ".a"
}

func (b *Builder) AppTempElfPath() string {
	pkgName := b.appPkg.Name()
	return b.PkgBinDir(pkgName) + "/" + filepath.Base(pkgName) + "_tmp.elf"
}

func (b *Builder) AppElfPath() string {
	pkgName := b.appPkg.Name()
	return b.PkgBinDir(pkgName) + "/" + filepath.Base(pkgName) + ".elf"
}

func (b *Builder) AppLinkerElfPath() string {
	pkgName := b.appPkg.Name()
	return b.PkgBinDir(pkgName) + "/" + filepath.Base(pkgName) + "linker.elf"
}

func (b *Builder) AppImgPath() string {
	pkgName := b.appPkg.Name()
	return b.PkgBinDir(pkgName) + "/" + filepath.Base(pkgName) + ".img"
}

func (b *Builder) AppPath() string {
	pkgName := b.appPkg.Name()
	return b.PkgBinDir(pkgName) + "/"
}

func (b *Builder) AppBinBasePath() string {
	pkgName := b.appPkg.Name()
	return b.PkgBinDir(pkgName) + "/" + filepath.Base(pkgName)
}

func TestTargetName(testPkgName string) string {
	return strings.Replace(testPkgName, "/", "_", -1)
}

func (b *Builder) TestExePath(pkgName string) string {
	return b.PkgBinDir(pkgName) + "/" + TestTargetName(pkgName)
}

func (b *Builder) FeatureString() string {
	var buffer bytes.Buffer

	featureMap := b.Cfg.Features()
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

// Makes sure all packages with required APIs have been augmented with a
// dependency that satisfies that requirement.  If there are any unsatisfied
// requirements, an error is returned.
func (b *Builder) verifyApisSatisfied() error {
	unsatisfied := map[*BuildPackage][]string{}

	for _, bpkg := range b.Packages {
		for api, status := range bpkg.reqApiMap {
			if status == REQ_API_STATUS_UNSATISFIED {
				slice := unsatisfied[bpkg]
				if slice == nil {
					unsatisfied[bpkg] = []string{api}
				} else {
					slice = append(slice, api)
				}
			}
		}
	}

	if len(unsatisfied) != 0 {
		var buffer bytes.Buffer
		for bpkg, apis := range unsatisfied {
			buffer.WriteString("Package " + bpkg.Name() +
				" has unsatisfied required APIs: ")
			for i, api := range apis {
				if i != 0 {
					buffer.WriteString(", ")
				}
				buffer.WriteString(api)
			}
			buffer.WriteString("\n")
		}
		return util.NewNewtError(buffer.String())
	}

	return nil
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
		bpkgs: make([]*BuildPackage, 0, len(b.Packages)),
	}

	for _, bpkg := range b.Packages {
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
	apis := make([]string, 0, len(b.apis))
	for api, _ := range b.apis {
		apis = append(apis, api)
	}
	sort.Strings(apis)

	log.Debugf("API set:")
	for _, api := range apis {
		bpkg := b.apis[api]
		log.Debugf("    * " + api + " (" + bpkg.Name() + ")")
	}

	// Log dependency graph.
	bpkgSorter := bpkgSorter{
		bpkgs: make([]*BuildPackage, 0, len(b.Packages)),
	}
	for _, bpkg := range b.Packages {
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
