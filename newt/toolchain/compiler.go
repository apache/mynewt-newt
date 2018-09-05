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

package toolchain

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/symbol"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

const COMPILER_FILENAME string = "compiler.yml"

const (
	COMPILER_TYPE_C       = 0
	COMPILER_TYPE_ASM     = 1
	COMPILER_TYPE_CPP     = 2
	COMPILER_TYPE_ARCHIVE = 3
)

type CompilerInfo struct {
	Includes    []string
	Cflags      []string
	CXXflags    []string
	Lflags      []string
	Aflags      []string
	IgnoreFiles []*regexp.Regexp
	IgnoreDirs  []*regexp.Regexp
}

type CompileCommand struct {
	Directory string `json:"directory"`
	Command   string `json:"command"`
	File      string `json:"file"`
}

type Compiler struct {
	objPathList   map[string]bool
	LinkerScripts []string

	// Needs to be locked whenever a mutable field in this struct is accessed
	// during a build.  Currently, objPathList is the only such member.
	mutex *sync.Mutex

	depTracker            DepTracker
	ccPath                string
	cppPath               string
	asPath                string
	arPath                string
	odPath                string
	osPath                string
	ocPath                string
	ldResolveCircularDeps bool
	ldMapFile             bool
	ldBinFile             bool
	baseDir               string
	srcDir                string
	dstDir                string

	// The info to be applied during compilation.
	info CompilerInfo

	// Info read from the compiler package itself.  This is kept separate from
	// the rest of the info because it has the lowest priority; it can only be
	// added immediately before compiling beings.
	lclInfo CompilerInfo

	// Indicates whether the local compiler info has been appended to the
	// common info set.  Ensures the local info only gets added once.
	lclInfoAdded bool

	compileCommands []CompileCommand

	extraDeps []string
}

func (c *Compiler) GetCompileCommands() []CompileCommand {
	return c.compileCommands
}

func (c *Compiler) GetCcPath() string {
	return c.ccPath
}

func (c *Compiler) GetCppPath() string {
	return c.cppPath
}

func (c *Compiler) GetAsPath() string {
	return c.asPath
}

func (c *Compiler) GetArPath() string {
	return c.arPath
}

func (c *Compiler) GetLdResolveCircularDeps() bool {
	return c.ldResolveCircularDeps
}

func (c *Compiler) GetCompilerInfo() CompilerInfo {
	return c.info
}

func (c *Compiler) GetLocalCompilerInfo() CompilerInfo {
	return c.lclInfo
}

type CompilerJob struct {
	Filename     string
	Compiler     *Compiler
	CompilerType int
}

func NewCompilerInfo() *CompilerInfo {
	ci := &CompilerInfo{}
	ci.Includes = []string{}
	ci.Cflags = []string{}
	ci.CXXflags = []string{}
	ci.Lflags = []string{}
	ci.Aflags = []string{}
	ci.IgnoreFiles = []*regexp.Regexp{}
	ci.IgnoreDirs = []*regexp.Regexp{}

	return ci
}

// Extracts the base of a flag string.  A flag base is used when detecting flag
// conflicts.  If two flags have identicial bases, then they are in conflict.
func flagsBase(cflags string) string {
	// "-O" (optimization level) is one possible flag base.  By singling these
	// out, newt can prevent the original optimization flag from being
	// overwritten by subsequent ones.
	if cflags == "-O" || len(cflags) == 3 && strings.HasPrefix(cflags, "-O") {
		return "-O"
	}

	// Identify <key>=<value> pairs.  Newt prevents subsequent assignments to
	// the same key from overriding the original.
	eqIdx := strings.IndexByte(cflags, '=')
	if eqIdx == -1 {
		return cflags
	} else {
		return cflags[:eqIdx]
	}
}

// Creates a map of flag bases to flag values, i.e.,
//     [flag-base] => flag
//
// This is used to make flag conflict detection more efficient.
func flagsMap(cflags []string) map[string]string {
	hash := map[string]string{}
	for _, cf := range cflags {
		hash[flagsBase(cf)] = cf
	}

	return hash
}

// Appends a new set of flags to an original set.  If a new flag conflicts with
// an original, the new flag is discarded.  The assumption is that flags from
// higher priority packages get added first.
//
// This is not terribly efficient: it results in flag maps being generated
// repeatedly when they could be cached.  Any inefficiencies here are probably
// negligible compared to the time spent compiling and linking.  If this
// assumption turns out to be incorrect, we should cache the flag maps.
func addFlags(flagType string, orig []string, new []string) []string {
	origMap := flagsMap(orig)

	combined := orig
	for _, c := range new {
		newBase := flagsBase(c)
		origVal := origMap[newBase]
		if origVal == "" {
			// New flag; add it.
			combined = append(combined, c)
		} else {
			// Flag already present from a higher priority package; discard the
			// new one.
			if origVal != c {
				log.Debugf("Discarding %s %s in favor of %s", flagType, c,
					origVal)
			}
		}
	}

	return combined
}

