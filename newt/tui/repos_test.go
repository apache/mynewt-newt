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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const testProjectYml = `project.name: "my_project"

project.repositories:
    - apache-mynewt-core

# The core repo.
repository.apache-mynewt-core:
    type: github
    vers: 0.0.0   # latest
    user: apache
    repo: mynewt-core

project.repositories.ignored:
#    - lvgl
`

func TestRepoVers(t *testing.T) {
	if v := repoVers(testProjectYml, "apache-mynewt-core"); v != "0.0.0" {
		t.Errorf("core: %q", v)
	}
	if v := repoVers(testProjectYml, "apache-mynewt-nimble"); v != "" {
		t.Errorf("nimble: %q", v)
	}
}

func TestSetRepoVers(t *testing.T) {
	github := repoOrigin{Type: "github", User: "apache",
		Repo: "mynewt-nimble"}

	// Only the vers line of a listed repo changes.
	out, err := setRepoVers(testProjectYml, "apache-mynewt-core",
		"master-commit", github)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(testProjectYml, "    vers: 0.0.0   # latest",
		"    vers: master-commit", 1)
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}

	// A dependency gets a block of its own.
	out, err = setRepoVers(testProjectYml, "apache-mynewt-nimble",
		"master-commit", github)
	if err != nil {
		t.Fatal(err)
	}
	want = testProjectYml + `
repository.apache-mynewt-nimble:
    type: github
    vers: master-commit
    user: apache
    repo: mynewt-nimble
`
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if v := repoVers(out, "apache-mynewt-nimble"); v != "master-commit" {
		t.Errorf("read back %q", v)
	}

	// Repos from a local path cannot be declared for the user.
	if _, err := setRepoVers(testProjectYml, "mine", "x-commit",
		repoOrigin{}); err == nil {
		t.Errorf("expected an error for a repo without an origin")
	}
}

// testBranches is what git for-each-ref lists for the fake repo.
const testBranches = `refs/remotes/origin/HEAD	2026-10-23	aaa	x
refs/remotes/origin/master	2026-10-23	aaa	ci: Bump
refs/heads/my_work	2026-10-22	bbb	Work
refs/heads/master	2026-10-01	ccc	Old master
refs/remotes/origin/1_9_0_dev	2021-03-24	ddd	Release
`

// checkoutFails makes the fake git checkout fail, as with local changes.
var checkoutFails bool

// fakeGit answers the git commands of the Repos category.
func fakeGit(t *testing.T) *[]string {
	ran := []string{}
	defaultRunTool := runTool
	runTool = func(stdin string, name string, args ...string) ([]byte,
		error) {
		if name != "git" {
			return nil, errors.New("not here")
		}
		cmd := strings.Join(args[2:], " ")
		ran = append(ran, cmd)
		switch {
		case strings.HasPrefix(cmd, "for-each-ref"):
			return []byte(testBranches), nil
		case cmd == "rev-parse HEAD":
			return []byte("bbb\n"), nil
		case cmd == fmt.Sprintf("log -n %d --date=short --format=%%h %%ad %%s",
			gitPaneCommits):
			// The whole repo.
			return []byte("ccc 2026-10-28 hw: Fix the clock\n" +
				"bbb 2026-10-22 apps/blinky: Blink faster\n"), nil
		case strings.HasPrefix(cmd, fmt.Sprintf("log -n %d ", gitPaneCommits)):
			return []byte("bbb 2026-10-22 apps/blinky: Blink faster\n" +
				"aaa 2026-10-20 apps: Use the new API\n"), nil
		case cmd == "symbolic-ref -q --short HEAD":
			return []byte("my_work\n"), nil
		case strings.HasPrefix(cmd, "log -n 500"):
			return []byte("bbb\tbbb\t2026-10-22\tMe\tWork\n" +
				"aaa\taaa\t2026-10-20\tYou\tci: Bump\n"), nil
		case strings.HasPrefix(cmd, "show"):
			return []byte("Work\n\nThe body.\n\n\n a.c | 2 +-\n"), nil
		case strings.HasPrefix(cmd, "fetch"):
			return nil, nil
		case strings.HasPrefix(cmd, "checkout") && checkoutFails:
			return []byte("error: Your local changes would be " +
				"overwritten by checkout"), errors.New("exit status 1")
		}
		return []byte(""), nil
	}
	t.Cleanup(func() { runTool = defaultRunTool })
	return &ran
}

