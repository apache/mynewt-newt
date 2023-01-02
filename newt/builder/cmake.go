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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

const CMAKELISTS_FILENAME string = "CMakeLists.txt"

func CmakeListsPath() string {
	return project.GetProject().BasePath + "/" + CMAKELISTS_FILENAME
}

func EscapePkgName(name string) string {
	name = strings.Replace(name, "/", "_", -1)
	if strings.HasPrefix(name, "@") {
		name = strings.Replace(name, "@", "repos_", 1)
	}
	return name
}

func ExtractLibraryName(filepath string) string {
	_, name := path.Split(filepath)
	name = strings.TrimPrefix(name, "lib")
	name = strings.TrimSuffix(name, ".a")
	return name
}

func replaceBackslashes(path string) string {
	return strings.Replace(path, "\\", "/", -1)
}

func replaceBackslashesSlice(elements []string) {
	for e := range elements {
		elements[e] = replaceBackslashes(elements[e])
	}
}

func escapeFlags(flag string) string {
	flag = strings.Replace(flag, "\"", "\\\\\\\"", -1)
	flag = strings.Replace(flag, "(", "\\\\(", -1)
	flag = strings.Replace(flag, ")", "\\\\)", -1)
	flag = strings.Replace(flag, "<", "\\\\<", -1)
	flag = strings.Replace(flag, ">", "\\\\>", -1)
	return flag
}

func escapeFlagsSlice(flags []string) []string {
	for f := range flags {
		flags[f] = escapeFlags(flags[f])
	}

	return flags
}

func trimProjectPath(path string) string {
	proj := interfaces.GetProject()
	if strings.HasPrefix(replaceBackslashes(path), proj.Path()) {
		path, _ = filepath.Rel(proj.Path(), path)
	}
	return replaceBackslashes(path)
}

func trimProjectPathSlice(elements []string) {
	for e := range elements {
		elements[e] = trimProjectPath(elements[e])
	}
}

func extractIncludes(flags *[]string, includes *[]string, other *[]string) {
	for _, f := range *flags {
		if strings.HasPrefix(f, "-I") {
			*includes = append(*includes, strings.TrimPrefix(f, "-I"))
		} else {
			*other = append(*other, f)
		}
	}
}

func CmakeSourceObjectWrite(w io.Writer, cj toolchain.CompilerJob,
	includeDirs *[]string, linkFlags *[]string) {
	c := cj.Compiler

	flags := []string{}
	otherFlags := []string{}

	switch cj.CompilerType {
	case toolchain.COMPILER_TYPE_C:
		flags = append(flags, c.GetCompilerInfo().Cflags...)
		flags = append(flags, c.GetLocalCompilerInfo().Cflags...)
	case toolchain.COMPILER_TYPE_ASM:
		flags = append(flags, c.GetCompilerInfo().Cflags...)
		flags = append(flags, c.GetLocalCompilerInfo().Cflags...)
		flags = append(flags, c.GetCompilerInfo().Aflags...)
		flags = append(flags, c.GetLocalCompilerInfo().Aflags...)
	case toolchain.COMPILER_TYPE_CPP:
		flags = append(flags, c.GetCompilerInfo().Cflags...)
		flags = append(flags, c.GetLocalCompilerInfo().Cflags...)
		flags = append(flags, c.GetCompilerInfo().CXXflags...)
		flags = append(flags, c.GetLocalCompilerInfo().CXXflags...)
	}

	*linkFlags = append(*linkFlags, c.GetCompilerInfo().Lflags...)
	*linkFlags = append(*linkFlags, c.GetLocalCompilerInfo().Lflags...)

	extractIncludes(&flags, includeDirs, &otherFlags)
	cj.Filename = trimProjectPath(cj.Filename)

	// Sort and remove duplicate flags
	otherFlags = util.SortFields(otherFlags...)

	fmt.Fprintf(w,
		`set_property(SOURCE %s APPEND_STRING
             PROPERTY COMPILE_FLAGS
             "%s")`,
		cj.Filename,
		strings.Join(escapeFlagsSlice(otherFlags), " "))
	fmt.Fprintln(w)
}

