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
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"mynewt.apache.org/newt/util"
)

// The console is the board's shell (e.g. btshell's), over a serial port or
// SEGGER RTT, whichever the target's syscfg enables.  It is shown in a pane
// at the bottom of the browser and stays open while you browse, build and
// load.

// Lines of console output that are kept for scrolling back.
const consoleKeepLines = 5000

// The port J-Link serves RTT on while it is connected to the chip.
const rttTelnetAddr = "localhost:19021"

type consoleSession struct {
	name string
	conn io.ReadWriteCloser
	// For RTT: J-Link Commander, which keeps the connection to the chip.
	server *exec.Cmd

	mu      sync.Mutex
	lines   []string
	partial []rune
	pendCR  bool
	inEsc   int
	err     error
	closed  bool
}

// write adds output from the board.  Escape sequences (colors, cursor
// movements) are dropped, a carriage return starts the line over and a
// backspace removes the last character, as a terminal would show them.
func (c *consoleSession) write(p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	newline := func() {
		c.lines = append(c.lines, string(c.partial))
		if len(c.lines) > consoleKeepLines {
			c.lines = c.lines[len(c.lines)-consoleKeepLines:]
		}
		c.partial = c.partial[:0]
	}

	for _, r := range string(p) {
		switch {
		case c.inEsc == 1:
			// ESC [ starts a control sequence; other escapes are one
			// character long.
			if r == '[' {
				c.inEsc = 2
			} else {
				c.inEsc = 0
			}
			continue
		case c.inEsc == 2:
			if r >= 0x40 && r <= 0x7e {
				c.inEsc = 0
			}
			continue
		}

		if c.pendCR && r != '\n' {
			c.partial = c.partial[:0]
		}
		c.pendCR = false

		switch {
		case r == 0x1b:
			c.inEsc = 1
		case r == '\n':
			newline()
		case r == '\r':
			c.pendCR = true
		case r == '\b' || r == 0x7f:
			if len(c.partial) > 0 {
				c.partial = c.partial[:len(c.partial)-1]
			}
		case r == '\t':
			c.partial = append(c.partial, ' ', ' ', ' ', ' ')
		case r < 0x20:
		default:
			c.partial = append(c.partial, r)
		}
	}
}

// view returns the console's lines, n of them, scrolled back by back lines.
func (c *consoleSession) view(n int, back int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	lines := append(append([]string{}, c.lines...), string(c.partial))
	end := max(len(lines)-back, min(n, len(lines)))
	start := max(end-n, 0)
	return lines[start:end]
}

func (c *consoleSession) state() (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed, c.err
}

func (c *consoleSession) read() {
	buf := make([]byte, 1024)
	for {
		n, err := c.conn.Read(buf)
		if n > 0 {
			c.write(buf[:n])
		}
		if err != nil {
			c.mu.Lock()
			if !c.closed {
				c.err = err
				c.closed = true
			}
			c.mu.Unlock()
			return
		}
	}
}

func (c *consoleSession) send(line string) error {
	_, err := c.conn.Write([]byte(line + "\r"))
	return err
}

func (c *consoleSession) close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.conn.Close()
	if c.server != nil && c.server.Process != nil {
		c.server.Process.Kill()
		c.server.Wait()
	}
}

type consoleTickMsg struct{ s *consoleSession }

// waitConsole redraws the console ten times a second while it is open.
func (m *Model) waitConsole() tea.Cmd {
	s := m.console
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return consoleTickMsg{s}
	})
}

func (m *Model) consoleTick(msg consoleTickMsg) tea.Cmd {
	if m.console == nil || msg.s != m.console {
		return nil
	}
	if closed, err := m.console.state(); closed {
		if err != nil {
			m.setError("console: " + m.console.name + ": " + err.Error() +
				" (was the board unplugged?)")
		}
		m.closeConsole()
		return nil
	}
	return m.waitConsole()
}

func openSerialPort(path string, baud int) (io.ReadWriteCloser, error) {
	return openSerial(path, baud)
}

// serialPorts lists the USB serial ports of the machine.
func serialPorts() []string {
	patterns := []string{"/dev/ttyACM*", "/dev/ttyUSB*"}
	if runtime.GOOS == "darwin" {
		patterns = []string{"/dev/cu.usbmodem*", "/dev/cu.usbserial*"}
	}
	ports := []string{}
	for _, p := range patterns {
		matches, _ := filepath.Glob(p)
		ports = append(ports, matches...)
	}
	sort.Strings(ports)
	return ports
}

