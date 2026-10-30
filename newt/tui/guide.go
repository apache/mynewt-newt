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
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The guide shows this package's README.md inside the browser, opened at
// the section about where the user is.  The README is built into newt, so
// it is there wherever newt is installed.

//go:embed README.md
var readme string

type guideSection struct {
	title string
	level int
	line  int
}

type guide struct {
	lines    []string
	sections []guideSection
	top      int

	// The last search, and whether one is being typed.
	search    string
	searching bool
}

var (
	mdBold = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdCode = regexp.MustCompile("`([^`]+)`")
	mdLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	// *italic*, but not a lone "*" or "**".
	mdItalic = regexp.MustCompile(`(^|[\s(])\*([^*\s][^*]*)\*`)
)

// newGuide turns the README into lines for the terminal: headings are
// emphasized, **bold** and `code` are styled, and links show their text.
func newGuide(md string) *guide {
	// Drop the license header.
	if i := strings.Index(md, "-->"); i >= 0 && strings.HasPrefix(
		strings.TrimSpace(md), "<!--") {
		md = md[i+3:]
	}

	g := &guide{}
	inCode := false
	for _, line := range strings.Split(strings.Trim(md, "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			g.lines = append(g.lines, styleDim.Render("  "+line))
			continue
		}

		level := len(line) - len(strings.TrimLeft(line, "#"))
		if level > 0 && strings.HasPrefix(line[level:], " ") {
			title := strings.TrimSpace(line[level:])
			if len(g.lines) > 0 {
				g.lines = append(g.lines, "")
			}
			g.sections = append(g.sections, guideSection{title, level,
				len(g.lines)})
			style := styleHeading
			if level > 2 {
				style = styleBold
			}
			g.lines = append(g.lines, style.Render(title))
			continue
		}

		line = mdLink.ReplaceAllString(line, "$1")
		line = mdItalic.ReplaceAllString(line, "$1$2")
		line = mdBold.ReplaceAllStringFunc(line, func(s string) string {
			return styleBold.Render(s[2 : len(s)-2])
		})
		line = mdCode.ReplaceAllStringFunc(line, func(s string) string {
			return styleKey.Render(s[1 : len(s)-1])
		})
		g.lines = append(g.lines, line)
	}
	return g
}

// sectionAt returns the index of the section that a line belongs to.
func (g *guide) sectionAt(line int) int {
	cur := 0
	for i, s := range g.sections {
		if s.line <= line {
			cur = i
		}
	}
	return cur
}

func (g *guide) find(title string) int {
	for _, s := range g.sections {
		if s.title == title {
			return s.line
		}
	}
	return 0
}

// searchFrom returns the next line, after from (or before it, going
// back), that contains the search, or -1.
func (g *guide) searchFrom(from int, back bool) int {
	q := strings.ToLower(g.search)
	if q == "" {
		return -1
	}
	n := len(g.lines)
	for i := 1; i <= n; i++ {
		l := (from + i) % n
		if back {
			l = ((from-i)%n + n) % n
		}
		if strings.Contains(strings.ToLower(ansi.Strip(g.lines[l])), q) {
			return l
		}
	}
	return -1
}

// guideTopic is the README section about where the user is.
func (m *Model) guideTopic() string {
	switch {
	case m.panel != nil && strings.HasPrefix(m.panel.title, "build of"):
		return "Build errors"
	case m.panel != nil && strings.HasPrefix(m.panel.title, "size of"):
		return "Size and dependencies"
	case m.panel != nil && (strings.HasPrefix(m.panel.title, "branches of") ||
		strings.HasPrefix(m.panel.title, "history of")):
		return "Repos"
	case m.panel != nil && strings.HasPrefix(m.panel.title, "load "):
		return "Choosing the image to load"
	case m.console != nil && m.consoleFocus:
		return "The console"
	case m.boards.focus && m.boardsVisible():
		return "The boards pane"
	case m.depth == depthDevice:
		return "Choosing a board"
	case m.depth == depthSyscfg || m.depth == depthChoice:
		if m.bluetoothOnly {
			return "The Bluetooth page"
		}
		return "Syscfg settings"
	case m.depth == depthFields || m.depth == depthOptions:
		return "Target settings"
	case m.depth == depthEntries && m.inRepos():
		return "Repos"
	case m.depth == depthEntries && m.inTargets():
		return "Targets"
	case m.depth == depthEntries:
		return "Moving around"
	default:
		return "Quick start: your first session"
	}
}

