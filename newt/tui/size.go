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
	"regexp"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// PkgSize is the flash and RAM a package takes in a build.
type PkgSize struct {
	// Archive name from "newt size", e.g. "@apache-mynewt-core_hw_hal.a".
	Archive string
	// Package it was built from, if known.
	Pkg   string
	Flash int64
	RAM   int64
}

// ImageSize is the result of "newt size".
type ImageSize struct {
	Pkgs []PkgSize
	// Totals from the "objsize" table: text+data and data+bss.
	Flash int64
	RAM   int64
}

var (
	sizeRowRe    = regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\S+)\s*$`)
	objsizeRowRe = regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+(\d+)\s+\d+\s+[0-9a-f]+\s+\S+\.elf\s*$`)
)

// parseSize reads the output of "newt size <target>".
func parseSize(out string) *ImageSize {
	size := &ImageSize{}
	for _, line := range strings.Split(out, "\n") {
		if m := objsizeRowRe.FindStringSubmatch(line); m != nil {
			text, _ := strconv.ParseInt(m[1], 10, 64)
			data, _ := strconv.ParseInt(m[2], 10, 64)
			bss, _ := strconv.ParseInt(m[3], 10, 64)
			size.Flash = text + data
			size.RAM = data + bss
			continue
		}
		if m := sizeRowRe.FindStringSubmatch(line); m != nil {
			boot, _ := strconv.ParseInt(m[1], 10, 64)
			flash, _ := strconv.ParseInt(m[2], 10, 64)
			ram, _ := strconv.ParseInt(m[3], 10, 64)
			size.Pkgs = append(size.Pkgs, PkgSize{Archive: m[5],
				Flash: boot + flash, RAM: ram})
		}
	}
	return size
}

// archiveName is the name newt gives a package's archive.
func archiveName(pkgName string) string {
	return strings.ReplaceAll(pkgName, "/", "_") + ".a"
}

// matchPackages fills in the package names of the archives.
func (size *ImageSize) matchPackages(info *BuildInfo) {
	byArchive := map[string]string{}
	if info != nil {
		for name := range info.Revdeps {
			byArchive[archiveName(name)] = name
		}
	}
	for i := range size.Pkgs {
		size.Pkgs[i].Pkg = byArchive[size.Pkgs[i].Archive]
	}
}

// displayName is a package's name in the size list.
func (p PkgSize) displayName() string {
	switch {
	case p.Pkg != "":
		return shortName(p.Pkg)
	case p.Archive == "*fill*":
		return "(alignment padding)"
	case strings.Contains(p.Archive, "-sysinit-"):
		return "(generated sysinit)"
	default:
		return strings.TrimSuffix(p.Archive, ".a")
	}
}

func bar(n int64, total int64, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	filled := int(n * int64(width) / total)
	if n > 0 && filled == 0 {
		return "▏"
	}
	return strings.Repeat("█", filled)
}

func percent(n int64, total int64) string {
	if total <= 0 {
		return ""
	}
	return fmt.Sprintf("%.0f%%", float64(n)*100/float64(total))
}

type sizeMsg struct {
	target string
	out    string
	err    error
}

// openSize runs "newt size" in the background; the panel opens when it is
// done.
func (m *Model) openSize() tea.Cmd {
	e := m.Selected()
	if e == nil {
		return nil
	}
	run := m.newtRun
	full, name := e.FullName, e.Name
	m.setStatus("measuring " + name + "...")
	return func() tea.Msg {
		out, err := run([]string{"size", full})
		return sizeMsg{name, out, err}
	}
}

func (m *Model) sizeDone(msg sizeMsg) {
	e := m.Selected()
	if e == nil || e.Name != msg.target {
		return
	}
	size := parseSize(msg.out)
	if msg.err != nil || len(size.Pkgs) == 0 {
		err := msg.err
		if err == nil {
			err = fmt.Errorf("no size information")
		}
		why := errorLine(msg.out, err)
		if strings.Contains(why, "no such file") ||
			strings.Contains(why, "not built") {
			why = "build it first (b)"
		}
		m.setError("size of " + msg.target + ": " + why)
		return
	}

	var info *BuildInfo
	if e.Syscfg != nil {
		info, _ = e.Syscfg()
	}
	size.matchPackages(info)
	m.openPanel(m.sizePanel(msg.target, size, info))
	m.setStatus(fmt.Sprintf("%s: %s flash, %s RAM", msg.target,
		humanSize(size.Flash), humanSize(size.RAM)))
}

