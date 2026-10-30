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
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/kballard/go-shellquote"
)

// Jobs run newt commands in the background, e.g. "newt build", while the
// browser stays usable.  Their output is shown in a pane at the bottom of
// the screen.  Only one job runs at a time.

// Lines of output the pane keeps.
const jobKeepLines = 2000

type job struct {
	id      int
	label   string
	command string
	target  string
	start   time.Time
	end     time.Time
	running bool
	stopped bool
	steps   [][]string
	err     error
	cancel  context.CancelFunc
	// Closed when the commands have stopped.
	finished chan struct{}

	mu     sync.Mutex
	lines  []string
	output strings.Builder
}

// lineWriter collects a job's output, splitting it into lines; a carriage
// return starts the line over, as progress bars do.
type lineWriter struct {
	j       *job
	partial string
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.j.mu.Lock()
	defer w.j.mu.Unlock()

	w.j.output.Write(p)
	for _, r := range string(p) {
		switch r {
		case '\n':
			w.j.lines = append(w.j.lines, w.partial)
			if len(w.j.lines) > jobKeepLines {
				w.j.lines = w.j.lines[len(w.j.lines)-jobKeepLines:]
			}
			w.partial = ""
		case '\r':
			w.partial = ""
		default:
			w.partial += string(r)
		}
	}
	return len(p), nil
}

// tail returns the last n lines of output, including an unfinished one.
func (j *job) tail(n int, partial string) []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	lines := j.lines
	if partial != "" {
		lines = append(append([]string{}, lines...), partial)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return append([]string{}, lines...)
}

func (j *job) elapsed() time.Duration {
	end := j.end
	if j.running {
		end = time.Now()
	}
	return end.Sub(j.start).Round(time.Second)
}

type jobOutputMsg struct{ id int }
type jobDoneMsg struct {
	id  int
	err error
}

