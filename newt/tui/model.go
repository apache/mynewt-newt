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

// Package tui implements "newt tui", a ranger-style browser for the targets
// and packages of a newt project.
//
// The screen is split into three Miller columns: the parent level on the
// left, the current level in the middle and a preview of the selected item
// on the right.  The levels are categories (Targets, Apps, BSPs, ...), the
// entries of a category and, for targets, the target's settings (app, bsp,
// ...) and the choices for a setting.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	depthCategories = 0
	depthEntries    = 1
	depthFields     = 2
	depthOptions    = 3
)

// LoadFunc (re)loads the project and returns the browser contents.
type LoadFunc func() ([]*Category, error)

// list holds the cursor state of one column.
type list struct {
	cursor int
	offset int
	filter string
}

type position struct {
	cat    int
	cursor int
}

type Model struct {
	title string
	// Shown dimmed next to the title, e.g. the project.yml name.
	note string
	load LoadFunc
	cats []*Category

	depth     int
	catList   list
	entList   []list
	fieldList list
	optList   list
	// Choices for the field being edited.
	opts []*Entry

	// Per category: hide entries from foreign repos.
	hideForeign []bool
	history     []position

	// Pending key prefix, e.g. "g" while waiting for "gg".
	prefix string

	filtering  bool
	helpHidden bool
	prompt     *prompt

	status    string
	statusErr bool

	width  int
	height int

	// Run the newt binary in the foreground / background; replaced in
	// tests.
	newtCmd func(steps [][]string, env []string) tea.ExecCommand
	newtRun func(args []string) (string, error)
}

func NewModel(title string, load LoadFunc) Model {
	m := Model{
		title:   title,
		load:    load,
		newtCmd: newtExec,
		newtRun: newtOutput,
	}
	m.reload()
	return m
}

// reload re-reads the project; on failure it reports the error and returns
// false.
func (m *Model) reload() bool {
	cats, err := m.load()
	if err != nil {
		m.setError(err.Error())
		return false
	}

	// Keep the cursor on the same entries where possible.
	prev := map[int]string{}
	for ci := range m.cats {
		if e := m.entryAt(ci); e != nil {
			prev[ci] = e.FullName
		}
	}

	m.cats = cats
	if len(m.entList) != len(cats) {
		// Like "newt target show", only list local targets by default.
		m.entList = make([]list, len(cats))
		m.hideForeign = make([]bool, len(cats))
		for ci, c := range cats {
			m.hideForeign[ci] = c.Kind == KindTarget
		}
	}
	for ci, name := range prev {
		if ci < len(cats) {
			m.selectEntry(ci, name)
		}
	}
	m.fix()
	return true
}

func (m *Model) setStatus(s string) {
	m.status = s
	m.statusErr = false
}

func (m *Model) setError(s string) {
	m.status = s
	m.statusErr = true
}

func (m Model) Init() tea.Cmd {
	return nil
}

