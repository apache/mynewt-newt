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

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The help pane at the bottom of the screen lists the keys that work at the
// current position, grouped by what they do.  It is toggled with "?".

type helpGroup struct {
	title string
	style lipgloss.Style
	keys  [][2]string
}

func (g *helpGroup) add(key string, desc string) {
	g.keys = append(g.keys, [2]string{key, desc})
}

// Below this terminal height the pane is hidden to leave room for the lists.
const helpMinHeight = 16

func (m *Model) helpGroups() []helpGroup {
	if m.prompt != nil && m.prompt.confirm {
		g := helpGroup{title: "Confirm", style: styleErr}
		g.add("y", "yes")
		g.add("other", "no, cancel")
		return []helpGroup{g}
	}
	if m.prompt != nil && m.prompt.help != nil {
		return []helpGroup{{title: m.prompt.title, style: styleBold,
			keys: m.prompt.help}}
	}
	if m.prompt != nil {
		g := helpGroup{title: "Enter a name", style: styleBold}
		g.add("Enter", "confirm")
		g.add("Esc", "cancel")
		g.add("names", `"foo" creates targets/foo`)
		return []helpGroup{g}
	}
	if m.filtering {
		g := helpGroup{title: "Filter", style: styleBold}
		g.add("type", "show only matching names")
		g.add("Enter", "keep the filter, navigate")
		g.add("Esc", "clear the filter")
		return []helpGroup{g}
	}

	if m.panel != nil {
		return m.panelHelp()
	}
	if m.depth == depthDevice {
		return m.devicePickerHelp()
	}
	if m.depth >= depthSyscfg {
		return m.syscfgHelp()
	}

	e := m.Selected()
	f := m.field()
	_, n := m.current()

	move := helpGroup{title: "Move", style: styleBold}
	move.add("j/k", "down / up")
	switch m.depth {
	case depthCategories:
		move.add("l", "open category")
	case depthEntries:
		move.add("h", "back to categories")
		switch {
		case e != nil && len(e.Fields) > 0:
			move.add("l", "settings")
		case e != nil && e.EditPath != "":
			move.add("l", "open "+fileName(e.EditPath)+" in $EDITOR")
		}
	case depthFields:
		move.add("h", "back to targets")
		if f != nil && f.Kind == KindSyscfg {
			move.add("l", "see and change syscfg settings")
		} else if f != nil && f.Help != "" {
			move.add("l", f.Help)
		} else if f != nil {
			move.add("l", "choose "+f.Label)
		}
	case depthOptions:
		move.add("h", "cancel")
		if f != nil {
			move.add("Enter", "set "+f.Label+" to this")
		}
	}
	// Paging keys only matter for long lists.  (listHeight depends on the
	// size of this pane, so estimate.)
	if n > m.height/2 {
		move.add("gg/G", "first / last")
		move.add("C-d/C-u", "half page down / up")
	}
	if m.depth == depthEntries && m.grouped(m.catList.cursor) {
		move.add("]/[", "next / previous repo")
	}
	if l := m.filterable(); l != nil {
		move.add("/", "filter")
		if l.filter != "" {
			move.add("Esc", "clear filter")
		}
	} else if m.depth == depthCategories {
		move.add("/", "filter the category")
	}
	if len(m.history) > 0 && m.depth < depthOptions {
		move.add("H", "back to where you jumped from")
	}

	groups := []helpGroup{move}

	if e != nil && m.depth < depthOptions && len(e.Actions) > 0 {
		title := e.Name
		if e.Kind == KindTarget {
			title = "Run on " + e.Name
		}
		run := helpGroup{title: title, style: kindStyle(e.Kind).Bold(true)}
		for _, a := range e.Actions {
			run.add(a.Key, a.Label)
		}
		if issues, ok := m.issues[e.Name]; ok {
			errs, warns := countIssues(issues)
			run.add("E", fmt.Sprintf("last build: %s, %s",
				plural(errs, "error"), plural(warns, "warning")))
		}
		groups = append(groups, run)
	}

	if e != nil && m.depth < depthOptions && len(e.Links) > 0 {
		jump := helpGroup{title: "Go to", style: styleBold}
		for _, l := range e.Links {
			short, _ := splitName(l.Name)
			jump.add("g"+l.Key, l.Label+styleDim.Render(" "+short))
		}
		groups = append(groups, jump)
	}

	if m.inTargets() && m.depth < depthOptions {
		tg := helpGroup{title: "Targets", style: kindStyle(KindTarget).Bold(true)}
		tg.add("n", "new target")
		if e != nil && !e.Foreign {
			tg.add("y", "copy")
			tg.add("a", "rename")
			tg.add("D", "delete")
		} else if e != nil {
			tg.add("", styleDim.Render("read-only ("+e.Hint+")"))
		}
		if e != nil && e.EditPath != "" {
			tg.add("e", "edit "+fileName(e.EditPath))
		}
		groups = append(groups, tg)
	} else if e != nil && e.EditPath != "" && m.depth == depthEntries {
		ed := helpGroup{title: "Edit", style: styleBold}
		ed.add("e", "open "+fileName(e.EditPath)+" in $EDITOR")
		groups = append(groups, ed)
	}

	other := helpGroup{title: "General", style: styleBold}
	if m.depth <= depthEntries && m.catList.cursor < len(m.cats) {
		if m.hideForeign[m.catList.cursor] {
			other.add(".", "show other repos too")
		} else {
			other.add(".", "show local only")
		}
	}
	other.add("R", "reload project")
	other = m.jobHelp(other)
	other.add("?", "hide this help")
	other.add("q", "quit")
	groups = append(groups, other)

	return groups
}