// consoleTransport describes how to reach a target's console.
type consoleTransport struct {
	rtt  bool
	baud int
	// J-Link device name, e.g. "NRF52", for RTT.
	device string
}

func (m *Model) consoleTransport() (consoleTransport, error) {
	e := m.Selected()
	if e == nil || e.Syscfg == nil {
		return consoleTransport{}, util.NewNewtError("not a target")
	}
	info, err := e.Syscfg()
	if err != nil {
		return consoleTransport{}, err
	}
	val := func(name string) string {
		if s := findSetting(info, name); s != nil {
			return s.Value
		}
		return ""
	}

	switch {
	case val("CONSOLE_UART") == "1":
		baud, _ := strconv.Atoi(val("CONSOLE_UART_BAUD"))
		if baud == 0 {
			baud = 115200
		}
		return consoleTransport{baud: baud}, nil
	case val("CONSOLE_RTT") == "1":
		return consoleTransport{rtt: true, device: val("JLINK_TARGET")}, nil
	default:
		return consoleTransport{}, util.NewNewtError("this target has no " +
			"console: CONSOLE_UART and CONSOLE_RTT are both 0")
	}
}

// consolePort picks the serial port of a board: its own (the one chosen in
// the picker, if it has several), or the only one on the machine.
func (m *Model) consolePort(d *Device) (string, error) {
	if d != nil && len(d.Ports) > 0 {
		return d.Ports[m.picker.portIdx[d.Serial]%len(d.Ports)], nil
	}
	ports := serialPorts()
	switch len(ports) {
	case 0:
		return "", util.NewNewtError("no serial port found; is the board " +
			"connected?")
	case 1:
		return ports[0], nil
	default:
		return "", util.FmtNewtError("several serial ports (%v); choose "+
			"the board in the list", ports)
	}
}

// consoleDescription says what the console of the picked board will be.
func (m *Model) consoleDescription(d *Device) string {
	t, err := m.consoleTransport()
	if err != nil {
		return err.Error()
	}
	if t.rtt {
		return "RTT console through J-Link"
	}
	port, err := m.consolePort(d)
	if err != nil {
		return err.Error()
	}
	desc := fmt.Sprintf("serial console on %s at %d baud", port, t.baud)
	if d != nil && len(d.Ports) > 1 {
		desc += fmt.Sprintf(" (port %d of %d; p switches)",
			m.picker.portIdx[d.Serial]%len(d.Ports)+1, len(d.Ports))
	}
	return desc
}

// openConsole connects to the picked board's console and shows it.
func (m *Model) openConsole(d *Device) tea.Cmd {
	t, err := m.consoleTransport()
	if err != nil {
		m.setError(err.Error())
		return nil
	}

	var s *consoleSession
	if t.rtt {
		s, err = openRTT(t.device, d)
	} else {
		var port string
		port, err = m.consolePort(d)
		if err == nil {
			var conn io.ReadWriteCloser
			conn, err = m.openPort(port, t.baud)
			s = &consoleSession{conn: conn,
				name: fmt.Sprintf("%s at %d baud", port, t.baud)}
		}
	}
	if err != nil {
		m.setError("console: " + err.Error())
		return nil
	}

	if m.console != nil {
		m.console.close()
	}
	m.console = s
	m.consoleFocus = true
	m.consoleInput = ""
	m.consoleBack = 0
	go s.read()
	m.setStatus("console on " + s.name + "; Esc goes back to the browser")
	return m.waitConsole()
}

