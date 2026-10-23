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
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// The board picker opens on "load" and "debug": it lists the connected
// boards so that the command runs on the chosen one.

type devicePicker struct {
	action      Action
	returnDepth int
	scanning    bool
	devices     []Device
	err         error
	list        list
	// Chip details by serial number, filled in after the scan.
	chips map[string]*chipState
}

type chipState struct {
	info *ChipInfo
	err  error
}

type devicesMsg struct {
	devices []Device
	err     error
}

type chipMsg struct {
	serial string
	info   *ChipInfo
	err    error
}

// readChips reads the chip details of the found boards one at a time, as
// each read connects to a probe.
func (m *Model) readChips() tea.Cmd {
	read := m.readChip
	cmds := []tea.Cmd{}
	for _, d := range m.picker.devices {
		if d.Source != "nrfutil" {
			continue
		}
		serial := d.Serial
		cmds = append(cmds, func() tea.Msg {
			info, err := read(serial)
			return chipMsg{serial, info, err}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Sequence(cmds...)
}

func (m *Model) chipFound(msg chipMsg) {
	if m.depth != depthDevice || m.picker.chips == nil {
		return
	}
	m.picker.chips[msg.serial] = &chipState{msg.info, msg.err}
}

// chip returns what is known about the chip on a board, or nil while it is
// being read.
func (m *Model) chip(d *Device) *chipState {
	if d == nil {
		return nil
	}
	return m.picker.chips[d.Serial]
}

// mcuTarget is the chip the selected target is built for, if the bsp says.
func (m *Model) mcuTarget() string {
	e := m.Selected()
	if e == nil || e.Syscfg == nil {
		return ""
	}
	info, err := e.Syscfg()
	if err != nil {
		return ""
	}
	if s := findSetting(info, "MCU_TARGET"); s != nil {
		return s.Value
	}
	return ""
}

// chipMismatch describes why the image will not run on a board, if so.
func (m *Model) chipMismatch(d *Device) string {
	c := m.chip(d)
	if c == nil || c.info == nil || c.info.Part == "" {
		return ""
	}
	want := m.mcuTarget()
	if chipMatches(c.info.Part, want) {
		return ""
	}
	return fmt.Sprintf("this board has an %s, but the target is built "+
		"for %s", c.info.Part, want)
}

func (m *Model) scanDevices() tea.Cmd {
	m.picker.scanning = true
	find := m.findDevices
	return func() tea.Msg {
		devices, err := find()
		return devicesMsg{devices, err}
	}
}

func (m *Model) openDevicePicker(a Action) tea.Cmd {
	m.picker = devicePicker{action: a, returnDepth: m.depth,
		chips: map[string]*chipState{}}
	m.depth = depthDevice
	return m.scanDevices()
}

// pickerTool is the tool that will use the serial number, which decides its
// format.  The debug scripts of the BSPs use J-Link.
func (m *Model) pickerTool() string {
	if m.picker.action.Args[0] == "load" {
		if e := m.Selected(); e != nil {
			for _, f := range e.Fields {
				if f.Var == downloaderSetting {
					f.ensure()
					return f.Value
				}
			}
		}
	}
	return "jlink"
}

// defaultDevice is the board the target is set up to use, if any.
func (m *Model) defaultDevice() string {
	e := m.Selected()
	if e == nil || e.Syscfg == nil {
		return ""
	}
	info, err := e.Syscfg()
	if err != nil {
		return ""
	}
	if s := findSetting(info, deviceSetting); s != nil {
		return s.Value
	}
	return ""
}

func (m *Model) devicesFound(msg devicesMsg) tea.Cmd {
	if m.depth != depthDevice {
		return nil
	}
	p := &m.picker
	p.scanning = false
	p.devices = msg.devices
	p.err = msg.err

	// Start on the target's default board.
	p.list = list{}
	dflt := m.defaultDevice()
	for i, d := range p.devices {
		if sameDevice(d.Serial, dflt) {
			p.list.cursor = i
		}
	}

	switch {
	case msg.err != nil:
		m.setError(msg.err.Error())
	case len(p.devices) == 0:
		m.setError("no boards found; connect one and press r to scan again")
	case len(p.devices) == 1:
		m.setStatus("found 1 board")
	default:
		m.setStatus(fmt.Sprintf("found %d boards", len(p.devices)))
	}

	p.chips = map[string]*chipState{}
	return m.readChips()
}

// pickedDevice returns the device under the cursor, or nil for "let the
// tool pick".
func (m *Model) pickedDevice() *Device {
	c := m.picker.list.cursor
	if c >= 0 && c < len(m.picker.devices) {
		return &m.picker.devices[c]
	}
	return nil
}

func (m *Model) deviceEnv(d *Device) []string {
	if d == nil {
		return nil
	}
	return []string{deviceEnv + "=" + deviceSN(m.pickerTool(), d.Serial)}
}

// pickDevice runs the command on the board under the cursor, after asking
// if the board's chip does not match the target.
func (m *Model) pickDevice(save bool) tea.Cmd {
	d := m.pickedDevice()
	if why := m.chipMismatch(d); why != "" {
		m.prompt = &prompt{confirm: true,
			label: why + "; " + m.picker.action.Label + " anyway? (y/N) ",
			done: func(m *Model, _ string) tea.Cmd {
				return m.runOnDevice(save)
			}}
		return nil
	}
	return m.runOnDevice(save)
}

func (m *Model) runOnDevice(save bool) tea.Cmd {
	p := m.picker
	d := m.pickedDevice()
	env := m.deviceEnv(d)

	if save {
		e := m.localTarget()
		if e == nil {
			return nil
		}
		if d == nil {
			m.setError("choose a board to make it the default")
			return nil
		}
		sn := deviceSN(m.pickerTool(), d.Serial)
		if err := e.SetSyscfg(deviceSetting, sn, false); err != nil {
			m.setError(err.Error())
			return nil
		}
		m.reload()
	}

	m.depth = p.returnDepth
	return m.runWith(p.action, env)
}

// deviceKey handles the keys of the board picker; it returns false if the
// key is not one of them.
func (m *Model) deviceKey(k string) (tea.Cmd, bool) {
	if m.depth != depthDevice {
		return nil, false
	}

	switch k {
	case "enter", "l":
		if m.picker.scanning {
			return nil, true
		}
		return m.pickDevice(false), true
	case "s":
		if m.picker.scanning {
			return nil, true
		}
		return m.pickDevice(true), true
	case "r":
		m.setStatus("looking for boards...")
		return m.scanDevices(), true
	case "esc":
		m.depth = m.picker.returnDepth
		m.setStatus("cancelled")
		return nil, true
	}
	return nil, false
}

func (m *Model) deviceRows() []row {
	if m.picker.scanning {
		return []row{{text: "looking for boards...", style: styleDim}}
	}

	dflt := m.defaultDevice()
	rows := []row{}
	for i := range m.picker.devices {
		d := &m.picker.devices[i]
		text := d.Title()
		if c := m.chip(d); c != nil && c.info != nil && c.info.Part != "" {
			text += " · " + c.info.Part
		}
		r := row{text: text, hint: d.Serial, style: kindStyle(KindTarget)}
		if sameDevice(d.Serial, dflt) {
			r.hint = "default · " + d.Serial
			r.style = r.style.Bold(true)
		}
		if m.chipMismatch(d) != "" {
			r.text = "⚠ " + r.text
			r.style = styleErr
		}
		rows = append(rows, r)
	}
	return append(rows, row{text: "let the tool pick", style: styleDim})
}

func (m *Model) devicePreviewLines() []string {
	cmd := "newt " + strings.Join(m.picker.action.Args, " ")

	if m.picker.scanning {
		return []string{styleDim.Render("Looking for connected boards...")}
	}
	if d := m.pickedDevice(); d != nil {
		lines := devicePreview(*d, strings.Join(m.deviceEnv(d), " ")+" "+cmd)
		lines = append(lines, m.chipPreview(d)...)
		return append(lines, m.debugNote()...)
	}

	lines := []string{styleBold.Render("Let the tool pick"), ""}
	if len(m.picker.devices) == 0 {
		lines = append(lines, styleErr.Render("No boards found."),
			"Connect a board, then press r to scan again.", "")
	}
	if dflt := m.defaultDevice(); dflt != "" {
		lines = append(lines, "Uses this target's default board ("+
			deviceSetting+" = "+dflt+").")
	} else {
		lines = append(lines, "Runs without choosing a board; the tool "+
			"uses the only connected one, or asks.")
	}
	lines = append(lines, section("Runs")...)
	return append(lines, cmd)
}

func (m *Model) devicePickerHelp() []helpGroup {
	verb := m.picker.action.Label
	g := helpGroup{title: "Choose a board for " + verb,
		style: kindStyle(KindTarget).Bold(true)}
	g.add("j/k", "down / up")
	if !m.picker.scanning {
		if m.pickedDevice() != nil {
			g.add("Enter", verb+" on this board")
			if e := m.Selected(); e != nil && !e.Foreign {
				g.add("s", verb+" and make it this target's default")
			}
		} else {
			g.add("Enter", verb+" and let the tool pick the board")
		}
	}
	g.add("r", "scan for boards again")
	g.add("h/Esc", "cancel")
	return []helpGroup{g, m.generalHelp()}
}

// chipPreview describes the chip on a board and how it fits the target.
func (m *Model) chipPreview(d *Device) []string {
	lines := section("Chip")
	want := m.mcuTarget()

	c := m.chip(d)
	switch {
	case d.Source != "nrfutil":
		return append(lines, styleDim.Render(
			"Install nrfutil to read the chip's details."))
	case c == nil:
		return append(lines, styleDim.Render("reading over SWD..."))
	case c.err != nil:
		return append(lines, styleErr.Render("could not read the chip: ")+
			c.err.Error(), styleDim.Render(
			"Is the board powered and the debug cable connected?"))
	}

	info := c.info
	part := info.Part
	if info.Variant != "" {
		part += " " + info.Variant
	}
	if info.Revision != "" {
		part += ", revision " + info.Revision
	}
	lines = append(lines, field("Part", styleBold.Render(part)))
	if info.Family != "" {
		lines = append(lines, field("Family", info.Family))
	}
	if info.Flash > 0 {
		lines = append(lines, field("Flash", humanSize(info.Flash)))
	}
	if info.RAM > 0 {
		lines = append(lines, field("RAM", humanSize(info.RAM)))
	}
	if info.Protection != "" {
		if info.Protected() {
			lines = append(lines, field("Access", styleErr.Render(
				info.Protection)))
		} else {
			lines = append(lines, field("Access", "not protected"))
		}
	}
	if info.ProbeFirmware != "" {
		lines = append(lines, field("Probe fw", info.ProbeFirmware))
	}

	if want != "" {
		lines = append(lines, "", field("Target", "built for "+want+
			styleDim.Render(" (MCU_TARGET)")))
	}
	if why := m.chipMismatch(d); why != "" {
		lines = append(lines, "", styleErr.Render("⚠ "+why+
			". The image will not run on this board."))
	} else if want != "" && info.Part != "" {
		lines = append(lines, styleOk.Render("✓ matches this board"))
	}
	if info.Protected() {
		lines = append(lines, "", styleErr.Render("⚠ The chip is "+
			info.Protection+"."), "Loading will fail until it is "+
			"recovered with: nrfutil device recover --serial-number "+
			d.Serial, styleDim.Render("Recovering erases the whole chip."))
	}
	return lines
}

// debugNote explains that "newt debug" attaches to what is on the board.
func (m *Model) debugNote() []string {
	if m.picker.action.Args[0] != "debug" {
		return nil
	}
	return append(section("Note"), "newt debug attaches to the firmware "+
		"already on the board; it does not load this build. If gdb shows "+
		"\"?? ()\" instead of functions, the board runs something else: "+
		"load it first (L, or F with the bootloader), or use r, which "+
		"builds, loads and debugs.")
}