func (ci *CompilerInfo) AddCflags(cflags []string) {
	ci.Cflags = addFlags("cflag", ci.Cflags, cflags)
}

func (ci *CompilerInfo) AddCompilerInfo(newCi *CompilerInfo) {
	ci.Includes = append(ci.Includes, newCi.Includes...)
	ci.Cflags = addFlags("cflag", ci.Cflags, newCi.Cflags)
	ci.CXXflags = addFlags("cxxflag", ci.CXXflags, newCi.CXXflags)
	ci.Lflags = addFlags("lflag", ci.Lflags, newCi.Lflags)
	ci.Aflags = addFlags("aflag", ci.Aflags, newCi.Aflags)
	ci.IgnoreFiles = append(ci.IgnoreFiles, newCi.IgnoreFiles...)
	ci.IgnoreDirs = append(ci.IgnoreDirs, newCi.IgnoreDirs...)
}

func NewCompiler(compilerDir string, dstDir string,
	buildProfile string) (*Compiler, error) {

	c := &Compiler{
		mutex:           &sync.Mutex{},
		objPathList:     map[string]bool{},
		baseDir:         project.GetProject().BasePath,
		srcDir:          "",
		dstDir:          dstDir,
		extraDeps:       []string{},
		compileCommands: []CompileCommand{},
	}

	c.depTracker = NewDepTracker(c)

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Loading compiler %s, buildProfile %s\n", compilerDir,
		buildProfile)
	err := c.load(compilerDir, buildProfile)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func loadFlags(yc ycfg.YCfg, settings map[string]string, key string) []string {
	flags := []string{}

	rawFlags := yc.GetValStringSlice(key, settings)
	for _, rawFlag := range rawFlags {
		if strings.HasPrefix(rawFlag, key) {
			expandedFlags := yc.GetValStringSlice(rawFlag, settings)
			flags = append(flags, expandedFlags...)
		} else {
			flags = append(flags, strings.Trim(rawFlag, "\n"))
		}
	}

	return flags
}

func (c *Compiler) load(compilerDir string, buildProfile string) error {
	yc, err := newtutil.ReadConfig(compilerDir, "compiler")
	if err != nil {
		return err
	}

	settings := map[string]string{
		buildProfile:                  "1",
		strings.ToUpper(runtime.GOOS): "1",
	}

	c.ccPath = yc.GetValString("compiler.path.cc", settings)
	c.cppPath = yc.GetValString("compiler.path.cpp", settings)
	c.asPath = yc.GetValString("compiler.path.as", settings)
	c.arPath = yc.GetValString("compiler.path.archive", settings)
	c.odPath = yc.GetValString("compiler.path.objdump", settings)
	c.osPath = yc.GetValString("compiler.path.objsize", settings)
	c.ocPath = yc.GetValString("compiler.path.objcopy", settings)

	c.lclInfo.Cflags = loadFlags(yc, settings, "compiler.flags")
	c.lclInfo.CXXflags = loadFlags(yc, settings, "compiler.cxx.flags")
	c.lclInfo.Lflags = loadFlags(yc, settings, "compiler.ld.flags")
	c.lclInfo.Aflags = loadFlags(yc, settings, "compiler.as.flags")

	c.ldResolveCircularDeps = yc.GetValBool(
		"compiler.ld.resolve_circular_deps", settings)
	c.ldMapFile = yc.GetValBool("compiler.ld.mapfile", settings)
	c.ldBinFile = yc.GetValBoolDflt("compiler.ld.binfile", settings, true)

	if len(c.lclInfo.Cflags) == 0 {
		// Assume no Cflags implies an unsupported build profile.
		return util.FmtNewtError("Compiler doesn't support build profile "+
			"specified by target on this OS (build_profile=\"%s\" OS=\"%s\")",
			buildProfile, runtime.GOOS)
	}

	return nil
}

func (c *Compiler) AddInfo(info *CompilerInfo) {
	c.info.AddCompilerInfo(info)
}

func (c *Compiler) DstDir() string {
	return c.dstDir
}

func (c *Compiler) SetSrcDir(srcDir string) {
	c.srcDir = filepath.ToSlash(filepath.Clean(srcDir))
}

func (c *Compiler) AddDeps(depFilenames ...string) {
	c.extraDeps = append(c.extraDeps, depFilenames...)
}

