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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"mynewt.apache.org/newt/util"
)

// Loading an app that runs under a bootloader loads one of its images.
// Besides the image "newt create-image" makes (which "newt build"
// deletes), the browser keeps a copy of every image it creates in the
// target's images directory, so that earlier versions can be loaded again.

// ImageFile is an image of a target that can be loaded.
type ImageFile struct {
	Path    string
	Version string
	Time    time.Time
	// The image newt made last, next to the build.
	Latest bool
	// Made after the target's last build.
	FromBuild bool
}

func imageName(app string, version string) string {
	return filepath.Base(app) + "-" + version + ".img"
}

// listImages returns a target's images, newest first.
func listImages(e *Entry) []ImageFile {
	var built time.Time
	if fi, err := os.Stat(e.ElfPath); err == nil {
		built = fi.ModTime()
	}

	images := []ImageFile{}
	add := func(path string, latest bool) {
		fi, err := os.Stat(path)
		if err != nil {
			return
		}
		ver, err := readImageVersion(path)
		if err != nil {
			return
		}
		images = append(images, ImageFile{Path: path, Version: ver,
			Time: fi.ModTime(), Latest: latest,
			FromBuild: !built.IsZero() && !fi.ModTime().Before(built)})
	}

	add(e.ImagePath, true)
	saved, _ := filepath.Glob(filepath.Join(e.ImagesDir, "*.img"))
	for _, path := range saved {
		// The latest image is usually saved too; show it once.
		if len(images) > 0 && images[0].Latest && sameFile(path, e.ImagePath) {
			continue
		}
		add(path, false)
	}

	sort.SliceStable(images, func(i, j int) bool {
		return images[i].Time.After(images[j].Time)
	})
	return images
}

// sameFile tells whether two files have the same contents.
func sameFile(a string, b string) bool {
	da, errA := os.ReadFile(a)
	db, errB := os.ReadFile(b)
	return errA == nil && errB == nil && string(da) == string(db)
}

// saveImage keeps a copy of the target's latest image in its images
// directory.
func saveImage(e *Entry) error {
	ver, err := readImageVersion(e.ImagePath)
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(e.ImagesDir, 0755); err != nil {
		return util.ChildNewtError(err)
	}
	src, err := os.Open(e.ImagePath)
	if err != nil {
		return util.ChildNewtError(err)
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(e.ImagesDir, imageName(e.App, ver)))
	if err != nil {
		return util.ChildNewtError(err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return util.ChildNewtError(err)
	}
	return nil
}

// savedImages counts the images in a target's images directory.
func savedImages(e *Entry) int {
	saved, _ := filepath.Glob(filepath.Join(e.ImagesDir, "*.img"))
	return len(saved)
}

// nextVersion suggests the version of a new image: the newest one with its
// revision increased, e.g. 1.2.3 after 1.2.2.
func nextVersion(images []ImageFile) string {
	newest := ""
	for _, img := range images {
		if newest == "" || compareVersions(img.Version, newest) > 0 {
			newest = img.Version
		}
	}
	if newest == "" {
		return "1.0.0"
	}
	parts := strings.Split(newest, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	rev, _ := strconv.Atoi(parts[2])
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], rev+1)
}

func compareVersions(a string, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x - y
		}
	}
	return 0
}

// openLoad starts "load" for a target: apps that run under a bootloader
// first choose the image to load.
func (m *Model) openLoad(a Action) tea.Cmd {
	e := m.Selected()
	if e == nil || !e.NeedsBoot {
		return m.openDevicePicker(a)
	}
	m.openPanel(m.imageChooser(e, a))
	return nil
}

