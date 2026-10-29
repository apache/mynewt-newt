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

package tui

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/apache/mynewt-artifact/flash"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

// Apps on BSPs with a bootloader flash area run under a bootloader such as
// mcuboot: the bootloader is a target of its own, loaded once, and the app
// is loaded as an image (.img) made by "newt create-image".

const imageMagic = 0x96f3b83d

// readImageVersion returns the version in a mynewt image's header.
func readImageVersion(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hdr := make([]byte, 32)
	if _, err := f.ReadAt(hdr, 0); err != nil {
		return "", err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != imageMagic {
		return "", util.FmtNewtError("%s is not a mynewt image", path)
	}
	major, minor := hdr[20], hdr[21]
	rev := binary.LittleEndian.Uint16(hdr[22:24])
	build := binary.LittleEndian.Uint32(hdr[24:28])
	return fmt.Sprintf("%d.%d.%d.%d", major, minor, rev, build), nil
}

// targetImagePath is where "newt create-image" puts a target's image.
func targetImagePath(t *target.Target) string {
	if t.App() == nil {
		return ""
	}
	elf := builder.AppElfPath(t.FullName(), builder.BUILD_NAME_APP,
		t.App().FullName())
	return strings.TrimSuffix(elf, ".elf") + ".img"
}

// bspFlashAreas returns the flash areas of a BSP, as newt's builder reads
// them.
func bspFlashAreas(lpkg *pkg.LocalPackage) map[string]flash.FlashArea {
	bsp, err := pkg.NewBspPackage(lpkg, nil)
	if err != nil {
		return nil
	}
	return bsp.FlashMap.Areas
}

// bspHasBootloader tells whether a BSP's flash map has a bootloader area.
func bspHasBootloader(bsp *pkg.LocalPackage, cache map[string]bool) bool {
	if bsp == nil {
		return false
	}
	if has, ok := cache[bsp.FullName()]; ok {
		return has
	}
	_, has := bspFlashAreas(bsp)["FLASH_AREA_BOOTLOADER"]
	cache[bsp.FullName()] = has
	return has
}

// addBootSupport adds the bootloader and image information and actions to
// the target entries.
func addBootSupport(proj *project.Project, entries []*Entry,
	targets map[string]*target.Target) {

	cache := map[string]bool{}

	// Bootloader targets by BSP, local ones first.
	boots := map[string][]*Entry{}
	for _, e := range entries {
		t := targets[e.FullName]
		if isBootloaderApp(t.AppName) {
			boots[t.BspName] = append(boots[t.BspName], e)
		}
	}

	for _, e := range entries {
		t := targets[e.FullName]
		if isBootloaderApp(t.AppName) {
			e.IsBoot = true
			continue
		}
		if t.App() == nil || !bspHasBootloader(t.Bsp(), cache) {
			continue
		}

		e.NeedsBoot = true
		if path := targetImagePath(t); path != "" {
			e.ImagePath = path
			e.ElfPath = strings.TrimSuffix(path, ".img") + ".elf"
			e.ImagesDir = filepath.Join(builder.TargetBinDir(t.FullName()),
				"images")
			e.App = t.App().FullName()
			e.Image, _ = readImageVersion(path)
		}
		// Prefer a local bootloader target over one from a repo.
		for _, b := range boots[t.BspName] {
			if e.Boot == "" || !b.Foreign {
				e.Boot, e.BootName = b.FullName, b.Name
			}
			if !b.Foreign {
				break
			}
		}
		for _, b := range boots[t.BspName] {
			if b.FullName == e.Boot {
				b.BootFor = append(b.BootFor, e.Name)
			}
		}

		e.Actions = append(e.Actions, Action{Key: "i", Label: "create image",
			Args: []string{"create-image", e.FullName}})
		if e.Boot != "" {
			e.Links = append(e.Links, Link{"B", "bootloader", KindTarget,
				e.Boot})
			version := e.Image
			if version == "" {
				version = nextVersion(listImages(e))
			}
			e.Actions = append(e.Actions, Action{Key: "F",
				Label: "flash bootloader + app", Args: []string{"load",
					e.FullName}, PickDevice: true,
				Steps: [][]string{
					{"build", e.Boot},
					{"load", e.Boot},
					{"build", e.FullName},
					{"create-image", e.FullName, version},
					{"load", e.FullName},
				}})
		}

		// "newt load" of such an app loads one of its images.
		for i := range e.Actions {
			a := &e.Actions[i]
			if a.runs("load") && a.Steps == nil {
				a.ChooseImage = true
			}
			if a.runs("clean") && savedImages(e) > 0 {
				a.Confirm = fmt.Sprintf("Delete the build output of %s, "+
					"including %s?", e.Name, plural(savedImages(e),
					"saved image"))
			}
		}
	}
}

// bootPreview describes a target's bootloader and image.
func bootPreview(e *Entry) []string {
	switch {
	case e.IsBoot && len(e.BootFor) > 0:
		return []string{field("Boots", strings.Join(e.BootFor, ", "))}
	case e.IsBoot:
		return []string{field("Boots", styleDim.Render(
			"no app target uses this bootloader's bsp"))}
	case !e.NeedsBoot:
		return nil
	}

	lines := []string{}
	if e.Boot != "" {
		lines = append(lines, field("Boot", e.BootName+
			styleDim.Render("  (gB to go there, F flashes both)")))
	} else {
		lines = append(lines, field("Boot", styleErr.Render(
			"needs a bootloader")), styleDim.Render("          Create a "+
			"target (n) with app @mcuboot/boot/mynewt and the same bsp."))
	}
	if e.Image != "" {
		lines = append(lines, field("Image", e.Image))
	} else {
		lines = append(lines, field("Image", styleDim.Render(
			"none since the last build (L creates one)")))
	}
	if n := savedImages(e); n > 0 {
		lines = append(lines, field("Saved", plural(n, "image")+
			styleDim.Render(" to choose from when loading")))
	}
	return lines
}

// createImage asks for the image version, then runs "newt create-image".
func (m *Model) createImage(a Action) tea.Cmd {
	e := m.localTarget()
	if e == nil {
		return nil
	}
	version := e.Image
	if version == "" {
		version = nextVersion(listImages(e))
	}
	name, full := e.Name, e.FullName
	m.prompt = &prompt{
		label: "Image version for " + name + ": ",
		input: version,
		title: "Create the image of " + name,
		help: [][2]string{
			{"type", "the version, e.g. 1.2.0 or 1.2.0.3"},
			{"Enter", "run newt create-image"},
			{"Esc", "cancel"},
		},
		done: func(m *Model, version string) tea.Cmd {
			return m.runOps("image "+version+" of "+name, full, false,
				[]string{"create-image", full, version})
		},
	}
	return nil
}