func (b *Builder) CMakeBuildPackageWrite(w io.Writer, bpkg *BuildPackage,
	linkFlags *[]string, libraries *[]string) (*BuildPackage, error) {
	entries, err := b.collectCompileEntriesBpkg(bpkg)
	if err != nil {
		return nil, err
	}

	if len(entries) <= 0 {
		return nil, nil
	}

	otherIncludes := []string{}
	files := []string{}
	linkDirs := []string{}

	for _, s := range entries {
		filename := filepath.ToSlash(s.Filename)
		if s.Compiler.ShouldIgnoreFile(filename) {
			log.Infof("Ignoring %s because package dictates it.\n", filename)
			continue
		}

		if s.CompilerType == toolchain.COMPILER_TYPE_ARCHIVE {
			arFile := trimProjectPath(s.Filename)
			linkDirs = append(linkDirs, filepath.Dir(arFile))
			*libraries = append(*libraries, arFile)
		} else {
			CmakeSourceObjectWrite(w, s, &otherIncludes, linkFlags)
			s.Filename = trimProjectPath(s.Filename)
			files = append(files, s.Filename)
		}
	}

	if len(linkDirs) > 0 {
		replaceBackslashesSlice(linkDirs)
		fmt.Fprintf(w, "link_directories(%s)\n", strings.Join(util.SortFields(linkDirs...), " "))
	}

	if len(files) <= 0 {
		return nil, nil
	}

	pkgName := bpkg.rpkg.Lpkg.NameWithRepo()

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Generating CMakeLists.txt for %s\n", pkgName)
	fmt.Fprintf(w, "\n# Generating CMakeLists.txt for %s\n", pkgName)
	fmt.Fprintf(w, "add_library(%s %s)\n",
		EscapePkgName(pkgName),
		strings.Join(files, " "))
	archivePath := trimProjectPath(filepath.Dir(b.ArchivePath(bpkg)))
	CmakeCompilerInfoWrite(w, archivePath, bpkg, entries[0], otherIncludes)

	return bpkg, nil
}

