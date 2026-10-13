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

// The generated CMakeLists.txt builds a target the same way "newt build" does:
//
//   * Each package is compiled with exactly the flags and include directories
//     newt uses.  Package archives are written to the same paths newt uses, so
//     linker scripts, external scripts and other newt commands (load, debug,
//     create-image) work with the output of either build system.
//   * Pre-build, pre-link and post-link commands are executed at the same
//     build stages as in newt, with the same environment.
//   * The elf file is linked with the same command line newt uses.  The list
//     of input libraries is calculated at build time, exactly as newt does it
//     (all .a files in package bin directories, objects of whole-archive
//     packages, .a files emitted by pre-link commands), and passed to the
//     linker in a response file.
//   * The .bin, .lst and .map files are generated after linking.
//
// Generated code, link tables and pre-build artifacts are produced when the
// CMakeLists.txt file is generated, so "newt target cmake" must be rerun
// whenever the target configuration changes.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/stage"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

const CMAKELISTS_FILENAME string = "CMakeLists.txt"

const cmakeMinimumVersion = "3.13"

const (
	cmakePreBuildTarget = "newt_pre_build"
	cmakePreLinkTarget  = "newt_pre_link"
)

func CmakeListsPath() string {
	return project.GetProject().BasePath + "/" + CMAKELISTS_FILENAME
}

// CmakeDir returns the directory where newt writes the helper scripts used by
// the generated CMakeLists.txt.
func CmakeDir(targetName string) string {
	return TargetBinDir(targetName) + "/cmake"
}

func EscapePkgName(name string) string {
	name = strings.Replace(name, "/", "_", -1)
	if strings.HasPrefix(name, "@") {
		name = strings.Replace(name, "@", "repos_", 1)
	}
	return name
}

func replaceBackslashes(path string) string {
	return strings.Replace(path, "\\", "/", -1)
}

// cmakeEscape escapes a string so that it can be used inside a quoted CMake
// argument.
func cmakeEscape(s string) string {
	s = strings.Replace(s, "\\", "\\\\", -1)
	s = strings.Replace(s, "\"", "\\\"", -1)
	s = strings.Replace(s, "$", "\\$", -1)
	return s
}

// cmakeQuote converts a string to a quoted CMake argument.
func cmakeQuote(s string) string {
	return "\"" + cmakeEscape(s) + "\""
}

// cmakeQuoteSlice converts each string to a quoted CMake argument.
func cmakeQuoteSlice(elements []string) []string {
	quoted := make([]string, len(elements))
	for i, e := range elements {
		quoted[i] = cmakeQuote(e)
	}
	return quoted
}

// cmakePath converts a path to a quoted CMake argument.  Paths inside the
// project are expressed relative to ${CMAKE_SOURCE_DIR}, so that they don't
// depend on the build directory.
func cmakePath(path string) string {
	path = replaceBackslashes(path)
	projPath := replaceBackslashes(project.GetProject().Path())

	if path == projPath {
		return "\"${CMAKE_SOURCE_DIR}\""
	}
	if strings.HasPrefix(path, projPath+"/") {
		return "\"${CMAKE_SOURCE_DIR}/" +
			cmakeEscape(strings.TrimPrefix(path, projPath+"/")) + "\""
	}
	if !filepath.IsAbs(path) {
		return "\"${CMAKE_SOURCE_DIR}/" + cmakeEscape(path) + "\""
	}

	return cmakeQuote(path)
}