// imageChooser lists a target's images to load, and offers to create one.
func (m *Model) imageChooser(e *Entry, load Action) *panel {
	images := listImages(e)
	name, full := e.Name, e.FullName

	// The last row creates a new image.
	create := len(images)

	p := &panel{
		title: "load " + name + ": choose the image",
		rows: func(width int) []row {
			rows := []row{}
			for _, img := range images {
				hint := img.Time.Format("2006-01-02 15:04")
				style := kindStyle(KindValue)
				switch {
				case img.Latest:
					hint = "latest · " + hint
					style = style.Bold(true)
				case !img.FromBuild:
					style = styleDim
				}
				rows = append(rows, row{text: img.Version, hint: hint,
					style: style})
			}
			return append(rows, row{text: "create a new image…",
				style: kindStyle(KindTarget)})
		},
		preview: func(i int) []string {
			if i == create {
				return []string{styleBold.Render("Create a new image"), "",
					"Builds " + name + ", creates its image with a version " +
						"you choose (" + nextVersion(images) + " is " +
						"suggested) and loads it.", "",
					styleDim.Render("The image is also saved, so that it " +
						"can be loaded again later.")}
			}
			img := images[i]
			lines := []string{styleBold.Render("Image " + img.Version), "",
				field("Made", img.Time.Format(time.DateTime)),
				field("File", relToCwd(img.Path))}
			if img.FromBuild {
				lines = append(lines, field("Build", styleOk.Render(
					"from the current build")))
			} else {
				lines = append(lines, field("Build", styleDim.Render(
					"from an older build")))
			}
			if img.Latest {
				lines = append(lines, "", styleDim.Render("The image newt "+
					"made last; newt build deletes it, the saved copy stays."))
			}
			return lines
		},
		enterHelp: "load the selected image, or create one",
		enter: func(m *Model, i int) tea.Cmd {
			m.closePanel()
			if i == create {
				return m.createAndLoad(full, name, nextVersion(images), load)
			}
			a := load
			a.Args = []string{"load", "--imgfile", images[i].Path, full}
			a.Label = "load " + images[i].Version
			return m.openDevicePicker(a)
		},
		keys: []panelKey{{key: "x", help: "delete this saved image",
			when: func(i int) bool { return i < create && !images[i].Latest },
			fn: func(m *Model, i int) tea.Cmd {
				img := images[i]
				m.prompt = &prompt{confirm: true,
					label: "Delete saved image " + img.Version + "? (y/N) ",
					done: func(m *Model, _ string) tea.Cmd {
						if err := os.Remove(img.Path); err != nil {
							m.setError(err.Error())
							return nil
						}
						m.setStatus("deleted image " + img.Version)
						if e := m.Selected(); e != nil {
							m.openPanel(m.imageChooser(e, load))
						}
						return nil
					}}
				return nil
			}}},
	}
	return p
}

// createAndLoad asks for a version, then builds, creates the image and
// loads it on a board.
func (m *Model) createAndLoad(full string, name string, version string,
	load Action) tea.Cmd {

	m.prompt = &prompt{
		label: "Version of the new image of " + name + ": ",
		input: version,
		title: "Create an image and load it",
		help: [][2]string{
			{"type", "the version, e.g. 1.2.0 or 1.2.0.3"},
			{"Enter", "choose the board, then build, create and load"},
			{"Esc", "cancel"},
		},
		done: func(m *Model, version string) tea.Cmd {
			a := load
			a.Label = "build, create image " + version + " and load"
			a.Steps = [][]string{
				{"build", full},
				{"create-image", full, version},
				{"load", full},
			}
			return m.openDevicePicker(a)
		},
	}
	return nil
}

// imageCreated saves the target's new image after "newt create-image".
func (m *Model) imageCreated(target string) {
	ci := m.findKind(KindTarget)
	if ci < 0 {
		return
	}
	for _, e := range m.cats[ci].Entries {
		if e.Name == target && e.ImagesDir != "" {
			if err := saveImage(e); err != nil {
				m.setError("the image was not saved: " + err.Error())
			}
		}
	}
}

func createsImage(steps [][]string) bool {
	for _, s := range steps {
		if len(s) > 0 && s[0] == "create-image" {
			return true
		}
	}
	return false
}

// relToCwd shortens a path for display; the browser runs in the project's
// directory.
func relToCwd(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil &&
			!strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return path
}