func (b *Builder) CMakeTargetWrite(w io.Writer, targetCompiler *toolchain.Compiler) error {
	bpkgs := b.sortedBuildPackages()
	var compileFlags []string
	var linkFlags []string
	var libraries []string

	c := targetCompiler
	c.AddInfo(b.GetCompilerInfo())

	builtPackages := []*BuildPackage{}
	for _, bpkg := range bpkgs {
		builtPackage, err := b.CMakeBuildPackageWrite(w, bpkg,
			&linkFlags, &libraries)
		if err != nil {
			return err
		}

		if builtPackage != nil {
			builtPackages = append(builtPackages, builtPackage)
		}
	}

	elfName := "cmake_" + filepath.Base(b.AppElfPath())
	fmt.Fprintf(w, "# Generating code for %s\n", elfName)

	var targetObjectsBuffer bytes.Buffer

	for _, bpkg := range builtPackages {
		targetObjectsBuffer.WriteString(fmt.Sprintf("%s ",
			EscapePkgName(bpkg.rpkg.Lpkg.NameWithRepo())))
	}

	for _, filename := range libraries {
		targetObjectsBuffer.WriteString(fmt.Sprintf("%s ",
			ExtractLibraryName(filename)))
	}

	elfOutputDir := trimProjectPath(filepath.Dir(b.AppElfPath()))
	fmt.Fprintf(w, "file(WRITE %s \"\")\n", replaceBackslashes(filepath.Join(elfOutputDir, "null.c")))
	fmt.Fprintf(w, "add_executable(%s %s)\n\n", elfName,
		replaceBackslashes(filepath.Join(elfOutputDir, "null.c")))

	if c.GetLdResolveCircularDeps() {
		fmt.Fprintf(w, "target_link_libraries(%s -Wl,--start-group %s -Wl,--end-group)\n",
			elfName, targetObjectsBuffer.String())
	} else {
		fmt.Fprintf(w, "target_link_libraries(%s %s)\n",
			elfName, targetObjectsBuffer.String())
	}

	compileFlags = append(compileFlags, c.GetCompilerInfo().Cflags...)
	compileFlags = append(compileFlags, c.GetLocalCompilerInfo().Cflags...)
	compileFlags = append(compileFlags, c.GetCompilerInfo().CXXflags...)
	compileFlags = append(compileFlags, c.GetLocalCompilerInfo().CXXflags...)
	compileFlags = util.SortFields(compileFlags...)

	fmt.Fprintf(w,
		`set_property(TARGET %s APPEND_STRING
             PROPERTY
             COMPILE_FLAGS
             "%s")`,
		elfName,
		strings.Join(escapeFlagsSlice(compileFlags), " "))
	fmt.Fprintln(w)

	lFlags := append(c.GetCompilerInfo().Lflags, c.GetLocalCompilerInfo().Lflags...)
	lFlags = append(lFlags, linkFlags...)
	lFlags = util.SortFields(lFlags...)

	for _, ld := range c.LinkerScripts {
		lFlags = append(lFlags, "-T"+ld)
	}

	var cFlags []string
	cFlags = append(cFlags, c.GetCompilerInfo().Cflags...)
	cFlags = append(cFlags, c.GetLocalCompilerInfo().Cflags...)
	cFlags = util.SortFields(cFlags...)
	lFlags = append(lFlags, cFlags...)

	var cxxFlags []string
	cxxFlags = append(cxxFlags, c.GetCompilerInfo().CXXflags...)
	cxxFlags = append(cxxFlags, c.GetLocalCompilerInfo().CXXflags...)
	cxxFlags = util.SortFields(cxxFlags...)
	lFlags = append(lFlags, cxxFlags...)

	fmt.Fprintf(w,
		`set_target_properties(%s
                      PROPERTIES
                      ARCHIVE_OUTPUT_DIRECTORY %s
                      LIBRARY_OUTPUT_DIRECTORY %s
                      RUNTIME_OUTPUT_DIRECTORY %s
                      LINK_FLAGS "%s"
                      LINKER_LANGUAGE C)`,
		elfName,
		elfOutputDir,
		elfOutputDir,
		elfOutputDir,
		strings.Join(escapeFlagsSlice(lFlags), " "))

	fmt.Fprintln(w)

	libs := strings.Join(getLibsFromLinkerFlags(lFlags), " ")
	fmt.Fprintf(w, "# Workaround for gcc linker woes\n")
	fmt.Fprintf(w, "set(CMAKE_C_LINK_EXECUTABLE \"${CMAKE_C_LINK_EXECUTABLE} %s\")\n", libs)
	fmt.Fprintln(w)

	return nil
}

func getLibsFromLinkerFlags(lflags []string) []string {
	libs := []string{}

	for _, flag := range lflags {
		if strings.HasPrefix(flag, "-l") {
			libs = append(libs, flag)
		}
	}

	return libs
}

func CmakeCompilerInfoWrite(w io.Writer, archiveFile string, bpkg *BuildPackage,
	cj toolchain.CompilerJob, otherIncludes []string) {
	c := cj.Compiler

	var includes []string

	includes = append(includes, c.GetCompilerInfo().Includes...)
	includes = append(includes, c.GetLocalCompilerInfo().Includes...)
	includes = append(includes, otherIncludes...)

	// Sort and remove duplicate flags
	includes = util.SortFields(includes...)
	trimProjectPathSlice(includes)
	replaceBackslashesSlice(includes)

	fmt.Fprintf(w,
		`set_target_properties(%s
                      PROPERTIES
                      ARCHIVE_OUTPUT_DIRECTORY %s
                      LIBRARY_OUTPUT_DIRECTORY %s
                      RUNTIME_OUTPUT_DIRECTORY %s)`,
		EscapePkgName(bpkg.rpkg.Lpkg.NameWithRepo()),
		archiveFile,
		archiveFile,
		archiveFile,
	)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "target_include_directories(%s PUBLIC %s)\n\n",
		EscapePkgName(bpkg.rpkg.Lpkg.NameWithRepo()),
		strings.Join(includes, " "))
}

