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

// prompt is a question shown in the status bar, either asking for text (a
// target name) or for a y/N confirmation.
type prompt struct {
	label   string
	input   string
	confirm bool
	done    func(m *Model, input string) tea.Cmd
	// Shown in the help pane instead of the default prompt help.
	title string
	help  [][2]string
	// For a choice: the value for each key, e.g. "v" -> "vim".
	choices map[string]string
}

// opDoneMsg reports the end of a background target operation.
type opDoneMsg struct {
	label string
	out   string
	err   error
	cmds  [][]string
	// Target to select after the project is reloaded.
	selectName string
	// Whether to open the selected target's settings.
	fields bool
}

// targetName converts a name typed by the user the same way "newt target
// create" does: "naked" names live in the targets directory.
func targetName(name string) string {
	if !strings.Contains(name, "/") {
		return "targets/" + name
	}
	return name
}

func (m *Model) promptKey(msg tea.KeyMsg) tea.Cmd {
	p := m.prompt

	if p.choices != nil {
		if v, ok := p.choices[msg.String()]; ok {
			m.prompt = nil
			return p.done(m, v)
		}
		if msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC {
			m.prompt = nil
			m.setStatus("files open in " + m.editorName())
		}
		return nil
	}

	if p.confirm {
		m.prompt = nil
		if msg.String() == "y" || msg.String() == "Y" {
			return p.done(m, "")
		}
		m.setStatus("cancelled")
		return nil
	}

	switch msg.Type {
	case tea.KeyEnter:
		m.prompt = nil
		input := strings.TrimSpace(p.input)
		if input == "" {
			m.setStatus("cancelled")
			return nil
		}
		return p.done(m, input)
	case tea.KeyEsc, tea.KeyCtrlC:
		m.prompt = nil
		m.setStatus("cancelled")
	case tea.KeyBackspace:
		if r := []rune(p.input); len(r) > 0 {
			p.input = string(r[:len(r)-1])
		}
	case tea.KeyRunes:
		p.input += string(msg.Runes)
	}
	return nil
}

// runOps runs newt commands one after another in the background, stopping
// at the first failure.
func (m *Model) runOps(label string, selectName string, fields bool,
	cmds ...[]string) tea.Cmd {

	run := m.newtRun
	m.setStatus(label + "...")
	return func() tea.Msg {
		for _, args := range cmds {
			if out, err := run(args); err != nil {
				return opDoneMsg{label: label, out: out, err: err}
			}
		}
		return opDoneMsg{label: label, selectName: selectName,
			fields: fields, cmds: cmds}
	}
}

func (m *Model) opDone(msg opDoneMsg) {
	ok := m.reload()
	if msg.err != nil {
		m.setError(msg.label + ": " + errorLine(msg.out, msg.err))
		return
	}
	if !ok {
		return
	}

	m.setStatus(msg.label + ": done")
	if createsImage(msg.cmds) {
		for _, e := range m.cats[m.findKind(KindTarget)].Entries {
			if e.FullName == msg.selectName {
				m.imageCreated(e.Name)
			}
		}
	}
	ci := m.findKind(KindTarget)
	if msg.selectName == "" || ci < 0 {
		return
	}
	m.entList[ci].filter = ""
	if m.selectEntry(ci, msg.selectName) {
		m.catList.cursor = ci
		m.depth = depthEntries
		if msg.fields {
			m.depth = depthFields
			m.fieldList = list{}
		}
	}
}

// errorLine picks the error message out of newt's output, which may be
// followed by usage text.
func errorLine(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if msg, ok := strings.CutPrefix(line, "Error: "); ok {
			return strings.TrimSpace(msg)
		}
	}
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return last
	}
	return err.Error()
}

// inTargets tells whether the cursor is in the Targets category.
func (m *Model) inTargets() bool {
	ci := m.catList.cursor
	return ci >= 0 && ci < len(m.cats) && m.cats[ci].Kind == KindTarget
}

// localTarget returns the selected target if it can be modified.
func (m *Model) localTarget() *Entry {
	e := m.Selected()
	if e == nil || e.Kind != KindTarget {
		return nil
	}
	if e.Foreign {
		m.setError(e.FullName + " is from repo " + e.Hint +
			"; only local targets can be changed")
		return nil
	}
	return e
}

// targetKey handles the target management keys; it returns false if the key
// is not one of them.
func (m *Model) targetKey(k string) (tea.Cmd, bool) {
	if !m.inTargets() || m.depth > depthFields {
		return nil, false
	}

	switch k {
	case "n":
		m.prompt = &prompt{label: "New target name: ",
			done: func(m *Model, name string) tea.Cmd {
				return m.runOps("create "+name, targetName(name), true,
					[]string{"target", "create", name})
			}}

	case "y":
		e := m.localTarget()
		if e == nil {
			break
		}
		m.prompt = &prompt{label: "Copy " + e.Name + " to: ",
			input: e.Name + "_copy",
			done: func(m *Model, name string) tea.Cmd {
				return m.runOps("copy "+e.Name+" to "+name, targetName(name),
					false, []string{"target", "copy", e.FullName, name})
			}}

	case "a":
		e := m.localTarget()
		if e == nil {
			break
		}
		// newt has no rename command; copy the target, then delete it.
		m.prompt = &prompt{label: "Rename " + e.Name + " to: ",
			input: e.Name,
			done: func(m *Model, name string) tea.Cmd {
				if targetName(name) == e.FullName {
					m.setStatus("name unchanged")
					return nil
				}
				return m.runOps("rename "+e.Name+" to "+name,
					targetName(name), false,
					[]string{"target", "copy", e.FullName, name},
					[]string{"target", "delete", "-f", e.FullName})
			}}

	case "D":
		e := m.localTarget()
		if e == nil {
			break
		}
		m.prompt = &prompt{confirm: true,
			label: "Delete " + e.FullName + " and its directory? (y/N) ",
			done: func(m *Model, _ string) tea.Cmd {
				return m.runOps("delete "+e.Name, "", false,
					[]string{"target", "delete", "-f", e.FullName})
			}}

	default:
		return nil, false
	}

	return nil, true
}