// isProjectRelPath indicates whether the specified string is a relative path
// pointing inside the project, e.g., "repos/apache-mynewt-core/kernel/os" or
// "bin/tfm_s_CMSE_lib.o".
func isProjectRelPath(s string) bool {
	if s == "" || filepath.IsAbs(s) || strings.HasPrefix(s, "$") {
		return false
	}

	parts := strings.SplitN(replaceBackslashes(s), "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[0] == "." ||
		parts[0] == ".." {

		return false
	}

	return util.NodeExist(project.GetProject().Path() + "/" + parts[0])
}

// cmakeFlag converts a compiler or linker flag to a quoted CMake argument.
//
// newt executes the compiler and the linker from the project directory, while
// CMake executes them from the build directory.  Paths inside the project are
// made absolute, so that they point to the same files in both cases.
func cmakeFlag(flag string) string {
	projPath := replaceBackslashes(project.GetProject().Path())

	// Macro definitions are passed as is.
	if strings.HasPrefix(flag, "-D") || strings.HasPrefix(flag, "-U") {
		return cmakeQuote(flag)
	}

	prefix := ""
	value := replaceBackslashes(flag)
	isPath := false
	if len(flag) > 2 && (strings.HasPrefix(flag, "-I") ||
		strings.HasPrefix(flag, "-L") || strings.HasPrefix(flag, "-T")) {

		prefix = flag[:2]
		value = replaceBackslashes(flag[2:])
		isPath = !filepath.IsAbs(value) && !strings.HasPrefix(value, "$")
	} else if i := strings.LastIndex(flag, "="); i >= 0 &&
		strings.HasPrefix(flag, "-") {

		prefix = flag[:i+1]
		value = replaceBackslashes(flag[i+1:])
	}

	if isPath || value == projPath || strings.HasPrefix(value, projPath+"/") ||
		isProjectRelPath(value) {

		path := cmakePath(value)
		return "\"" + cmakeEscape(prefix) + path[1:]
	}

	return cmakeQuote(flag)
}

// cmakeFlagSlice converts each flag to a quoted CMake argument.
func cmakeFlagSlice(flags []string) []string {
	quoted := make([]string, len(flags))
	for i, f := range flags {
		quoted[i] = cmakeFlag(f)
	}
	return quoted
}

// cmakeOptionList converts a list of flags to a quoted CMake list.
func cmakeOptionList(flags []string) string {
	items := cmakeFlagSlice(flags)
	for i, item := range items {
		items[i] = strings.Replace(
			strings.TrimSuffix(strings.TrimPrefix(item, "\""), "\""),
			";", "\\;", -1)
	}
	return "\"" + strings.Join(items, ";") + "\""
}

// dedupKeepLast removes duplicate flags, keeping the last occurrence of each.
// CMake removes duplicate compile and link options, keeping the first
// occurrence.  The compiler uses the last occurrence of a conflicting flag,
// so the duplicates are removed here in a way that doesn't change the
// meaning of the command line.
func dedupKeepLast(flags []string) []string {
	seen := map[string]bool{}
	result := []string{}

	for i := len(flags) - 1; i >= 0; i-- {
		if !seen[flags[i]] {
			seen[flags[i]] = true
			result = append([]string{flags[i]}, result...)
		}
	}

	return result
}

// joinLinkerArgs joins linker arguments that take a separate value (e.g.,
// "-T" "file") into a single argument, because CMake treats each link item
// separately.
func joinLinkerArgs(flags []string) []string {
	result := []string{}

	for i := 0; i < len(flags); i++ {
		if (flags[i] == "-T" || flags[i] == "-L") && i+1 < len(flags) {
			result = append(result, flags[i]+flags[i+1])
			i++
		} else {
			result = append(result, flags[i])
		}
	}

	return result
}

func cmakeLanguage(compilerType int) string {
	switch compilerType {
	case toolchain.COMPILER_TYPE_C:
		return "C"
	case toolchain.COMPILER_TYPE_ASM:
		return "ASM"
	case toolchain.COMPILER_TYPE_CPP:
		return "CXX"
	default:
		return ""
	}
}

// cmakeObjSortKey returns the key used to sort source files in the same order
// as newt sorts the corresponding object files.
func cmakeObjSortKey(filename string) string {
	return strings.TrimSuffix(filename, filepath.Ext(filename)) + ".o"
}

// cmakeLinkPkg holds the information about a package that is required to
// calculate the list of the linker input files at build time.
type cmakeLinkPkg struct {
	name      string
	wholeArch bool
	hasTarget bool

	// Pre-compiled archives found in package sources, mapped to the paths
	// they are copied to.
	archives map[string]string
}

// cmakePkgObjsPath returns the path of the file listing the objects of a
// whole-archive package.
func cmakePkgObjsPath(binaryDir string, name string) string {
	return binaryDir + "/newt/" + name + ".objs"
}

// CMakeBuildPackageWrite writes the library target that builds the specified
// package.
func (b *Builder) CMakeBuildPackageWrite(w io.Writer, bpkg *BuildPackage,
	hasPreBuild bool) (*cmakeLinkPkg, error) {

	entries, err := b.collectCompileEntriesBpkg(bpkg)
	if err != nil {
		return nil, err
	}

	ci, err := bpkg.CompilerInfo(b)
	if err != nil {
		return nil, err
	}

	pkgName := bpkg.rpkg.Lpkg.NameWithRepo()
	lp := &cmakeLinkPkg{
		name:      EscapePkgName(pkgName),
		wholeArch: ci.WholeArch,
		archives:  map[string]string{},
	}

	if len(entries) == 0 {
		return lp, nil
	}

	files := []string{}
	filesByType := map[int][]string{}
	for _, s := range entries {
		filename := filepath.ToSlash(s.Filename)
		if s.Compiler.ShouldIgnoreFile(filename) {
			log.Infof("Ignoring %s because package dictates it.\n", filename)
			continue
		}

		if s.CompilerType == toolchain.COMPILER_TYPE_ARCHIVE {
			lp.archives[filename] = s.Compiler.ArchiveCopyPath(filename)
		} else {
			files = append(files, filename)
			filesByType[s.CompilerType] = append(
				filesByType[s.CompilerType], filename)
		}
	}

	if len(files) == 0 {
		return lp, nil
	}

	lp.hasTarget = true

	// newt adds objects to the package archive sorted by name.  The order of
	// archive members affects the layout of the linked image.
	sort.SliceStable(files, func(i, j int) bool {
		return cmakeObjSortKey(files[i]) < cmakeObjSortKey(files[j])
	})

	// All entries of a package are compiled by the same compiler.
	c := entries[0].Compiler

	libType := "STATIC"
	if lp.wholeArch {
		libType = "OBJECT"
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Generating CMakeLists.txt for %s\n", pkgName)
	fmt.Fprintf(w, "# %s\n", pkgName)
	fmt.Fprintf(w, "add_library(%s %s\n    %s)\n", lp.name, libType,
		strings.Join(cmakeQuoteSlice(trimProjectPathSlice(files)), "\n    "))

	types := []int{}
	for t := range filesByType {
		types = append(types, t)
	}
	sort.Ints(types)

	for _, t := range types {
		flags, err := c.CompileFlags(t)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(w, "set_source_files_properties(\n    %s\n"+
			"    PROPERTIES\n    LANGUAGE %s\n    COMPILE_OPTIONS %s)\n",
			strings.Join(
				cmakeQuoteSlice(trimProjectPathSlice(filesByType[t])),
				"\n    "),
			cmakeLanguage(t),
			cmakeOptionList(dedupKeepLast(flags)))
	}

	includes := c.IncludeDirs()
	if len(includes) > 0 {
		paths := make([]string, len(includes))
		for i, inc := range includes {
			paths[i] = cmakePath(inc)
		}
		fmt.Fprintf(w, "target_include_directories(%s PRIVATE\n    %s)\n",
			lp.name, strings.Join(paths, "\n    "))
	}

	if lp.wholeArch {
		// newt links all objects of a whole-archive package; their list is
		// read at link time.
		fmt.Fprintf(w, "file(GENERATE OUTPUT \"%s\"\n"+
			"     CONTENT \"$<JOIN:$<TARGET_OBJECTS:%s>,\\n>\\n\")\n",
			cmakePkgObjsPath("${CMAKE_BINARY_DIR}", lp.name), lp.name)
	} else {
		archivePath := b.ArchivePath(bpkg)
		fmt.Fprintf(w, "set_target_properties(%s PROPERTIES\n"+
			"    ARCHIVE_OUTPUT_DIRECTORY %s\n"+
			"    OUTPUT_NAME %s\n"+
			"    PREFIX \"\"\n"+
			"    SUFFIX \".a\")\n",
			lp.name,
			cmakePath(filepath.Dir(archivePath)),
			cmakeQuote(strings.TrimSuffix(filepath.Base(archivePath), ".a")))
	}

	if hasPreBuild {
		fmt.Fprintf(w, "add_dependencies(%s %s)\n", lp.name,
			cmakePreBuildTarget)
	}
	fmt.Fprintln(w)

	return lp, nil
}

// writeCmakeScript writes a CMake script executed during the build.
func writeCmakeScript(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return util.ChildNewtError(err)
	}

	content = "# Generated by newt; do not edit.\n\n" +
		"cmake_minimum_required(VERSION " + cmakeMinimumVersion + ")\n\n" +
		content

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return util.ChildNewtError(err)
	}

	return nil
}