// Skips compilation of the specified C or assembly file, but adds the name of
// the object file that would have been generated to the compiler's list of
// object files.  This function is used when the object file is already up to
// date, so no compilation is necessary.  The name of the object file should
// still be remembered so that it gets linked in to the final library or
// executable.
func (c *Compiler) SkipSourceFile(srcFile string) error {
	objPath := c.dstFilePath(srcFile) + ".o"

	c.mutex.Lock()
	c.objPathList[filepath.ToSlash(objPath)] = true
	c.mutex.Unlock()

	// Update the dependency tracker with the object file's modification time.
	// This is necessary later for determining if the library / executable
	// needs to be rebuilt.
	err := c.depTracker.ProcessFileTime(objPath)
	if err != nil {
		return err
	}

	return nil
}

// Generates a string consisting of all the necessary include path (-I)
// options.  The result is sorted and contains no duplicate paths.
func (c *Compiler) includesStrings() []string {
	if len(c.info.Includes) == 0 {
		return nil
	}

	includes := util.SortFields(c.info.Includes...)

	tokens := make([]string, len(includes))
	for i, s := range includes {
		s = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(s)), c.baseDir+"/")
		tokens[i] = "-I" + s
	}

	return tokens
}

func (c *Compiler) cflagsStrings() []string {
	cflags := util.SortFields(c.info.Cflags...)
	return cflags
}

func (c *Compiler) cxxflagsStrings() []string {
	cxxflags := util.SortFields(c.info.CXXflags...)
	return cxxflags
}

func (c *Compiler) aflagsStrings() []string {
	aflags := util.SortFields(c.info.Aflags...)
	return aflags
}

func (c *Compiler) lflagsStrings() []string {
	lflags := util.SortFields(c.info.Lflags...)
	return lflags
}

func (c *Compiler) depsString() string {
	extraDeps := util.SortFields(c.extraDeps...)
	return strings.Join(extraDeps, " ") + "\n"
}

func (c *Compiler) dstFilePath(srcPath string) string {
	relSrcPath := strings.TrimPrefix(filepath.ToSlash(srcPath), c.baseDir+"/")
	relDstPath := strings.TrimSuffix(relSrcPath, filepath.Ext(srcPath))
	dstPath := fmt.Sprintf("%s/%s", c.dstDir, relDstPath)
	return dstPath
}

// Calculates the command-line invocation necessary to compile the specified C
// or assembly file.
//
// @param file                  The filename of the source file to compile.
// @param compilerType          One of the COMPILER_TYPE_[...] constants.
//
// @return                      (success) The command arguments.
func (c *Compiler) CompileFileCmd(file string, compilerType int) (
	[]string, error) {

	objPath := c.dstFilePath(file) + ".o"

	var cmdName string
	var flags []string
	switch compilerType {
	case COMPILER_TYPE_C:
		cmdName = c.ccPath
		flags = c.cflagsStrings()
	case COMPILER_TYPE_ASM:
		cmdName = c.asPath

		// Include both the compiler flags and the assembler flags.
		// XXX: This is not great.  We don't have a way of specifying compiler
		// flags without also passing them to the assembler.
		flags = append(c.cflagsStrings(), c.aflagsStrings()...)
	case COMPILER_TYPE_CPP:
		cmdName = c.cppPath
		flags = append(c.cflagsStrings(), c.cxxflagsStrings()...)
	default:
		return nil, util.NewNewtError("Unknown compiler type")
	}

	srcPath := strings.TrimPrefix(file, c.baseDir+"/")
	cmd := []string{cmdName}
	cmd = append(cmd, flags...)
	cmd = append(cmd, c.includesStrings()...)
	cmd = append(cmd, []string{
		"-c",
		"-o",
		objPath,
		srcPath,
	}...)

	return cmd, nil
}

// Generates a dependency Makefile (.d) for the specified source C file.
//
// @param file                  The name of the source file.
func (c *Compiler) GenDepsForFile(file string) error {
	depPath := c.dstFilePath(file) + ".d"
	depDir := filepath.Dir(depPath)
	if util.NodeNotExist(depDir) {
		os.MkdirAll(depDir, 0755)
	}

	srcPath := strings.TrimPrefix(file, c.baseDir+"/")
	cmd := []string{c.ccPath}
	cmd = append(cmd, c.cflagsStrings()...)
	cmd = append(cmd, c.includesStrings()...)
	cmd = append(cmd, []string{"-MM", "-MG", srcPath}...)

	o, err := util.ShellCommandLimitDbgOutput(cmd, nil, true, 0)
	if err != nil {
		return err
	}

	// Write the compiler output to a dependency file.
	f, err := os.OpenFile(depPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return util.ChildNewtError(err)
	}
	defer f.Close()

	if _, err := f.Write(o); err != nil {
		return util.ChildNewtError(err)
	}

	// Append the extra dependencies (.yml files) to the .d file.
	objFile := strings.TrimSuffix(file, filepath.Ext(file)) + ".o"
	if _, err := f.WriteString(objFile + ": " + c.depsString()); err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}

