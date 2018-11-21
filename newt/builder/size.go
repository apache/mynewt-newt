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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/util"
)

/*
 * These are different memory regions as specified in linker script.
 */
type MemSection struct {
	Name   string
	Offset uint64
	EndOff uint64
}
type MemSectionArray []*MemSection

var globalMemSections map[string]*MemSection

func (array MemSectionArray) Len() int {
	return len(array)
}

func (array MemSectionArray) Less(i, j int) bool {
	return array[i].Offset < array[j].Offset
}

func (array MemSectionArray) Swap(i, j int) {
	array[i], array[j] = array[j], array[i]
}

func MakeMemSection(name string, off uint64, size uint64) *MemSection {
	memsection := &MemSection{
		Name:   name,
		Offset: off,
		EndOff: off + size,
	}
	return memsection
}

func (m *MemSection) PartOf(addr uint64) bool {
	if addr >= m.Offset && addr < m.EndOff {
		return true
	} else {
		return false
	}
}

/*
 * Info about specific symbol size
 */
type SymbolData struct {
	Name    string
	ObjName string            /* Which object file it came from */
	Sizes   map[string]uint32 /* Sizes indexed by mem section name */
}

type SymbolDataArray []*SymbolData

/*
 * We accumulate the size of libraries to elements in this.
 */
type PkgSize struct {
	Name  string
	Sizes map[string]uint32      /* Sizes indexed by mem section name */
	Syms  map[string]*SymbolData /* Symbols indexed by symbol name */
}

type PkgSizeArray []*PkgSize

func (array PkgSizeArray) Len() int {
	return len(array)
}

func (array PkgSizeArray) Less(i, j int) bool {
	return array[i].Name < array[j].Name
}

func (array PkgSizeArray) Swap(i, j int) {
	array[i], array[j] = array[j], array[i]
}

func (array SymbolDataArray) Len() int {
	return len(array)
}

func (array SymbolDataArray) Less(i, j int) bool {
	return array[i].Name < array[j].Name
}

func (array SymbolDataArray) Swap(i, j int) {
	array[i], array[j] = array[j], array[i]
}

func MakeSymbolData(name string, objName string) *SymbolData {
	sym := &SymbolData{
		Name:    name,
		ObjName: objName,
	}
	sym.Sizes = make(map[string]uint32)
	for _, sec := range globalMemSections {
		sym.Sizes[sec.Name] = 0
	}
	return sym
}

func MakePkgSize(name string) *PkgSize {
	pkgSize := &PkgSize{
		Name: name,
	}
	pkgSize.Sizes = make(map[string]uint32)
	for _, sec := range globalMemSections {
		pkgSize.Sizes[sec.Name] = 0
	}
	pkgSize.Syms = make(map[string]*SymbolData)
	return pkgSize
}

func (ps *PkgSize) addSymSize(symName string, objName string, size uint32, addr uint64) {
	for _, section := range globalMemSections {
		if section.PartOf(addr) {
			name := section.Name
			size32 := uint32(size)
			if size32 > 0 {
				sym := ps.Syms[symName]
				if sym == nil {
					sym = MakeSymbolData(symName, objName)
					ps.Syms[symName] = sym
				}
				ps.Sizes[name] += size32
				sym.Sizes[name] += size32
			}
			break
		}
	}
}

/*
 * Go through GCC generated mapfile, and collect info about symbol sizes
 */
