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
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"mynewt.apache.org/newt/newt/config"
	"mynewt.apache.org/newt/util"
)

// The editor that opens yml and source files is chosen on the first launch
// and remembered in ~/.newt/tui.yml.  Without a choice, $VISUAL, $EDITOR
// and finally vi are used.

// Editors offered on the first launch, with the key that picks each.
var editorChoices = []struct{ key, name string }{
	{"v", "vim"},
	{"n", "nano"},
}

// tuiConfigPath is where the browser's own settings are kept; replaced in
// tests.
var tuiConfigPath = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".newt", "tui.yml")
}

// savedEditor returns the editor chosen earlier, if any.
func savedEditor() string {
	path := tuiConfigPath()
	if path == "" || !util.NodeExist(path) {
		return ""
	}
	yc, err := config.ReadFile(path)
	if err != nil {
		return ""
	}
	editor, _ := yc.GetValString("editor", nil)
	return editor
}

func saveEditor(editor string) error {
	path := tuiConfigPath()
	if path == "" {
		return util.NewNewtError("no home directory to save the editor in")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return util.ChildNewtError(err)
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return util.ChildNewtError(err)
	}
	data = []byte(setYmlVar(string(data), "editor", editor))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return util.ChildNewtError(err)
	}
	return nil
}

// installedEditors returns the offered editors that are installed.
func installedEditors() map[string]string {
	choices := map[string]string{}
	for _, c := range editorChoices {
		if _, err := lookPath(c.name); err == nil {
			choices[c.key] = c.name
		}
	}
	return choices
}

// askEditor asks which editor to use.  On the first launch, Esc leaves the
// question for the next launch.
func (m *Model) askEditor(first bool) {
	choices := installedEditors()
	if len(choices) == 0 {
		m.setError("neither vim nor nano is installed; files open in " +
			"$EDITOR")
		return
	}

	options := []string{}
	help := [][2]string{}
	for _, c := range editorChoices {
		if name, ok := choices[c.key]; ok {
			options = append(options, c.key+" "+name)
			help = append(help, [2]string{c.key, "open files in " + name})
		}
	}
	if first {
		help = append(help, [2]string{"Esc", "decide later (uses $EDITOR " +
			"for now)"})
	} else {
		help = append(help, [2]string{"Esc", "keep " + m.editorName()})
	}

	m.prompt = &prompt{
		label:   "Which editor should open files? " + strings.Join(options, " · "),
		choices: choices,
		title:   "Choose your editor",
		help:    help,
		done: func(m *Model, editor string) tea.Cmd {
			m.editor = editor
			if err := saveEditor(editor); err != nil {
				m.setError("using " + editor + ", but it was not saved: " +
					err.Error())
				return nil
			}
			m.setStatus("files open in " + editor + "; V changes it")
			return nil
		},
	}
}

// editorName is the editor that will open files.
func (m *Model) editorName() string {
	if m.editor != "" {
		return m.editor
	}
	if words, err := userEditor(""); err == nil {
		return filepath.Base(words[0])
	}
	return "vi"
}