func serializeCommand(cmd []string) []byte {
	// Use a newline as the separator rather than a space to disambiguate cases
	// where arguments contain spaces.
	return []byte(strings.Join(cmd, "\n"))
}

// Writes a file containing the command-line invocation used to generate the
// specified file.  The file that this function writes can be used later to
// determine if the set of compiler options has changed.
//
// @param dstFile               The output file whose build invocation is being
//                                  recorded.
// @param cmd                   The command strings to write.
func writeCommandFile(dstFile string, cmd []string) error {
	cmdPath := dstFile + ".cmd"
	content := serializeCommand(cmd)
	err := ioutil.WriteFile(cmdPath, content, 0644)
	if err != nil {
		return err
	}

	return nil
}

// Adds the info from the compiler package to the common set if it hasn't
// already been added.  The compiler package's info needs to be added last
// because the compiler is the lowest priority package.
func (c *Compiler) ensureLclInfoAdded() {
	if !c.lclInfoAdded {
		log.Debugf("Generating build flags for compiler")
		c.AddInfo(&c.lclInfo)
		c.lclInfoAdded = true
	}
}

// Compile the specified C or assembly file.
//
// @param file                  The filename of the source file to compile.
// @param compilerType          One of the COMPILER_TYPE_[...] constants.
func (c *Compiler) CompileFile(file string, compilerType int) error {
	objPath := c.dstFilePath(file) + ".o"
	objDir := filepath.Dir(objPath)
	if util.NodeNotExist(objDir) {
		os.MkdirAll(objDir, 0755)
	}

	c.mutex.Lock()
	c.objPathList[filepath.ToSlash(objPath)] = true
	c.mutex.Unlock()

	cmd, err := c.CompileFileCmd(file, compilerType)
	if err != nil {
		return err
	}

	srcPath := strings.TrimPrefix(file, c.baseDir+"/")
	switch compilerType {
	case COMPILER_TYPE_C:
		util.StatusMessage(util.VERBOSITY_DEFAULT, "Compiling %s\n", srcPath)
	case COMPILER_TYPE_CPP:
		util.StatusMessage(util.VERBOSITY_DEFAULT, "Compiling %s\n", srcPath)
	case COMPILER_TYPE_ASM:
		util.StatusMessage(util.VERBOSITY_DEFAULT, "Assembling %s\n", srcPath)
	default:
		return util.NewNewtError("Unknown compiler type")
	}

	_, err = util.ShellCommand(cmd, nil)
	if err != nil {
		return err
	}

	c.compileCommands = append(c.compileCommands,
		CompileCommand{
			Command: strings.Join(cmd, " "),
			File:    file,
		})

	err = writeCommandFile(objPath, cmd)
	if err != nil {
		return err
	}

	// Tell the dependency tracker that an object file was just rebuilt.
	c.depTracker.MostRecent = time.Now()

	return nil
}

func (c *Compiler) ShouldIgnoreFile(file string) bool {
	file = strings.TrimPrefix(file, c.srcDir)
	file = strings.TrimLeft(file, "/\\")
	for _, re := range c.info.IgnoreFiles {
		if match := re.MatchString(file); match {
			return true
		}
	}

	return false
}

func compilerTypeToExts(compilerType int) ([]string, error) {
	switch compilerType {
	case COMPILER_TYPE_C:
		return []string{"c"}, nil
	case COMPILER_TYPE_ASM:
		return []string{"s", "S"}, nil
	case COMPILER_TYPE_CPP:
		return []string{"cc", "cpp", "cxx"}, nil
	case COMPILER_TYPE_ARCHIVE:
		return []string{"a"}, nil
	default:
		return nil, util.NewNewtError("Wrong compiler type specified to " +
			"compilerTypeToExts")
	}
}

// Compiles all C files matching the specified file glob.
func (c *Compiler) CompileC(filename string) error {
	filename = filepath.ToSlash(filename)

	if c.ShouldIgnoreFile(filename) {
		log.Infof("Ignoring %s because package dictates it.", filename)
		return nil
	}

	compileRequired, err := c.depTracker.CompileRequired(filename,
		COMPILER_TYPE_C)
	if err != nil {
		return err
	}
	if compileRequired {
		err = c.CompileFile(filename, COMPILER_TYPE_C)
	} else {
		err = c.SkipSourceFile(filename)
	}
	if err != nil {
		return err
	}

	return nil
}