func ParseMapFileSizes(fileName string) (map[string]*PkgSize, error) {
	var state int = 0

	file, err := os.Open(fileName)
	if err != nil {
		return nil, util.NewNewtError("Mapfile failed: " + err.Error())
	}

	var symName string = ""

	globalMemSections = make(map[string]*MemSection)
	pkgSizes := make(map[string]*PkgSize)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		switch state {
		case 0:
			if strings.Contains(scanner.Text(), "Memory Configuration") {
				state = 1
			}
		case 1:
			if strings.Contains(scanner.Text(), "Origin") {
				state = 2
			}
		case 2:
			if strings.Contains(scanner.Text(), "*default*") {
				state = 3
				continue
			}
			array := strings.Fields(scanner.Text())
			offset, err := strconv.ParseUint(array[1], 0, 64)
			if err != nil {
				return nil, util.NewNewtError("Can't parse mem info")
			}
			size, err := strconv.ParseUint(array[2], 0, 64)
			if err != nil {
				return nil, util.NewNewtError("Can't parse mem info")
			}
			globalMemSections[array[0]] = MakeMemSection(array[0], offset,
				size)
		case 3:
			if strings.Contains(scanner.Text(),
				"Linker script and memory map") {
				state = 4
			}
		case 4:
			var addrStr string = ""
			var sizeStr string = ""
			var srcFile string = ""

			if strings.Contains(scanner.Text(), "/DISCARD/") ||
				strings.HasPrefix(scanner.Text(), "OUTPUT(") {
				/*
				 * After this there is only discarded symbols
				 */
				state = 5
				continue
			}

			array := strings.Fields(scanner.Text())

			switch len(array) {
			case 1:
				/*
				 * section name on it's own, e.g.
				 * *(.text*)
				 *
				 * section name + symbol name, e.g.
				 * .text.Reset_Handler
				 *
				 * ignore these for now
				 */
				symName = array[0]
				continue
			case 2:
				/*
				 * Either stuff from beginning to first useful data e.g.
				 * END GROUP
				 *
				 * or address of symbol + symbol name, e.g.
				 * 0x00000000080002c8                SystemInit
				 *
				 * or section names with multiple input things, e.g.
				 * *(.ARM.extab* .gnu.linkonce.armextab.*)
				 *
				 * or space set aside in linker script e.g.
				 * 0x0000000020002e80      0x400
				 * (that's the initial stack)
				 *
				 * ignore these for now
				 */
				continue
			case 3:
				/*
				 * address, size, and name of file, e.g.
				 * 0x000000000800bb04     0x1050 /Users/marko/foo/tadpole/hw//mcu/stm/stm32f3xx/bin/blinky_f3/libstm32f3xx.a(stm32f30x_syscfg.o)
				 *
				 * padding, or empty areas defined in linker script:
				 * *fill*         0x000000000800cb71        0x3
				 *
				 * output section name, location, size, e.g.:
				 * .bss            0x0000000020000ab0     0x23d0
				 */
				/*
				 * Record addr, size and name to find library.
				 */
				if array[0] == "*fill*" {
					addrStr = array[1]
					sizeStr = array[2]
					srcFile = array[0]
					symName = array[0]
				} else {
					addrStr = array[0]
					sizeStr = array[1]
					srcFile = array[2]
				}
			case 4:
				/*
				 * section, address, size, name of file, e.g.
				 * COMMON         0x0000000020002d28        0x8 /Users/marko/foo/tadpole/libs//os/bin/blinky_f3/libos.a(os_arch_arm.o)
				 *
				 * linker script symbol definitions:
				 * 0x0000000020002e80                _ebss = .
				 *
				 * crud, e.g.:
				 * 0x8 (size before relaxing)
				 */
				symName = array[0]
				addrStr = array[1]
				sizeStr = array[2]
				srcFile = array[3]
			default:
				continue
			}
			addr, err := strconv.ParseUint(addrStr, 0, 64)
			if err != nil {
				continue
			}
			size, err := strconv.ParseUint(sizeStr, 0, 64)
			if err != nil {
				continue
			}
			if size == 0 {
				continue
			}

			// srcFile might be : mylib.a(object_file.o) or object_file.o
			tmpStrArr := strings.Split(srcFile, "(")
			srcLib := tmpStrArr[0]
			objName := ""
			if srcLib != "*fill*" {
				if len(tmpStrArr) > 1 {
					tmpStrArr = strings.Split(tmpStrArr[1], ")")
					objName = tmpStrArr[0]
				} else {
					objName = filepath.Base(tmpStrArr[0])
				}
			}
			tmpStrArr = strings.Split(symName, ".")
			if len(tmpStrArr) > 2 {
				if tmpStrArr[1] == "rodata" && tmpStrArr[2] == "str1" {
					symName = ".rodata.str1"
				} else {
					symName = tmpStrArr[2]
				}
			}
			pkgSize := pkgSizes[srcLib]
			if pkgSize == nil {
				pkgSize = MakePkgSize(srcLib)
				pkgSizes[srcLib] = pkgSize
			}
			pkgSize.addSymSize(symName, objName, uint32(size), addr)
			symName = ".unknown"
		default:
		}
	}
	file.Close()
	for name, section := range globalMemSections {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Mem %s: 0x%x-0x%x\n",
			name, section.Offset, section.EndOff)
	}

	return pkgSizes, nil
}

