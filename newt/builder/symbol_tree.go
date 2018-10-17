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
	"fmt"
	"sort"
	"strings"
	"strconv"
)

type Symbol struct {
	Name    string
	Section string
	Size    uint64
}

type File struct {
	Name    string
	Symbols map[string]*Symbol
}

type Folder struct {
	Name    string
	Files   map[string]*File
	Folders map[string]*Folder
}

type outputFormatter interface {
	Header(nameStr string, sizeStr string, percentStr string) string
	Container(level int, name string, size uint64, percent float64) string
	Symbol(level int, name string, size uint64, percent float64) string
	Separator() string
}

type outputFormatterDefault struct {
	indentStr string
	headerStr string
	symbolStr string
}

type outputFormatterDiffable struct {
	indentStr    string
	headerStr    string
	containerStr string
	symbolStr    string
}

func newSymbolFormatterDefault() *outputFormatterDefault {
	return &outputFormatterDefault{
		indentStr: "  ",
		headerStr: "%-59s %9s %9s\n",
		symbolStr: "%-59s %9d %8.2f%%\n",
	}
}

func (fmtr *outputFormatterDefault) Header(nameStr string, sizeStr string, percentStr string) string {
	return fmt.Sprintf(fmtr.headerStr, nameStr, sizeStr, percentStr)
}

func (fmtr *outputFormatterDefault) Container(level int, name string, size uint64, percent float64) string {
	return fmtr.Symbol(level, name, size, percent)
}

func (fmtr *outputFormatterDefault) Symbol(level int, name string, size uint64, percent float64) string {
	return fmt.Sprintf(fmtr.symbolStr, strings.Repeat(fmtr.indentStr, level) + name, size, percent)
}

func (fmtr *outputFormatterDefault) Separator() string {
	// -1 is to cut \n
	return strings.Repeat("=", len(fmtr.Header("", "", "")) - 1) + "\n"
}

func newSymbolFormatterDiffable() *outputFormatterDiffable {
	return &outputFormatterDiffable{
		indentStr:    "  ",
		headerStr:    "%-70s %9s\n",
		containerStr: "%-70s\n",
		symbolStr:    "%-70s %9d\n",
	}
}

func (fmtr *outputFormatterDiffable) Header(nameStr string, sizeStr string, percentStr string) string {
	return fmt.Sprintf(fmtr.headerStr, nameStr, sizeStr)
}

func (fmtr *outputFormatterDiffable) Container(level int, name string, size uint64, percent float64) string {
	return fmt.Sprintf(fmtr.containerStr, strings.Repeat(fmtr.indentStr, level) + name)
}

func (fmtr *outputFormatterDiffable) Symbol(level int, name string, size uint64, percent float64) string {
	return fmt.Sprintf(fmtr.symbolStr, strings.Repeat(fmtr.indentStr, level) + name, size)
}

func (fmtr *outputFormatterDiffable) Separator() string {
	// -1 is to cut \n
	return strings.Repeat("=", len(fmtr.Header("", "", "")) - 1) + "\n"
}

func newFolder(name string) *Folder {
	return &Folder{name, make(map[string]*File), make(map[string]*Folder)}
}

func newFile(name string) *File {
	return &File{name, make(map[string]*Symbol)}
}

func (f *File) sumSize() uint64 {
	var sum uint64
	for _, symbol := range f.Symbols {
		sum += symbol.Size
	}
	return sum
}

func (f *Folder) sumSize() uint64 {
	var sum uint64
	for _, folder := range f.Folders {
		sum += folder.sumSize()
	}

	for _, file := range f.Files {
		sum += file.sumSize()
	}
	return sum
}

func (f *Folder) getFolder(name string) *Folder {
	if nextF, ok := f.Folders[name]; ok {
		return nextF
	} else {
		f.Folders[name] = newFolder(name)
		return f.Folders[name]
	}
	return &Folder{} // cannot happen
}

func (f *Folder) getFile(name string) *File {
	if nextF, ok := f.Files[name]; ok {
		return nextF
	} else {
		f.Files[name] = newFile(name)
		return f.Files[name]
	}
	return &File{} // cannot happen
}

func (f *File) getSymbol(name string) *Symbol {
	if nextF, ok := f.Symbols[name]; ok {
		return nextF
	} else {
		f.Symbols[name] = &Symbol{name, "", 0}
		return f.Symbols[name]
	}
	return &Symbol{} // cannot happen
}

func (f *Folder) addFolder(path []string) *Folder {
	if len(path) == 1 {
		// last segment == new folder
		return f.getFolder(path[0])
	} else {
		return f.getFolder(path[0]).addFolder(path[1:])
	}
}

func (f *Folder) addFile(path []string) *File {
	if len(path) == 1 {
		// last segment == file
		return f.getFile(path[0])
	} else {
		return f.getFolder(path[0]).addFile(path[1:])
	}
}

func (f *Folder) addSymbol(symbol *Symbol, path string) *Symbol {
	segments := strings.Split(path, "/")
	file := f.addFile(segments)
	sym := file.getSymbol(symbol.Name)
	sym.Section = symbol.Section
	sym.Size += symbol.Size
	return sym
}

func (f *File) toString(fmtr outputFormatter, level int, total uint64) string {
	var str string
	if f.sumSize() <= 0 {
		return ""
	}
	size := f.sumSize()
	percent := 100 * float64(size) / float64(total)
	str += fmtr.Container(level, f.Name, size, percent)

	var sorted []string
	for symName := range f.Symbols {
		sorted = append(sorted, symName)
	}
	sort.Strings(sorted)
	for _, sym := range sorted {
		size := f.Symbols[sym].Size
		percent := 100 * float64(size) / float64(total)
		if f.Symbols[sym].Size > 0 {
			str += fmtr.Symbol(level + 1, f.Symbols[sym].Name, size, percent)
		}
	}
	return str
}

func (f *Folder) toString(fmtr outputFormatter, level int, total uint64) string {
	var str string

	var sorted []string
	for folderName := range f.Folders {
		sorted = append(sorted, folderName)
	}
	for fileName := range f.Files {
		sorted = append(sorted, fileName)
	}
	sort.Strings(sorted)

	for _, name := range sorted {
		if folder, ok := f.Folders[name]; ok {
			size := folder.sumSize()
			percent := 100 * float64(size) / float64(total)
			str += fmtr.Container(level, folder.Name, size, percent)
			str += folder.toString(fmtr, level+1, total)
		} else {
			str += f.Files[name].toString(fmtr, level, total)
		}
	}
	return str
}

func (f *Folder) ToString(total uint64, diffFriendly bool) string {
	var str string
	var fmtr outputFormatter

	if diffFriendly {
		fmtr = newSymbolFormatterDiffable()
	} else {
		fmtr = newSymbolFormatterDefault()
	}

	str += fmtr.Header("Path", "Size", "%")
	str += fmtr.Separator()
	str += f.toString(fmtr, 0, total)
	str += fmtr.Separator()
	str += fmtr.Header("Total symbol size (i.e. excluding padding, etc.)",
		strconv.FormatUint(f.sumSize(), 10), "")
	return str
}