// Compiles all CPP files
func (c *Compiler) CompileCpp(filename string) error {
	filename = filepath.ToSlash(filename)

	if c.ShouldIgnoreFile(filename) {
		log.Infof("Ignoring %s because package dictates it.", filename)
		return nil
	}

	compileRequired, err := c.depTracker.CompileRequired(filename,
		COMPILER_TYPE_CPP)
	if err != nil {
		return err
	}

	if compileRequired {
		err = c.CompileFile(filename, COMPILER_TYPE_CPP)
	} else {
		err = c.SkipSourceFile(filename)
	}

	if err != nil {
		return err
	}

	return nil
}

// Compiles all assembly files matching the specified file glob.
//
// @param match                 The file glob specifying which assembly files
//                                  to compile.
func (c *Compiler) CompileAs(filename string) error {
	filename = filepath.ToSlash(filename)

	if c.ShouldIgnoreFile(filename) {
		log.Infof("Ignoring %s because package dictates it.", filename)
		return nil
	}

	compileRequired, err := c.depTracker.CompileRequired(filename,
		COMPILER_TYPE_ASM)
	if err != nil {
		return err
	}
	if compileRequired {
		err = c.CompileFile(filename, COMPILER_TYPE_ASM)
	} else {
		err = c.SkipSourceFile(filename)
	}
	if err != nil {
		return err
	}

	return nil
}

// Copies all archive files matching the specified file glob.
//
// @param match                 The file glob specifying which assembly files
//                                  to compile.
func (c *Compiler) CopyArchive(filename string) error {
	filename = filepath.ToSlash(filename)

	if c.ShouldIgnoreFile(filename) {
		log.Infof("Ignoring %s because package dictates it.", filename)
		return nil
	}

	tgtFile := c.dstDir + "/" + filepath.Base(filename)
	copyRequired, err := c.depTracker.CopyRequired(filename)
	if err != nil {
		return err
	}
	if copyRequired {
		err = util.CopyFile(filename, tgtFile)
		util.StatusMessage(util.VERBOSITY_DEFAULT, "Copying %s\n",
			filepath.ToSlash(tgtFile))
	}

	if err != nil {
		return err
	}

	return nil
}

func (c *Compiler) processEntry(node os.FileInfo, cType int,
	ignDirs []string) ([]CompilerJob, error) {

	// check to see if we ignore this element
	for _, dir := range ignDirs {
		if dir == node.Name() {
			return nil, nil
		}
	}

	// Check in the user specified ignore directories
	for _, dir := range c.info.IgnoreDirs {
		if dir.MatchString(node.Name()) {
			return nil, nil
		}
	}

	// If not, recurse into the directory.  Make the output directory
	// structure mirror that of the source tree.
	prevSrcDir := c.srcDir
	prevDstDir := c.dstDir

	c.srcDir += "/" + node.Name()
	c.dstDir += "/" + node.Name()

	entries, err := c.RecursiveCollectEntries(cType, ignDirs)

	// Restore the compiler destination directory now that the child
	// directory has been fully built.
	c.srcDir = prevSrcDir
	c.dstDir = prevDstDir

	return entries, err
}

func (c *Compiler) RecursiveCollectEntries(cType int,
	ignDirs []string) ([]CompilerJob, error) {

	// Make sure the compiler package info is added to the global set.
	c.ensureLclInfoAdded()

	if err := os.Chdir(c.baseDir); err != nil {
		return nil, util.ChildNewtError(err)
	}

	// Get a list of files in the current directory, and if they are a
	// directory, and that directory is not in the ignDirs variable, then
	// recurse into that directory and compile the files in there

	ls, err := ioutil.ReadDir(c.srcDir)
	if err != nil {
		return nil, util.NewNewtError(err.Error())
	}

	entries := []CompilerJob{}
	for _, node := range ls {
		if node.IsDir() {
			subEntries, err := c.processEntry(node, cType, ignDirs)
			if err != nil {
				return nil, err
			}

			entries = append(entries, subEntries...)
		}
	}

	exts, err := compilerTypeToExts(cType)
	if err != nil {
		return nil, err
	}

	for _, ext := range exts {
		files, _ := filepath.Glob(c.srcDir + "/*." + ext)
		for _, file := range files {
			file = filepath.ToSlash(file)
			entries = append(entries, CompilerJob{
				Filename:     file,
				Compiler:     c,
				CompilerType: cType,
			})
		}
	}

	return entries, nil
}

func RunJob(record CompilerJob) error {
	switch record.CompilerType {
	case COMPILER_TYPE_C:
		return record.Compiler.CompileC(record.Filename)
	case COMPILER_TYPE_ASM:
		return record.Compiler.CompileAs(record.Filename)
	case COMPILER_TYPE_CPP:
		return record.Compiler.CompileCpp(record.Filename)
	case COMPILER_TYPE_ARCHIVE:
		return record.Compiler.CopyArchive(record.Filename)
	default:
		return util.NewNewtError("Wrong compiler type specified to " +
			"RunJob")
	}
}