func (m *Model) sizePanel(target string, size *ImageSize, info *BuildInfo) *panel {
	byRAM := false
	sorted := func() []PkgSize {
		pkgs := append([]PkgSize{}, size.Pkgs...)
		sort.SliceStable(pkgs, func(i, j int) bool {
			if byRAM {
				return pkgs[i].RAM > pkgs[j].RAM
			}
			return pkgs[i].Flash > pkgs[j].Flash
		})
		return pkgs
	}

	var slot, ram int64
	if info != nil {
		slot, ram = info.FlashSlot, info.RAMSize
	}

	totalLine := func() string {
		s := "Total  " + humanSize(size.Flash) + " flash"
		if slot > 0 {
			s += " of " + humanSize(slot) + " (" + percent(size.Flash, slot) + ")"
		}
		s += ", " + humanSize(size.RAM) + " RAM"
		if ram > 0 {
			s += " of " + humanSize(ram) + " (" + percent(size.RAM, ram) + ")"
		}
		return s
	}

	p := &panel{
		title: "size of " + target + " by flash",
		rows: func(width int) []row {
			pkgs := sorted()
			rows := []row{{text: totalLine(), style: styleBold}}

			var maxVal int64
			for _, pk := range pkgs {
				maxVal = max(maxVal, pk.Flash, pk.RAM)
			}
			nameW := max(width-30, 12)
			for _, pk := range pkgs {
				val := pk.Flash
				if byRAM {
					val = pk.RAM
				}
				name := clip(pk.displayName(), nameW)
				text := fmt.Sprintf("%-*s %8s %-10s", nameW, name,
					humanSize(val), bar(val, maxVal, 10))
				style := kindStyle(KindLib)
				if pk.Pkg == "" {
					style = styleDim
				}
				rows = append(rows, row{text: text, style: style})
			}
			return rows
		},
		preview: func(i int) []string {
			if i == 0 {
				return m.sizeSummary(target, size, info)
			}
			return m.pkgSizePreview(sorted()[i-1], size, info)
		},
		keys: []panelKey{{key: "o", help: "sort by RAM / flash",
			fn: func(m *Model, i int) tea.Cmd {
				byRAM = !byRAM
				m.panel.title = "size of " + target + " by flash"
				if byRAM {
					m.panel.title = "size of " + target + " by RAM"
				}
				return nil
			}}},
	}
	return p
}

func (m *Model) sizeSummary(target string, size *ImageSize, info *BuildInfo) []string {
	lines := []string{styleBold.Render(target), ""}
	lines = append(lines, field("Flash", humanSize(size.Flash)))
	if info != nil && info.FlashSlot > 0 {
		lines = append(lines, field("Slot", humanSize(info.FlashSlot)+
			styleDim.Render(" flash area for the image")),
			field("Used", percent(size.Flash, info.FlashSlot)+"  "+
				bar(size.Flash, info.FlashSlot, 20)))
	}
	lines = append(lines, "", field("RAM", humanSize(size.RAM)))
	if info != nil && info.RAMSize > 0 {
		lines = append(lines, field("Chip RAM", humanSize(info.RAMSize)),
			field("Used", percent(size.RAM, info.RAMSize)+"  "+
				bar(size.RAM, info.RAMSize, 20)))
	}
	lines = append(lines, "", styleDim.Render(fmt.Sprintf(
		"%d packages. Move down to see what each one costs and why it is "+
			"part of the build; o sorts by RAM.", len(size.Pkgs))))
	return lines
}

func (m *Model) pkgSizePreview(pk PkgSize, size *ImageSize, info *BuildInfo) []string {
	lines := []string{styleBold.Render(pk.displayName()), ""}
	if pk.Pkg != "" {
		lines = append(lines, field("Package", pk.Pkg))
	} else {
		lines = append(lines, field("Archive", pk.Archive))
	}
	lines = append(lines,
		field("Flash", humanSize(pk.Flash)+"  "+styleDim.Render(
			percent(pk.Flash, size.Flash)+" of the image")),
		field("RAM", humanSize(pk.RAM)+"  "+styleDim.Render(
			percent(pk.RAM, size.RAM)+" of the image")))

	if pk.Pkg == "" {
		return lines
	}

	lines = append(lines, section("Why it is in the build")...)
	chain := info.whyIncluded(pk.Pkg)
	if len(chain) == 0 {
		lines = append(lines, styleDim.Render("unknown"))
	}
	// Each package is shown with the reason its parent depends on it.
	for i, step := range chain {
		text := shortName(step.Pkg)
		if i > 0 {
			text = strings.Repeat("  ", i-1) + "└ " + text
			if why := chain[i-1].Why; why != "" {
				text += styleDim.Render("  " + why)
			}
		}
		lines = append(lines, text)
	}

	if deps := info.Revdeps[pk.Pkg]; len(deps) > 1 {
		lines = append(lines, section(fmt.Sprintf("All %d packages that "+
			"depend on it", len(deps)))...)
		sort.Slice(deps, func(i, j int) bool { return deps[i].Pkg < deps[j].Pkg })
		for _, d := range deps {
			line := shortName(d.Pkg)
			if d.Why != "" {
				line += styleDim.Render("  " + d.Why)
			}
			lines = append(lines, line)
		}
	}
	return lines
}
