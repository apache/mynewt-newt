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
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The boards pane, on the right of the screen, lists the connected boards
// and keeps the list up to date.  It resets, erases and recovers the board
// under its cursor.  It is open from the start, unless the user hid it
// (remembered in tui.yml as "boards: hidden").

// boardsWidth is the width of the pane; it is not shown on terminals too
// narrow for it and the columns.
const (
	boardsWidth    = 34
	boardsMinWidth = 100
)

// How often the pane looks for boards.  Listing boards does not connect to
// their chips; reading a chip's details does, so that is done once per
// board.
var boardsInterval = 5 * time.Second

type boardPane struct {
	shown    bool
	focus    bool
	scanning bool
	reading  bool
	// Increased each time the pane is shown, so that the timers of an
	// earlier time stop.
	gen     int
	devices []Device
	err     error
	list    list
	// Chip details by serial number; nil while being read.
	chips map[string]*chipState
	// What runs on a board, e.g. "resetting", by serial number.
	busy map[string]string
}

type boardsMsg struct {
	gen     int
	devices []Device
	err     error
}

type boardsTickMsg struct{ gen int }

type boardChipMsg struct {
	serial string
	info   *ChipInfo
	err    error
}

type boardActionMsg struct {
	serial string
	action boardAction
	cores  []string
	out    []byte
	err    error
}

// boardAction is something the pane does to a board.
type boardAction struct {
	verb     string // "reset"
	doing    string // "resetting"
	nrfutil  []string
	nrfjprog string
	// Asked before it runs; empty for none.
	confirm string
	// Whether it works on one core of a chip with several.
	perCore bool
}

var boardActions = map[string]boardAction{
	"r": {verb: "reset", doing: "resetting", nrfutil: []string{"reset"},
		nrfjprog: "--reset"},
	"E": {verb: "erase", doing: "erasing", nrfutil: []string{"erase", "--all"},
		nrfjprog: "--eraseall", perCore: true,
		confirm: "Erase all flash of %s? (y/N) "},
	"X": {verb: "recover", doing: "recovering", nrfutil: []string{"recover"},
		nrfjprog: "--recover", perCore: true,
		confirm: "Recover %s? This erases the whole chip and removes " +
			"its readback protection. (y/N) "},
}

// The cores of an nRF53 that erase and recover can work on, with the key
// that picks each.  Both are done network core first, as Nordic recovers
// them in that order.
var nrf53Cores = []struct{ key, core, nrfjprog string }{
	{"a", "application", "CP_APPLICATION"},
	{"n", "network", "CP_NETWORK"},
}

// boardCores returns the cores of a board's chip when it has several, as
// far as its chip has been read.
func (m *Model) boardCores(d *Device) []string {
	c := m.boards.chips[d.Serial]
	if c == nil || c.info == nil ||
		!strings.HasPrefix(strings.ToUpper(c.info.Part), "NRF53") {
		return nil
	}
	return []string{"application", "network"}
}

// boardTool is the program that resets, erases and recovers boards.
func boardTool() string {
	for _, tool := range []string{"nrfutil", "nrfjprog"} {
		if _, err := lookPath(tool); err == nil {
			return tool
		}
	}
	return ""
}

// args returns the tool's arguments for the action on a board; core is
// empty for the tool's default.
func (a boardAction) args(tool string, serial string, core string) []string {
	if tool == "nrfutil" {
		args := append(append([]string{"device"}, a.nrfutil...),
			"--serial-number", serial)
		if core != "" {
			args = append(args, "--core", core)
		}
		return args
	}
	args := []string{"-s", deviceSN("nrfjprog", serial), a.nrfjprog}
	for _, c := range nrf53Cores {
		if c.core == core {
			args = append(args, "--coprocessor", c.nrfjprog)
		}
	}
	return args
}

func (m *Model) boardsVisible() bool {
	return m.boards.shown && m.width >= boardsMinWidth && m.guide == nil
}

// toggleBoards shows the pane and moves the cursor into it; on a shown
// pane it moves the cursor into it, or hides it if the cursor is there.
func (m *Model) toggleBoards() tea.Cmd {
	b := &m.boards
	switch {
	case b.shown && b.focus:
		b.shown, b.focus = false, false
		m.saveBoardsShown(false)
		return nil
	case b.shown:
		b.focus = true
		return nil
	}
	m.showBoards()
	b.focus = true
	m.saveBoardsShown(true)
	if m.width < boardsMinWidth {
		m.setStatus(fmt.Sprintf("the boards pane needs a terminal at "+
			"least %d columns wide", boardsMinWidth))
	}
	return m.scanBoards()
}