func fileName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// renderGroup renders a help group as a block of equally wide lines.
func renderGroup(g helpGroup) []string {
	keyW := 0
	for _, k := range g.keys {
		keyW = max(keyW, ansi.StringWidth(k[0]))
	}

	lines := []string{g.style.Render(g.title)}
	for _, k := range g.keys {
		lines = append(lines, styleKey.Render(k[0])+
			strings.Repeat(" ", keyW-ansi.StringWidth(k[0])+2)+k[1])
	}

	w := 0
	for _, l := range lines {
		w = max(w, ansi.StringWidth(l))
	}
	for i, l := range lines {
		lines[i] = l + strings.Repeat(" ", w-ansi.StringWidth(l))
	}
	return lines
}

// inlineGroup renders a group as wrapped "key desc · key desc" lines.
func inlineGroup(g helpGroup, width int) []string {
	items := []string{}
	for _, k := range g.keys {
		if k[0] == "" {
			items = append(items, k[1])
		} else {
			items = append(items, styleKey.Render(k[0])+" "+k[1])
		}
	}

	title := ""
	if g.title != "" {
		title = g.style.Render(g.title) + styleDim.Render(": ")
	}
	sep := styleDim.Render(" · ")

	lines := []string{}
	line := " " + title
	empty := true
	for _, it := range items {
		if !empty && ansi.StringWidth(line+sep+it) > width {
			lines = append(lines, line)
			line, empty = "   ", true
		}
		if !empty {
			line += sep
		}
		line += it
		empty = false
	}
	lines = append(lines, line)

	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "…")
	}
	return lines
}

// gridLines lays the groups out side by side, wrapping to more rows when the
// terminal is too narrow.
func gridLines(groups []helpGroup, width int) []string {
	const gap = "   "

	out := []string{}
	var row [][]string
	rowW := 0

	flush := func() {
		h := 0
		for _, b := range row {
			h = max(h, len(b))
		}
		for i := 0; i < h; i++ {
			parts := []string{}
			for _, b := range row {
				if i < len(b) {
					parts = append(parts, b[i])
				} else {
					parts = append(parts, strings.Repeat(" ",
						ansi.StringWidth(b[0])))
				}
			}
			line := " " + strings.Join(parts, gap)
			out = append(out, ansi.Truncate(line, width, "…"))
		}
		row, rowW = nil, 0
	}

	for _, g := range groups {
		b := renderGroup(g)
		w := ansi.StringWidth(b[0]) + len(gap)
		if len(row) > 0 && rowW+w > width-1 {
			flush()
		}
		row = append(row, b)
		rowW += w
	}
	flush()
	return out
}