// cmakeScriptHelpers contains functions used by the build-time scripts.
const cmakeScriptHelpers = `# Replaces the contents of the "old" directory with the contents of the "new"
# directory if they differ, so that unchanged artifacts don't trigger a
# rebuild.
function(newt_replace_if_changed new old)
    file(GLOB_RECURSE new_files RELATIVE "${new}" "${new}/*")
    file(GLOB_RECURSE old_files RELATIVE "${old}" "${old}/*")
    list(SORT new_files)
    list(SORT old_files)
    set(equal TRUE)
    if(NOT "${new_files}" STREQUAL "${old_files}")
        set(equal FALSE)
    else()
        foreach(f IN LISTS new_files)
            file(SHA1 "${new}/${f}" new_hash)
            file(SHA1 "${old}/${f}" old_hash)
            if(NOT new_hash STREQUAL old_hash)
                set(equal FALSE)
                break()
            endif()
        endforeach()
    endif()
    if(NOT equal)
        get_filename_component(parent "${old}" DIRECTORY)
        file(REMOVE_RECURSE "${old}")
        file(MAKE_DIRECTORY "${parent}")
        file(RENAME "${new}" "${old}")
    endif()
endfunction()

# Writes the file only if its contents change.
function(newt_write_if_changed path content)
    if(EXISTS "${path}")
        file(READ "${path}" old_content)
        if(old_content STREQUAL content)
            return()
        endif()
    endif()
    file(WRITE "${path}" "${content}")
endfunction()

`

