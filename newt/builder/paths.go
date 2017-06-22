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
	"path/filepath"

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

const BUILD_NAME_APP = "app"
const BUILD_NAME_LOADER = "loader"

func BinRoot() string {
	return project.GetProject().Path() + "/bin"
}

func TargetBinDir(targetName string) string {
	return BinRoot() + "/" + targetName
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

func PkgSyscfgPath(pkgPath string) string {
	return pkgPath + "/" + pkg.SYSCFG_YAML_FILENAME
}

func BinDir(targetName string, buildName string) string {
	return BinRoot() + "/" + targetName + "/" + buildName
}

func FileBinDir(targetName string, buildName string, pkgName string) string {
	return BinDir(targetName, buildName) + "/" + pkgName
}

func PkgBinDir(targetName string, buildName string, pkgName string,
	pkgType interfaces.PackageType) string {

	switch pkgType {
	case pkg.PACKAGE_TYPE_GENERATED:
		return GeneratedBinDir(targetName)
	default:
		return FileBinDir(targetName, buildName, pkgName)
	}
}

func ArchivePath(targetName string, buildName string, pkgName string,
	pkgType interfaces.PackageType) string {

	filename := util.FilenameFromPath(pkgName) + ".a"
	return PkgBinDir(targetName, buildName, pkgName, pkgType) + "/" + filename
}

func AppElfPath(targetName string, buildName string, appName string) string {
	return FileBinDir(targetName, buildName, appName) + "/" +
		filepath.Base(appName) + ".elf"
}

func AppBinPath(targetName string, buildName string, appName string) string {
	return AppElfPath(targetName, buildName, appName) + ".bin"
}

func TestExePath(targetName string, buildName string, pkgName string,
	pkgType interfaces.PackageType) string {

	return PkgBinDir(targetName, buildName, pkgName, pkgType) + "/" +
		TestTargetName(pkgName) + ".elf"
}

func ManifestPath(targetName string, buildName string, pkgName string) string {
	return FileBinDir(targetName, buildName, pkgName) + "/manifest.json"
}

func AppImgPath(targetName string, buildName string, appName string) string {
	return FileBinDir(targetName, buildName, appName) + "/" +
		filepath.Base(appName) + ".img"
}

func MfgBinDir(mfgPkgName string) string {
	return BinRoot() + "/" + mfgPkgName
}

func MfgBootDir(mfgPkgName string) string {
	return MfgBinDir(mfgPkgName) + "/bootloader"
}

func (b *Builder) BinDir() string {
	return BinDir(b.targetPkg.rpkg.Lpkg.Name(), b.buildName)
}

func (b *Builder) FileBinDir(pkgName string) string {
	return FileBinDir(b.targetPkg.rpkg.Lpkg.Name(), b.buildName, pkgName)
}

func (b *Builder) PkgBinDir(bpkg *BuildPackage) string {
	return PkgBinDir(b.targetPkg.rpkg.Lpkg.Name(), b.buildName, bpkg.rpkg.Lpkg.Name(),
		bpkg.rpkg.Lpkg.Type())
}

// Generates the path+filename of the specified package's .a file.
func (b *Builder) ArchivePath(bpkg *BuildPackage) string {
	return ArchivePath(b.targetPkg.rpkg.Lpkg.Name(), b.buildName, bpkg.rpkg.Lpkg.Name(),
		bpkg.rpkg.Lpkg.Type())
}

func (b *Builder) AppTentativeElfPath() string {
	return b.PkgBinDir(b.appPkg) + "/" + filepath.Base(b.appPkg.rpkg.Lpkg.Name()) +
		"_tmp.elf"
}

func (b *Builder) AppElfPath() string {
	return AppElfPath(b.targetPkg.rpkg.Lpkg.Name(), b.buildName,
		b.appPkg.rpkg.Lpkg.Name())
}

func (b *Builder) AppLinkerElfPath() string {
	return b.PkgBinDir(b.appPkg) + "/" + filepath.Base(b.appPkg.rpkg.Lpkg.Name()) +
		"linker.elf"
}

func (b *Builder) AppImgPath() string {
	return b.PkgBinDir(b.appPkg) + "/" + filepath.Base(b.appPkg.rpkg.Lpkg.Name()) +
		".img"
}

func (b *Builder) AppHexPath() string {
	return b.PkgBinDir(b.appPkg) + "/" + filepath.Base(b.appPkg.rpkg.Lpkg.Name()) +
		".hex"
}

func (b *Builder) AppBinPath() string {
	return b.AppElfPath() + ".bin"
}

func (b *Builder) AppPath() string {
	return b.PkgBinDir(b.appPkg) + "/"
}

func (b *Builder) TestExePath(bpkg *BuildPackage) string {
	return TestExePath(b.targetPkg.rpkg.Lpkg.Name(), b.buildName,
		bpkg.rpkg.Lpkg.Name(), bpkg.rpkg.Lpkg.Type())
}

func (b *Builder) ManifestPath() string {
	return ManifestPath(b.targetPkg.rpkg.Lpkg.Name(), b.buildName,
		b.appPkg.rpkg.Lpkg.Name())
}

func (b *Builder) AppBinBasePath() string {
	return b.PkgBinDir(b.appPkg) + "/" +
		filepath.Base(b.appPkg.rpkg.Lpkg.Name())
}
