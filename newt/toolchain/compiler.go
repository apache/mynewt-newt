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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/symbol"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
)

const COMPILER_FILENAME string = "compiler.yml"

const (
	COMPILER_TYPE_C   = 0
	COMPILER_TYPE_ASM = 1
)

type CompilerInfo struct {
	Includes    []string
	Cflags      []string
	Lflags      []string
	Aflags      []string
	IgnoreFiles []*regexp.Regexp
	IgnoreDirs  []*regexp.Regexp
}

type Compiler struct {
	ObjPathList  map[string]bool
	LinkerScript string

	depTracker            DepTracker
	ccPath                string
	asPath                string
	arPath                string
	odPath                string
	osPath                string
	ocPath                string
	ldResolveCircularDeps bool
	ldMapFile             bool
	ldBinFile             bool
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

	extraDeps []string
}

func NewCompilerInfo() *CompilerInfo {
	ci := &CompilerInfo{}
	ci.Includes = []string{}
	ci.Cflags = []string{}
	ci.Lflags = []string{}
	ci.Aflags = []string{}
	ci.IgnoreFiles = []*regexp.Regexp{}
	ci.IgnoreDirs = []*regexp.Regexp{}

	return ci
}

// Extracts the base of a flag string.  A flag base is used when detecting flag
// conflicts.  If two flags have identicial bases, then they are in conflict.
func flagsBase(cflags string) string {
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

func (ci *CompilerInfo) AddCompilerInfo(newCi *CompilerInfo) {
	ci.Includes = append(ci.Includes, newCi.Includes...)
	ci.Cflags = addFlags("cflag", ci.Cflags, newCi.Cflags)
	ci.Lflags = addFlags("lflag", ci.Lflags, newCi.Lflags)
	ci.Aflags = addFlags("aflag", ci.Aflags, newCi.Aflags)
	ci.IgnoreFiles = append(ci.IgnoreFiles, newCi.IgnoreFiles...)
	ci.IgnoreDirs = append(ci.IgnoreDirs, newCi.IgnoreDirs...)
}

func NewCompiler(compilerDir string, dstDir string,
	buildProfile string) (*Compiler, error) {

	c := &Compiler{
		ObjPathList: map[string]bool{},
		dstDir:      filepath.Clean(dstDir),
		extraDeps:   []string{compilerDir + COMPILER_FILENAME},
	}

	c.depTracker = NewDepTracker(c)

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Loading compiler %s, def %s\n", compilerDir, buildProfile)
	err := c.load(compilerDir, buildProfile)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func loadFlags(v *viper.Viper, features map[string]bool,
	key string) []string {

	flags := []string{}

	rawFlags := newtutil.GetStringSliceFeatures(v, features, key)
	for _, rawFlag := range rawFlags {
		if strings.HasPrefix(rawFlag, key) {
			expandedFlags := newtutil.GetStringSliceFeatures(v, features,
				rawFlag)

			flags = append(flags, expandedFlags...)
		} else {
			flags = append(flags, strings.Trim(rawFlag, "\n"))
		}
	}

	return flags
}

func (c *Compiler) load(compilerDir string, buildProfile string) error {
	v, err := util.ReadConfig(compilerDir, "compiler")
	if err != nil {
		return err
	}

	features := map[string]bool{
		buildProfile:                  true,
		strings.ToUpper(runtime.GOOS): true,
	}

	c.ccPath = newtutil.GetStringFeatures(v, features, "compiler.path.cc")
	c.asPath = newtutil.GetStringFeatures(v, features, "compiler.path.as")
	c.arPath = newtutil.GetStringFeatures(v, features, "compiler.path.archive")
	c.odPath = newtutil.GetStringFeatures(v, features, "compiler.path.objdump")
	c.osPath = newtutil.GetStringFeatures(v, features, "compiler.path.objsize")
	c.ocPath = newtutil.GetStringFeatures(v, features, "compiler.path.objcopy")

	c.lclInfo.Cflags = loadFlags(v, features, "compiler.flags")
	c.lclInfo.Lflags = loadFlags(v, features, "compiler.ld.flags")
	c.lclInfo.Aflags = loadFlags(v, features, "compiler.as.flags")

	c.ldResolveCircularDeps, err = newtutil.GetBoolFeatures(v, features,
		"compiler.ld.resolve_circular_deps")
	if err != nil {
		return err
	}

	c.ldMapFile, err = newtutil.GetBoolFeatures(v, features,
		"compiler.ld.mapfile")
	if err != nil {
		return err
	}

	c.ldBinFile, err = newtutil.GetBoolFeaturesDflt(v, features,
		"compiler.ld.binfile", true)
	if err != nil {
		return err
	}

	if len(c.lclInfo.Cflags) == 0 {
		// Assume no Cflags implies an unsupported build profile.
		return util.FmtNewtError("Compiler doesn't support build profile "+
			"specified by target on this OS (build_profile=\"%s\" OS=\"%s\")",
			buildProfile, runtime.GOOS)
	}

	log.Infof("ccPath = %s, arPath = %s, flags = %s", c.ccPath,
		c.arPath, c.info.Cflags)

	return nil
}

func (c *Compiler) AddInfo(info *CompilerInfo) {
	c.info.AddCompilerInfo(info)
}

func (c *Compiler) DstDir() string {
	return c.dstDir
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
	objFile := c.dstDir + "/" +
		strings.TrimSuffix(srcFile, filepath.Ext(srcFile)) + ".o"
	c.ObjPathList[filepath.ToSlash(objFile)] = true

	// Update the dependency tracker with the object file's modification time.
	// This is necessary later for determining if the library / executable
	// needs to be rebuilt.
	err := c.depTracker.ProcessFileTime(objFile)
	if err != nil {
		return err
	}

	return nil
}

// Generates a string consisting of all the necessary include path (-I)
// options.  The result is sorted and contains no duplicate paths.
func (c *Compiler) includesString() string {
	if len(c.info.Includes) == 0 {
		return ""
	}

	includes := util.SortFields(c.info.Includes...)
	return "-I" + strings.Join(includes, " -I")
}

func (c *Compiler) cflagsString() string {
	cflags := util.SortFields(c.info.Cflags...)
	return strings.Join(cflags, " ")
}

func (c *Compiler) lflagsString() string {
	lflags := util.SortFields(c.info.Lflags...)
	return strings.Join(lflags, " ")
}

func (c *Compiler) depsString() string {
	extraDeps := util.SortFields(c.extraDeps...)
	return strings.Join(extraDeps, " ") + "\n"
}

// Calculates the command-line invocation necessary to compile the specified C
// or assembly file.
//
// @param file                  The filename of the source file to compile.
// @param compilerType          One of the COMPILER_TYPE_[...] constants.
//
// @return                      (success) The command string.
func (c *Compiler) CompileFileCmd(file string,
	compilerType int) (string, error) {

	objFile := strings.TrimSuffix(file, filepath.Ext(file)) + ".o"
	objPath := filepath.ToSlash(c.dstDir + "/" + objFile)

	var cmd string

	switch compilerType {
	case COMPILER_TYPE_C:
		cmd = c.ccPath
	case COMPILER_TYPE_ASM:
		cmd = c.asPath
	default:
		return "", util.NewNewtError("Unknown compiler type")
	}

	cmd += " -c " + "-o " + objPath + " " + file +
		" " + c.cflagsString() + " " + c.includesString()

	return cmd, nil
}

// Generates a dependency Makefile (.d) for the specified source C file.
//
// @param file                  The name of the source file.
func (c *Compiler) GenDepsForFile(file string) error {
	if util.NodeNotExist(c.dstDir) {
		os.MkdirAll(c.dstDir, 0755)
	}

	depFile := c.dstDir + "/" +
		strings.TrimSuffix(file, filepath.Ext(file)) + ".d"
	depFile = filepath.ToSlash(depFile)

	var cmd string
	var err error

	cmd = c.ccPath + " " + c.cflagsString() + " " + c.includesString() +
		" -MM -MG " + file + " > " + depFile
	o, err := util.ShellCommand(cmd)
	if err != nil {
		return util.NewNewtError(string(o))
	}

	// Append the extra dependencies (.yml files) to the .d file.
	f, err := os.OpenFile(depFile, os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer f.Close()

	objFile := strings.TrimSuffix(file, filepath.Ext(file)) + ".o"
	if _, err := f.WriteString(objFile + ": " + c.depsString()); err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}

// Writes a file containing the command-line invocation used to generate the
// specified file.  The file that this function writes can be used later to
// determine if the set of compiler options has changed.
//
// @param dstFile               The output file whose build invocation is being
//                                  recorded.
// @param cmd                   The command to write.
func writeCommandFile(dstFile string, cmd string) error {
	cmdPath := dstFile + ".cmd"
	err := ioutil.WriteFile(cmdPath, []byte(cmd), 0644)
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
	if util.NodeNotExist(c.dstDir) {
		os.MkdirAll(c.dstDir, 0755)
	}

	objFile := strings.TrimSuffix(file, filepath.Ext(file)) + ".o"

	objPath := c.dstDir + "/" + objFile
	c.ObjPathList[filepath.ToSlash(objPath)] = true

	cmd, err := c.CompileFileCmd(file, compilerType)
	if err != nil {
		return err
	}

	switch compilerType {
	case COMPILER_TYPE_C:
		util.StatusMessage(util.VERBOSITY_DEFAULT, "Compiling %s\n", file)
	case COMPILER_TYPE_ASM:
		util.StatusMessage(util.VERBOSITY_DEFAULT, "Assembling %s\n", file)
	default:
		return util.NewNewtError("Unknown compiler type")
	}

	_, err = util.ShellCommand(cmd)
	if err != nil {
		return err
	}

	err = writeCommandFile(objPath, cmd)
	if err != nil {
		return err
	}

	// Tell the dependency tracker that an object file was just rebuilt.
	c.depTracker.MostRecent = time.Now()

	return nil
}

func (c *Compiler) shouldIgnoreFile(file string) bool {
	for _, re := range c.info.IgnoreFiles {
		if match := re.MatchString(file); match {
			return true
		}
	}

	return false
}

// Compiles all C files matching the specified file glob.
//
// @param match                 The file glob specifying which C files to
//                                  compile.
func (c *Compiler) CompileC() error {
	files, _ := filepath.Glob("*.c")

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	log.Infof("Compiling C if outdated (%s/*.c) %s", wd,
		strings.Join(files, " "))

	for _, file := range files {
		file = filepath.ToSlash(file)

		if shouldIgnore := c.shouldIgnoreFile(file); shouldIgnore {
			log.Infof("Ignoring %s because package dictates it.", file)
			continue
		}

		compileRequired, err := c.depTracker.CompileRequired(file,
			COMPILER_TYPE_C)
		if err != nil {
			return err
		}
		if compileRequired {
			err = c.CompileFile(file, COMPILER_TYPE_C)
		} else {
			err = c.SkipSourceFile(file)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// Compiles all assembly files matching the specified file glob.
//
// @param match                 The file glob specifying which assembly files
//                                  to compile.
func (c *Compiler) CompileAs() error {
	files, _ := filepath.Glob("*.s")
	Sfiles, _ := filepath.Glob("*.S")
	files = append(files, Sfiles...)

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	log.Infof("Compiling assembly if outdated (%s/*.s) %s", wd,
		strings.Join(files, " "))
	for _, file := range files {
		if shouldIgnore := c.shouldIgnoreFile(file); shouldIgnore {
			log.Infof("Ignoring %s because package dictates it.", file)
			continue
		}

		compileRequired, err := c.depTracker.CompileRequired(file,
			COMPILER_TYPE_ASM)
		if err != nil {
			return err
		}
		if compileRequired {
			err = c.CompileFile(file, COMPILER_TYPE_ASM)
		} else {
			err = c.SkipSourceFile(file)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Compiler) processEntry(wd string, node os.FileInfo, cType int,
	ignDirs []string) error {

	// check to see if we ignore this element
	for _, entry := range ignDirs {
		if entry == node.Name() {
			return nil
		}
	}

	// Check in the user specified ignore directories
	for _, entry := range c.info.IgnoreDirs {
		if entry.MatchString(node.Name()) {
			return nil
		}
	}

	// if not, recurse into the directory
	os.Chdir(wd + "/" + node.Name())
	return c.RecursiveCompile(cType, ignDirs)
}

func (c *Compiler) RecursiveCompile(cType int, ignDirs []string) error {
	// Make sure the compiler package info is added to the global set.
	c.ensureLclInfoAdded()

	// Get a list of files in the current directory, and if they are a
	// directory, and that directory is not in the ignDirs variable, then
	// recurse into that directory and compile the files in there

	wd, err := os.Getwd()
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	wd = filepath.ToSlash(wd)

	dirList, err := ioutil.ReadDir(wd)
	if err != nil {
		return util.NewNewtError(err.Error())
	}

	for _, node := range dirList {
		if node.IsDir() {
			err = c.processEntry(wd, node, cType, ignDirs)
			if err != nil {
				return err
			}
		}
	}

	err = os.Chdir(wd)
	if err != nil {
		return err
	}

	switch cType {
	case COMPILER_TYPE_C:
		return c.CompileC()
	case COMPILER_TYPE_ASM:
		return c.CompileAs()
	default:
		return util.NewNewtError("Wrong compiler type specified to " +
			"RecursiveCompile")
	}
}

func (c *Compiler) getObjFiles(baseObjFiles []string) string {
	for objName, _ := range c.ObjPathList {
		baseObjFiles = append(baseObjFiles, objName)
	}

	sort.Strings(baseObjFiles)
	objList := strings.Join(baseObjFiles, " ")
	return objList
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
// @return                      (success) The command string.
func (c *Compiler) CompileBinaryCmd(dstFile string, options map[string]bool,
	objFiles []string, keepSymbols []string, elfLib string) string {

	objList := c.getObjFiles(util.UniqueStrings(objFiles))

	cmd := c.ccPath + " -o " + dstFile + " " + " " + c.cflagsString()

	if elfLib != "" {
		cmd += " -Wl,--just-symbols=" + elfLib
	}

	if c.ldResolveCircularDeps {
		cmd += " -Wl,--start-group " + objList + " -Wl,--end-group "
	} else {
		cmd += " " + objList
	}

	if keepSymbols != nil {
		for _, name := range keepSymbols {
			cmd += " -Wl,--undefined=" + name
		}
	}

	cmd += " " + c.lflagsString()

	/* so we don't get multiple global definitions of the same vartiable */
	//cmd += " -Wl,--warn-common "

	if c.LinkerScript != "" {
		cmd += " -T " + c.LinkerScript
	}
	if options["mapFile"] {
		cmd += " -Wl,-Map=" + dstFile + ".map"
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
	_, err := util.ShellCommand(cmd)
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

	var cmd string

	if options["binFile"] {
		binFile := elfFilename + ".bin"
		cmd = c.ocPath + " -R .bss -R .bss.core -R .bss.core.nz -O binary " +
			elfFilename + " " + binFile
		_, err := util.ShellCommand(cmd)
		if err != nil {
			return err
		}
	}

	if options["listFile"] {
		listFile := elfFilename + ".lst"
		// if list file exists, remove it
		if util.NodeExist(listFile) {
			if err := os.RemoveAll(listFile); err != nil {
				return err
			}
		}

		cmd = c.odPath + " -wxdS " + elfFilename + " >> " + listFile
		_, err := util.ShellCommand(cmd)
		if err != nil {
			// XXX: gobjdump appears to always crash.  Until we get that sorted
			// out, don't fail the link process if lst generation fails.
			return nil
		}

		sects := []string{".text", ".rodata", ".data"}
		for _, sect := range sects {
			cmd = c.odPath + " -s -j " + sect + " " + elfFilename + " >> " +
				listFile
			util.ShellCommand(cmd)
		}

		cmd = c.osPath + " " + elfFilename + " >> " + listFile
		_, err = util.ShellCommand(cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Compiler) PrintSize(elfFilename string) (string, error) {
	cmd := c.osPath + " " + elfFilename
	rsp, err := util.ShellCommand(cmd)
	if err != nil {
		return "", err
	}
	return string(rsp), nil
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

func (c *Compiler) RenameSymbolsCmd(sm *symbol.SymbolMap, libraryFile string, ext string) string {
	val := c.ocPath

	for s, _ := range *sm {
		val += " --redefine-sym " + s + "=" + s + ext
	}

	val += " " + libraryFile
	return val
}

func (c *Compiler) ParseLibraryCmd(libraryFile string) string {
	val := c.odPath + " -t " + libraryFile
	return val
}

func (c *Compiler) CopySymbolsCmd(infile string, outfile string, sm *symbol.SymbolMap) string {

	val := c.ocPath + " -S "

	for symbol, _ := range *sm {
		val += " -K " + symbol
	}

	val += " " + infile
	val += " " + outfile
	return val
}

// Calculates the command-line invocation necessary to archive the specified
// static library.
//
// @param archiveFile           The filename of the library to archive.
// @param objFiles              An array of the source .o filenames.
//
// @return                      The command string.
func (c *Compiler) CompileArchiveCmd(archiveFile string,
	objFiles []string) string {

	objList := c.getObjFiles(objFiles)
	return c.arPath + " rcs " + archiveFile + " " + objList
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
	f, err := os.OpenFile(ar_script_name, os.O_CREATE|os.O_WRONLY, 0666)
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

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Archiving %s\n",
		path.Base(archiveFile))
	objList := c.getObjFiles([]string{})
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Archiving %s with object "+
		"files %s\n", archiveFile, objList)

	// Delete the old archive, if it exists.
	err = os.Remove(archiveFile)
	if err != nil && !os.IsNotExist(err) {
		return util.NewNewtError(err.Error())
	}

	cmd := c.CompileArchiveCmd(archiveFile, objFiles)
	_, err = util.ShellCommand(cmd)
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

	_, err := util.ShellCommand(cmd)

	return err
}

func (c *Compiler) ParseLibrary(libraryFile string) (error, []byte) {
	cmd := c.ParseLibraryCmd(libraryFile)

	out, err := util.ShellCommand(cmd)
	if err != nil {
		return err, nil
	}
	return err, out
}

func (c *Compiler) CopySymbols(infile string, outfile string, sm *symbol.SymbolMap) error {
	cmd := c.CopySymbolsCmd(infile, outfile, sm)

	_, err := util.ShellCommand(cmd)
	if err != nil {
		return err
	}
	return err
}
