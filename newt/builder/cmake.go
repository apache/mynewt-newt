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
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/toolchain"
	"fmt"
	"os"
	"bufio"
	"io"
	"bytes"
	"strings"
)

const CMAKELISTS_FILENAME string = "CMakeLists.txt"

func (t *TargetBuilder) CmakeListsPath() string {
	return TargetBinDir(t.target.Name()) + "/" + CMAKELISTS_FILENAME
}

func (b *Builder) Generate(w io.Writer) error {
	//b.CleanArtifacts()

	// Build the packages alphabetically to ensure a consistent order.
	bpkgs := b.sortedBuildPackages()

	// Calculate the list of jobs.  Each record represents a single file that
	// needs to be compiled.
	entries := []toolchain.CompilerJob{}
	bpkgCompilerMap := map[*BuildPackage]*toolchain.Compiler{}
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
		bpkgCompilerMap[bpkg] = subEntries[0].Compiler
		for _, s := range subEntries {
			files = append(files, s.Filename)
		}
		c := subEntries[0].Compiler

		fmt.Fprintf(w, "# Generating code for %s\n\n", bpkg.rpkg.Lpkg.Name())
		CmakeCompilerWrite(w, c)

		fmt.Fprintf(w, "add_library(%s %s)\n\n", bpkg.rpkg.Lpkg.EscapedName(),
			strings.Join(files, " "))
		CmakeCompilerInfoWrite(w, bpkg, c)

		fmt.Printf("%s\n", bpkg.rpkg.Lpkg.BasePath())

		for _, entry := range subEntries {
			fmt.Printf("    %s\n", entry.Filename)
		}
	}
	return nil
}
func CmakeCompilerInfoWrite(w io.Writer, bpkg *BuildPackage, c *toolchain.Compiler) {
	fmt.Fprintf(w, "target_include_directories(%s PUBLIC %s %s)\n\n",
		bpkg.rpkg.Lpkg.EscapedName(), strings.Join(c.GetCompilerInfo().Includes, " "),
		strings.Join(c.GetLocalCompilerInfo().Includes, " "))
	fmt.Fprintf(w, "SET(CMAKE_C_FLAGS  \"${CMAKE_C_FLAGS_BACKUP} %s %s\")\n\n",
		strings.Join(c.GetCompilerInfo().Cflags, " "),
		strings.Join(c.GetLocalCompilerInfo().Cflags, " "))
	fmt.Fprintf(w, "SET(CMAKE_ASM_FLAGS  \"${CMAKE_ASM_FLAGS_BACKUP} %s %s ${CMAKE_C_FLAGS_BACKUP} %s %s\")\n\n",
		strings.Join(c.GetCompilerInfo().Aflags, " "),
		strings.Join(c.GetLocalCompilerInfo().Aflags, " "),
		strings.Join(c.GetCompilerInfo().Cflags, " "),
		strings.Join(c.GetLocalCompilerInfo().Cflags, " "))
}

func CmakeCompilerWrite(w io.Writer, c *toolchain.Compiler) {
	/* Since CMake 3 it is required to set a full path to the compiler */
	/* TODO: get rid of the prefix to /usr/bin */
	fmt.Fprintf(w, "set(CMAKE_C_COMPILER %s)\n", "/usr/bin/"+c.GetCcPath())
	fmt.Fprintf(w, "set(CMAKE_CXX_COMPILER %s)\n", "/usr/bin/"+c.GetCppPath())
	fmt.Fprintf(w, "set(CMAKE_ASM_COMPILER %s)\n", "/usr/bin/"+c.GetAsPath())
	fmt.Fprintf(w, "set(CMAKE_AR %s)\n", "/usr/bin/"+c.GetArPath())
	fmt.Fprintln(w, "enable_language(C CXX ASM)")
	fmt.Fprintln(w)
}

func (t *TargetBuilder) Generate() error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	/* Build the Apps */
	project.ResetDeps(t.AppList)

	if err := t.bspPkg.Reload(t.AppBuilder.cfg.Features()); err != nil {
		return err
	}

	fmt.Println(t.CmakeListsPath())
	CmakeFileHandle, _ := os.Create(t.CmakeListsPath())
	var b = bytes.Buffer{}
	writer := bufio.NewWriter(&b)
	defer CmakeFileHandle.Close()

	fmt.Fprintln(writer, "cmake_minimum_required(VERSION 3.7)\n")
	fmt.Fprintln(writer, "SET(CMAKE_C_FLAGS_BACKUP  \"${CMAKE_C_FLAGS}\")")
	fmt.Fprintln(writer, "SET(CMAKE_CXX_FLAGS_BACKUP  \"${CMAKE_CXX_FLAGS}\")")
	fmt.Fprintln(writer, "SET(CMAKE_ASM_FLAGS_BACKUP  \"${CMAKE_ASM_FLAGS}\")")
	fmt.Fprintln(writer)

	if err := t.AppBuilder.Generate(writer); err != nil {
		return err
	}

	writer.Flush()
	CmakeFileHandle.Write(b.Bytes())

	//var linkerScripts []string
	//if t.LoaderBuilder == nil {
	//	linkerScripts = t.bspPkg.LinkerScripts
	//} else {
	//	if err := t.buildLoader(); err != nil {
	//		return err
	//	}
	//	linkerScripts = t.bspPkg.Part2LinkerScripts
	//}
	//
	///* Link the app. */
	//if err := t.AppBuilder.Link(linkerScripts); err != nil {
	//	return err
	//}

	return nil
}
