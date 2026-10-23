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
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kballard/go-shellquote"
	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/util"
)

type execDoneMsg struct {
	label  string
	err    error
	reload bool
}

// pausedCmd runs commands one after another on the normal terminal screen,
// stopping at the first failure, and waits for the user to press Enter
// before returning to the browser, so that the output of e.g. "newt build"
// can be read.
type pausedCmd struct {
	cmds []*exec.Cmd
	// Extra environment, shown in front of the command.
	env []string
	in  io.Reader
	out io.Writer
}

func (p *pausedCmd) SetStdin(r io.Reader) {
	p.in = r
	for _, c := range p.cmds {
		c.Stdin = r
	}
}

func (p *pausedCmd) SetStdout(w io.Writer) {
	p.out = w
	for _, c := range p.cmds {
		c.Stdout = w
	}
}

func (p *pausedCmd) SetStderr(w io.Writer) {
	for _, c := range p.cmds {
		c.Stderr = w
	}
}

func (p *pausedCmd) Run() error {
	var err error
	for i, c := range p.cmds {
		if len(p.cmds) > 1 {
			fmt.Fprintf(p.out, "\n%s", styleDim.Render(fmt.Sprintf(
				"[%d/%d]", i+1, len(p.cmds))))
		}
		args := append([]string{filepath.Base(c.Args[0])}, c.Args[1:]...)
		fmt.Fprintf(p.out, "\n$ %s\n\n",
			shellquote.Join(append(append([]string{}, p.env...), args...)...))
		if err = c.Run(); err != nil {
			break
		}
	}

	if err != nil {
		fmt.Fprintf(p.out, "\n%s\n", styleErr.Render(err.Error()))
	} else {
		fmt.Fprintf(p.out, "\n%s\n", styleOk.Render("done"))
	}
	fmt.Fprint(p.out, styleDim.Render("Press Enter to return to newt tui"))
	bufio.NewReader(p.in).ReadString('\n')

	return err
}

// newtExec runs newt commands, e.g. {{"build", "t"}, {"load", "t"}}.
func newtExec(steps [][]string, env []string) tea.ExecCommand {
	self, err := os.Executable()
	if err != nil {
		self = "newt"
	}
	p := &pausedCmd{env: env}
	for _, args := range steps {
		cmd := exec.Command(self, args...)
		if len(env) > 0 {
			cmd.Env = append(os.Environ(), env...)
		}
		p.cmds = append(p.cmds, cmd)
	}
	return p
}

// newtOutput runs the newt binary in the background and returns its output.
func newtOutput(args []string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		self = "newt"
	}
	out, err := exec.Command(self, args...).CombinedOutput()
	return string(out), err
}

// userEditor returns the editor command: $VISUAL, $EDITOR or vi.
func userEditor() ([]string, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	words, err := shellquote.Split(editor)
	if err != nil || len(words) == 0 {
		return nil, util.FmtNewtError("invalid editor: %s", editor)
	}
	return words, nil
}

// editorExec opens a file in the user's editor.
func editorExec(path string) (*exec.Cmd, error) {
	editor, err := userEditor()
	if err != nil {
		return nil, err
	}
	return exec.Command(editor[0], append(editor[1:], path)...), nil
}

// placeholderName is the project name that "newt new" writes to
// project.yml; most projects keep it.
const placeholderName = "my_project"

// ProjectTitle returns the name the browser shows for a project: the
// directory that holds it, and the project.yml name as a note if that says
// something else.
func ProjectTitle(dir string, name string) (string, string) {
	title := filepath.Base(dir)
	if name == "" || name == title || name == placeholderName {
		return title, ""
	}
	return title, name
}

// Run starts the browser and blocks until the user quits.  The title is
// the project's name and the note, if any, is shown dimmed next to it.
func Run(title string, note string, load LoadFunc) error {
	// Newt status messages and log output would corrupt the screen.
	verbosity := util.Verbosity
	logOut := log.StandardLogger().Out
	util.Verbosity = util.VERBOSITY_SILENT
	log.SetOutput(io.Discard)
	defer func() {
		util.Verbosity = verbosity
		log.SetOutput(logOut)
	}()

	m := NewModel(title, load)
	m.note = note
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		return util.NewNewtError(strings.TrimSpace(err.Error()))
	}
	return nil
}