func TestRepoBranches(t *testing.T) {
	ran := fakeGit(t)
	dir := t.TempDir()
	yml := dir + "/project.yml"
	os.WriteFile(yml, []byte(testProjectYml), 0644)

	ri := &RepoInfo{Name: "apache-mynewt-core", Path: dir,
		RelPath: "repos/apache-mynewt-core", Vers: "0.0.0", ProjectYml: yml,
		Origin: repoOrigin{Type: "github", User: "apache",
			Repo: "mynewt-core"}}
	load := func() ([]*Category, error) {
		return []*Category{{Title: "Repos", Kind: KindRepo,
			Entries: []*Entry{repoEntry(ri)}}}, nil
	}
	m := NewModel("proj", load)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	// l lists origin's branches and local ones that origin does not have,
	// with the checked out one marked.
	m = keys(t, m, "l", "l")
	if m.panel == nil || m.panel.title != "branches of apache-mynewt-core" {
		t.Fatalf("expected the branches, got %v (%s)", m.panel, m.status)
	}
	rows := m.panel.rows(80)
	names := []string{}
	for _, r := range rows {
		names = append(names, r.text)
	}
	if strings.Join(names, " ") != "master my_work 1_9_0_dev" {
		t.Errorf("branches %v", names)
	}
	if !strings.Contains(rows[1].hint, "checked out") ||
		!strings.Contains(rows[1].hint, "local") {
		t.Errorf("my_work hint %q", rows[1].hint)
	}

	// c shows the history of the branch under the cursor.
	m = keys(t, m, "j", "c")
	if m.panel == nil || m.panel.title != "history of apache-mynewt-core "+
		"my_work" || !strings.Contains((*ran)[len(*ran)-1], "my_work") {
		t.Fatalf("history: %v, ran %v", m.panel, *ran)
	}
	if p := strings.Join(m.panel.preview(0), "\n"); !strings.Contains(p,
		"The body.") || !strings.Contains(p, "a.c | 2 +-") {
		t.Errorf("commit preview:\n%s", p)
	}

	// Enter on a branch checks it out, then pins the repo to it.
	pin := func(m Model) Model {
		m = keys(t, m, "h", "l", "enter")
		if m.prompt == nil {
			t.Fatalf("expected a question")
		}
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes,
			Runes: []rune("y")})
		return update(t, next.(Model), cmd())
	}

	// A checkout that git refuses leaves project.yml alone.
	checkoutFails = true
	m = pin(m)
	checkoutFails = false
	got, _ := os.ReadFile(yml)
	if string(got) != testProjectYml || !m.statusErr ||
		!strings.Contains(m.status, "local changes") {
		t.Errorf("status %q, project.yml:\n%s", m.status, got)
	}

	m = pin(m)
	if last := (*ran)[len(*ran)-1]; last != "checkout -q aaa" {
		t.Errorf("ran %q", last)
	}
	got, _ = os.ReadFile(yml)
	if repoVers(string(got), "apache-mynewt-core") != "master-commit" ||
		m.panel != nil || !strings.Contains(m.status, "checked out master") {
		t.Errorf("status %q, project.yml:\n%s", m.status, got)
	}
}