// visible returns the entries of a category after applying the foreign-repo
// toggle and the filter.
func (m *Model) visible(ci int) []*Entry {
	if ci < 0 || ci >= len(m.cats) {
		return nil
	}

	filter := strings.ToLower(m.entList[ci].filter)
	out := []*Entry{}
	for _, e := range m.cats[ci].Entries {
		if e.Foreign && m.hideForeign[ci] {
			continue
		}
		if filter != "" &&
			!strings.Contains(strings.ToLower(e.Name+" "+e.Hint), filter) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (m *Model) entryAt(ci int) *Entry {
	if ci < 0 || ci >= len(m.entList) {
		return nil
	}
	entries := m.visible(ci)
	cur := m.entList[ci].cursor
	if cur < 0 || cur >= len(entries) {
		return nil
	}
	return entries[cur]
}

// Selected returns the entry under the cursor (or whose fields are being
// browsed), or nil if a category is selected.
func (m *Model) Selected() *Entry {
	if m.depth == depthCategories {
		return nil
	}
	return m.entryAt(m.catList.cursor)
}

func (m *Model) field() *Field {
	e := m.Selected()
	if e == nil || m.depth < depthFields {
		return nil
	}
	if c := m.fieldList.cursor; c >= 0 && c < len(e.Fields) {
		return e.Fields[c]
	}
	return nil
}

// options returns the choices for a field, with the "none" choice first for
// optional fields.
func (m *Model) options(f *Field) []*Entry {
	opts := []*Entry{}
	if f.Optional {
		opts = append(opts, &Entry{Name: "none", Hint: "unset",
			Kind: KindValue})
	}

	if f.Kind == KindValue {
		for _, v := range f.Values() {
			opts = append(opts, &Entry{Name: v, FullName: v, Kind: KindValue})
		}
	} else if ci := m.findKind(f.Kind); ci >= 0 {
		opts = append(opts, m.cats[ci].Entries...)
	}
	return opts
}

func (m *Model) visibleOpts() []*Entry {
	filter := strings.ToLower(m.optList.filter)
	out := []*Entry{}
	for _, o := range m.opts {
		if filter == "" ||
			strings.Contains(strings.ToLower(o.Name+" "+o.Hint), filter) {
			out = append(out, o)
		}
	}
	return out
}

func (m *Model) option() *Entry {
	opts := m.visibleOpts()
	if c := m.optList.cursor; m.depth == depthOptions && c < len(opts) {
		return opts[c]
	}
	return nil
}

func indexOf(entries []*Entry, fullName string) int {
	for i, e := range entries {
		if e.FullName == fullName {
			return i
		}
	}
	return -1
}

func (m *Model) current() (*list, int) {
	switch m.depth {
	case depthCategories:
		return &m.catList, len(m.cats)
	case depthFields:
		return &m.fieldList, len(m.Selected().Fields)
	case depthOptions:
		return &m.optList, len(m.visibleOpts())
	default:
		return &m.entList[m.catList.cursor], len(m.visible(m.catList.cursor))
	}
}

// filterable returns the list that "/" filters at the current level.
func (m *Model) filterable() *list {
	switch m.depth {
	case depthEntries:
		return &m.entList[m.catList.cursor]
	case depthOptions:
		return &m.optList
	default:
		return nil
	}
}

func (m *Model) listHeight() int {
	return max(m.height-2-len(m.helpPane()), 1)
}

// fix clamps all cursors and scrolls the current column so that the cursor
// stays visible.
func (m *Model) fix() {
	// The height depends on the help pane, which looks at the category.
	m.catList.cursor = max(min(m.catList.cursor, len(m.cats)-1), 0)
	h := m.listHeight()
	clamp := func(l *list, n int) {
		l.cursor = max(min(l.cursor, n-1), 0)
		if l.cursor < l.offset {
			l.offset = l.cursor
		}
		if l.cursor >= l.offset+h {
			l.offset = l.cursor - h + 1
		}
		l.offset = max(min(l.offset, n-h), 0)
	}

	clamp(&m.catList, len(m.cats))
	for ci := range m.entList {
		m.fixEntries(ci, h)
	}

	// The entry may have disappeared after a reload.
	if m.depth >= depthFields && m.Selected() == nil {
		m.depth = depthEntries
	}
	if e := m.Selected(); e != nil {
		clamp(&m.fieldList, len(e.Fields))
	}
	clamp(&m.optList, len(m.visibleOpts()))
}

// fixEntries clamps the cursor of a category's entries and scrolls them.
// The cursor counts entries, the offset counts rows, which include the repo
// headers.
func (m *Model) fixEntries(ci int, h int) {
	l := &m.entList[ci]
	rows, rowOf := m.entryLayout(ci)
	l.cursor = max(min(l.cursor, len(rowOf)-1), 0)
	if len(rowOf) == 0 {
		l.offset = 0
		return
	}
	r := rowOf[l.cursor]
	top := r
	// Show the header of the cursor's group when scrolling up to it.
	if r > 0 && rows[r-1].header {
		top = r - 1
	}
	if top < l.offset {
		l.offset = top
	}
	if r >= l.offset+h {
		l.offset = r - h + 1
	}
	l.offset = max(min(l.offset, len(rows)-h), 0)
}

// jumpRepo moves to the first entry of the next repo, or of the current one
// (the previous one when already there).
func (m *Model) jumpRepo(next bool) {
	ci := m.catList.cursor
	entries := m.visible(ci)
	l := &m.entList[ci]
	if l.cursor < 0 || l.cursor >= len(entries) {
		return
	}
	repo := entries[l.cursor].Repo
	if next {
		for i := l.cursor + 1; i < len(entries); i++ {
			if entries[i].Repo != repo {
				l.cursor = i
				return
			}
		}
		return
	}
	i := l.cursor
	if i > 0 && entries[i-1].Repo != repo {
		i--
		repo = entries[i].Repo
	}
	for i > 0 && entries[i-1].Repo == repo {
		i--
	}
	l.cursor = i
}

func (m *Model) selectEntry(ci int, fullName string) bool {
	for i, e := range m.visible(ci) {
		if e.FullName == fullName {
			m.entList[ci].cursor = i
			return true
		}
	}
	return false
}

func (m *Model) findKind(k Kind) int {
	for ci, c := range m.cats {
		if c.Kind == k {
			return ci
		}
	}
	return -1
}

func (m *Model) follow(l Link) {
	ci := m.findKind(l.Kind)
	if ci < 0 {
		return
	}

	m.history = append(m.history, position{m.catList.cursor,
		m.entList[m.catList.cursor].cursor})

	// The link target may be hidden by a filter or the foreign toggle.
	m.entList[ci].filter = ""
	if !m.selectEntry(ci, l.Name) {
		m.hideForeign[ci] = false
		if !m.selectEntry(ci, l.Name) {
			m.history = m.history[:len(m.history)-1]
			m.setError("not found: " + l.Name)
			return
		}
	}
	m.catList.cursor = ci
	m.depth = depthEntries
	m.setStatus(l.Label + ": " + l.Name)
}

func (m *Model) back() {
	if len(m.history) == 0 {
		return
	}
	p := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.catList.cursor = p.cat
	m.entList[p.cat].cursor = p.cursor
	m.depth = depthEntries
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case execDoneMsg:
		if msg.err != nil {
			m.setError(msg.label + ": " + msg.err.Error())
		} else {
			m.setStatus(msg.label + ": done")
		}
		if msg.reload {
			m.reload()
		}

	case opDoneMsg:
		m.opDone(msg)

	case tea.KeyMsg:
		if m.prompt != nil {
			cmd = m.promptKey(msg)
		} else if m.filtering {
			m.filterKey(msg)
		} else if msg.Type == tea.KeyRunes && !msg.Paste && !msg.Alt {
			// Runes that arrive together (e.g. from key repeat) are
			// delivered as one message; handle them one key at a time.
			for _, r := range msg.Runes {
				if cmd != nil {
					break
				}
				// Keys typed right after "/" or "n" belong to the input.
				one := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
				if m.prompt != nil {
					cmd = m.promptKey(one)
				} else if m.filtering {
					m.filterKey(one)
				} else {
					cmd = m.key(string(r))
				}
				// Keep the cursors in their lists for the next key.
				m.fix()
			}
		} else {
			cmd = m.key(msg.String())
		}
	}

	m.fix()
	return m, cmd
}

func (m *Model) filterKey(msg tea.KeyMsg) {
	l := m.filterable()
	if l == nil {
		m.filtering = false
		return
	}

	switch msg.Type {
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyEsc, tea.KeyCtrlC:
		m.filtering = false
		m.clearFilter(l)
		return
	case tea.KeyBackspace:
		if l.filter == "" {
			m.filtering = false
		} else {
			r := []rune(l.filter)
			l.filter = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		l.filter += string(msg.Runes)
	default:
		return
	}
	l.cursor = 0
}

func (m *Model) key(k string) tea.Cmd {
	prefix := m.prefix
	m.prefix = ""
	if prefix != "" {
		return m.prefixKey(prefix + k)
	}

	if cmd, ok := m.targetKey(k); ok {
		return cmd
	}

	l, n := m.current()
	half := max(m.listHeight()/2, 1)

	switch k {
	case "q", "ctrl+c":
		return tea.Quit
	case "?":
		m.helpHidden = !m.helpHidden
	case "j", "down":
		l.cursor++
	case "k", "up":
		l.cursor--
	case "ctrl+d":
		l.cursor += half
	case "ctrl+u":
		l.cursor -= half
	case "ctrl+f", "pgdown":
		l.cursor += m.listHeight()
	case "ctrl+b", "pgup":
		l.cursor -= m.listHeight()
	case "G", "end":
		l.cursor = n - 1
	case "home":
		l.cursor = 0
	case "g", "z":
		m.prefix = k
	case "h", "left", "backspace":
		m.depth = max(m.depth-1, depthCategories)
	case "l", "right", "enter":
		return m.enter(k == "right")
	case "H":
		m.back()
	case "]", "[":
		if m.depth == depthEntries {
			m.jumpRepo(k == "]")
		}
	case "/":
		if m.depth == depthCategories {
			m.depth = depthEntries
		}
		if l := m.filterable(); l != nil {
			m.filtering = true
			l.filter = ""
		}
	case "esc":
		if l := m.filterable(); l != nil && l.filter != "" {
			m.clearFilter(l)
		}
		m.status = ""
	case ".":
		m.toggleForeign()
	case "e":
		return m.edit()
	case "R":
		if m.reload() {
			m.setStatus("reloaded")
		}
	default:
		if e := m.Selected(); e != nil && m.depth < depthOptions {
			for _, a := range e.Actions {
				if a.Key == k {
					return m.run(a)
				}
			}
		}
	}

	return nil
}

func (m *Model) prefixKey(seq string) tea.Cmd {
	switch seq {
	case "gg":
		l, _ := m.current()
		l.cursor = 0
	case "zh":
		m.toggleForeign()
	default:
		if e := m.Selected(); e != nil && strings.HasPrefix(seq, "g") {
			for _, l := range e.Links {
				if "g"+l.Key == seq {
					m.follow(l)
					return nil
				}
			}
		}
	}
	return nil
}

func (m *Model) toggleForeign() {
	// Keep the cursor on the current entry if it stays visible.
	ci := m.catList.cursor
	if ci >= len(m.cats) {
		return
	}
	sel := m.Selected()
	m.hideForeign[ci] = !m.hideForeign[ci]
	if sel != nil {
		m.selectEntry(ci, sel.FullName)
	}
	if !m.hideForeign[ci] {
		m.setStatus("showing packages from all repos")
	} else {
		m.setStatus("showing local packages only")
	}
}

// enter descends one level.  On an entry without fields (and not using the
// right arrow) it opens the entry's file in the editor instead, like ranger
// opens files.
func (m *Model) enter(arrow bool) tea.Cmd {
	switch m.depth {
	case depthCategories:
		m.depth = depthEntries
	case depthEntries:
		e := m.Selected()
		if e != nil && len(e.Fields) > 0 {
			m.depth = depthFields
		} else if !arrow {
			return m.edit()
		}
	case depthFields:
		if f := m.field(); f != nil && m.localTarget() != nil {
			m.opts = m.options(f)
			cur := max(indexOf(m.opts, f.Value), 0)
			m.optList = list{cursor: cur, offset: cur - m.listHeight()/2}
			m.depth = depthOptions
		}
	case depthOptions:
		if !arrow {
			m.choose()
		}
	}
	return nil
}

// choose sets the field being edited to the option under the cursor.
func (m *Model) choose() {
	e, f, o := m.Selected(), m.field(), m.option()
	if e == nil || f == nil || o == nil {
		return
	}

	m.depth = depthFields
	m.optList = list{}
	if o.FullName == f.Value {
		m.setStatus(f.Label + " unchanged")
		return
	}

	if err := e.Set(f.Var, o.FullName); err != nil {
		m.setError(err.Error())
		return
	}
	if m.reload() {
		if o.FullName == "" {
			m.setStatus(fmt.Sprintf("%s: unset %s", e.Name, f.Var))
		} else {
			m.setStatus(fmt.Sprintf("%s: set %s to %s", e.Name, f.Var,
				o.FullName))
		}
	}
}

func (m *Model) run(a Action) tea.Cmd {
	if a.Confirm != "" {
		m.prompt = &prompt{confirm: true, label: a.Confirm + " (y/N) ",
			done: func(m *Model, _ string) tea.Cmd {
				a.Confirm = ""
				return m.run(a)
			}}
		return nil
	}
	return m.runWith(a, nil)
}

// runWith runs an action's newt command with extra environment variables.
func (m *Model) runWith(a Action, env []string) tea.Cmd {
	label := strings.Join(append(append([]string{}, env...), "newt"), " ") +
		" " + strings.Join(a.Args, " ")
	return tea.Exec(m.newtCmd([][]string{a.Args}, env),
		func(err error) tea.Msg {
			return execDoneMsg{label: label, err: err}
		})
}

func (m *Model) edit() tea.Cmd {
	e := m.Selected()
	if e == nil || e.EditPath == "" {
		return nil
	}
	c, err := editorExec(e.EditPath)
	if err != nil {
		m.setError(err.Error())
		return nil
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{label: "edit " + e.EditPath, err: err,
			reload: true}
	})
}

/* View */

type row struct {
	text  string
	hint  string
	style lipgloss.Style
	// A group header, e.g. a repo, drawn as a rule; the cursor skips it.
	header bool
}

// renderList draws a column of rows, highlighting the cursor row.
func renderList(rows []row, l list, width int, height int,
	showCursor bool) []string {

	out := make([]string, 0, height)
	for i := l.offset; i < len(rows) && len(out) < height; i++ {
		r := rows[i]
		if r.header {
			line := " ── " + r.text + " "
			line += strings.Repeat("─", max(width-ansi.StringWidth(line), 0))
			out = append(out, styleGroup.Render(ansi.Truncate(line, width, "…")))
			continue
		}
		sel := showCursor && i == l.cursor

		text := " " + r.text
		hint := ""
		if r.hint != "" {
			hint = " " + r.hint + " "
		}
		// Squeeze, then drop the hint if there is no room for it.
		if ansi.StringWidth(text)+ansi.StringWidth(hint) > width {
			hint = strings.TrimSuffix(hint, " ")
		}
		if ansi.StringWidth(text)+ansi.StringWidth(hint) > width {
			hint = ""
		}
		text = ansi.Truncate(text, width-ansi.StringWidth(hint), "…")
		pad := width - ansi.StringWidth(text) - ansi.StringWidth(hint)
		text += strings.Repeat(" ", max(pad, 0))

		if sel {
			out = append(out, r.style.Inherit(styleSel).Render(text)+
				styleSelDim.Render(hint))
		} else {
			out = append(out, r.style.Render(text)+styleDim.Render(hint))
		}
	}
	return out
}

func (m *Model) categoryRows() []row {
	rows := make([]row, len(m.cats))
	for i, c := range m.cats {
		rows[i] = row{
			text:  c.Title,
			hint:  fmt.Sprint(len(m.visible(i))),
			style: kindStyle(c.Kind).Bold(true),
		}
	}
	return rows
}

// grouped tells whether a category's entries come from more than one repo.
func (m *Model) grouped(ci int) bool {
	entries := m.visible(ci)
	for _, e := range entries {
		if e.Repo != entries[0].Repo {
			return true
		}
	}
	return false
}

// entryLayout returns the rows of a category's entries and the row of each
// entry.  Entries from more than one repo are grouped under a header per
// repo, which replaces the "@repo" hint of each entry.
func (m *Model) entryLayout(ci int) ([]row, []int) {
	entries := m.visible(ci)
	grouped := m.grouped(ci)
	rows := make([]row, 0, len(entries))
	rowOf := make([]int, len(entries))
	for i, e := range entries {
		r := row{text: e.Name, hint: e.Hint, style: kindStyle(e.Kind)}
		if grouped {
			if i == 0 || e.Repo != entries[i-1].Repo {
				rows = append(rows, row{text: e.Repo, header: true})
			}
			if r.hint == "@"+e.Repo {
				r.hint = ""
			}
		}
		rowOf[i] = len(rows)
		rows = append(rows, r)
	}
	return rows, rowOf
}

// entryList draws the entries of a category.
func (m *Model) entryList(ci int, width int, height int,
	showCursor bool) []string {

	rows, rowOf := m.entryLayout(ci)
	l := m.entList[ci]
	if l.cursor >= 0 && l.cursor < len(rowOf) {
		l.cursor = rowOf[l.cursor]
	}
	return renderList(rows, l, width, height, showCursor)
}

func (m *Model) fieldRows(e *Entry) []row {
	rows := make([]row, len(e.Fields))
	for i, f := range e.Fields {
		val, repo := splitName(f.Value)
		if val == "" {
			val = "-"
		}
		if repo != "" {
			repo = "@" + repo
		}
		rows[i] = row{text: fmt.Sprintf("%-8s %s", f.Label, val), hint: repo,
			style: kindStyle(f.Kind)}
	}
	return rows
}

func optionRows(opts []*Entry, current string) []row {
	rows := make([]row, len(opts))
	for i, o := range opts {
		r := row{text: o.Name, hint: o.Hint, style: kindStyle(o.Kind)}
		if o.FullName == current {
			r.style = r.style.Bold(true)
			r.hint = "current"
		}
		rows[i] = r
	}
	return rows
}

func optionPreview(f *Field, o *Entry) []string {
	if p := o.Preview(); p != nil {
		return p
	}
	if o.FullName == "" {
		return []string{styleDim.Render("Unset " + f.Var)}
	}
	return []string{field(f.Label, styleBold.Render(o.Name))}
}

func previewLines(lines []string, width int, height int) []string {
	out := []string{}
	for _, line := range lines {
		// Keep a one column margin on wrapped lines too.
		wrapped := ansi.Wrap(line, max(width-1, 1), "")
		for _, w := range strings.Split(wrapped, "\n") {
			if len(out) == height {
				return out
			}
			out = append(out, " "+w)
		}
	}
	return out
}

// column pads lines to an exact width x height block.
func column(lines []string, width int, height int) string {
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).
		Height(height).MaxHeight(height)
	return style.Render(strings.Join(lines, "\n"))
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	h := m.listHeight()

	// Ranger's default column ratio is 1:3:4.
	avail := m.width - 2
	w1 := max(avail/8, 8)
	w2 := max(avail*3/8, 10)
	w3 := max(avail-w1-w2, 0)

	var left, mid, right []string
	ci := m.catList.cursor

	if m.depth == depthCategories {
		left = renderList([]row{{text: m.title, hint: m.note,
			style: styleBold}},
			list{}, w1, h, true)
		mid = renderList(m.categoryRows(), m.catList, w2, h, true)
		if ci < len(m.cats) {
			right = m.entryList(ci, w3, h, false)
		}
	} else if m.depth == depthEntries {
		left = renderList(m.categoryRows(), m.catList, w1, h, true)
		mid = m.entryList(ci, w2, h, true)
		if len(mid) == 0 {
			mid = []string{styleDim.Render(" empty")}
		}
		if e := m.Selected(); e != nil {
			right = previewLines(e.Preview(), w3, h)
		}
	} else if e := m.Selected(); m.depth == depthFields {
		left = m.entryList(ci, w1, h, true)
		mid = renderList(m.fieldRows(e), m.fieldList, w2, h, true)
		if f := m.field(); f != nil {
			// Preview the choices with the current value highlighted.
			opts := m.options(f)
			idx := indexOf(opts, f.Value)
			l := list{cursor: idx, offset: max(min(idx-h/2, len(opts)-h), 0)}
			right = renderList(optionRows(opts, f.Value), l, w3, h, idx >= 0)
		}
	} else {
		f := m.field()
		left = renderList(m.fieldRows(e), m.fieldList, w1, h, true)
		mid = renderList(optionRows(m.visibleOpts(), f.Value), m.optList,
			w2, h, true)
		if len(mid) == 0 {
			mid = []string{styleDim.Render(" no match")}
		}
		if o := m.option(); o != nil {
			right = previewLines(optionPreview(f, o), w3, h)
		}
	}

	sep := column(strings.Split(strings.Repeat("│\n", h), "\n")[:h], 1, h)
	sep = styleSep.Render(sep)
	body := joinColumns(column(left, w1, h), sep, column(mid, w2, h), sep,
		column(right, w3, h))

	out := m.titleBar() + "\n" + body + "\n"
	if pane := m.helpPane(); pane != nil {
		out += strings.Join(pane, "\n") + "\n"
	}
	return out + m.statusBar()
}