// openRTT connects to the chip with J-Link Commander and reads RTT from the
// telnet port that J-Link serves while connected.
func openRTT(device string, d *Device) (*consoleSession, error) {
	jlink := downloaderProgram("jlink")
	if _, err := lookPath(jlink); err != nil {
		return nil, util.FmtNewtError("the RTT console needs %s, from the "+
			"SEGGER J-Link software", jlink)
	}
	if device == "" {
		return nil, util.NewNewtError("JLINK_TARGET is not set; set it in " +
			"the target's syscfg for the RTT console")
	}

	args := []string{"-device", device, "-if", "SWD", "-speed", "4000",
		"-autoconnect", "1", "-NoGui", "1"}
	if d != nil {
		args = append(args, "-SelectEmuBySN", deviceSN("jlink", d.Serial))
	}
	server := exec.Command(jlink, args...)
	// J-Link Commander waits for commands on stdin; keep it open.
	if _, err := server.StdinPipe(); err != nil {
		return nil, err
	}
	ownProcessGroup(server)
	if err := server.Start(); err != nil {
		return nil, util.ChildNewtError(err)
	}

	var conn net.Conn
	var err error
	for i := 0; i < 30; i++ {
		if conn, err = net.Dial("tcp", rttTelnetAddr); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		server.Process.Kill()
		server.Wait()
		return nil, util.FmtNewtError("J-Link did not start RTT on %s: %s",
			rttTelnetAddr, err)
	}
	return &consoleSession{conn: conn, server: server,
		name: "RTT through J-Link"}, nil
}

func (m *Model) closeConsole() {
	if m.console != nil {
		if closed, _ := m.console.state(); !closed {
			m.setStatus("closed the console on " + m.console.name)
		}
		m.console.close()
	}
	m.console = nil
	m.consoleFocus = false
}

// consoleKey handles keys while typing in the console.
func (m *Model) consoleKey(msg tea.KeyMsg) tea.Cmd {
	s := m.console
	page := max(m.dockHeight()-2, 1)

	switch msg.Type {
	case tea.KeyEsc:
		m.consoleFocus = false
		m.setStatus("console stays open; C types in it again, Ctrl-] " +
			"closes it")
	case tea.KeyCtrlCloseBracket:
		m.closeConsole()
	case tea.KeyEnter:
		line := m.consoleInput
		if err := s.send(line); err != nil {
			m.setError("console: " + err.Error())
			return nil
		}
		if line != "" && (len(m.consoleHistory) == 0 ||
			m.consoleHistory[len(m.consoleHistory)-1] != line) {
			m.consoleHistory = append(m.consoleHistory, line)
		}
		m.consoleHistIdx = len(m.consoleHistory)
		m.consoleInput = ""
		m.consoleBack = 0
	case tea.KeyBackspace:
		if r := []rune(m.consoleInput); len(r) > 0 {
			m.consoleInput = string(r[:len(r)-1])
		}
	case tea.KeyUp:
		if m.consoleHistIdx > 0 {
			m.consoleHistIdx--
			m.consoleInput = m.consoleHistory[m.consoleHistIdx]
		}
	case tea.KeyDown:
		if m.consoleHistIdx < len(m.consoleHistory)-1 {
			m.consoleHistIdx++
			m.consoleInput = m.consoleHistory[m.consoleHistIdx]
		} else {
			m.consoleHistIdx = len(m.consoleHistory)
			m.consoleInput = ""
		}
	case tea.KeyPgUp:
		m.consoleBack += page
	case tea.KeyPgDown:
		m.consoleBack = max(m.consoleBack-page, 0)
	case tea.KeyRunes, tea.KeySpace:
		m.consoleInput += string(msg.Runes)
	}
	return nil
}

// consoleView draws the console pane: a title, the output and the input
// line.
func (m *Model) consoleView(width int, h int) []string {
	s := m.console
	state := styleDim.Render("Esc: back to the browser · Ctrl-] closes")
	if !m.consoleFocus {
		state = styleDim.Render("C: type here · Ctrl-] closes")
	}
	if m.consoleBack > 0 {
		state = styleKey.Render("scrolled back") + styleDim.Render(
			" · PgDn returns")
	}
	title := fmt.Sprintf("── console %s · %s ", s.name, state)
	title = ansi.Truncate(title, width, "…")
	if pad := width - ansi.StringWidth(title); pad > 0 {
		title += strings.Repeat("─", pad)
	}

	lines := []string{styleDockSep.Render(title)}
	for _, l := range s.view(h-2, m.consoleBack) {
		lines = append(lines, " "+ansi.Truncate(l, width-1, "…"))
	}
	for len(lines) < h-1 {
		lines = append(lines, "")
	}

	input := styleDim.Render(" > ")
	if m.consoleFocus {
		input = styleKey.Render(" > ") + m.consoleInput + styleSel.Render(" ")
	}
	return append(lines, ansi.Truncate(input, width, "…"))
}
