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
	"errors"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The git pane: in the left column, the branch of a package's repo and the
// latest commits that changed the package.  It shows the selected package
// under the categories, and a target's app or BSP under the targets.  git
// runs in the background, once per package and load.

const gitPaneCommits = 25

type gitPaneInfo struct {
	branch  string
	commits []string
	err     error
}

type gitPaneMsg struct {
	dir  string
	info *gitPaneInfo
}

// gitPaneEntry is the package the git pane shows, if it is shown.
func (m *Model) gitPaneEntry() *Entry {
	if m.gitPaneHidden || m.panel != nil || m.guide != nil {
		return nil
	}
	var e *Entry
	above := 0
	switch m.depth {
	case depthEntries:
		// The selected package, under the categories.
		e, above = m.Selected(), len(m.cats)
	case depthFields:
		// A target's app or BSP, under the targets.
		e = m.fieldPackage()
		rows, _ := m.entryLayout(m.catList.cursor)
		above = len(rows)
	case depthOptions:
		// The app or BSP under the cursor while choosing one, under the
		// target's settings.
		if m.choosingPackage() {
			e = m.option()
			above = len(m.fieldRows(m.Selected()))
		}
	}
	// It needs room for a commit.
	if e == nil || e.Dir == "" || e.RepoDir == "" ||
		m.listHeight() < above+4 {
		return nil
	}
	switch e.Kind {
	case KindApp, KindBsp, KindLib, KindUnittest:
		return e
	}
	return nil
}

// choosingPackage tells whether the user is choosing a target's app or
// BSP.
func (m *Model) choosingPackage() bool {
	f := m.field()
	return m.depth == depthOptions && f != nil &&
		(f.Kind == KindApp || f.Kind == KindBsp)
}

// targetHasPackages tells whether the selected target's settings name a
// package that the git pane can show.
func (m *Model) targetHasPackages() bool {
	e := m.Selected()
	if e == nil {
		return false
	}
	for _, f := range e.Fields {
		if (f.Kind == KindApp || f.Kind == KindBsp) && m.findPackage(f.Kind,
			f.Value) != nil {
			return true
		}
	}
	return false
}

// fieldPackage is the package that the target setting under the cursor
// names, e.g. its app or BSP.
func (m *Model) fieldPackage() *Entry {
	f := m.field()
	if f == nil || f.Value == "" || (f.Kind != KindApp && f.Kind != KindBsp) {
		return nil
	}
	return m.findPackage(f.Kind, f.Value)
}

// findPackage returns the package of a kind with a full name.
func (m *Model) findPackage(kind Kind, fullName string) *Entry {
	if ci := m.findKind(kind); ci >= 0 && fullName != "" {
		for _, e := range m.cats[ci].Entries {
			if e.FullName == fullName && e.Dir != "" {
				return e
			}
		}
	}
	return nil
}

// loadGitPane reads the selected package's git history, and its repo's,
// in the background, if the pane needs them.  The repo's history is kept
// under the repo's directory.
func (m *Model) loadGitPane() tea.Cmd {
	e := m.gitPaneEntry()
	if e == nil {
		return nil
	}
	cmds := []tea.Cmd{}
	for _, path := range []string{e.Dir, e.RepoDir} {
		if m.gitInfo[path] != nil || m.gitLoading[path] {
			continue
		}
		m.gitLoading[path] = true
		cmds = append(cmds, readGitLog(e.RepoDir, path))
	}
	return tea.Batch(cmds...)
}

// readGitLog reads the latest commits that changed a path of a repo; the
// repo's own directory gives all of its commits.
func readGitLog(repoDir string, path string) tea.Cmd {
	return func() tea.Msg {
		info := &gitPaneInfo{branch: branchName(repoDir)}
		args := []string{"log", "-n", strconv.Itoa(gitPaneCommits),
			"--date=short", "--format=%h %ad %s"}
		if path != repoDir {
			args = append(args, "--", path)
		}
		out, err := git(repoDir, args...)
		if err != nil {
			info.err = err
			if why := err.Error(); strings.Contains(why, "not a git "+
				"repository") || strings.Contains(why, "filesystem boundary") {
				info.err = errors.New("not in a git repository")
			}
		} else if out = strings.TrimRight(out, "\n"); out != "" {
			info.commits = strings.Split(out, "\n")
		}
		return gitPaneMsg{path, info}
	}
}

func (m *Model) gitPaneDone(msg gitPaneMsg) {
	delete(m.gitLoading, msg.dir)
	m.gitInfo[msg.dir] = msg.info
}

// branchName describes what a repo has checked out: its branch, or the
// branches whose tip a detached checkout is at.
func branchName(dir string) string {
	if b, err := git(dir, "symbolic-ref", "-q", "--short", "HEAD"); err == nil {
		return strings.TrimSpace(b)
	}
	if on := headBranches(dir); len(on) > 0 {
		return strings.Join(on, ", ") + " (detached)"
	}
	return "detached"
}

// gitPaneView returns the pane's lines for the left column: the commits
// that changed the package, under its repo and branch, and below them the
// latest commits of the whole repo, when there is room.
func (m *Model) gitPaneView(width int, height int) []string {
	e := m.gitPaneEntry()
	info, repo := m.gitInfo[e.Dir], m.gitInfo[e.RepoDir]
	// line indents a text and cuts it to the column.
	line := func(indent string, s string) string {
		return indent + ansi.Truncate(s, width-len(indent), "…")
	}
	header := func(title string) string {
		title = "── " + title + " "
		return styleGitSep.Render(ansi.Truncate(title, width, "…") +
			strings.Repeat("─", max(width-ansi.StringWidth(title), 0)))
	}

	lines := []string{header("git: this package")}
	where := e.Repo
	if info != nil && info.err == nil {
		where += " · " + info.branch
	}
	if where != "" {
		lines = append(lines, line(" ", strings.TrimPrefix(where, " · ")))
	}

	// The package gets half of the lines left for commits, or fewer if it
	// has fewer commits; the repo gets the rest.  The repo's part needs a
	// header and a commit.
	room := height - len(lines)
	own := gitPaneLines(info, width)
	if room-min(len(own), (room+1)/2) < 2 {
		lines = append(lines, own...)
	} else {
		lines = append(lines, own[:min(len(own), (room+1)/2)]...)
		lines = append(lines, header("git: whole repo"))
		lines = append(lines, gitPaneLines(repo, width)...)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// gitPaneLines shows commits read for the pane, one line each, or why
// there are none.
func gitPaneLines(info *gitPaneInfo, width int) []string {
	switch {
	case info == nil:
		return []string{styleDim.Render(" reading git...")}
	case info.err != nil:
		return []string{styleDim.Render(" " + ansi.Truncate(info.err.Error(),
			width-1, "…"))}
	case len(info.commits) == 0:
		return []string{styleDim.Render(" no commits")}
	}
	lines := []string{}
	for _, c := range info.commits {
		// "<hash> <date> <subject>"
		f := strings.SplitN(c, " ", 3)
		if len(f) < 3 {
			continue
		}
		date := f[1]
		if width < 1+len(f[0])+1+len(date)+1+20 && len(date) == 10 {
			date = date[2:]
		}
		lines = append(lines, " "+ansi.Truncate(f[0]+" "+
			styleDim.Render(date)+" "+f[2], width-1, "…"))
	}
	return lines
}