// cmakeExtCmds generates the part of a build-time script which executes a set
// of external commands.
func (t *TargetBuilder) cmakeExtCmds(sfs []stage.StageFunc, userSrcDir string,
	userIncDir string, workDir string) (string, error) {

	var b bytes.Buffer

	for _, sf := range sfs {
		cmd, err := t.prepareExtCmd(sf, userSrcDir, userIncDir, workDir)
		if err != nil {
			return "", err
		}

		args, err := util.PrependScriptInterpreter(cmd.args)
		if err != nil {
			return "", util.ChildNewtError(err)
		}

		fmt.Fprintf(&b, "message(%s)\n",
			cmakeQuote("Executing "+sf.Pkg.FullName()+"/"+sf.Name))

		keys := make([]string, 0, len(cmd.env))
		for k := range cmd.env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "set(ENV{%s} %s)\n", k, cmakeQuote(cmd.env[k]))
		}

		fmt.Fprintf(&b, "execute_process(COMMAND %s\n"+
			"    WORKING_DIRECTORY %s\n"+
			"    RESULT_VARIABLE result)\n",
			strings.Join(cmakeQuoteSlice(args), " "),
			cmakeQuote(cmd.dir))
		fmt.Fprintf(&b, "if(NOT result EQUAL 0)\n"+
			"    message(FATAL_ERROR \"%s failed: ${result}\")\n"+
			"endif()\n\n",
			cmakeEscape(sf.Pkg.FullName()+"/"+sf.Name))
	}

	return b.String(), nil
}

func (t *TargetBuilder) cmakeWorkDir() string {
	return CmakeDir(t.target.FullName()) + "/work"
}

// writeCmakePreBuildScript writes the script which executes the pre-build
// commands.
func (t *TargetBuilder) writeCmakePreBuildScript(path string) error {
	tmpDir := CmakeDir(t.target.FullName()) + "/pre_build"
	tmpSrcDir := UserTempSrcDir(tmpDir)
	tmpIncDir := UserTempIncludeDir(tmpDir)

	cmds, err := t.cmakeExtCmds(t.res.PreBuildCmdCfg.StageFuncs,
		tmpSrcDir, tmpIncDir, t.cmakeWorkDir())
	if err != nil {
		return err
	}

	content := cmakeScriptHelpers +
		fmt.Sprintf("file(REMOVE_RECURSE %s)\n", cmakeQuote(tmpDir)) +
		fmt.Sprintf("file(MAKE_DIRECTORY %s %s %s)\n\n",
			cmakeQuote(tmpSrcDir), cmakeQuote(tmpIncDir),
			cmakeQuote(t.cmakeWorkDir())) +
		cmds +
		fmt.Sprintf("newt_replace_if_changed(%s %s)\n",
			cmakeQuote(tmpSrcDir),
			cmakeQuote(UserPreBuildSrcDir(t.target.FullName()))) +
		fmt.Sprintf("newt_replace_if_changed(%s %s)\n",
			cmakeQuote(tmpIncDir),
			cmakeQuote(UserPreBuildIncludeDir(t.target.FullName())))

	return writeCmakeScript(path, content)
}

