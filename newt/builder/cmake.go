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

func (b *Builder) Generate(w io.Writer, name string) error {
	//b.CleanArtifacts()

	// Build the packages alphabetically to ensure a consistent order.
	bpkgs := b.sortedBuildPackages()

	// Calculate the list of jobs.  Each record represents a single file that
	// needs to be compiled.
	entries := []toolchain.CompilerJob{}
	builtPackages := []*BuildPackage{}
	for _, bpkg := range bpkgs {
		subEntries, err := b.collectCompileEntriesBpkg(bpkg)
		if err != nil {
			return err
		}

		if len(subEntries) <= 0 {
			continue
		}

		entries = append(entries, subEntries...)
		files := []string{}
		for _, s := range subEntries {
			files = append(files, s.Filename)
		}
		c := subEntries[0].Compiler

		builtPackages = append(builtPackages, bpkg)

		fmt.Fprintf(w, "# Generating code for %s\n\n", bpkg.rpkg.Lpkg.Name())

		fmt.Fprintf(w, "add_library(%s_%s OBJECT %s)\n\n",
			name, bpkg.rpkg.Lpkg.EscapedName(),
			strings.Join(files, " "))

		CmakeCompilerInfoWrite(w, name, bpkg, c)

		fmt.Printf("%s\n", bpkg.rpkg.Lpkg.BasePath())
	}

	fmt.Fprintf(w, "# Generating code for %s\n\n", name)

	var targetObjectsBuffer bytes.Buffer

	for _, bpkg := range builtPackages {
		targetObjectsBuffer.WriteString(fmt.Sprintf("$<TARGET_OBJECTS:%s_%s> ",
			name, bpkg.rpkg.Lpkg.EscapedName()))
	}
	fmt.Fprintf(w, "add_library(%s %s)\n\n", name, targetObjectsBuffer.String())
	fmt.Fprintf(w, ` set_target_properties(%s
									PROPERTIES ARCHIVE_OUTPUT_DIRECTORY %s
									LIBRARY_OUTPUT_DIRECTORY %s
									RUNTIME_OUTPUT_DIRECTORY %s
									)`,
		name,
		filepath.Dir(b.AppElfPath()),
		filepath.Dir(b.AppElfPath()),
		filepath.Dir(b.AppElfPath()))


	return nil
}
func CmakeCompilerInfoWrite(w io.Writer, target_name string, bpkg *BuildPackage, c *toolchain.Compiler) {
	fmt.Fprintf(w, "target_include_directories(%s_%s PUBLIC %s %s)\n\n",
		target_name, bpkg.rpkg.Lpkg.EscapedName(), strings.Join(c.GetCompilerInfo().Includes, " "),
		strings.Join(c.GetLocalCompilerInfo().Includes, " "))
	fmt.Fprintf(w, "set(CMAKE_C_FLAGS  \"${CMAKE_C_FLAGS_BACKUP} %s %s\")\n\n",
		strings.Join(c.GetCompilerInfo().Cflags, " "),
		strings.Join(c.GetLocalCompilerInfo().Cflags, " "))
	fmt.Fprintf(w, "set(CMAKE_ASM_FLAGS  \"${CMAKE_ASM_FLAGS_BACKUP} %s %s ${CMAKE_C_FLAGS_BACKUP} %s %s\")\n\n",
		strings.Join(c.GetCompilerInfo().Aflags, " "),
		strings.Join(c.GetLocalCompilerInfo().Aflags, " "),
		strings.Join(c.GetCompilerInfo().Cflags, " "),
		strings.Join(c.GetLocalCompilerInfo().Cflags, " "))
}

func CmakeCompilerWrite(w io.Writer, c *toolchain.Compiler) {
	/* Since CMake 3 it is required to set a full path to the compiler */
	/* TODO: get rid of the prefix to /usr/bin */
	fmt.Fprintln(w, "set(CMAKE_SYSTEM_NAME Generic)")
	fmt.Fprintln(w, "set(CMAKE_TRY_COMPILE_TARGET_TYPE STATIC_LIBRARY)")
	fmt.Fprintf(w, "set(CMAKE_C_COMPILER %s)\n", "/usr/bin/"+c.GetCcPath())
	fmt.Fprintf(w, "set(CMAKE_CXX_COMPILER %s)\n", "/usr/bin/"+c.GetCppPath())
	fmt.Fprintf(w, "set(CMAKE_ASM_COMPILER %s)\n", "/usr/bin/"+c.GetAsPath())
	fmt.Fprintf(w, "set(CMAKE_AR %s)\n", "/usr/bin/"+c.GetArPath())
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

func (t *TargetBuilder) CMakeGenerateTarget(w io.Writer) error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	/* Build the Apps */
	project.ResetDeps(t.AppList)

	if err := t.bspPkg.Reload(t.AppBuilder.cfg.Features()); err != nil {
		return err
	}

	if err := t.AppBuilder.Generate(w, t.target.ShortName()); err != nil {
		return err
	}

	return nil
}

func CMakeGenerate(target *target.Target) error {
	CmakeFileHandle, _ := os.Create(CmakeListsPath())
	var b = bytes.Buffer{}
	w := bufio.NewWriter(&b)
	defer CmakeFileHandle.Close()


	targetBuilder, err := NewTargetBuilder(target)
	if err != nil {
		return util.NewNewtError(err.Error())
	}

	c, err := targetBuilder.NewCompiler("")
	if  err != nil {
		return err
	}

	CmakeHeaderWrite(w, c)

	if err := targetBuilder.CMakeGenerateTarget(w); err != nil {
		return util.NewNewtError(err.Error())
	}

	w.Flush()

	CmakeFileHandle.Write(b.Bytes())
	return  nil
}