func (c *Compiler) getObjFiles(baseObjFiles []string) []string {
	c.mutex.Lock()
	for objName, _ := range c.objPathList {
		baseObjFiles = append(baseObjFiles, objName)
	}
	c.mutex.Unlock()

	sort.Strings(baseObjFiles)
	return baseObjFiles
}

// Calculates the command-line invocation necessary to link the specified elf
// file.
//
// @param dstFile               The filename of the destination elf file to
//                                  link.
// @param options               Some build options specifying how the elf file
//                                  gets generated.
// @param objFiles              An array of the source .o and .a filenames.
//
// @return                      (success) The command tokens.
func (c *Compiler) CompileBinaryCmd(dstFile string, options map[string]bool,
	objFiles []string, keepSymbols []string, elfLib string) []string {

	objList := c.getObjFiles(util.UniqueStrings(objFiles))

	cmd := []string{
		c.ccPath,
		"-o",
		dstFile,
	}
	cmd = append(cmd, c.cflagsStrings()...)

	if elfLib != "" {
		cmd = append(cmd, "-Wl,--just-symbols="+elfLib)
	}

	if c.ldResolveCircularDeps {
		cmd = append(cmd, "-Wl,--start-group")
		cmd = append(cmd, objList...)
		cmd = append(cmd, "-Wl,--end-group")
	} else {
		cmd = append(cmd, objList...)
	}

	if keepSymbols != nil {
		for _, name := range keepSymbols {
			cmd = append(cmd, "-Wl,--undefined="+name)
		}
	}

	cmd = append(cmd, c.lflagsStrings()...)

	/* so we don't get multiple global definitions of the same vartiable */
	//cmd += " -Wl,--warn-common "

	for _, ls := range c.LinkerScripts {
		cmd = append(cmd, "-T")
		cmd = append(cmd, ls)
	}
	if options["mapFile"] {
		cmd = append(cmd, "-Wl,-Map="+dstFile+".map")
	}

	return cmd
}

// Links the specified elf file.
//
// @param dstFile               The filename of the destination elf file to
//                                  link.
// @param options               Some build options specifying how the elf file
//                                  gets generated.
// @param objFiles              An array of the source .o and .a filenames.
func (c *Compiler) CompileBinary(dstFile string, options map[string]bool,
	objFiles []string, keepSymbols []string, elfLib string) error {

	// Make sure the compiler package info is added to the global set.
	c.ensureLclInfoAdded()

	objList := c.getObjFiles(util.UniqueStrings(objFiles))

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Linking %s\n", dstFile)
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Linking %s with input files %s\n",
		dstFile, objList)

	if elfLib != "" {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Linking %s with rom image %s\n",
			dstFile, elfLib)
	}

	cmd := c.CompileBinaryCmd(dstFile, options, objFiles, keepSymbols, elfLib)
	_, err := util.ShellCommand(cmd, nil)
	if err != nil {
		return err
	}

	err = writeCommandFile(dstFile, cmd)
	if err != nil {
		return err
	}

	return nil
}

// Generates the following build artifacts:
//    * lst file
//    * map file
//    * bin file
//
// @param elfFilename           The filename of the elf file corresponding to
//                                  the artifacts to be generated.
// @param options               Some build options specifying which artifacts
//                                  get generated.
func (c *Compiler) generateExtras(elfFilename string,
	options map[string]bool) error {

	if options["binFile"] {
		binFile := elfFilename + ".bin"
		cmd := []string{
			c.ocPath,
			"-R",
			".bss",
			"-R",
			".bss.core",
			"-R",
			".bss.core.nz",
			"-O",
			"binary",
			elfFilename,
			binFile,
		}
		_, err := util.ShellCommand(cmd, nil)
		if err != nil {
			return err
		}
	}

	if options["listFile"] {
		listFile := elfFilename + ".lst"
		f, err := os.OpenFile(listFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
			0666)
		if err != nil {
			return util.NewNewtError(err.Error())
		}
		defer f.Close()

		cmd := []string{
			c.odPath,
			"-wxdS",
			elfFilename,
		}
		o, err := util.ShellCommandLimitDbgOutput(cmd, nil, true, 0)
		if err != nil {
			// XXX: gobjdump appears to always crash.  Until we get that sorted
			// out, don't fail the link process if lst generation fails.
			return nil
		}

		if _, err := f.Write(o); err != nil {
			return util.ChildNewtError(err)
		}

		sects := []string{".text", ".rodata", ".data"}
		for _, sect := range sects {
			cmd := []string{
				c.odPath,
				"-s",
				"-j",
				sect,
				elfFilename,
			}
			o, err := util.ShellCommandLimitDbgOutput(cmd, nil, true, 0)
			if err != nil {
				if _, err := f.Write(o); err != nil {
					return util.NewNewtError(err.Error())
				}
			}
		}

		cmd = []string{
			c.osPath,
			elfFilename,
		}
		o, err = util.ShellCommandLimitDbgOutput(cmd, nil, true, 0)
		if err != nil {
			return err
		}
		if _, err := f.Write(o); err != nil {
			return util.NewNewtError(err.Error())
		}
	}

	return nil
}