// writeCmakePreLinkScript writes the script which executes the pre-link
// commands and calculates the list of linker input files.  The list is
// calculated the same way newt does it at link time.
func (t *TargetBuilder) writeCmakePreLinkScript(path string,
	bpkgs []*BuildPackage, lpkgs map[*BuildPackage]*cmakeLinkPkg) error {

	var b bytes.Buffer

	b.WriteString(cmakeScriptHelpers)
	b.WriteString("if(NOT NEWT_BINARY_DIR)\n" +
		"    message(FATAL_ERROR \"NEWT_BINARY_DIR not set\")\n" +
		"endif()\n\n")

	// newt copies pre-compiled archives to the package bin directory at
	// compile time.
	for _, bpkg := range bpkgs {
		lp := lpkgs[bpkg]
		srcs := make([]string, 0, len(lp.archives))
		for src := range lp.archives {
			srcs = append(srcs, src)
		}
		sort.Strings(srcs)
		for _, src := range srcs {
			fmt.Fprintf(&b, "execute_process(COMMAND \"${CMAKE_COMMAND}\" "+
				"-E copy_if_different %s %s)\n",
				cmakeQuote(src), cmakeQuote(lp.archives[src]))
		}
	}
	b.WriteString("\n")

	if len(t.res.PreLinkCmdCfg.StageFuncs) > 0 {
		tmpDir := CmakeDir(t.target.FullName()) + "/pre_link"
		tmpSrcDir := UserTempSrcDir(tmpDir)

		cmds, err := t.cmakeExtCmds(t.res.PreLinkCmdCfg.StageFuncs,
			tmpSrcDir, "", t.cmakeWorkDir())
		if err != nil {
			return err
		}

		fmt.Fprintf(&b, "file(REMOVE_RECURSE %s)\n", cmakeQuote(tmpDir))
		fmt.Fprintf(&b, "file(MAKE_DIRECTORY %s %s)\n\n",
			cmakeQuote(tmpSrcDir), cmakeQuote(t.cmakeWorkDir()))
		b.WriteString(cmds)
		fmt.Fprintf(&b, "newt_replace_if_changed(%s %s)\n\n",
			cmakeQuote(tmpSrcDir),
			cmakeQuote(UserPreLinkSrcDir(t.target.FullName())))
	}

	dirs, err := t.AppBuilder.linkInputDirs(t.extraADirs())
	if err != nil {
		return err
	}

	b.WriteString("set(inputs)\n")
	for _, d := range dirs {
		if d.wholeArch {
			// newt passes the objects of a whole-archive package to the
			// linker in a list file, sorted by name.  CMake doesn't create
			// the list file; the objects are read from a file generated by
			// CMake instead.
			if lp := lpkgs[d.bpkg]; lp != nil && lp.hasTarget {
				fmt.Fprintf(&b, "file(STRINGS \"%s\" files)\n",
					cmakePkgObjsPath("${NEWT_BINARY_DIR}", lp.name))
				b.WriteString("list(SORT files)\n")
				b.WriteString("list(APPEND inputs ${files})\n")
			}
		} else {
			fmt.Fprintf(&b, "file(GLOB files %s)\n", cmakeQuote(d.pattern()))
			b.WriteString("list(APPEND inputs ${files})\n")
		}
	}

	b.WriteString(`list(REMOVE_DUPLICATES inputs)

# The response file lists the linker input files.  The stamp file changes
# whenever any of the input files changes; it triggers relinking.
set(rsp "")
set(stamp "")
foreach(input IN LISTS inputs)
    string(APPEND rsp "\"${input}\"\n")
    file(SHA1 "${input}" hash)
    string(APPEND stamp "${hash} ${input}\n")
endforeach()
newt_write_if_changed("${NEWT_BINARY_DIR}/newt/link.rsp" "${rsp}")
newt_write_if_changed("${NEWT_BINARY_DIR}/newt/link.stamp" "${stamp}")
`)

	return writeCmakeScript(path, b.String())
}