// showBoards opens the pane; the caller starts the scan.
func (m *Model) showBoards() {
	b := &m.boards
	b.shown = true
	b.gen++
	if b.chips == nil {
		b.chips = map[string]*chipState{}
		b.busy = map[string]string{}
	}
}

func (m *Model) saveBoardsShown(shown bool) {
	value := "hidden"
	if shown {
		value = "shown"
	}
	if err := saveSetting("boards", value); err != nil {
		m.setError(err.Error())
	}
}

func (m *Model) scanBoards() tea.Cmd {
	m.boards.scanning = true
	return m.boardsScan()
}

// boardsScan looks for boards; it does not change the model, so that Init
// can start it.
func (m Model) boardsScan() tea.Cmd {
	gen, find := m.boards.gen, m.findDevices
	return func() tea.Msg {
		devices, err := find()
		return boardsMsg{gen, devices, err}
	}
}

func (m *Model) boardsTick() tea.Cmd {
	gen := m.boards.gen
	return tea.Tick(boardsInterval, func(time.Time) tea.Msg {
		return boardsTickMsg{gen}
	})
}

// boardsIdle tells whether the pane may talk to the probes: not while a
// newt command or a board action may be using them.
func (m *Model) boardsIdle() bool {
	return !m.jobRunning() && len(m.boards.busy) == 0
}

func (m *Model) boardsFound(msg boardsMsg) tea.Cmd {
	b := &m.boards
	if !b.shown || msg.gen != b.gen {
		return nil
	}
	b.scanning = false

	// Keep the cursor on the same board.
	var sel string
	if d := m.boardSelected(); d != nil {
		sel = d.Serial
	}
	b.devices, b.err = msg.devices, msg.err
	for i, d := range b.devices {
		if d.Serial == sel {
			b.list.cursor = i
		}
	}
	b.list.cursor = max(min(b.list.cursor, len(b.devices)-1), 0)
	return tea.Batch(m.readBoardChips(), m.boardsTick())
}

func (m *Model) boardsTicked(msg boardsTickMsg) tea.Cmd {
	b := &m.boards
	if !b.shown || msg.gen != b.gen {
		return nil
	}
	if b.scanning || !m.boardsIdle() {
		return m.boardsTick()
	}
	return m.scanBoards()
}

// readBoardChips reads the chip of the next board seen for the first time;
// when it is done, the next one is read, as each read connects to a probe.
// The RTT console keeps its own connection to a chip, so no chips are read
// while it is open.
func (m *Model) readBoardChips() tea.Cmd {
	b := &m.boards
	if b.reading || !m.boardsIdle() ||
		(m.console != nil && m.console.server != nil) {
		return nil
	}
	for _, d := range b.devices {
		if _, seen := b.chips[d.Serial]; seen || d.Source != "nrfutil" {
			continue
		}
		b.chips[d.Serial] = nil
		b.reading = true
		read, serial := m.readChip, d.Serial
		return func() tea.Msg {
			info, err := read(serial)
			return boardChipMsg{serial, info, err}
		}
	}
	return nil
}

func (m *Model) boardChipRead(msg boardChipMsg) tea.Cmd {
	b := &m.boards
	b.reading = false
	if b.chips == nil {
		return nil
	}
	b.chips[msg.serial] = &chipState{msg.info, msg.err}
	return m.readBoardChips()
}

func (m *Model) boardSelected() *Device {
	b := &m.boards
	if c := b.list.cursor; c >= 0 && c < len(b.devices) {
		return &b.devices[c]
	}
	return nil
}

