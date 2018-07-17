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

/* this file parses a library file for the build and returns
 * a list of all the symbols with their types and sizes */

/* gets an objdump -t and parses into a symbolMap" */

package builder

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strconv"

	"mynewt.apache.org/newt/newt/symbol"
	"mynewt.apache.org/newt/util"
)

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
* 00000100       O *COM*	00000004 g_idle_task_stack

*/
func parseObjectLine(line string, r *regexp.Regexp) (error, *symbol.SymbolInfo) {

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

	/*  Common section has length in a different spot. Also, these
	 * are really global variables so mark them as such */
	if si.IsSection("*COM*") {
		si.Size = (*si).Loc
		si.Code = "g" + si.Code[1:]
	}

	return nil, si
}

func getParseRexeg() (error, *regexp.Regexp) {
	r, err := regexp.Compile("^([0-9A-Fa-f]+)[\t ]+([lgu! ][w ][C ][W ][Ii ][Dd ][FfO ])[\t ]+([^\t\n\f\r ]+)[\t ]+([0-9a-fA-F]+)[\t ]([^\t\n\f\r ]+)")

	if err != nil {
		return err, nil
	}

	return nil, r
}

func (b *Builder) ParseObjectLibrary(bp *BuildPackage) (
	error, *symbol.SymbolMap) {

	file := b.ArchivePath(bp)
	return b.ParseObjectLibraryFile(bp, file, true)
}

func (b *Builder) ParseObjectElf(elf_file string) (error, *symbol.SymbolMap) {
	return b.ParseObjectLibraryFile(nil, elf_file, false)
}

func (b *Builder) ParseObjectLibraryFile(bp *BuildPackage,
	file string, textDataOnly bool) (error, *symbol.SymbolMap) {

	c, err := b.targetBuilder.NewCompiler(b.AppElfPath(), "")

	ext := filepath.Ext(file)

	if err != nil {
		return err, nil
	}

	err, out := c.ParseLibrary(file)

	if err != nil {
		return err, nil
	}

	sm := symbol.NewSymbolMap()

	buffer := bytes.NewBuffer(out)

	err, r := getParseRexeg()

	if err != nil {
		return err, nil
	}

	for {
		line, err := buffer.ReadString('\n')
		if err != nil {
			break
		}
		err, si := parseObjectLine(line, r)

		if err == nil && si != nil {

			/* assign the library */
			if bp != nil {
				(*si).Bpkg = bp.rpkg.Lpkg.Name()
			} else {
				(*si).Bpkg = "elf"
			}

			/*  discard undefined */
			if (*si).IsSection("*UND*") {
				continue
			}

			/* discard debug symbols */
			if (*si).IsDebug() {
				continue
			}

			if (*si).IsFile() {
				continue
			}

			/* if we are looking for text and data only, do a special check */
			if textDataOnly {
				include := (*si).IsSection(".bss") ||
					(*si).IsSection(".text") ||
					(*si).IsSection(".data") ||
					(*si).IsSection("*COM*") ||
					(*si).IsSection(".rodata")

				if !include {
					continue
				}
			}

			/* add the symbol to the map */
			(*si).Ext = ext
			sm.Add(*si)
			util.StatusMessage(util.VERBOSITY_VERBOSE,
				"Keeping Symbol %s in package %s\n", (*si).Name, (*si).Bpkg)
		}
	}

	return nil, sm
}

func (b *Builder) CopySymbols(sm *symbol.SymbolMap) error {
	c, err := b.targetBuilder.NewCompiler(b.AppElfPath(), "")

	if err != nil {
		return err
	}

	err = c.CopySymbols(b.AppElfPath(), b.AppLinkerElfPath(), sm)

	return err
}
