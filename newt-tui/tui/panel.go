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
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// A panel is a full screen list with a preview column, opened on top of the
// browser for things like build errors, sizes or the images on a board.

type panelKey struct {
	key  string
	help string
	// Whether the key applies to the row under the cursor; nil means always.
	when func(i int) bool
	fn   func(m *Model, i int) tea.Cmd
}

type panel struct {
	title string
	// Rows and previews by row index; rows get the width of the list.
	rows    func(width int) []row
	preview func(i int) []string
	// Shown instead of the list when there are no rows.
	empty string

	enterHelp string
	enter     func(m *Model, i int) tea.Cmd
	keys      []panelKey

	list list
}

// visible returns the indexes of the rows that match the filter.
func (p *panel) visible(width int) ([]row, []int) {
	rows := p.rows(width)
	filter := strings.ToLower(p.list.filter)
	out := []row{}
	idx := []int{}
	for i, r := range rows {
		if filter != "" &&
			!strings.Contains(strings.ToLower(r.text+" "+r.hint), filter) {
			continue
		}
		out = append(out, r)
		idx = append(idx, i)
	}
	return out, idx
}

// panelWidths splits the screen between the list and the preview.
func (m *Model) panelWidths() (int, int) {
	avail := m.width - 1
	w1 := max(avail*11/20, 10)
	return w1, max(avail-w1, 0)
}

// panelRow is the index of the row under the cursor, or -1.
func (m *Model) panelRow() int {
	if m.panel == nil {
		return -1
	}
	w, _ := m.panelWidths()
	_, idx := m.panel.visible(w)
	if c := m.panel.list.cursor; c >= 0 && c < len(idx) {
		return idx[c]
	}
	return -1
}

func (m *Model) panelLen() int {
	w, _ := m.panelWidths()
	_, idx := m.panel.visible(w)
	return len(idx)
}

func (m *Model) openPanel(p *panel) {
	m.panel = p
	m.filtering = false
}

func (m *Model) closePanel() {
	m.panel = nil
	m.filtering = false
}

// panelKeyPressed handles the keys of an open panel; it returns false for
// keys that the browser handles, such as moving the cursor.
func (m *Model) panelKeyPressed(k string) (tea.Cmd, bool) {
	p := m.panel
	if p == nil {
		return nil, false
	}
	i := m.panelRow()

	switch k {
	case "h", "left", "backspace":
		m.closePanel()
		return nil, true
	case "esc":
		if p.list.filter != "" {
			p.list.filter = ""
		} else {
			m.closePanel()
		}
		return nil, true
	case "/":
		m.filtering = true
		p.list.filter = ""
		return nil, true
	case "enter", "l", "right":
		if p.enter != nil && i >= 0 && k != "right" {
			return p.enter(m, i), true
		}
		return nil, true
	}

	for _, pk := range p.keys {
		if pk.key == k {
			if pk.when == nil || (i >= 0 && pk.when(i)) {
				return pk.fn(m, i), true
			}
			return nil, true
		}
	}

	// Keep target actions from running underneath the panel.
	switch k {
	case "j", "k", "down", "up", "g", "G", "home", "end", "ctrl+d", "ctrl+u",
		"ctrl+f", "ctrl+b", "pgdown", "pgup", "q", "ctrl+c", "?", "R":
		return nil, false
	}
	return nil, true
}

func (m *Model) panelView(h int) string {
	p := m.panel
	w1, w2 := m.panelWidths()

	rows, idx := p.visible(w1)
	var left []string
	if len(rows) == 0 {
		msg := p.empty
		if p.list.filter != "" {
			msg = "no match"
		}
		left = previewLines([]string{styleDim.Render(msg)}, w1, h)
	} else {
		left = renderList(rows, p.list, w1, h, true)
	}

	var right []string
	if c := p.list.cursor; c >= 0 && c < len(idx) && p.preview != nil {
		right = previewLines(p.preview(idx[c]), w2, h)
	}

	sep := column(strings.Split(strings.Repeat("│\n", h), "\n")[:h], 1, h)
	return joinColumns(column(left, w1, h), styleSep.Render(sep),
		column(right, w2, h))
}

func (m *Model) panelHelp() []helpGroup {
	p := m.panel
	i := m.panelRow()

	g := helpGroup{title: p.title, style: styleBold}
	g.add("j/k", "down / up")
	if p.enter != nil && i >= 0 && p.enterHelp != "" {
		g.add("Enter", p.enterHelp)
	}
	for _, pk := range p.keys {
		if pk.when == nil || (i >= 0 && pk.when(i)) {
			g.add(pk.key, pk.help)
		}
	}
	g.add("/", "filter")
	g.add("h/Esc", "close")
	return []helpGroup{g, m.generalHelp()}
}