// boardsKey handles the keys of the pane while it has the cursor; keys
// that are not its own, e.g. q, work as usual.
func (m *Model) boardsKey(k string) (tea.Cmd, bool) {
	b := &m.boards
	if !b.focus || !m.boardsVisible() {
		return nil, false
	}
	switch k {
	case "j", "down":
		b.list.cursor = min(b.list.cursor+1, max(len(b.devices)-1, 0))
	case "k", "up":
		b.list.cursor = max(b.list.cursor-1, 0)
	case "G", "end":
		b.list.cursor = max(len(b.devices)-1, 0)
	case "home":
		b.list.cursor = 0
	case "h", "left", "esc":
		b.focus = false
	case "s":
		if !b.scanning {
			return m.scanBoards(), true
		}
	case "r", "E", "X":
		return m.boardAction(boardActions[k]), true
	case "q", "?", "ctrl+c", "O", "ctrl+]", "z", "R":
		return nil, false
	}
	return nil, true
}

// boardAction runs an action on the board under the cursor, after asking
// when it erases.
func (m *Model) boardAction(a boardAction) tea.Cmd {
	d := m.boardSelected()
	tool := boardTool()
	switch {
	case d == nil:
		m.setError("no board to " + a.verb)
		return nil
	case tool == "":
		m.setError(a.verb + " needs nrfutil or nrfjprog")
		return nil
	case m.jobRunning():
		m.setError("wait for " + m.job.label + " to finish")
		return nil
	case m.boards.busy[d.Serial] != "":
		m.setError(d.Title() + " is " + m.boards.busy[d.Serial])
		return nil
	}
	dev := *d
	name := dev.Title() + " (" + dev.Serial + ")"
	if a.perCore && len(m.boardCores(d)) > 1 {
		m.askCores(a, tool, dev, name)
		return nil
	}
	if a.confirm == "" {
		return m.runBoardAction(a, tool, dev, nil)
	}
	m.prompt = &prompt{confirm: true,
		label: fmt.Sprintf(a.confirm, name),
		done: func(m *Model, _ string) tea.Cmd {
			return m.runBoardAction(a, tool, dev, nil)
		}}
	return nil
}

// askCores asks which cores of the chip to erase or recover; picking one
// answers the question too.
func (m *Model) askCores(a boardAction, tool string, d Device, name string) {
	choices := map[string]string{"b": "network,application"}
	options := []string{}
	help := [][2]string{}
	for _, c := range nrf53Cores {
		choices[c.key] = c.core
		options = append(options, c.key+" "+c.core)
		help = append(help, [2]string{c.key, a.verb + " the " + c.core +
			" core only"})
	}
	options = append(options, "b both")
	help = append(help, [2]string{"b", a.verb + " both cores, the network " +
		"core first"}, [2]string{"Esc", "cancel"})

	question := "Erase all flash of " + name
	if a.verb == "recover" {
		question = "Recover " + name + " (erases it, removes readback " +
			"protection)"
	}
	m.prompt = &prompt{
		label:   question + ": which core? " + strings.Join(options, " · "),
		choices: choices,
		title:   strings.ToUpper(a.verb[:1]) + a.verb[1:] + " " + name,
		help:    help,
		cancel:  "cancelled",
		done: func(m *Model, cores string) tea.Cmd {
			return m.runBoardAction(a, tool, d, strings.Split(cores, ","))
		},
	}
}

// runBoardAction runs an action on the given cores of a board, one after
// another, stopping at the first failure; no cores is the tool's default.
func (m *Model) runBoardAction(a boardAction, tool string, d Device,
	cores []string) tea.Cmd {

	m.boards.busy[d.Serial] = a.doing
	m.setStatus(a.doing + " " + d.Title() + coresText(cores) + "...")
	steps := [][]string{a.args(tool, d.Serial, "")}
	if len(cores) > 0 {
		steps = nil
		for _, core := range cores {
			steps = append(steps, a.args(tool, d.Serial, core))
		}
	}
	return func() tea.Msg {
		for _, args := range steps {
			if out, err := runTool("", tool, args...); err != nil {
				return boardActionMsg{d.Serial, a, cores, out, err}
			}
		}
		return boardActionMsg{d.Serial, a, cores, nil, nil}
	}
}

// coresText names the cores an action works on, e.g. ", network core".
func coresText(cores []string) string {
	switch len(cores) {
	case 0:
		return ""
	case 1:
		return ", " + cores[0] + " core"
	}
	return ", " + strings.Join(cores, " and ") + " cores"
}