func TestGitPane(t *testing.T) {
	ran := fakeGit(t)
	load := func() ([]*Category, error) {
		cats := testCats()
		for _, e := range cats[1].Entries {
			e.Repo, e.RepoDir = "core", "/repos/core"
			e.Dir = "/repos/core/" + e.Name
			e.Actions = []Action{{Key: "c", Label: "commit history",
				Fn: func(m *Model) tea.Cmd {
					return m.openHistory("history of "+e.Name, e.RepoDir,
						"HEAD", e.Dir)
				}}}
		}
		cats[0].Entries[0].Fields = []*Field{
			{Label: "app", Var: "target.app", Kind: KindApp,
				Value: "@nimble/apps/bleprph"},
			{Label: "profile", Var: "target.build_profile",
				Kind: KindValue, Value: "debug"}}
		return cats, nil
	}
	m := NewModel("proj", load)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	// In a target's settings, the pane shows the app under the cursor,
	// under the targets; the column keeps its width on other settings.
	// Below the package's commits are the repo's, read once per repo.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ll")})
	m = next.(Model)
	for _, msg := range runCmd(cmd) {
		m = update(t, m, msg)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "── git: this package ──") ||
		!strings.Contains(view, "bbb 26-10-22 apps/blinky:") ||
		!strings.Contains(view, "── git: whole repo ──") ||
		!strings.Contains(view, "ccc 26-10-28 hw: Fix the clock") ||
		!strings.Contains(strings.Join(*ran, "\n"),
			"-- /repos/core/apps/bleprph") {
		t.Errorf("target settings:\n%s", view)
	}
	at := strings.Index(strings.Split(view, "\n")[1], "│")

	// While choosing the app, it shows the app under the cursor.
	m = keys(t, m, "l")
	if view = ansi.Strip(m.View()); !strings.Contains(view, "── git: this") ||
		strings.Index(strings.Split(view, "\n")[1], "│") != at {
		t.Errorf("choosing the app:\n%s", view)
	}
	if o := m.option(); o == nil || o.Name != "apps/bleprph" {
		t.Errorf("option %v", o)
	}
	m = keys(t, m, "h")

	m = keys(t, m, "j")
	view = ansi.Strip(m.View())
	if strings.Contains(view, "── git") ||
		strings.Index(strings.Split(view, "\n")[1], "│") != at {
		t.Errorf("profile:\n%s", view)
	}
	m = keys(t, m, "h", "h")

	// The pane shows under the categories once git has answered.
	// (apps/bleprph is known already; git is read once per package.)
	m = keys(t, m, "j", "l")
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if !strings.Contains(ansi.Strip(m.View()), "reading git...") {
		t.Errorf("expected the pane to wait for git:\n%s", m.View())
	}
	m = update(t, m, runCmd(cmd)[0])
	view = ansi.Strip(m.View())
	lines := strings.Split(view, "\n")
	if !strings.HasPrefix(lines[4], "── git: this package ──") ||
		!strings.HasPrefix(lines[5], " core · my_work") ||
		!strings.HasPrefix(lines[6], " bbb 26-10-22 apps/blinky: Blink") ||
		!strings.HasPrefix(lines[7], " aaa 26-10-20 apps: Use") ||
		!strings.HasPrefix(lines[8], "── git: whole repo ──") ||
		!strings.HasPrefix(lines[9], " ccc 26-10-28 hw: Fix the clock") {
		t.Errorf("view:\n%s", view)
	}
	if last := (*ran)[len(*ran)-1]; !strings.HasSuffix(last,
		"-- /repos/core/apps/blinky") {
		t.Errorf("ran %q", last)
	}

	// zg hides it.
	m = keys(t, m, "z", "g")
	if strings.Contains(ansi.Strip(m.View()), "── git") {
		t.Errorf("zg should hide the pane")
	}

	// c lists the package's whole history.
	m = keys(t, m, "c")
	if m.panel == nil || m.panel.title != "history of apps/blinky" ||
		!strings.HasSuffix((*ran)[len(*ran)-1], "-- /repos/core/apps/blinky") {
		t.Errorf("panel %v, ran %v", m.panel, *ran)
	}
}

// runCmd runs a command the way the tea program would, and returns its
// messages.  Commands that wait (timers, external programs) are dropped.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if batch, ok := msg.(tea.BatchMsg); ok {
			msgs := []tea.Msg{}
			for _, c := range batch {
				msgs = append(msgs, runCmd(c)...)
			}
			return msgs
		}
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(20 * time.Millisecond):
		return nil
	}
}
