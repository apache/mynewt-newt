/*
 Copyright 2015 Stack Inc.
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
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Compiler struct {
	ConfigPath   string
	Arch         string
	BaseIncludes []string
	ObjPathList  map[string]bool
	LinkerScript string

	ccPath  string
	asPath  string
	arPath  string
	odPath  string
	osPath  string
	ccFlags string
}

func NewCompiler(ccPath string, cDef string, arch string, includes []string) (
	*Compiler, error) {
	c := &Compiler{
		ConfigPath:   ccPath,
		Arch:         arch,
		BaseIncludes: includes,
	}

	log.Printf("[DEBUG] Loading compiler %s, arch %s, def %s", ccPath, arch, cDef)

	err := c.ReadSettings(cDef)
	if err != nil {
		return nil, err
	}

	c.ObjPathList = make(map[string]bool)

	return c, nil
}

func (c *Compiler) ReadSettings(cDef string) error {
	v, err := ReadConfig(c.ConfigPath, "compiler")
	if err != nil {
		return err
	}

	c.ccPath = strings.Trim(v.GetString("compiler.path.cc"), "\n")
	c.asPath = strings.Trim(v.GetString("compiler.path.as"), "\n")
	c.arPath = strings.Trim(v.GetString("compiler.path.archive"), "\n")
	c.odPath = strings.Trim(v.GetString("compiler.path.objdump"), "\n")
	c.osPath = strings.Trim(v.GetString("compiler.path.objsize"), "\n")

	cflags := v.GetStringSlice("compiler.flags." + cDef)
	for _, flag := range cflags {
		if strings.HasPrefix(flag, "compiler.flags") {
			c.ccFlags += strings.Trim(v.GetString(flag), "\n") + " "
		} else {
			c.ccFlags += strings.Trim(flag, "\n") + " "
		}
	}

	log.Printf("[DEBUG] ccPath = %s, arPath = %s, flags = %s", c.ccPath,
		c.arPath, c.ccFlags)

	return nil
}

// file type 0 = cc, file type 1 = as
func (c *Compiler) CompileFile(file string, compilerType int) error {
	wd, _ := os.Getwd()
	objDir := wd + "/obj/" + c.Arch + "/"

	if NodeNotExist(objDir) {
		os.MkdirAll(objDir, 0755)
	}

	objFile := strings.TrimSuffix(file, filepath.Ext(file)) + ".o"

	objPath := objDir + objFile
	c.ObjPathList[objPath] = true

	var cmd string

	switch compilerType {
	case 0:
		cmd = c.ccPath
	case 1:
		cmd = c.asPath
	default:
		return errors.New("Unknown compiler type")
	}

	cmd += " -c " + "-o " + objPath + " " + file +
		" " + c.ccFlags + " -I" + strings.Join(c.BaseIncludes, " -I")

	_, err := ShellCommand(cmd)
	return err
}

func (c *Compiler) Compile(match string) error {
	files, _ := filepath.Glob(match)

	for _, file := range files {
		err := c.CompileFile(file, 0)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) CompileAs(match string) error {
	files, _ := filepath.Glob(match)

	for _, file := range files {
		err := c.CompileFile(file, 1)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) RecursiveClean(path string, arch string) error {
	// Find all the subdirectories of path that contain an "obj/" directory, and
	// remove that directory either altogether, or just the arch specific directory.
	dirList, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}

	for _, node := range dirList {
		if node.IsDir() {
			if node.Name() == "obj" {
				if arch == "" {
					os.RemoveAll(path + "/obj/")
				} else {
					os.RemoveAll(path + "/obj/" + arch + "/")
				}
			} else {
				// recurse into the directory.
				err = c.RecursiveClean(path+"/"+node.Name(), arch)
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
		return err
	}

	dirList, err := ioutil.ReadDir(wd)
	if err != nil {
		return err
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
		return errors.New("Wrong compiler type specified to RecursiveCompile")
	}
}

func (c *Compiler) getObjFiles(baseObjFiles string) string {
	objList := baseObjFiles
	for objName, _ := range c.ObjPathList {
		objList += " " + objName
	}
	return objList
}

func (c *Compiler) CompileBinary(binFile string, options map[string]bool,
	objFiles string) error {
	objList := c.getObjFiles(objFiles)

	log.Printf("[DEBUG] compiling binary %s with object files %s", binFile,
		objList)

	cmd := c.ccPath + " -o " + binFile + " -static -lgcc " + c.ccFlags +
		" -Wl,--start-group " + objList + " -Wl,--end-group "
	if c.LinkerScript != "" {
		cmd += " -T " + c.LinkerScript
	}
	if checkBoolMap(options, "mapFile") {
		cmd += " -Wl,-Map=" + binFile + ".map"
	}

	_, err := ShellCommand(cmd)
	if err != nil {
		return err
	}

	if checkBoolMap(options, "listFile") {
		listFile := binFile + ".lst"
		cmd = c.odPath + " -wxdS " + binFile + " >> " + listFile
		_, err := ShellCommand(cmd)
		if err != nil {
			return err
		}

		sects := []string{".text", ".rodata", ".fc", ".data"}
		for _, sect := range sects {
			cmd = c.odPath + " -s -j " + sect + " " + binFile + " >> " + listFile
			_, err := ShellCommand(cmd)
			if err != nil {
				return err
			}
		}

		cmd = c.osPath + " " + binFile + " >> " + listFile
	}

	return nil

}

func (c *Compiler) CompileElf(binFile string, options map[string]bool,
	objFiles string) error {
	binFile += ".elf"
	return c.CompileBinary(binFile, options, objFiles)
}

func (c *Compiler) CompileArchive(archiveFile string, objFiles string) error {
	objList := c.getObjFiles(objFiles)

	log.Printf("[DEBUG] compiling archive %s with object files %s",
		archiveFile, objList)

	cmd := c.arPath + " rcs " + archiveFile + " " + objList

	_, err := ShellCommand(cmd)
	return err
}