// runSteps runs newt commands one after another into out, stopping at the
// first failure; replaced in tests.
var runSteps = func(ctx context.Context, steps [][]string, env []string,
	out io.Writer) error {

	self, err := os.Executable()
	if err != nil {
		self = "newt"
	}
	for i, args := range steps {
		if len(steps) > 1 {
			fmt.Fprintf(out, "[%d/%d]\n", i+1, len(steps))
		}
		fmt.Fprintf(out, "$ %s\n", shellquote.Join(append(append(
			[]string{}, env...), append([]string{"newt"}, args...)...)...))

		cmd := exec.Command(self, args...)
		cmd.Stdout = out
		cmd.Stderr = out
		if len(env) > 0 {
			cmd.Env = append(os.Environ(), env...)
		}
		ownProcessGroup(cmd)
		if err := cmd.Start(); err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
		case <-ctx.Done():
			killProcessGroup(cmd)
			<-done
			return fmt.Errorf("stopped")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Model) jobRunning() bool {
	return m.job != nil && m.job.running
}

// startJob runs newt commands in the background and shows their output.
func (m *Model) startJob(a Action, steps [][]string, env []string,
	label string) tea.Cmd {

	if m.jobRunning() {
		m.setError("wait for " + m.job.label + " to finish, or stop it " +
			"with Ctrl-C")
		return nil
	}

	target := ""
	if e := m.Selected(); e != nil {
		target = e.Name
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.jobSeq++
	j := &job{id: m.jobSeq, label: label, command: a.Args[0],
		target: target, start: time.Now(), running: true, cancel: cancel,
		finished: make(chan struct{}), steps: steps}
	m.job = j
	m.dockShown = true
	m.setStatus(label + "...")

	w := &lineWriter{j: j}
	m.jobPartial = func() string {
		j.mu.Lock()
		defer j.mu.Unlock()
		return w.partial
	}

	ch := make(chan tea.Msg, 1)
	m.jobCh = ch
	run := m.runSteps
	go func() {
		err := run(ctx, steps, env, w)
		close(j.finished)
		ch <- jobDoneMsg{j.id, err}
	}()

	return m.waitJob()
}

// waitJob wakes the browser when the job is done, and ten times a second
// until then to show new output.
func (m *Model) waitJob() tea.Cmd {
	ch, id := m.jobCh, m.job.id
	return func() tea.Msg {
		select {
		case msg := <-ch:
			return msg
		case <-time.After(100 * time.Millisecond):
			return jobOutputMsg{id}
		}
	}
}

func (m *Model) jobMsg(msg tea.Msg) tea.Cmd {
	j := m.job
	switch msg := msg.(type) {
	case jobOutputMsg:
		if j != nil && j.id == msg.id && j.running {
			return m.waitJob()
		}
	case jobDoneMsg:
		if j == nil || j.id != msg.id {
			return nil
		}
		j.running = false
		j.end = time.Now()
		j.err = msg.err
		j.cancel()

		label := fmt.Sprintf("%s (%s)", j.label, j.elapsed())
		if msg.err != nil {
			m.setError(label + ": " + msg.err.Error())
		} else {
			m.setStatus(label + ": done")
		}
		// Builds, loads and cleans change what the browser shows, e.g. the
		// image version or the build status.
		ok := m.reload()
		j.mu.Lock()
		out := j.output.String()
		j.mu.Unlock()
		if ok && msg.err == nil && createsImage(j.steps) {
			m.imageCreated(j.target)
		}
		if ok {
			m.commandDone(execDoneMsg{label: label, err: msg.err,
				command: j.command, target: j.target, output: out})
		}
	}
	return nil
}

// stopJob stops the running job, e.g. on Ctrl-C.
func (m *Model) stopJob() {
	if m.jobRunning() {
		m.job.stopped = true
		m.job.cancel()
		m.setStatus("stopping " + m.job.label + "...")
	}
}

// quit leaves the browser, after asking to stop a running job.
func (m *Model) quit() tea.Cmd {
	if !m.jobRunning() {
		m.closeConsole()
		return tea.Quit
	}
	j := m.job
	m.prompt = &prompt{confirm: true,
		label: "Stop " + j.label + " and quit? (y/N) ",
		done: func(m *Model, _ string) tea.Cmd {
			j.cancel()
			m.closeConsole()
			return func() tea.Msg {
				select {
				case <-j.finished:
				case <-time.After(5 * time.Second):
				}
				return tea.QuitMsg{}
			}
		}}
	return nil
}

// dockHeight is the number of rows of the panes at the bottom of the
// screen (the output pane and the console), or 0.
func (m *Model) dockHeight() int {
	jobShown := m.job != nil && m.dockShown
	switch {
	case m.height < 14 || (!jobShown && m.console == nil):
		return 0
	case m.console != nil:
		return max(min(m.height*2/5, 20), 6)
	default:
		return max(min(m.height/3, 16), 5)
	}
}

// dockView draws the output pane: a title line and the last lines of
// output.
func (m *Model) dockView(width int, h int) []string {
	j := m.job

	state := styleKey.Render("running") + styleDim.Render(
		" · Ctrl-C stops it")
	switch {
	case j.running:
	case j.stopped:
		state = styleErr.Render("stopped") + styleDim.Render(" · Esc hides this")
	case j.err != nil:
		state = styleErr.Render("failed") + styleDim.Render(" · Esc hides this")
	default:
		state = styleOk.Render("done") + styleDim.Render(" · Esc hides this")
	}
	title := fmt.Sprintf("── %s · %s · %s ", j.label, j.elapsed(), state)
	title = ansi.Truncate(title, width, "…")
	if pad := width - ansi.StringWidth(title); pad > 0 {
		title += styleDockSep.Render(strings.Repeat("─", pad))
	}

	partial := ""
	if m.jobPartial != nil {
		partial = m.jobPartial()
	}
	lines := []string{styleDockSep.Render(title)}
	for _, l := range j.tail(h-1, partial) {
		l = strings.ReplaceAll(ansi.Strip(l), "\t", "    ")
		lines = append(lines, " "+ansi.Truncate(l, width-1, "…"))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}
