/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cli

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	COMPILER_TYPE_C   = 0
	COMPILER_TYPE_ASM = 1
)

type Compiler struct {
	ConfigPath   string
	TargetName   string
	BaseIncludes map[string]bool
	ObjPathList  map[string]bool
	LinkerScript string

	Cflags string
	Aflags string
	Lflags string

	depTracker            DepTracker
	ccPath                string
	asPath                string
	arPath                string
	odPath                string
	osPath                string
	ocPath                string
	ldFlags               string
	ldResolveCircularDeps bool
	ldMapFile             bool
}

func NewCompiler(ccPath string, cDef string, tName string, includes []string) (
	*Compiler, error) {

	c := &Compiler{
		ConfigPath:   ccPath,
		TargetName:   tName,
		BaseIncludes: map[string]bool{},
		ObjPathList:  map[string]bool{},
	}

	c.depTracker = NewDepTracker(c)
	for _, incl := range includes {
		c.BaseIncludes[incl] = true
	}

	StatusMessage(VERBOSITY_VERBOSE,
		"Loading compiler %s, target %s, def %s\n", ccPath, tName, cDef)

	err := c.ReadSettings(cDef)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Compiler) ReadSettings(cDef string) error {
	v, err := ReadConfig(c.ConfigPath, "compiler")
	if err != nil {
		return err
	}

	c.ccPath = v.GetString("compiler.path.cc")
	c.asPath = v.GetString("compiler.path.as")
	c.arPath = v.GetString("compiler.path.archive")
	c.odPath = v.GetString("compiler.path.objdump")
	c.osPath = v.GetString("compiler.path.objsize")
	c.ocPath = v.GetString("compiler.path.objcopy")

	cflags := v.GetStringSlice("compiler.flags." + cDef)
	for _, flag := range cflags {
		if strings.HasPrefix(flag, "compiler.flags") {
			c.Cflags += " " + strings.Trim(v.GetString(flag), "\n")
		} else {
			c.Cflags += " " + strings.Trim(flag, "\n")
		}
	}

	c.ldFlags = v.GetString("compiler.ld.flags")
	c.ldResolveCircularDeps = v.GetBool("compiler.ld.resolve_circular_deps")
	c.ldMapFile = v.GetBool("compiler.ld.mapfile")

	log.Printf("[INFO] ccPath = %s, arPath = %s, flags = %s", c.ccPath,
		c.arPath, c.Cflags)

	return nil
}

