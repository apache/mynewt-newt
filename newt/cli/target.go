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
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const TARGET_SECT_PREFIX = "_target_"

type Target struct {
	Vars map[string]string

	Identities map[string]string

	Capabilities []string

	Dependencies []string

	Cflags string
	Lflags string
	Aflags string

	Name string

	Arch string
	Cdef string

	Bsp string

	Repo *Repo
}

// Check if the target specified by name exists for the Repo specified by
// r
func TargetExists(repo *Repo, name string) bool {
	_, err := repo.GetConfig(TARGET_SECT_PREFIX+name, "name")
	if err == nil {
		return true
	} else {
		return false
	}
}

func parseTargetStringSlice(str string) ([]string, error) {
	slice := strings.Split(str, " ")
	return slice, nil
}

func (t *Target) SetDefaults() error {
	var err error

	t.Name = t.Vars["name"]

	// Must have an architecture set, default to sim.
	if t.Vars["arch"] == "" {
		t.Vars["arch"] = "sim"
		t.Arch = "sim"
	} else {
		t.Arch = t.Vars["arch"]
	}

	t.Cdef = t.Vars["compiler_def"]
	if t.Cdef == "" {
		t.Cdef = "default"
	}

	t.Bsp = t.Vars["bsp"]
	t.Cflags = t.Vars["cflags"]
	t.Lflags = t.Vars["lflags"]

	identities, err := parseTargetStringSlice(t.Vars["identities"])
	if err != nil {
		return err
	}
	t.Identities = map[string]string{}
	for _, ident := range identities {
	StatusMessage(VERBOSITY_VERBOSE, "  set default ident %s\n", ident)
		t.Identities[ident] = t.Name
	}
	t.Capabilities, err = parseTargetStringSlice(t.Vars["capabilities"])
	if err != nil {
		return err
	}

	t.Dependencies, err = parseTargetStringSlice(t.Vars["dependencies"])
	if err != nil {
		return err
	}

	return nil
}

func (t *Target) HasIdentity(identity string) bool {
	for cur, _ := range t.Identities {
		if cur == identity {
			return true
		}
	}

	return false
}

// Load the target specified by name for the repository specified by r
func LoadTarget(repo *Repo, name string) (*Target, error) {
	t := &Target{
		Repo: repo,
	}

	var err error

	t.Vars, err = repo.GetConfigSect(TARGET_SECT_PREFIX + name)
	if err != nil {
		return nil, err
	}

	// Cannot have both a project and package set
	err = t.SetDefaults()
	if err != nil {
		return nil, err
	}

	return t, nil
}

// Export a target, or all targets.  If exportAll is true, then all targets are exported, if false,
// then only the target represented by targetName is exported
func ExportTargets(repo *Repo, name string, exportAll bool, fp *os.File) error {
	targets, err := GetTargets(repo)
	if err != nil {
		return err
	}

	for _, target := range targets {
		log.Printf("[DEBUG] Exporting target %s", target.Name)

		if !exportAll && target.Name != name {
			continue
		}

		fmt.Fprintf(fp, "@target=%s\n", target.Name)

		for k, v := range target.GetVars() {
			fmt.Fprintf(fp, "%s=%s\n", k, v)
		}
	}
	fmt.Fprintf(fp, "@endtargets\n")

	return nil
}

func ImportTargets(repo *Repo, name string, importAll bool, fp *os.File) error {
	s := bufio.NewScanner(fp)

	var currentTarget *Target = nil

	targets := make([]*Target, 0, 10)

	if importAll {
		StatusMessage(VERBOSITY_VERBOSE, "Importing all targets from %s",
			fp.Name())
	} else {
		StatusMessage(VERBOSITY_VERBOSE, "Importing target %s from %s",
			name, fp.Name())
	}

	for s.Scan() {
		line := s.Text()

		// scan lines
		// lines defining a target start with @
		if idx := strings.Index(line, "@"); idx == 0 {
			// save existing target if it exists
			if currentTarget != nil {
				targets = append(targets, currentTarget)
				currentTarget = nil
			}

			// look either for an end of target definitions, or a new target definition
			if line == "@endtargets" {
				break
			} else {
				elements := strings.SplitN(line, "=", 2)
				// name is elements[0], and value is elements[1]

				if importAll || elements[1] == name {
					// create a current target
					currentTarget = &Target{
						Repo: repo,
					}

					var err error
					currentTarget.Vars = map[string]string{}
					if err != nil {
						return err
					}

					currentTarget.Vars["name"] = elements[1]
				}
			}
		} else {
			if currentTarget != nil {
				// target variables, set these on the current target
				elements := strings.SplitN(line, "=", 2)
				currentTarget.Vars[elements[0]] = elements[1]
			}
		}
	}

	if err := s.Err(); err != nil {
		return err
	}

	for _, target := range targets {
		if err := target.SetDefaults(); err != nil {
			return err
		}

		if err := target.Save(); err != nil {
			return err
		}
	}

	return nil
}