// openGuide shows the README at the section about where the user is.
func (m *Model) openGuide() {
	g := newGuide(readme)
	g.top = g.find(m.guideTopic())
	m.guide = g
}

func (m *Model) guideHeight() int {
	return m.listHeight()
}

// guideKey handles the keys of the guide.
func (m *Model) guideKey(msg tea.KeyMsg) tea.Cmd {
	g := m.guide
	h := m.guideHeight()

	if g.searching {
		switch msg.Type {
		case tea.KeyEnter:
			g.searching = false
			if l := g.searchFrom(g.top, false); l >= 0 {
				g.top = l
			} else {
				m.setStatus("not found: " + g.search)
			}
		case tea.KeyEsc:
			g.searching = false
			g.search = ""
		case tea.KeyBackspace:
			if r := []rune(g.search); len(r) > 0 {
				g.search = string(r[:len(r)-1])
			}
		case tea.KeyRunes, tea.KeySpace:
			g.search += string(msg.Runes)
		}
		return nil
	}

	k := msg.String()
	if m.prefix == "g" {
		m.prefix = ""
		if k == "g" {
			g.top = 0
		}
		return nil
	}

	cur := g.sectionAt(g.top)
	switch k {
	case "q", "esc", "h", "left":
		m.guide = nil
	case "j", "down":
		g.top++
	case "k", "up":
		g.top--
	case "ctrl+d":
		g.top += h / 2
	case "ctrl+u":
		g.top -= h / 2
	case " ", "pgdown", "ctrl+f":
		g.top += h - 1
	case "pgup", "ctrl+b":
		g.top -= h - 1
	case "g":
		m.prefix = "g"
	case "G", "end":
		g.top = len(g.lines) - h
	case "home":
		g.top = 0
	case "]", "tab":
		if cur+1 < len(g.sections) {
			g.top = g.sections[cur+1].line
		}
	case "[", "shift+tab":
		if g.sections[cur].line < g.top {
			g.top = g.sections[cur].line
		} else if cur > 0 {
			g.top = g.sections[cur-1].line
		}
	case "/":
		g.searching = true
		g.search = ""
	case "n", "N":
		if l := g.searchFrom(g.top, k == "N"); l >= 0 {
			g.top = l
		} else if g.search != "" {
			m.setStatus("not found: " + g.search)
		}
	}
	g.top = max(min(g.top, len(g.lines)-h), 0)
	return nil
}

// guideView draws the guide: the sections on the left, the text on the
// right.
func (m *Model) guideView(h int) string {
	g := m.guide
	avail := m.width - 1
	w1 := min(max(avail/4, 16), 34)
	w2 := max(avail-w1, 10)

	cur := g.sectionAt(g.top)
	rows := make([]row, len(g.sections))
	for i, s := range g.sections {
		text := s.title
		if s.level > 2 {
			text = "  " + text
		}
		rows[i] = row{text: text, style: styleBold}
		if s.level > 2 {
			rows[i].style = styleDim
		}
	}
	l := list{cursor: cur, offset: max(min(cur-h/2, len(rows)-h), 0)}
	left := renderList(rows, l, w1, h, true)

	right := []string{}
	for i := g.top; i < len(g.lines) && len(right) < h; i++ {
		line := g.lines[i]
		if g.search != "" && strings.Contains(strings.ToLower(ansi.Strip(line)),
			strings.ToLower(g.search)) {
			line = styleSel.Render(ansi.Strip(line))
		}
		right = append(right, " "+ansi.Truncate(line, w2-1, "…"))
	}

	sep := styleSep.Render(column(strings.Split(strings.Repeat("│\n", h),
		"\n")[:h], 1, h))
	return joinColumns(column(left, w1, h), sep, column(right, w2, h))
}

func (m *Model) guideHelp() []helpGroup {
	g := helpGroup{title: "Guide", style: styleBold}
	g.add("j/k", "scroll")
	g.add("Space/PgUp", "page down / up")
	g.add("] / [", "next / previous section")
	g.add("/", "search")
	if m.guide.search != "" {
		g.add("n/N", fmt.Sprintf("next / previous %q", m.guide.search))
	}
	g.add("Esc/q", "close the guide")
	return []helpGroup{g}
}
