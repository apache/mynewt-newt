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

var outputFormatting string = "%-59s %9d %8.2f%%\n"

func (f *File) String(indent string, level int, total uint64) string {
	var str string
	if f.sumSize() <= 0 {
		return ""
	}
	size := f.sumSize()
	percent := 100 * float64(size) / float64(total)
	str += fmt.Sprintf(outputFormatting, strings.Repeat(indent, level)+
		f.Name, size, percent)

	var sorted []string
	for symName := range f.Symbols {
		sorted = append(sorted, symName)
	}
	sort.Strings(sorted)
	for _, sym := range sorted {
		size := f.Symbols[sym].Size
		percent := 100 * float64(size) / float64(total)
		if f.Symbols[sym].Size > 0 {
			str += fmt.Sprintf(outputFormatting,
				strings.Repeat(indent, level+1)+ f.Symbols[sym].Name,
				size, percent)
		}
	}
	return str
}

func (f *Folder) StringRec(indent string, level int, total uint64) string {
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
			str += fmt.Sprintf(outputFormatting,
				strings.Repeat(indent, level)+folder.Name, size, percent)
			str += folder.StringRec(indent, level+1, total)
		} else {
			str += f.Files[name].String(indent, level, total)
		}
	}
	return str
}

func (f *Folder) ToString(total uint64) string {
	indent := "  "
	var str string
	str += fmt.Sprintf("%-59s %9s %9s\n", "Path", "Size", "%")
	str += strings.Repeat("=", 79) + "\n"
	str += f.StringRec(indent, 0, total)
	str += strings.Repeat("=", 79) + "\n"
	str += fmt.Sprintf("%-59s %9d %9s\n",
		"Total symbol size (i.e. excluding padding, etc.)", f.sumSize(), "")
	return str
}