/*
 * Return a printable string containing size data for the libraries
 */
func PrintSizes(libs map[string]*PkgSize) error {
	/*
	 * Order sections by offset, and display lib sizes in that order.
	 */
	memSections := make(MemSectionArray, len(globalMemSections))
	var i int = 0
	for _, sec := range globalMemSections {
		memSections[i] = sec
		i++
	}
	sort.Sort(memSections)

	/*
	 * Order libraries by name, and display them in that order.
	 */
	pkgSizes := make(PkgSizeArray, len(libs))
	i = 0
	for _, es := range libs {
		pkgSizes[i] = es
		i++
	}
	sort.Sort(pkgSizes)

	for _, sec := range memSections {
		fmt.Printf("%7s ", sec.Name)
	}
	fmt.Printf("\n")
	for _, es := range pkgSizes {
		for i := 0; i < len(memSections); i++ {
			fmt.Printf("%7d ", es.Sizes[memSections[i].Name])
		}
		fmt.Printf("%s\n", filepath.Base(es.Name))
	}

	return nil
}

func (t *TargetBuilder) Size() error {

	err := t.PrepBuild()

	if err != nil {
		return err
	}

	fmt.Printf("Size of Application Image: %s\n", t.AppBuilder.buildName)
	err = t.AppBuilder.Size()

	if err == nil {
		if t.LoaderBuilder != nil {
			fmt.Printf("Size of Loader Image: %s\n", t.LoaderBuilder.buildName)
			err = t.LoaderBuilder.Size()
		}
	}

	return err
}

func (b *Builder) FindPkgNameByArName(arName string) string {
	for rpkg, bpkg := range b.PkgMap {
		if b.ArchivePath(bpkg) == arName {
			return rpkg.Lpkg.FullName()
		}
	}
	return filepath.Base(arName)
}

func (b *Builder) Size() error {
	if b.appPkg == nil {
		return util.NewNewtError("app package not specified for this target")
	}

	err := b.targetBuilder.PrepBuild()
	if err != nil {
		return err
	}
	if b.targetBuilder.bspPkg.Arch == "sim" {
		fmt.Println("'newt size' not supported for sim targets.")
		return nil
	}
	mapFile := b.AppElfPath() + ".map"

	pkgSizes, err := ParseMapFileSizes(mapFile)
	if err != nil {
		return err
	}
	err = PrintSizes(pkgSizes)
	if err != nil {
		return err
	}

	c, err := b.newCompiler(b.appPkg, b.FileBinDir(b.AppElfPath()))
	if err != nil {
		return err
	}

	fmt.Printf("\nobjsize\n")
	output, err := c.PrintSize(b.AppElfPath())
	if err != nil {
		return err
	}
	fmt.Printf("%s", output)

	return nil
}

func (t *TargetBuilder) SizeReport(sectionName string, diffFriendly bool) error {

	err := t.PrepBuild()

	if err != nil {
		return err
	}

	fmt.Printf("Size of Application Image: %s\n", t.AppBuilder.buildName)
	err = t.AppBuilder.SizeReport(sectionName, diffFriendly)

	if err == nil {
		if t.LoaderBuilder != nil {
			fmt.Printf("Size of Loader Image: %s\n", t.LoaderBuilder.buildName)
			err = t.LoaderBuilder.SizeReport(sectionName, diffFriendly)
		}
	}

	return err
}

func (b *Builder) SizeReport(sectionName string, diffFriendly bool) error {
	srcBase := interfaces.GetProject().Path() + "/"

	err := SizeReport(b.AppElfPath(), srcBase, sectionName, diffFriendly)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	return nil
}