// writeCmakePostLinkScript writes the script which executes the post-link
// commands.
func (t *TargetBuilder) writeCmakePostLinkScript(path string) error {
	cmds, err := t.cmakeExtCmds(t.res.PostLinkCmdCfg.StageFuncs,
		"", "", t.cmakeWorkDir())
	if err != nil {
		return err
	}

	content := fmt.Sprintf("file(MAKE_DIRECTORY %s)\n\n",
		cmakeQuote(t.cmakeWorkDir())) + cmds

	return writeCmakeScript(path, content)
}

// writeCmakeListFileScript writes the script which generates the .lst file.
func writeCmakeListFileScript(path string, c *toolchain.Compiler,
	elfFile string) error {

	listFile := elfFile + ".lst"
	content := fmt.Sprintf(`execute_process(COMMAND %s -wxdS %s
    OUTPUT_FILE %s
    RESULT_VARIABLE result)
if(result EQUAL 0)
    execute_process(COMMAND %s %s OUTPUT_VARIABLE size)
    file(APPEND %s "${size}")
endif()
`,
		cmakeQuote(c.GetObjdumpPath()), cmakeQuote(elfFile),
		cmakeQuote(listFile),
		cmakeQuote(c.GetSizePath()), cmakeQuote(elfFile),
		cmakeQuote(listFile))

	return writeCmakeScript(path, content)
}

// cmakeRunScript returns the CMake arguments of a command which executes the
// specified script.
func cmakeRunScript(path string, defines ...string) string {
	args := []string{"${CMAKE_COMMAND}"}
	for _, d := range defines {
		args = append(args, "\"-D"+d+"\"")
	}
	args = append(args, "-P", cmakePath(path))
	return strings.Join(args, " ")
}

