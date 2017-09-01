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
	"strings"

	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
	"path/filepath"
)

const CMAKELISTS_FILENAME string = "CMakeLists.txt"

func CmakeListsPath() string {
	return project.GetProject().BasePath + "/" + CMAKELISTS_FILENAME
}

func EscapeName(name string) string {
	return strings.Replace(name, "/","_", -1)
}

func CmakeSourceObjectWrite(w io.Writer, cj toolchain.CompilerJob) {
	c := cj.Compiler

	compileFlags := []string{}

	switch cj.CompilerType {
	case toolchain.COMPILER_TYPE_C:
		compileFlags = append(compileFlags, c.GetCompilerInfo().Cflags...)
		compileFlags = append(compileFlags, c.GetLocalCompilerInfo().Cflags...)
	case toolchain.COMPILER_TYPE_ASM:
		compileFlags = append(compileFlags, c.GetCompilerInfo().Cflags...)
		compileFlags = append(compileFlags, c.GetLocalCompilerInfo().Cflags...)
		compileFlags = append(compileFlags, c.GetCompilerInfo().Aflags...)
		compileFlags = append(compileFlags, c.GetLocalCompilerInfo().Aflags...)
	case toolchain.COMPILER_TYPE_CPP:
		compileFlags = append(compileFlags, c.GetCompilerInfo().Cflags...)
		compileFlags = append(compileFlags, c.GetLocalCompilerInfo().Cflags...)
	}

	fmt.Fprintf(w, `set_property(SOURCE %s APPEND_STRING
														PROPERTY
														COMPILE_FLAGS
														"%s")`,
		cj.Filename,
		strings.Replace(strings.Join(compileFlags, " "), "\"", "\\\\\\\"", -1))
	fmt.Fprintln(w)
}

func (b *Builder) CMakeBuildPackageWrite(w io.Writer, bpkg *BuildPackage) (*BuildPackage, error) {
	entries, err := b.collectCompileEntriesBpkg(bpkg)
	if err != nil {
		return nil, err
	}

	if len(entries) <= 0 {
		return nil, nil
	}

	files := []string{}
	for _, s := range entries {
		CmakeSourceObjectWrite(w, s)
		files = append(files, s.Filename)
	}

	pkgName := bpkg.rpkg.Lpkg.Name()

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Generating CMakeLists.txt for %s\n", pkgName)
	fmt.Fprintf(w, "# Generating CMakeLists.txt for %s\n\n", pkgName)
	fmt.Fprintf(w, "add_library(%s %s)\n\n",
		EscapeName(pkgName),
		strings.Join(files, " "))

	archivePath, _ := filepath.Abs(filepath.Dir(b.ArchivePath(bpkg)))
	CmakeCompilerInfoWrite(w, archivePath, bpkg, entries[0])

	return bpkg, nil
}

func (b *Builder) CMakeTargetWrite(w io.Writer, targetCompiler *toolchain.Compiler) error {
	bpkgs := b.sortedBuildPackages()

	c := targetCompiler

	builtPackages := []*BuildPackage{}
	for _, bpkg := range bpkgs {
		builtPackage, err := b.CMakeBuildPackageWrite(w, bpkg)
		if err != nil {
			return err
		}

		if builtPackage != nil {
			builtPackages = append(builtPackages, builtPackage)
		}
	}

	elfName := "lib" + filepath.Base(b.AppElfPath())
	fmt.Fprintf(w, "# Generating code for %s\n\n", elfName)

	var targetObjectsBuffer bytes.Buffer

	for _, bpkg := range builtPackages {
		targetObjectsBuffer.WriteString(fmt.Sprintf("%s ",
			EscapeName(bpkg.rpkg.Lpkg.Name())))
	}

	elfOutputDir := filepath.Dir(b.AppElfPath())
	fmt.Fprintf(w, "file(WRITE %s \"\")\n", filepath.Join(elfOutputDir, "null.c"))
	fmt.Fprintf(w, "add_executable(%s %s)\n\n", elfName, filepath.Join(elfOutputDir, "null.c"))

	fmt.Fprintf(w, " target_link_libraries(%s -Wl,--start-group %s -Wl,--end-group)\n",
		elfName, targetObjectsBuffer.String())

	fmt.Fprintf(w, `set_property(TARGET %s APPEND_STRING
														PROPERTY
														COMPILE_FLAGS
														"%s")`,
		elfName,
		strings.Replace(strings.Join(append(c.GetCompilerInfo().Cflags,
			c.GetLocalCompilerInfo().Cflags...), " "), "\"", "\\\\\\\"", -1))
	fmt.Fprintln(w)

	lFlags := append(c.GetCompilerInfo().Lflags, c.GetLocalCompilerInfo().Lflags...)
	for _, ld := range c.LinkerScripts {
		lFlags = append(lFlags, "-T"+ld)
	}

	fmt.Fprintf(w, `
	set_target_properties(%s
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
		strings.Join(lFlags, " "))

	fmt.Fprintln(w)

	return nil
}

func CmakeCompilerInfoWrite(w io.Writer, archiveFile string, bpkg *BuildPackage, cj toolchain.CompilerJob) {
	c := cj.Compiler

	fmt.Fprintf(w, `
	set_target_properties(%s
							PROPERTIES
							ARCHIVE_OUTPUT_DIRECTORY %s
							LIBRARY_OUTPUT_DIRECTORY %s
							RUNTIME_OUTPUT_DIRECTORY %s)`,
		EscapeName(bpkg.rpkg.Lpkg.Name()),
		archiveFile,
		archiveFile,
		archiveFile,
	)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "target_include_directories(%s PUBLIC %s %s)\n\n",
		EscapeName(bpkg.rpkg.Lpkg.Name()),
		strings.Join(c.GetCompilerInfo().Includes, " "),
		strings.Join(c.GetLocalCompilerInfo().Includes, " "))
}

func (t *TargetBuilder) CMakeTargetBuilderWrite(w io.Writer, targetCompiler *toolchain.Compiler) error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	/* Build the Apps */
	project.ResetDeps(t.AppList)

	targetCompiler.LinkerScripts = t.bspPkg.LinkerScripts

	if err := t.bspPkg.Reload(t.AppBuilder.cfg.Features()); err != nil {
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

func CmakeHeaderWrite(w io.Writer, c *toolchain.Compiler) {
	fmt.Fprintln(w, "cmake_minimum_required(VERSION 3.7)\n")
	CmakeCompilerWrite(w, c)
	fmt.Fprintln(w, "project(Mynewt VERSION 0.0.0 LANGUAGES C ASM)\n")
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

	targetCompiler, err := targetBuilder.NewCompiler("")
	if err != nil {
		return err
	}

	CmakeHeaderWrite(w, targetCompiler)

	if err := targetBuilder.CMakeTargetBuilderWrite(w, targetCompiler); err != nil {
		return err
	}

	w.Flush()

	CmakeFileHandle.Write(b.Bytes())
	return nil
}