func (m *Model) boardActionDone(msg boardActionMsg) tea.Cmd {
	b := &m.boards
	delete(b.busy, msg.serial)
	name := msg.serial
	for _, d := range b.devices {
		if d.Serial == msg.serial {
			name = d.Title() + " (" + d.Serial + ")"
		}
	}
	name += coresText(msg.cores)
	if msg.err != nil {
		m.setError(msg.action.verb + " " + name + ": " +
			nrfutilError(msg.out, msg.err))
		return nil
	}
	m.setStatus(msg.action.verb + " " + name + ": done")
	// Erasing and recovering change what the chip reports, e.g. its
	// protection; read it again.
	if msg.action.verb != "reset" {
		delete(b.chips, msg.serial)
		return m.readBoardChips()
	}
	return nil
}

// boardsView draws the pane: a title, the boards and the details of the
// one under the cursor.
func (m *Model) boardsView(width int, h int) []string {
	b := &m.boards
	title := "── boards "
	lines := []string{styleBoardsSep.Render(title + strings.Repeat("─",
		max(width-ansi.StringWidth(title), 0)))}

	switch {
	case b.devices == nil && b.err == nil:
		return append(lines, styleDim.Render(" looking for boards..."))
	case len(b.devices) == 0 && b.err != nil:
		return append(lines, previewLines([]string{styleErr.Render(
			b.err.Error())}, width, h-1)...)
	case len(b.devices) == 0:
		return append(lines, styleDim.Render(" no boards connected"))
	}

	rows := []row{}
	for i := range b.devices {
		d := &b.devices[i]
		r := row{text: d.Title(), style: kindStyle(KindTarget)}
		if c := b.chips[d.Serial]; c != nil && c.info != nil {
			if c.info.Part != "" {
				r.text += " · " + c.info.Part
			}
			if c.info.Protected() {
				r.text = "⚠ " + r.text
				r.style = styleErr
			}
		}
		if doing := b.busy[d.Serial]; doing != "" {
			r.hint = doing + "…"
		}
		rows = append(rows, r)
	}
	listH := min(len(rows), max(h/2, 1))
	l := b.list
	l.offset = max(min(l.cursor-listH+1, len(rows)-listH), 0)
	if l.cursor < l.offset+listH && l.offset > l.cursor {
		l.offset = l.cursor
	}
	lines = append(lines, renderList(rows, l, width, listH, b.focus)...)

	if d := m.boardSelected(); d != nil {
		lines = append(lines, "")
		lines = append(lines, previewLines(m.boardDetails(d), width,
			max(h-len(lines), 0))...)
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

// boardDetails describes a board in the pane.
func (m *Model) boardDetails(d *Device) []string {
	lines := []string{field("Serial", d.Serial)}
	if d.Product != "" {
		lines = append(lines, field("Probe", d.Product))
	}
	c := m.boards.chips[d.Serial]
	switch {
	case d.Source != "nrfutil":
		lines = append(lines, styleDim.Render("Install nrfutil to read "+
			"the chip."))
	case c == nil:
		lines = append(lines, styleDim.Render("reading the chip..."))
	case c.err != nil:
		lines = append(lines, styleErr.Render("chip: ")+c.err.Error())
	default:
		if c.info.Part != "" {
			lines = append(lines, field("Chip", c.info.Part))
		}
		if c.info.Flash > 0 {
			lines = append(lines, field("Flash", humanSize(c.info.Flash)))
		}
		if cores := m.boardCores(d); len(cores) > 1 {
			lines = append(lines, field("Cores", strings.Join(cores, ", ")))
		}
		if c.info.Protected() {
			lines = append(lines, field("Access", styleErr.Render(
				c.info.Protection)), styleDim.Render("X recovers it."))
		} else if c.info.Protection != "" {
			lines = append(lines, field("Access", "not protected"))
		}
	}
	if len(d.Ports) > 0 {
		lines = append(lines, field("Ports", strings.Join(d.Ports, " ")))
	}
	return lines
}

func (m *Model) boardsHelp() []helpGroup {
	g := helpGroup{title: "Boards", style: kindStyle(KindTarget).Bold(true)}
	g.add("j/k", "down / up")
	if m.boardSelected() != nil {
		g.add("r", "reset the board")
		g.add("E", "erase all its flash")
		g.add("X", "recover it (erases it, removes readback protection)")
	}
	g.add("s", "look for boards now")
	g.add("h/Esc", "back to the lists (the pane stays)")
	g.add("zb", "hide the pane")
	return []helpGroup{g, m.generalHelp()}
}
