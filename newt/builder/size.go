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
 * We accumulate the size of libraries to elements in this.
 */
type PkgSize struct {
	Name  string
	Sizes map[string]uint32 /* Sizes indexed by mem section name */
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

func MakePkgSize(name string, memSections map[string]*MemSection) *PkgSize {
	pkgSize := &PkgSize{
		Name: name,
	}
	pkgSize.Sizes = make(map[string]uint32)
	for secName, _ := range memSections {
		pkgSize.Sizes[secName] = 0
	}
	return pkgSize
}

/*
 * Go through GCC generated mapfile, and collect info about symbol sizes
 */
func ParseMapFileSizes(fileName string) (map[string]*PkgSize, map[string]*MemSection,
	error) {
	var state int = 0

	file, err := os.Open(fileName)
	if err != nil {
		return nil, nil,
			util.NewNewtError("Mapfile failed: " + err.Error())
	}

	memSections := make(map[string]*MemSection)
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
				return nil, nil, util.NewNewtError("Can't parse mem info")
			}
			size, err := strconv.ParseUint(array[2], 0, 64)
			if err != nil {
				return nil, nil, util.NewNewtError("Can't parse mem info")
			}
			memSections[array[0]] = MakeMemSection(array[0], offset,
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
			tmpStrArr := strings.Split(srcFile, "(")
			srcLib := filepath.Base(tmpStrArr[0])
			for name, section := range memSections {
				if section.PartOf(addr) {
					pkgSize := pkgSizes[srcLib]
					if pkgSize == nil {
						pkgSize =
							MakePkgSize(srcLib, memSections)
						pkgSizes[srcLib] = pkgSize
					}
					pkgSize.Sizes[name] += uint32(size)
					break
				}
			}
		default:
		}
	}
	file.Close()
	for name, section := range memSections {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "Mem %s: 0x%x-0x%x\n",
			name, section.Offset, section.EndOff)
	}

	return pkgSizes, memSections, nil
}

/*
 * Return a printable string containing size data for the libraries
 */
func PrintSizes(libs map[string]*PkgSize,
	sectMap map[string]*MemSection) (string, error) {
	ret := ""

	/*
	 * Order sections by offset, and display lib sizes in that order.
	 */
	memSections := make(MemSectionArray, len(sectMap))
	var i int = 0
	for _, sec := range sectMap {
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
		ret += fmt.Sprintf("%7s ", sec.Name)
	}
	ret += "\n"
	for _, es := range pkgSizes {
		for i := 0; i < len(memSections); i++ {
			ret += fmt.Sprintf("%7d ", es.Sizes[memSections[i].Name])
		}
		ret += fmt.Sprintf("%s\n", es.Name)
	}
	return ret, nil
}

func (t *TargetBuilder) Size() error {

	err := t.PrepBuild()

	if err != nil {
		return err
	}

	fmt.Printf("Size of Application Image: %s\n", t.App.buildName)
	err = t.App.Size()

	if err == nil {
		if t.Loader != nil {
			fmt.Printf("Size of Loader Image: %s\n", t.Loader.buildName)
			err = t.Loader.Size()
		}
	}

	return err
}

func (b *Builder) Size() error {
	if b.appPkg == nil {
		return util.NewNewtError("app package not specified for this target")
	}

	err := b.target.PrepBuild()
	if err != nil {
		return err
	}
	if b.target.Bsp.Arch == "sim" {
		fmt.Println("'newt size' not supported for sim targets.")
		return nil
	}
	mapFile := b.AppElfPath() + ".map"

	pkgSizes, memSections, err := ParseMapFileSizes(mapFile)
	if err != nil {
		return err
	}
	output, err := PrintSizes(pkgSizes, memSections)
	if err != nil {
		return err
	}
	fmt.Printf("%s", output)

	c, err := b.newCompiler(b.appPkg, b.PkgBinDir(b.AppElfPath()))
	if err != nil {
		return err
	}

	fmt.Printf("\nobjsize\n")
	output, err = c.PrintSize(b.AppElfPath())
	if err != nil {
		return err
	}
	fmt.Printf("%s", output)

	return nil
}
