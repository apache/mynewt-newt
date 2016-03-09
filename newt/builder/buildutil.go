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

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

func (b *Builder) binDir() string {
	return project.GetProject().Path() + "/bin/" + b.target.ShortName()
}

func (b *Builder) pkgBinDir(pkgName string) string {
	return b.binDir() + "/" + pkgName
}

// Generates the path+filename of the specified package's .a file.
func (b *Builder) archivePath(pkgName string) string {
	return b.pkgBinDir(pkgName) + "/" + filepath.Base(pkgName) + ".a"
}

func (b *Builder) AppElfPath() string {
	pkgName := b.target.App().Name()
	return b.pkgBinDir(pkgName) + "/" + filepath.Base(pkgName) + ".elf"
}

func (b *Builder) AppImgPath() string {
	pkgName := b.target.App().Name()
	return b.pkgBinDir(pkgName) + "/" + filepath.Base(pkgName) + ".img"
}

func (b *Builder) AppPath() string {
	pkgName := b.target.App().Name()
	return b.pkgBinDir(pkgName) + "/"
}

func (b *Builder) appBinBasePath() string {
	pkgName := b.target.App().Name()
	return b.pkgBinDir(pkgName) + "/" + filepath.Base(pkgName)
}

func (b *Builder) testExePath(pkgName string) string {
	return b.pkgBinDir(pkgName) + "/test_" + filepath.Base(pkgName)
}

func (b *Builder) featureString() string {
	var buffer bytes.Buffer

	first := true
	for feature, _ := range b.Features() {
		if !first {
			buffer.WriteString(" ")
		} else {
			first = false
		}

		buffer.WriteString(feature)
	}
	return buffer.String()
}

func (b *Builder) resolveCompiler() *pkg.LocalPackage {
	if b.Bsp.CompilerName == "" {
		return nil
	}
	dep, _ := pkg.NewDependency(b.Bsp.Repo(), b.Bsp.CompilerName)
	mypkg := project.GetProject().ResolveDependency(dep).(*pkg.LocalPackage)
	return mypkg
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