// helpPane returns the lines of the help pane, or nil if it is hidden.
func (m *Model) helpPane() []string {
	if m.helpHidden || m.height < helpMinHeight || m.width < 20 {
		return nil
	}

	groups := m.helpGroups()
	maxH := m.height / 2

	// The general keys go on one line below the other groups.
	var general []string
	if last := groups[len(groups)-1]; last.title == "General" {
		groups = groups[:len(groups)-1]
		last.title = ""
		general = inlineGroup(last, m.width)
	}

	body := gridLines(groups, m.width)
	if 1+len(body)+len(general) > maxH {
		// Too tall: one compact line per group instead.
		body = nil
		for _, g := range groups {
			body = append(body, inlineGroup(g, m.width)...)
		}
	}

	out := []string{styleSep.Render(strings.Repeat("─", m.width))}
	out = append(out, body...)
	out = append(out, general...)
	if len(out) > maxH {
		out = append(out[:maxH-1], styleDim.Render(
			" … more keys: enlarge the terminal"))
	}
	return out
}

func (m *Model) syscfgHelp() []helpGroup {
	s := m.setting()
	_, n := m.current()
	readOnly := false
	if e := m.Selected(); e != nil && e.Foreign {
		readOnly = true
	}

	move := helpGroup{title: "Move", style: styleBold}
	move.add("j/k", "down / up")
	if m.depth == depthChoice {
		move.add("h", "cancel")
		if s != nil {
			move.add("Enter", "set "+s.Name+" to this")
		}
		return []helpGroup{move, m.generalHelp()}
	}

	move.add("h", "back to target settings")
	if n > m.height/2 {
		move.add("gg/G", "first / last")
		move.add("C-d/C-u", "half page down / up")
	}
	move.add("/", "search names and descriptions")
	if m.setList.filter != "" {
		move.add("Esc", "clear search")
	}

	change := helpGroup{title: "Change", style: styleOverride}
	switch {
	case readOnly:
		change.add("", styleDim.Render("read-only: target from another repo"))
	case s == nil:
	case s.Const:
		change.add("", styleDim.Render(s.Name+" is a constant"))
	case len(s.Choices) > 0:
		change.add("Enter", fmt.Sprintf("pick from %d choices",
			len(s.Choices)))
	default:
		change.add("Enter", "type a new value")
	}
	if !readOnly && s != nil && s.IsFlag() && !s.Const {
		change.add("space", "toggle 0 / 1")
	}
	if !readOnly && s != nil && s.Overridden {
		change.add("x", "remove this target's value")
	}
	if m.changedOnly {
		change.add("o", "show all settings")
	} else {
		change.add("o", "show only changed settings")
	}
	if m.hasBluetooth() {
		if m.bluetoothOnly {
			change.add("B", "show all settings, not just Bluetooth")
		} else {
			change.add("B", "Bluetooth essentials only")
		}
	}

	legend := helpGroup{title: "Colors", style: styleBold}
	legend.add(styleOverride.Render("●"), styleOverride.Render("set by this target"))
	legend.add(" ", "changed by app, bsp or a package")
	legend.add(" ", styleDim.Render("default value"))

	return []helpGroup{move, change, legend, m.generalHelp()}
}

func (m *Model) generalHelp() helpGroup {
	g := helpGroup{title: "General", style: styleBold}
	g.add("R", "reload project")
	g = m.jobHelp(g)
	g.add("?", "hide this help")
	g.add("q", "quit")
	return g
}

// jobHelp adds the keys of the output pane.
func (m *Model) jobHelp(g helpGroup) helpGroup {
	switch {
	case m.jobRunning():
		g.add("Ctrl-C", "stop "+m.job.label)
	case m.job != nil && m.dockShown:
		g.add("Esc", "hide the output")
	case m.job != nil:
		g.add("O", "show the last output")
	}
	return g
}