func (t *TargetBuilder) CMakeTargetBuilderWrite(w io.Writer, targetCompiler *toolchain.Compiler) error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	if len(t.res.PreBuildCmdCfg.StageFuncs) > 0 ||
		len(t.res.PreLinkCmdCfg.StageFuncs) > 0 ||
		len(t.res.PostLinkCmdCfg.StageFuncs) > 0 {

		util.OneTimeWarning(
			"custom commands not included in cmake output (unsupported)")
	}

	/* Build the Apps */
	project.ResetDeps(t.AppList)

	targetCompiler.LinkerScripts = t.bspPkg.LinkerScripts

	if err := t.bspPkg.Reload(t.AppBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	if err := t.AppBuilder.CMakeTargetWrite(w, targetCompiler); err != nil {
		return err
	}

	return nil
}

func CmakeCompilerWrite(w io.Writer, c *toolchain.Compiler) {
	/* Since CMake 3 it is required to set a full path to the compiler */
	/* TODO: get rid of the prefix to /usr/bin */
	fmt.Fprintln(w, "set(CMAKE_SYSTEM_NAME Generic)")
	fmt.Fprintln(w, "set(CMAKE_TRY_COMPILE_TARGET_TYPE STATIC_LIBRARY)")
	fmt.Fprintf(w, "set(CMAKE_C_COMPILER %s)\n", c.GetCcPath())
	fmt.Fprintf(w, "set(CMAKE_CXX_COMPILER %s)\n", c.GetCppPath())
	fmt.Fprintf(w, "set(CMAKE_ASM_COMPILER %s)\n", c.GetAsPath())
	/* TODO: cmake returns error on link */
	//fmt.Fprintf(w, "set(CMAKE_AR %s)\n", c.GetArPath())
	fmt.Fprintln(w)
}

func CmakeHeaderWrite(w io.Writer, c *toolchain.Compiler, targetName string) {
	fmt.Fprintln(w, "cmake_minimum_required(VERSION 3.7)\n")
	CmakeCompilerWrite(w, c)
	fmt.Fprintf(w, "project(%s VERSION 0.0.0 LANGUAGES C CXX ASM)\n\n", targetName)
	fmt.Fprintln(w, "SET(CMAKE_C_FLAGS_BACKUP  \"${CMAKE_C_FLAGS}\")")
	fmt.Fprintln(w, "SET(CMAKE_CXX_FLAGS_BACKUP  \"${CMAKE_CXX_FLAGS}\")")
	fmt.Fprintln(w, "SET(CMAKE_ASM_FLAGS_BACKUP  \"${CMAKE_ASM_FLAGS}\")")
	fmt.Fprintln(w)
}

func CMakeTargetGenerate(target *target.Target) error {
	CmakeFileHandle, err := os.Create(CmakeListsPath())
	if err != nil {
		return util.ChildNewtError(err)
	}

	var b = bytes.Buffer{}
	w := bufio.NewWriter(&b)
	defer CmakeFileHandle.Close()

	targetBuilder, err := NewTargetBuilder(target)
	if err != nil {
		return err
	}

	targetCompiler, err := targetBuilder.NewCompiler("", "")
	if err != nil {
		return err
	}

	CmakeHeaderWrite(w, targetCompiler, target.ShortName())

	if err := targetBuilder.CMakeTargetBuilderWrite(w, targetCompiler); err != nil {
		return err
	}

	w.Flush()

	CmakeFileHandle.Write(b.Bytes())
	return nil
}