func (b *Builder) CMakeTargetWrite(w io.Writer) error {
	t := b.targetBuilder
	targetName := t.target.FullName()
	cmakeDir := CmakeDir(targetName)
	bpkgs := b.sortedBuildPackages()

	err := b.appendAppCflags(bpkgs)
	if err != nil {
		return err
	}

	hasPreBuild := len(t.res.PreBuildCmdCfg.StageFuncs) > 0
	if hasPreBuild {
		scriptPath := cmakeDir + "/pre_build.cmake"
		if err := t.writeCmakePreBuildScript(scriptPath); err != nil {
			return err
		}

		// Declare the artifacts as byproducts, so that the build system
		// notices when they change.
		byproducts := []string{}
		for _, dir := range []string{
			UserPreBuildSrcDir(targetName),
			UserPreBuildIncludeDir(targetName),
		} {
			filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					byproducts = append(byproducts, cmakePath(p))
				}
				return nil
			})
		}

		fmt.Fprintf(w, "add_custom_target(%s\n"+
			"    COMMAND %s\n", cmakePreBuildTarget, cmakeRunScript(scriptPath))
		if len(byproducts) > 0 {
			fmt.Fprintf(w, "    BYPRODUCTS\n    %s\n",
				strings.Join(byproducts, "\n    "))
		}
		fmt.Fprintf(w, "    COMMENT \"Executing pre-build commands\"\n"+
			"    VERBATIM)\n\n")
	}

	lpkgs := map[*BuildPackage]*cmakeLinkPkg{}
	libTargets := []string{}
	for _, bpkg := range bpkgs {
		lp, err := b.CMakeBuildPackageWrite(w, bpkg, hasPreBuild)
		if err != nil {
			return err
		}

		lpkgs[bpkg] = lp
		if lp.hasTarget {
			libTargets = append(libTargets, lp.name)
		}
	}

	elfFile := b.AppElfPath()
	elfName := filepath.Base(elfFile)

	c, err := b.linkCompiler(elfFile, t.bspPkg.LinkerScripts)
	if err != nil {
		return err
	}
	options := c.LinkOptions()
	preFlags, postFlags := c.LinkFlags(elfFile, options, nil, "")

	preLinkScript := cmakeDir + "/pre_link.cmake"
	if err := t.writeCmakePreLinkScript(preLinkScript, bpkgs,
		lpkgs); err != nil {
		return err
	}

	fmt.Fprintf(w, "# %s\n", elfName)
	fmt.Fprintf(w, "add_custom_target(%s\n"+
		"    COMMAND %s\n"+
		"    BYPRODUCTS\n"+
		"    \"${CMAKE_BINARY_DIR}/newt/link.rsp\"\n"+
		"    \"${CMAKE_BINARY_DIR}/newt/link.stamp\"\n"+
		"    COMMENT \"Preparing link of %s\"\n"+
		"    VERBATIM)\n",
		cmakePreLinkTarget,
		cmakeRunScript(preLinkScript, "NEWT_BINARY_DIR=${CMAKE_BINARY_DIR}"),
		elfName)
	if len(libTargets) > 0 {
		fmt.Fprintf(w, "add_dependencies(%s\n    %s)\n", cmakePreLinkTarget,
			strings.Join(libTargets, "\n    "))
	}
	fmt.Fprintln(w)

	// CMake requires at least one source file for an executable.  The file
	// is not compiled, because any additional linker input changes the
	// layout of the linked image.
	fmt.Fprintf(w, "if(NOT EXISTS \"${CMAKE_BINARY_DIR}/newt/null.c\")\n"+
		"    file(WRITE \"${CMAKE_BINARY_DIR}/newt/null.c\" \"\")\n"+
		"endif()\n")
	fmt.Fprintf(w, "set_source_files_properties(\"${CMAKE_BINARY_DIR}/newt/null.c\"\n"+
		"    PROPERTIES HEADER_FILE_ONLY TRUE)\n")
	fmt.Fprintf(w, "add_executable(%s \"${CMAKE_BINARY_DIR}/newt/null.c\")\n",
		elfName)
	fmt.Fprintf(w, "add_dependencies(%s %s)\n", elfName, cmakePreLinkTarget)
	fmt.Fprintf(w, "set_target_properties(%s PROPERTIES\n"+
		"    RUNTIME_OUTPUT_DIRECTORY %s\n"+
		"    LINKER_LANGUAGE C\n"+
		"    LINK_DEPENDS \"${CMAKE_BINARY_DIR}/newt/link.stamp\")\n",
		elfName, cmakePath(filepath.Dir(elfFile)))

	pre := cmakeFlagSlice(dedupKeepLast(preFlags))
	fmt.Fprintf(w, "target_link_options(%s PRIVATE\n    %s)\n", elfName,
		strings.Join(pre, "\n    "))

	linkItems := []string{}
	if c.GetLdResolveCircularDeps() {
		linkItems = append(linkItems, cmakeQuote("-Wl,--start-group"))
	}
	linkItems = append(linkItems, "\"-Wl,@${CMAKE_BINARY_DIR}/newt/link.rsp\"")
	if c.GetLdResolveCircularDeps() {
		linkItems = append(linkItems, cmakeQuote("-Wl,--end-group"))
	}
	linkItems = append(linkItems, cmakeFlagSlice(joinLinkerArgs(postFlags))...)
	fmt.Fprintf(w, "target_link_libraries(%s PRIVATE\n    %s)\n", elfName,
		strings.Join(linkItems, "\n    "))

	// Generate the same artifacts as newt does after linking.
	postBuild := []string{}
	if options["binFile"] {
		postBuild = append(postBuild, strings.Join(cmakeFlagSlice(
			c.BinFileCmd(elfFile, elfFile+".bin")), " "))
	}
	if options["listFile"] {
		listFileScript := cmakeDir + "/list_file.cmake"
		if err := writeCmakeListFileScript(listFileScript, c,
			elfFile); err != nil {

			return err
		}
		postBuild = append(postBuild, cmakeRunScript(listFileScript))
	}
	if len(t.res.PostLinkCmdCfg.StageFuncs) > 0 {
		postLinkScript := cmakeDir + "/post_link.cmake"
		if err := t.writeCmakePostLinkScript(postLinkScript); err != nil {
			return err
		}
		postBuild = append(postBuild, cmakeRunScript(postLinkScript))
	}

	if len(postBuild) > 0 {
		fmt.Fprintf(w, "add_custom_command(TARGET %s POST_BUILD\n", elfName)
		for _, cmd := range postBuild {
			fmt.Fprintf(w, "    COMMAND %s\n", cmd)
		}
		fmt.Fprintf(w, "    VERBATIM)\n")
	}
	fmt.Fprintln(w)

	return nil
}