func (c *Compiler) PrintSize(elfFilename string) (string, error) {
	cmd := []string{
		c.osPath,
		elfFilename,
	}
	o, err := util.ShellCommand(cmd, nil)
	if err != nil {
		return "", err
	}
	return string(o), nil
}

// Links the specified elf file and generates some associated artifacts (lst,
// bin, and map files).
//
// @param binFile               The filename of the destination elf file to
//                                  link.
// @param options               Some build options specifying how the elf file
//                                  gets generated.
// @param objFiles              An array of the source .o and .a filenames.
func (c *Compiler) CompileElf(binFile string, objFiles []string,
	keepSymbols []string, elfLib string) error {
	options := map[string]bool{"mapFile": c.ldMapFile,
		"listFile": true, "binFile": c.ldBinFile}

	// Make sure the compiler package info is added to the global set.
	c.ensureLclInfoAdded()

	linkRequired, err := c.depTracker.LinkRequired(binFile, options,
		objFiles, keepSymbols, elfLib)
	if err != nil {
		return err
	}
	if linkRequired {
		if err := os.MkdirAll(filepath.Dir(binFile), 0755); err != nil {
			return util.NewNewtError(err.Error())
		}
		err := c.CompileBinary(binFile, options, objFiles, keepSymbols, elfLib)
		if err != nil {
			return err
		}
	}

	err = c.generateExtras(binFile, options)
	if err != nil {
		return err
	}

	return nil
}

func (c *Compiler) RenameSymbolsCmd(
	sm *symbol.SymbolMap, libraryFile string, ext string) []string {

	cmd := []string{c.ocPath}
	for s, _ := range *sm {
		cmd = append(cmd, "--redefine-sym")
		cmd = append(cmd, s+"="+s+ext)
	}

	cmd = append(cmd, libraryFile)
	return cmd
}

func (c *Compiler) ParseLibraryCmd(libraryFile string) []string {
	return []string{
		c.odPath,
		"-t",
		libraryFile,
	}
}

func (c *Compiler) CopySymbolsCmd(infile string, outfile string, sm *symbol.SymbolMap) []string {

	cmd := []string{c.ocPath, "-S"}
	for symbol, _ := range *sm {
		cmd = append(cmd, "-K")
		cmd = append(cmd, symbol)
	}

	cmd = append(cmd, infile)
	cmd = append(cmd, outfile)

	return cmd
}

// Calculates the command-line invocation necessary to archive the specified
// static library.
//
// @param archiveFile           The filename of the library to archive.
// @param objFiles              An array of the source .o filenames.
//
// @return                      The command string.
func (c *Compiler) CompileArchiveCmd(archiveFile string,
	objFiles []string) []string {

	cmd := []string{
		c.arPath,
		"rcs",
		archiveFile,
	}
	cmd = append(cmd, c.getObjFiles(objFiles)...)
	return cmd
}

func linkerScriptFileName(archiveFile string) string {
	ar_script_name := strings.TrimSuffix(archiveFile, filepath.Ext(archiveFile)) + "_ar.mri"
	return ar_script_name
}

/* this create a new library combining all of the other libraries */
func createSplitArchiveLinkerFile(archiveFile string,
	archFiles []string) error {

	/* create a name for this script */
	ar_script_name := linkerScriptFileName(archiveFile)

	// open the file and write out the script
	f, err := os.OpenFile(ar_script_name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0666)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer f.Close()

	if _, err := f.WriteString("CREATE " + archiveFile + "\n"); err != nil {
		return util.NewNewtError(err.Error())
	}

	for _, arch := range archFiles {
		if _, err := f.WriteString("ADDLIB " + arch + "\n"); err != nil {
			return util.NewNewtError(err.Error())
		}
	}

	if _, err := f.WriteString("SAVE\n"); err != nil {
		return util.NewNewtError(err.Error())
	}

	if _, err := f.WriteString("END\n"); err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}

// calculates the command-line invocation necessary to build a split all
// archive from the collection of archive files
func (c *Compiler) BuildSplitArchiveCmd(archiveFile string) string {

	str := c.arPath + " -M < " + linkerScriptFileName(archiveFile)
	return str
}