func joinColumns(cols ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func (m *Model) titleBar() string {
	name := styleBold.Render(m.title)
	if m.note != "" {
		name += styleDim.Render(" (" + m.note + ")")
	}
	parts := []string{stylePath.Render("newt") + " " + name}
	ci := m.catList.cursor
	if ci < len(m.cats) {
		parts = append(parts, m.cats[ci].Title)
		if e := m.Selected(); e != nil {
			parts = append(parts, kindStyle(e.Kind).Bold(true).Render(e.Name))
		}
		if f := m.field(); f != nil {
			parts = append(parts, f.Label)
		}
	}
	return ansi.Truncate(strings.Join(parts, styleDim.Render(" › ")),
		m.width, "…")
}

func (m *Model) statusBar() string {
	l, n := m.current()
	pos := fmt.Sprintf("%d/%d", min(l.cursor+1, n), n)

	var left string
	switch {
	case m.prompt != nil && m.prompt.confirm:
		left = styleErr.Render(m.prompt.label)
	case m.prompt != nil:
		left = styleKey.Render(m.prompt.label) + m.prompt.input +
			styleSel.Render(" ")
	case m.filtering:
		left = styleKey.Render("/") + l.filter + styleSel.Render(" ")
	case m.prefix != "":
		left = styleKey.Render(m.prefix)
	case m.status != "" && m.statusErr:
		left = styleErr.Render(m.status)
	case m.status != "":
		left = m.status
	case m.helpPane() == nil:
		left = styleKey.Render("?") + styleDim.Render(" show help")
	}

	right := pos
	if l.filter != "" && !m.filtering {
		right = styleDim.Render("filter: ") + l.filter + "  " + pos
	}
	if m.depth <= depthEntries && m.catList.cursor < len(m.cats) &&
		m.hideForeign[m.catList.cursor] {
		right = styleDim.Render("local ") + right
	}

	space := m.width - ansi.StringWidth(right) - 1
	left = ansi.Truncate(left, max(space, 0), "…")
	pad := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", pad) + right
}

// clearFilter removes a filter, keeping the cursor on the selected entry.
func (m *Model) clearFilter(l *list) {
	sel := m.Selected()
	l.filter = ""
	if sel != nil && m.depth == depthEntries {
		m.selectEntry(m.catList.cursor, sel.FullName)
	}
}