// cmakePrepare performs the steps of "newt build" which precede compilation
// and generate the build inputs.
func (t *TargetBuilder) cmakePrepare() error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	if t.res.LoaderSet != nil {
		return util.NewNewtError(
			"split images are not supported by cmake generator")
	}
	if t.appPkg == nil {
		return util.NewNewtError("cmake generator requires a target with an app")
	}

	// Pre-build commands are executed now, so that the artifacts they emit
	// are included in the generated CMakeLists.txt.  They are executed again
	// at build time.
	workDir := t.cmakeWorkDir()
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return util.ChildNewtError(err)
	}

	return t.prepareBuildInputs(workDir)
}

func (t *TargetBuilder) CMakeTargetBuilderWrite(w io.Writer,
	projectName string) error {

	if err := t.cmakePrepare(); err != nil {
		return err
	}

	c, err := t.NewCompiler("", "")
	if err != nil {
		return err
	}

	CmakeHeaderWrite(w, c, projectName)

	return t.AppBuilder.CMakeTargetWrite(w)
}

func CmakeCompilerWrite(w io.Writer, c *toolchain.Compiler) {
	fmt.Fprintln(w, "set(CMAKE_SYSTEM_NAME Generic)")
	fmt.Fprintln(w, "set(CMAKE_TRY_COMPILE_TARGET_TYPE STATIC_LIBRARY)")
	fmt.Fprintf(w, "set(CMAKE_C_COMPILER %s)\n", cmakeQuote(c.GetCcPath()))
	fmt.Fprintf(w, "set(CMAKE_CXX_COMPILER %s)\n", cmakeQuote(c.GetCppPath()))
	fmt.Fprintf(w, "set(CMAKE_ASM_COMPILER %s)\n", cmakeQuote(c.GetAsPath()))
	fmt.Fprintf(w, "set(CMAKE_AR %s)\n", cmakeQuote(c.GetArPath()))
	fmt.Fprintln(w)
}

func CmakeHeaderWrite(w io.Writer, c *toolchain.Compiler, projectName string) {
	fmt.Fprintf(w, "# Generated by newt; do not edit.\n\n")
	fmt.Fprintf(w, "cmake_minimum_required(VERSION %s)\n\n", cmakeMinimumVersion)
	CmakeCompilerWrite(w, c)
	fmt.Fprintf(w, "project(%s VERSION 0.0.0 LANGUAGES C CXX ASM)\n\n",
		cmakeQuote(projectName))

	// newt passes source file paths relative to the project directory to the
	// compiler, while CMake passes absolute paths.  Strip the project
	// directory from paths expanded by macros (e.g., __FILE__), so that the
	// generated code is the same.
	fmt.Fprintln(w, "include(CheckCCompilerFlag)")
	fmt.Fprintln(w, "check_c_compiler_flag(\"-fmacro-prefix-map=${CMAKE_SOURCE_DIR}/=\"\n"+
		"    NEWT_HAVE_MACRO_PREFIX_MAP)")
	fmt.Fprintln(w, "if(NEWT_HAVE_MACRO_PREFIX_MAP)")
	fmt.Fprintln(w, "    add_compile_options(\"-fmacro-prefix-map=${CMAKE_SOURCE_DIR}/=\")")
	fmt.Fprintln(w, "endif()")
	fmt.Fprintln(w)
}

func CMakeTargetGenerate(target *target.Target) error {
	targetBuilder, err := NewTargetBuilder(target)
	if err != nil {
		return err
	}

	proj, err := project.TryGetProject()
	if err != nil {
		return err
	}

	var b = bytes.Buffer{}
	w := bufio.NewWriter(&b)

	if err := targetBuilder.CMakeTargetBuilderWrite(w,
		fmt.Sprintf("%s (%s)", proj.Name(), target.ShortName())); err != nil {

		return err
	}

	w.Flush()

	if err := os.WriteFile(CmakeListsPath(), b.Bytes(), 0644); err != nil {
		return util.ChildNewtError(err)
	}

	return nil
}

func trimProjectPath(path string) string {
	projPath := replaceBackslashes(project.GetProject().Path())
	path = replaceBackslashes(path)
	if strings.HasPrefix(path, projPath+"/") {
		path = strings.TrimPrefix(path, projPath+"/")
	}
	return path
}

func trimProjectPathSlice(elements []string) []string {
	trimmed := make([]string, len(elements))
	for i, e := range elements {
		trimmed[i] = trimProjectPath(e)
	}
	return trimmed
}