// Get a list of targets for the repository specified by r
func GetTargets(repo *Repo) ([]*Target, error) {
	targets := []*Target{}
	for sect, _ := range repo.Config {
		if strings.HasPrefix(sect, TARGET_SECT_PREFIX) {
			target, err := LoadTarget(repo, sect[len(TARGET_SECT_PREFIX):len(sect)])
			if err != nil {
				return nil, err
			}

			targets = append(targets, target)
		}
	}
	return targets, nil
}

// Get a map[] of variables for this target
func (t *Target) GetVars() map[string]string {
	return t.Vars
}

// Return the compiler definition file for this target
func (t *Target) GetCompiler() string {
	path := t.Repo.BasePath + "/compiler/"
	if t.Vars["compiler"] != "" {
		path += t.Vars["compiler"]
	} else {
		path += t.Arch
	}
	path += "/"

	return path
}

// Build the target
func (t *Target) Build() error {
	if t.Vars["project"] != "" {
		StatusMessage(VERBOSITY_DEFAULT, "Building target %s (project = %s)\n",
			t.Name, t.Vars["project"])
		// Now load and build the project.
		p, err := LoadProject(t.Repo, t, t.Vars["project"])
		if err != nil {
			return err
		}
		// The project is the target, and builds itself.
		if err = p.Build(); err != nil {
			return err
		}
	} else if t.Vars["pkg"] != "" {
		pkgList, err := NewPkgList(t.Repo)
		if err != nil {
			return err
		}

		err = pkgList.Build(t, t.Vars["pkg"], nil, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Target) BuildClean(cleanAll bool) error {
	if t.Vars["project"] != "" {
		p, err := LoadProject(t.Repo, t, t.Vars["project"])
		if err != nil {
			return err
		}

		// The project is the target, and build cleans itself.
		if err = p.BuildClean(cleanAll); err != nil {
			return err
		}
	} else if t.Vars["pkg"] != "" {
		pkgList, err := NewPkgList(t.Repo)
		if err != nil {
			return err
		}
		err = pkgList.BuildClean(t, t.Vars["pkg"], cleanAll)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Target) Test(cmd string, flag bool) error {
	pkgList, err := NewPkgList(t.Repo)
	if err != nil {
		return err
	}

	switch cmd {
	case "test":
		err = pkgList.Test(t, t.Vars["pkg"], flag)
	case "testclean":
		err = pkgList.TestClean(t, t.Vars["pkg"], flag)
	default:
		err = NewNewtError("Unknown command to Test() " + cmd)
	}
	if err != nil {
		return err
	}

	return nil
}

func (t *Target) DeleteVar(name string) error {
	targetCfgSect := TARGET_SECT_PREFIX + t.Vars["name"]

	if err := t.Repo.DelConfig(targetCfgSect, name); err != nil {
		return err
	}

	return nil
}

// Save the target's configuration elements
func (t *Target) Save() error {
	repo := t.Repo

	if _, ok := t.Vars["name"]; !ok {
		return NewNewtError("Cannot save a target without a name")
	}

	targetCfg := TARGET_SECT_PREFIX + t.Vars["name"]

	for k, v := range t.Vars {
		if err := repo.SetConfig(targetCfg, k, v); err != nil {
			return err
		}
	}

	return nil
}

func (t *Target) Remove() error {
	repo := t.Repo

	if _, ok := t.Vars["name"]; !ok {
		return NewNewtError("Cannot remove a target without a name")
	}

	cfgSect := TARGET_SECT_PREFIX + t.Vars["name"]

	for k, _ := range t.Vars {
		if err := repo.DelConfig(cfgSect, k); err != nil {
			return err
		}
	}

	return nil
}

func (t *Target) Download() error {
	pkgList, err := NewPkgList(t.Repo)
	if err != nil {
		return err
	}

	pkg, err := pkgList.ResolvePkgName(t.Bsp)
	if err != nil {
		return err
	}

	err = pkg.LoadConfig(t, false)
	if err != nil {
		return err
	}
	if pkg.DownloadScript == "" {
		return NewNewtError(fmt.Sprintf("No pkg.downloadscript defined for %s",
			pkg.FullName))
	}
	downloadScript := filepath.Join(pkg.BasePath, pkg.DownloadScript)

	if t.Vars["project"] == "" {
		return NewNewtError(fmt.Sprintf("No project associated with target %s",
			t.Name))
	}
	p, err := LoadProject(t.Repo, t, t.Vars["project"])
	if err != nil {
		return err
	}

	os.Chdir(t.Repo.BasePath)

	identString := ""
	for ident, _ := range t.Identities {
		identString = identString + ident
	}

	StatusMessage(VERBOSITY_DEFAULT, "Downloading with %s\n", downloadScript)
	rsp, err := ShellCommand(fmt.Sprintf("%s %s %s", downloadScript,
		filepath.Join(p.BinPath(), p.Name), identString))
	if err != nil {
		StatusMessage(VERBOSITY_DEFAULT, "%s", rsp);
		return err
	}

	return nil
}

func (t *Target) Debug() error {
	pkgList, err := NewPkgList(t.Repo)
	if err != nil {
		return err
	}

	pkg, err := pkgList.ResolvePkgName(t.Bsp)
	if err != nil {
		return err
	}

	err = pkg.LoadConfig(t, false)
	if err != nil {
		return err
	}
	if pkg.DebugScript == "" {
		return NewNewtError(fmt.Sprintf("No pkg.debugscript defined for %s",
			pkg.FullName))
	}
	debugScript := filepath.Join(pkg.BasePath, pkg.DebugScript)

	if t.Vars["project"] == "" {
		return NewNewtError(fmt.Sprintf("No project associated with target %s",
			t.Name))
	}
	p, err := LoadProject(t.Repo, t, t.Vars["project"])
	if err != nil {
		return err
	}

	os.Chdir(t.Repo.BasePath)

	identString := ""
	for ident, _ := range t.Identities {
		identString = identString + ident
	}

	StatusMessage(VERBOSITY_DEFAULT, "Debugging with %s %s\n", debugScript, p.Name)

	cmdLine := []string{debugScript, filepath.Join(p.BinPath(), p.Name)}
	cmdLine = append(cmdLine, identString)
	err = ShellInteractiveCommand(cmdLine)
	if err != nil {
		return err
	}

	return nil
}

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
		return nil, nil, err
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
				return nil, nil, NewNewtError("Can't parse mem info")
			}
			size, err := strconv.ParseUint(array[2], 0, 64)
			if err != nil {
				return nil, nil, NewNewtError("Can't parse mem info")
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
		StatusMessage(VERBOSITY_VERBOSE, "Mem %s: 0x%x-0x%x\n",
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

func (t *Target) GetSize() (string, error) {
	if t.Vars["project"] != "" {
		StatusMessage(VERBOSITY_DEFAULT, "Inspecting target %s (project = %s)\n",
			t.Name, t.Vars["project"])
		// Now load the project, mapfile settings
		p, err := LoadProject(t.Repo, t, t.Vars["project"])
		if err != nil {
			return "", err
		}

		c, err := NewCompiler(t.GetCompiler(), t.Cdef, t.Name, []string{})
		if err != nil {
			return "", err
		}
		if c.ldMapFile != true {
			return "", NewNewtError("Build does not generate mapfile")
		}
		mapFile := p.BinPath() + p.Name + ".elf.map"

		pkgSizes, memSections, err := ParseMapFileSizes(mapFile)
		if err != nil {
			return "", err
		}
		return PrintSizes(pkgSizes, memSections)
	}
	return "", NewNewtError("Target needs a project")
}
