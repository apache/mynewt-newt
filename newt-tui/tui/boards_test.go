/*
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
	"errors"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestBoardsPane(t *testing.T) {
	lookPath = func(prog string) (string, error) {
		if prog == "nrfutil" {
			return "/bin/nrfutil", nil
		}
		return "", errors.New("not installed")
	}
	defer func() { lookPath = exec.LookPath }()
	ran := []string{}
	defaultRunTool := runTool
	runTool = func(stdin string, name string, args ...string) ([]byte,
		error) {
		ran = append(ran, name+" "+strings.Join(args, " "))
		if args[1] == "erase" {
			return []byte(`{"type":"task_end","data":{"error":` +
					`{"description":"Failed to connect"}}}`),
				errors.New("exit status 1")
		}
		return nil, nil
	}
	defer func() { runTool = defaultRunTool }()
	cfg := t.TempDir() + "/tui.yml"
	defaultPath := tuiConfigPath
	tuiConfigPath = func() string { return cfg }
	defer func() { tuiConfigPath = defaultPath }()

	m, _ := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 30})
	boards := []Device{
		{Serial: "001050000001", Board: "PCA10056", Source: "nrfutil"},
		{Serial: "001050000002", Board: "PCA10040", Source: "nrfutil"}}
	scans := 0
	m.findDevices = func() ([]Device, error) {
		scans++
		return boards, nil
	}
	reads := []string{}
	m.readChip = func(serial string) (*ChipInfo, error) {
		reads = append(reads, serial)
		info := &ChipInfo{Part: "NRF52840", Protection: "none"}
		if serial == "001050000002" {
			info = &ChipInfo{Part: "NRF52832", Protection: "All"}
		}
		return info, nil
	}

	// press sends a key and delivers what its commands answer, as the tea
	// program would.  Timers are not waited for.
	press := func(k string) {
		t.Helper()
		var msg tea.Msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if k == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		queue := []tea.Msg{msg}
		for len(queue) > 0 {
			next, cmd := m.Update(queue[0])
			m = next.(Model)
			queue = append(queue[1:], runCmd(cmd)...)
		}
	}

	press("z")
	press("b")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "── boards") ||
		!strings.Contains(view, "PCA10056 · NRF52840") ||
		!strings.Contains(view, "⚠ PCA10040 · NRF52832") {
		t.Fatalf("boards pane:\n%s", view)
	}
	if strings.Join(reads, " ") != "001050000001 001050000002" {
		t.Errorf("chips read: %v", reads)
	}
	if !strings.Contains(strings.Join(strings.Fields(view), " "),
		"Serial 001050000001") {
		t.Errorf("details of the first board:\n%s", view)
	}
	help := ansi.Strip(strings.Join(m.helpPane(), "\n"))
	if !strings.Contains(help, "reset the board") {
		t.Errorf("help:\n%s", help)
	}

	// The pane has the cursor: j moves in it, not in the lists.
	cat := m.catList.cursor
	press("j")
	if m.boards.list.cursor != 1 || m.catList.cursor != cat {
		t.Fatalf("cursor %d, lists %d", m.boards.list.cursor, m.catList.cursor)
	}

	// Reset runs at once.
	press("r")
	if ran[len(ran)-1] != "nrfutil device reset --serial-number 001050000002" ||
		!strings.Contains(m.status, "reset PCA10040 (001050000002): done") {
		t.Fatalf("reset: %v, status %q", ran, m.status)
	}

	// Recover asks first, then reads the chip again.
	press("X")
	if m.prompt == nil || !strings.Contains(m.prompt.label, "Recover PCA10040") {
		t.Fatalf("expected a question, got %+v", m.prompt)
	}
	press("y")
	if ran[len(ran)-1] != "nrfutil device recover --serial-number 001050000002" ||
		len(reads) != 3 {
		t.Fatalf("recover: %v, reads %v", ran, reads)
	}

	// Erase shows nrfutil's error; no answers no.
	n := len(ran)
	press("E")
	press("n")
	if len(ran) != n {
		t.Fatalf("erase ran without a yes: %v", ran)
	}
	press("E")
	press("y")
	if ran[len(ran)-1] != "nrfutil device erase --all --serial-number "+
		"001050000002" || !m.statusErr ||
		!strings.Contains(m.status, "Failed to connect") {
		t.Fatalf("erase: %v, status %q", ran, m.status)
	}

	// Esc gives the cursor back to the lists and keeps the pane; zb moves
	// into it again, and then hides it.
	press("esc")
	press("j")
	if !m.boards.shown || m.boards.focus || m.catList.cursor == cat {
		t.Fatalf("after Esc: shown %v focus %v", m.boards.shown, m.boards.focus)
	}
	press("z")
	press("b")
	if !m.boards.focus {
		t.Fatalf("zb should move into the pane")
	}
	press("z")
	press("b")
	if m.boards.shown || strings.Contains(ansi.Strip(m.View()), "── boards") {
		t.Fatalf("zb should hide the pane")
	}
	if savedSetting("boards") != "hidden" {
		t.Errorf("hiding the pane is not remembered")
	}

	// The timer of a hidden pane does not look for boards.
	before := scans
	next, cmd := m.Update(boardsTickMsg{m.boards.gen})
	m = next.(Model)
	if cmd != nil || scans != before {
		t.Fatalf("a hidden pane scanned")
	}
}

// The pane is open from the start, without the cursor, unless it was
// hidden.
func TestBoardsPaneAtStart(t *testing.T) {
	m, _ := newTestModel(t)
	m.findDevices = func() ([]Device, error) {
		return []Device{{Serial: "683220560", Board: "PCA10056",
			Source: "jlink"}}, nil
	}
	m.showBoards()
	m.boards.scanning = true
	m = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 30})
	for _, msg := range runCmd(m.Init()) {
		m = update(t, m, msg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "── boards") ||
		!strings.Contains(view, "PCA10056") || m.boards.focus {
		t.Fatalf("focus %v, view:\n%s", m.boards.focus, view)
	}
	if help := ansi.Strip(strings.Join(m.helpPane(), "\n")); !strings.Contains(
		help, "zb go to the boards pane") {
		t.Errorf("help:\n%s", help)
	}
	// The keys still move in the lists.
	cat := m.catList.cursor
	m = keys(t, m, "j")
	if m.catList.cursor == cat {
		t.Errorf("j should move in the lists")
	}
}

// On an nRF53, erase and recover ask which core.
func TestBoardsPaneNrf53(t *testing.T) {
	lookPath = func(prog string) (string, error) {
		if prog == "nrfutil" {
			return "/bin/nrfutil", nil
		}
		return "", errors.New("not installed")
	}
	defer func() { lookPath = exec.LookPath }()
	ran := []string{}
	defaultRunTool := runTool
	runTool = func(stdin string, name string, args ...string) ([]byte,
		error) {
		ran = append(ran, strings.Join(args, " "))
		return nil, nil
	}
	defer func() { runTool = defaultRunTool }()

	m, _ := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m.findDevices = func() ([]Device, error) {
		return []Device{{Serial: "001050072337", Board: "PCA10095",
			Source: "nrfutil"}}, nil
	}
	m.readChip = func(serial string) (*ChipInfo, error) {
		return &ChipInfo{Part: "NRF5340", Protection: "none"}, nil
	}
	// deliver runs commands and what they answer, as the tea program
	// would.
	deliver := func(queue []tea.Msg) {
		for len(queue) > 0 {
			next, cmd := m.Update(queue[0])
			m = next.(Model)
			queue = append(queue[1:], runCmd(cmd)...)
		}
	}
	press := func(k string) {
		t.Helper()
		var msg tea.Msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		if k == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		deliver([]tea.Msg{msg})
	}
	m.showBoards()
	m.boards.focus = true
	deliver(runCmd(m.scanBoards()))
	if view := ansi.Strip(m.View()); !strings.Contains(strings.Join(
		strings.Fields(view), " "), "Cores application, network") {
		t.Fatalf("view:\n%s", view)
	}

	// Both cores: the network core first.
	press("E")
	if m.prompt == nil || !strings.Contains(m.prompt.label, "which core?") {
		t.Fatalf("expected the core question, got %+v", m.prompt)
	}
	press("b")
	sn := "--serial-number 001050072337"
	if strings.Join(ran, "\n") != "device erase --all "+sn+" --core network\n"+
		"device erase --all "+sn+" --core application" ||
		!strings.Contains(m.status, "network and application cores: done") {
		t.Fatalf("erase both: %q, status %q", ran, m.status)
	}

	// One core.
	ran = nil
	press("X")
	press("n")
	if strings.Join(ran, "\n") != "device recover "+sn+" --core network" {
		t.Fatalf("recover network: %q", ran)
	}

	// Esc cancels; reset is not per core.
	ran = nil
	press("E")
	press("esc")
	press("r")
	if strings.Join(ran, "\n") != "device reset "+sn || m.prompt != nil {
		t.Fatalf("after Esc and reset: %q", ran)
	}

	// nrfjprog picks the core with --coprocessor.
	args := boardActions["E"].args("nrfjprog", "001050072337", "network")
	if strings.Join(args, " ") != "-s 1050072337 --eraseall --coprocessor "+
		"CP_NETWORK" {
		t.Errorf("nrfjprog: %v", args)
	}
}
