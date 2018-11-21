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
	"mynewt.apache.org/newt/util"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func runNmCommand(elfFilePath string) ([]byte, error) {
	var (
		cmdOut []byte
		err    error
	)
	cmdName := "arm-none-eabi-nm"
	cmdArgs := []string{elfFilePath, "-S", "-l", "--size-sort"}

	if cmdOut, err = exec.Command(cmdName, cmdArgs...).Output(); err != nil {
		fmt.Fprintln(os.Stderr, "There was an error running nm command: ", err)
		os.Exit(1)
	}

	return cmdOut, err
}

func runObjdumpCommand(elfFilePath string, params string) ([]byte, error) {
	var (
		cmdOut []byte
		err    error
	)
	cmdName := "arm-none-eabi-objdump"
	cmdArgs := []string{params, elfFilePath}
	if cmdOut, err = exec.Command(cmdName, cmdArgs...).Output(); err != nil {
		fmt.Fprintln(os.Stderr, "There was an error running objdump command: ",
			err)
		os.Exit(1)
	}

	return cmdOut, err
}

func runAddr2lineCommand(elfFilePath string, address string) ([]byte, error) {
	var (
		cmdOut []byte
		err    error
	)
	cmdName := "arm-none-eabi-addr2line"
	cmdArgs := []string{"-e", elfFilePath, address}

	cmdOut, err = exec.Command(cmdName, cmdArgs...).Output()

	/* This can fail and it's not a critical error */

	return cmdOut, err
}

func loadSymbolsAndPaths(elfFilePath, pathToStrip string) (map[string]string,
	error) {
	symbolsPath := make(map[string]string)

	nmOut, err := runNmCommand(elfFilePath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(nmOut), "\n")

	for _, line := range lines {
		fields := strings.Fields(strings.Replace(line, "\t", " ", -1))
		if len(fields) < 4 {
			continue
		}
		var path string

		if len(fields) < 5 {
			addr2lineOut, err := runAddr2lineCommand(elfFilePath, fields[0])
			if err != nil {
				path = "(other)"
			} else {
				aline := strings.Split(string(addr2lineOut), "\n")[0]
				path = strings.Split(aline, ":")[0]
			}
		} else {
			path = strings.Split(fields[4], ":")[0]
		}
		if pathToStrip != "" {
			if strings.Contains(path, pathToStrip) {
				path = strings.Replace(path, pathToStrip, "", -1)
			} else {
				path = "(other)"
			}
		}
		symbolsPath[fields[3]] = path
	}
	return symbolsPath, nil
}

func MakeSymbol(name string, section string, size uint64) *Symbol {
	symbol := &Symbol{
		name,
		section,
		size,
	}
	return symbol
}

type MemoryRegion struct {
	Name         string
	Offset       uint64
	EndOff       uint64
	TotalSize    uint64
	SectionNames map[string]struct{}
	NamesSizes   map[string]uint64
}

func MakeMemoryRegion() *MemoryRegion {
	section := &MemoryRegion{
		"", 0, 0, 0,
		make(map[string]struct{}),
		make(map[string]uint64),
	}
	return section
}

func (m *MemoryRegion) PartOf(addr uint64) bool {
	return addr >= m.Offset && addr < m.EndOff
}

func loadSymbolsAndSections(elfFilePath string) (map[string]*Symbol, error) {
	objdumpOut, err := runObjdumpCommand(elfFilePath, "-tw")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(objdumpOut), "\n")
	symbols := make(map[string]*Symbol)
	for _, line := range lines {
		fields := strings.Fields(strings.Replace(line, "\t", " ", -1))

		if len(fields) == 5 {
			size, err := strconv.ParseUint(fields[3], 16, 64)
			if err != nil {
				continue
			}
			symbols[fields[4]] = MakeSymbol(fields[4], fields[2], size)
		} else if len(fields) == 6 {
			size, err := strconv.ParseUint(fields[4], 16, 64)
			if err != nil {
				continue
			}
			symbols[fields[5]] = MakeSymbol(fields[5], fields[3], size)
		}

	}

	return symbols, nil
}

func getMemoryRegion(elfFilePath string, sectionName string) (*MemoryRegion,
	error) {

	mapFile := elfFilePath + ".map"
	sectionRegion, err := parseMapFileRegions(mapFile, sectionName)
	if err != nil {
		return nil, err
	}

	objdumpOut, err := runObjdumpCommand(elfFilePath, "-hw")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(objdumpOut), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		size, err := strconv.ParseUint(fields[2], 16, 64)
		if err != nil {
			continue
		}
		address, err := strconv.ParseUint(fields[3], 16, 64)
		if err != nil {
			continue
		}

		if sectionRegion.PartOf(address) {
			sectionRegion.TotalSize += size
			sectionRegion.SectionNames[fields[1]] = struct{}{}
			sectionRegion.NamesSizes[fields[1]] = size
			continue
		}
	}

	return sectionRegion, nil
}

/*
 * Go through GCC generated mapfile, and collect info about symbol sizes
 */
func parseMapFileRegions(fileName string, sectionName string) (*MemoryRegion,
	error) {
	var state int = 0

	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}

	sectionRegion := MakeMemoryRegion()

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
				continue
			}
			size, err := strconv.ParseUint(array[2], 0, 64)
			if err != nil {
				continue
			}

			if strings.EqualFold(array[0], sectionName) {
				sectionRegion.Name = array[0]
				sectionRegion.Offset = offset
				sectionRegion.EndOff = offset + size
			}
		case 3:
			fallthrough
		default:
			return sectionRegion, nil
		}

	}
	return sectionRegion, nil
}

func logMemoryRegionStats(memRegion *MemoryRegion, sectionName string) {
	memName := fmt.Sprintf("Mem %s:", sectionName)
	util.StatusMessage(util.VERBOSITY_VERBOSE, "%-10s 0x%08x-0x%08x\n",
		memName, memRegion.Offset, memRegion.EndOff)
	util.StatusMessage(util.VERBOSITY_VERBOSE, "\n")
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Mem: %s\n", sectionName)
	util.StatusMessage(util.VERBOSITY_VERBOSE, "%-20s %10s\n", "Name", "Size")
	for sectionName, size := range memRegion.NamesSizes {
		util.StatusMessage(util.VERBOSITY_VERBOSE, "%-20s %10d\n",
			sectionName, size)
	}
	util.StatusMessage(util.VERBOSITY_VERBOSE, "%-20s %10d\n", "Total",
		memRegion.TotalSize)
	util.StatusMessage(util.VERBOSITY_VERBOSE, "\n")
}

func SizeReport(elfFilePath, srcBase string, sectionName string, diffFriendly bool) error {
	symbolsPath, err := loadSymbolsAndPaths(elfFilePath, srcBase)
	if err != nil {
		return err
	}
	loadedSectionSizes, err := loadSymbolsAndSections(elfFilePath)
	if err != nil {
		return err
	}
	sectionRegion, err := getMemoryRegion(elfFilePath, sectionName)
	if err != nil {
		return err
	}

	logMemoryRegionStats(sectionRegion, sectionName)

	startPath := "."

	sectionNodes := newFolder(startPath)
	for _, symbol := range loadedSectionSizes {
		if _, ok := sectionRegion.SectionNames[symbol.Section]; ok {
			sectionNodes.addSymbol(symbol, symbolsPath[symbol.Name])
		}
	}
	fmt.Printf("%s report:\n", sectionName)
	fmt.Printf("%v", sectionNodes.ToString(sectionRegion.TotalSize, diffFriendly))
	return nil
}