// Archives the specified static library.
//
// @param archiveFile           The filename of the library to archive.
// @param objFiles              An array of the source .o filenames.
func (c *Compiler) CompileArchive(archiveFile string) error {
	objFiles := []string{}

	// Make sure the compiler package info is added to the global set.
	c.ensureLclInfoAdded()

	arRequired, err := c.depTracker.ArchiveRequired(archiveFile, objFiles)
	if err != nil {
		return err
	}
	if !arRequired {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(archiveFile), 0755); err != nil {
		return util.NewNewtError(err.Error())
	}

	objList := c.getObjFiles([]string{})
	if len(objList) == 0 {
		return nil
	}

	if len(objList) == 0 {
		util.StatusMessage(util.VERBOSITY_VERBOSE,
			"Not archiving %s; no object files\n", archiveFile)
		return nil
	}

	// Delete the old archive, if it exists.
	os.Remove(archiveFile)

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Archiving %s",
		path.Base(archiveFile))
	util.StatusMessage(util.VERBOSITY_VERBOSE, " with object files %s",
		strings.Join(objList, " "))
	util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")

	if err != nil && !os.IsNotExist(err) {
		return util.NewNewtError(err.Error())
	}

	cmd := c.CompileArchiveCmd(archiveFile, objFiles)
	_, err = util.ShellCommand(cmd, nil)
	if err != nil {
		return err
	}

	err = writeCommandFile(archiveFile, cmd)
	if err != nil {
		return err
	}

	return nil
}

func getParseRexeg() (error, *regexp.Regexp) {
	r, err := regexp.Compile("^([0-9A-Fa-f]+)[\t ]+([lgu! ][w ][C ][W ][Ii ][Dd ][FfO ])[\t ]+([^\t\n\f\r ]+)[\t ]+([0-9a-fA-F]+)[\t ]([^\t\n\f\r ]+)")

	if err != nil {
		return err, nil
	}

	return nil, r
}

/* This is a tricky thing to parse. Right now, I keep all the
 * flags together and just store the offset, size, name and flags.
* 00012970 l       .bss	00000000 _end
* 00011c60 l       .init_array	00000000 __init_array_start
* 00011c60 l       .init_array	00000000 __preinit_array_start
* 000084b0 g     F .text	00000034 os_arch_start
* 00000000 g       .debug_aranges	00000000 __HeapBase
* 00011c88 g     O .data	00000008 g_os_task_list
* 000082cc g     F .text	0000004c os_idle_task
* 000094e0 g     F .text	0000002e .hidden __gnu_uldivmod_helper
* 00000000 g       .svc_table	00000000 SVC_Count
* 000125e4 g     O .bss	00000004 g_console_is_init
* 00009514 g     F .text	0000029c .hidden __divdi3
* 000085a8 g     F .text	00000054 os_eventq_put
*/
func ParseObjectLine(line string, r *regexp.Regexp) (error, *symbol.SymbolInfo) {

	answer := r.FindAllStringSubmatch(line, 11)

	if len(answer) == 0 {
		return nil, nil
	}

	data := answer[0]

	if len(data) != 6 {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Not enough content in object file line --- %s", line)
		return nil, nil
	}

	si := symbol.NewSymbolInfo()

	si.Name = data[5]

	v, err := strconv.ParseUint(data[1], 16, 32)

	if err != nil {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Could not convert location from object file line --- %s", line)
		return nil, nil
	}

	si.Loc = int(v)

	v, err = strconv.ParseUint(data[4], 16, 32)

	if err != nil {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Could not convert size form object file line --- %s", line)
		return nil, nil
	}

	si.Size = int(v)
	si.Code = data[2]
	si.Section = data[3]

	return nil, si
}

func (c *Compiler) RenameSymbols(sm *symbol.SymbolMap, libraryFile string, ext string) error {

	cmd := c.RenameSymbolsCmd(sm, libraryFile, ext)

	_, err := util.ShellCommand(cmd, nil)

	return err
}

func (c *Compiler) ParseLibrary(libraryFile string) (error, []byte) {
	cmd := c.ParseLibraryCmd(libraryFile)

	out, err := util.ShellCommand(cmd, nil)
	if err != nil {
		return err, nil
	}
	return err, out
}

func (c *Compiler) CopySymbols(infile string, outfile string, sm *symbol.SymbolMap) error {
	cmd := c.CopySymbolsCmd(infile, outfile, sm)

	_, err := util.ShellCommand(cmd, nil)
	if err != nil {
		return err
	}
	return err
}

func (c *Compiler) ConvertBinToHex(inFile string, outFile string, baseAddr int) error {
	cmd := []string{
		c.ocPath,
		"-I",
		"binary",
		"-O",
		"ihex",
		"--adjust-vma",
		"0x" + strconv.FormatInt(int64(baseAddr), 16),
		inFile,
		outFile,
	}
	_, err := util.ShellCommand(cmd, nil)
	if err != nil {
		return err
	}
	return nil
}