// Skips compilation of the specified C or assembly file, but adds the name of
// the object file that would have been generated to the compiler's list of
// object files.  This function is used when the object file is already up to
// date, so no compilation is necessary.  The name of the object file should
// still be remembered so that it gets linked in to the final library or
// executable.
func (c *Compiler) SkipSourceFile(srcFile string) error {
	wd, _ := os.Getwd()
	objDir := wd + "/obj/" + c.TargetName + "/"
	objFile := objDir + strings.TrimSuffix(srcFile, filepath.Ext(srcFile)) +
		".o"
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
func (c *Compiler) IncludesString() string {
	includes := make([]string, 0, len(c.BaseIncludes))
	for k, _ := range c.BaseIncludes {
		includes = append(includes, filepath.ToSlash(k))
	}

	sort.Strings(includes)

	return "-I" + strings.Join(includes, " -I")
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

	wd, _ := os.Getwd()
	objDir := wd + "/obj/" + c.TargetName + "/"
	objFile := strings.TrimSuffix(file, filepath.Ext(file)) + ".o"
	objPath := filepath.ToSlash(objDir + objFile)

	var cmd string

	switch compilerType {
	case COMPILER_TYPE_C:
		cmd = c.ccPath
	case COMPILER_TYPE_ASM:
		cmd = c.asPath
	default:
		return "", NewNewtError("Unknown compiler type")
	}

	cmd += " -c " + "-o " + objPath + " " + file +
		" " + c.Cflags + " " + c.IncludesString()

	return cmd, nil
}

// Generates a dependency Makefile (.d) for the specified source C file.
//
// @param file                  The name of the source file.
func (c *Compiler) GenDepsForFile(file string) error {
	wd, _ := os.Getwd()
	objDir := wd + "/obj/" + c.TargetName + "/"

	if NodeNotExist(objDir) {
		os.MkdirAll(objDir, 0755)
	}

	depFile := objDir + strings.TrimSuffix(file, filepath.Ext(file)) + ".d"
	depFile = filepath.ToSlash(depFile)
	cFlags := c.Cflags + " " + c.IncludesString()

	var cmd string
	var err error

	cmd = c.ccPath + " " + cFlags + " -MM -MG " + file + " > " + depFile
	_, err = ShellCommand(cmd)
	if err != nil {
		return err
	}

	return nil
}

// Compile the specified C or assembly file.
//
// @param file                  The filename of the source file to compile.
// @param compilerType          One of the COMPILER_TYPE_[...] constants.
func (c *Compiler) CompileFile(file string, compilerType int) error {
	wd, _ := os.Getwd()
	objDir := wd + "/obj/" + c.TargetName + "/"

	if NodeNotExist(objDir) {
		os.MkdirAll(objDir, 0755)
	}

	objFile := strings.TrimSuffix(file, filepath.Ext(file)) + ".o"

	objPath := objDir + objFile
	c.ObjPathList[filepath.ToSlash(objPath)] = true

	cmd, err := c.CompileFileCmd(file, compilerType)
	if err != nil {
		return err
	}

	switch compilerType {
	case COMPILER_TYPE_C:
		StatusMessage(VERBOSITY_DEFAULT, "Compiling %s\n", file)
	case COMPILER_TYPE_ASM:
		StatusMessage(VERBOSITY_DEFAULT, "Assembling %s\n", file)
	default:
		return NewNewtError("Unknown compiler type")
	}

	rsp, err := ShellCommand(cmd)
	if err != nil {
		StatusMessage(VERBOSITY_QUIET, string(rsp))
		return err
	}

	err = WriteCommandFile(objPath, cmd)
	if err != nil {
		return err
	}

	// Tell the dependency tracker that an object file was just rebuilt.
	c.depTracker.MostRecent = time.Now()

	return nil
}

// Compiles all C files matching the specified file glob.
//
// @param match                 The file glob specifying which C files to
//                                  compile.
func (c *Compiler) Compile(match string) error {
	files, _ := filepath.Glob(match)

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	log.Printf("[INFO] Compiling C if outdated (%s/%s) %s", wd, match,
		strings.Join(files, " "))
	for _, file := range files {
		file = filepath.ToSlash(file)
		compileRequired, err := c.depTracker.CompileRequired(file, 0)
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
func (c *Compiler) CompileAs(match string) error {
	files, _ := filepath.Glob(match)

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	log.Printf("[INFO] Compiling assembly if outdated (%s/%s) %s", wd, match,
		strings.Join(files, " "))
	for _, file := range files {
		compileRequired, err := c.depTracker.CompileRequired(file, 1)
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

func (c *Compiler) RecursiveClean(path string, tName string) error {
	// Find all the subdirectories of path that contain an "obj/" directory,
	// and remove that directory either altogether, or just the arch specific
	// directory.
	dirList, err := ioutil.ReadDir(path)
	if err != nil {
		return NewNewtError(err.Error())
	}

	for _, node := range dirList {
		if node.IsDir() {
			if node.Name() == "obj" || node.Name() == "bin" {
				if tName == "" {
					os.RemoveAll(path + "/" + node.Name() + "/")
				} else {
					os.RemoveAll(path + "/" + node.Name() + "/" + tName + "/")
				}
			} else {
				// recurse into the directory.
				err = c.RecursiveClean(path+"/"+node.Name(), tName)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (c *Compiler) processEntry(wd string, node os.FileInfo, match string, cType int,
	ignDirs []string) error {
	// check to see if we ignore this element
	for _, entry := range ignDirs {
		if entry == node.Name() {
			return nil
		}
	}

	// if not, recurse into the directory
	os.Chdir(wd + "/" + node.Name())
	return c.RecursiveCompile(match, cType, ignDirs)
}

func (c *Compiler) RecursiveCompile(match string, cType int, ignDirs []string) error {
	// Get a list of files in the current directory, and if they are a directory,
	// and that directory is not in the ignDirs variable, then recurse into that
	// directory and compile the files in there
	wd, err := os.Getwd()
	if err != nil {
		return NewNewtError(err.Error())
	}
	wd = filepath.ToSlash(wd)

	dirList, err := ioutil.ReadDir(wd)
	if err != nil {
		return NewNewtError(err.Error())
	}

	for _, node := range dirList {
		if node.IsDir() {
			err = c.processEntry(wd, node, match, cType, ignDirs)
			if err != nil {
				return err
			}
		}
	}

	os.Chdir(wd)

	switch cType {
	case 0:
		return c.Compile(match)
	case 1:
		return c.CompileAs(match)
	default:
		return NewNewtError("Wrong compiler type specified to RecursiveCompile")
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
	objFiles []string) string {

	objList := c.getObjFiles(UniqueStrings(objFiles))

	cmd := c.ccPath + " -o " + dstFile + " " + c.ldFlags + " " + c.Cflags
	if c.ldResolveCircularDeps {
		cmd += " -Wl,--start-group " + objList + " -Wl,--end-group "
	} else {
		cmd += " " + objList
	}

	if c.LinkerScript != "" {
		cmd += " -T " + c.LinkerScript
	}
	if checkBoolMap(options, "mapFile") {
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
	objFiles []string) error {

	objList := c.getObjFiles(UniqueStrings(objFiles))

	StatusMessage(VERBOSITY_DEFAULT, "Linking %s\n", path.Base(dstFile))
	StatusMessage(VERBOSITY_VERBOSE, "Linking %s with input files %s\n",
		dstFile, objList)

	cmd := c.CompileBinaryCmd(dstFile, options, objFiles)
	rsp, err := ShellCommand(cmd)
	if err != nil {
		StatusMessage(VERBOSITY_QUIET, string(rsp))
		return err
	}

	err = WriteCommandFile(dstFile, cmd)
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

	if checkBoolMap(options, "listFile") {
		listFile := elfFilename + ".lst"
		// if list file exists, remove it
		if NodeExist(listFile) {
			if err := os.RemoveAll(listFile); err != nil {
				return err
			}
		}

		cmd = c.odPath + " -wxdS " + elfFilename + " >> " + listFile
		_, err := ShellCommand(cmd)
		if err != nil {
			// XXX: gobjdump appears to always crash.  Until we get that sorted
			// out, don't fail the link process if lst generation fails.
			return nil
		}

		sects := []string{".text", ".rodata", ".data"}
		for _, sect := range sects {
			cmd = c.odPath + " -s -j " + sect + " " + elfFilename + " >> " +
				listFile
			ShellCommand(cmd)
		}

		cmd = c.osPath + " " + elfFilename + " >> " + listFile
		_, err = ShellCommand(cmd)
		if err != nil {
			return err
		}
	}

	if checkBoolMap(options, "binFile") {
		binFile := elfFilename + ".bin"
		cmd = c.ocPath + " -R .bss -R .bss.core -R .bss.core.nz -O binary " +
			elfFilename + " " + binFile
		_, err := ShellCommand(cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

// Links the specified elf file and generates some associated artifacts (lst,
// bin, and map files).
//
// @param binFile               The filename of the destination elf file to
//                                  link.
// @param options               Some build options specifying how the elf file
//                                  gets generated.
// @param objFiles              An array of the source .o and .a filenames.
func (c *Compiler) CompileElf(binFile string, options map[string]bool,
	objFiles []string) error {

	binFile += ".elf"

	linkRequired, err := c.depTracker.LinkRequired(binFile, options, objFiles)
	if err != nil {
		return err
	}
	if linkRequired {
		err := c.CompileBinary(binFile, options, objFiles)
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

// Archives the specified static library.
//
// @param archiveFile           The filename of the library to archive.
// @param objFiles              An array of the source .o filenames.
func (c *Compiler) CompileArchive(archiveFile string, objFiles []string) error {
	arRequired, err := c.depTracker.ArchiveRequired(archiveFile, objFiles)
	if err != nil {
		return err
	}
	if !arRequired {
		return nil
	}

	objList := c.getObjFiles(objFiles)

	StatusMessage(VERBOSITY_DEFAULT, "Archiving %s\n", path.Base(archiveFile))
	StatusMessage(VERBOSITY_VERBOSE, "Archiving %s with object files %s",
		archiveFile, objList)

	// Delete the old archive, if it exists.
	err = os.Remove(archiveFile)
	if err != nil && !os.IsNotExist(err) {
		return NewNewtError(err.Error())
	}

	cmd := c.CompileArchiveCmd(archiveFile, objFiles)
	rsp, err := ShellCommand(cmd)
	if err != nil {
		StatusMessage(VERBOSITY_QUIET, string(rsp))
		return err
	}

	err = WriteCommandFile(archiveFile, cmd)
	if err != nil {
		return err
	}

	return nil
}
