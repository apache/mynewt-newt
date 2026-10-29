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
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

// The project's repos: their git state, branches and commit history, and
// switching a repo to a branch, which is also pinned in project.yml.

// RepoInfo is what the browser knows about a repo without running git.
type RepoInfo struct {
	Name string
	// The repo's directory, and the same relative to the project.
	Path    string
	RelPath string
	// The version that project.yml asks for; empty when the repo is not in
	// project.yml but comes with another repo's dependencies.
	Vers string
	// The project.yml file, and how a block for the repo would declare
	// where it comes from.
	ProjectYml string
	Origin     repoOrigin
}

// repoOrigin is where a repo comes from, as project.yml declares it.
type repoOrigin struct {
	Type   string
	Server string
	User   string
	Repo   string
	Url    string
}

// git runs git in a repo.
func git(dir string, args ...string) (string, error) {
	out, err := runTool("", "git", append([]string{"-C", dir}, args...)...)
	if err != nil {
		return "", util.FmtNewtError("git %s: %s", args[0],
			errorLine(string(out), err))
	}
	return string(out), nil
}

func repoEntries(proj *project.Project) []*Entry {
	ymlPath := filepath.Join(proj.Path(), project.PROJECT_FILE_NAME)
	yml, _ := os.ReadFile(ymlPath)

	repos := proj.Repos()
	names := []string{}
	for name, r := range repos {
		if !r.IsLocal() {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	entries := []*Entry{}
	for _, name := range names {
		r := repos[name]
		ri := &RepoInfo{Name: name, Path: r.Path(),
			RelPath: relPath(proj, r.Path()), Vers: repoVers(string(yml), name),
			ProjectYml: ymlPath}
		switch d := r.Downloader().(type) {
		case *downloader.GithubDownloader:
			ri.Origin = repoOrigin{Type: "github", Server: d.Server,
				User: d.User, Repo: d.Repo}
		case *downloader.GitDownloader:
			ri.Origin = repoOrigin{Type: "git", Url: d.Url}
		}
		entries = append(entries, repoEntry(ri))
	}
	return entries
}

func repoEntry(ri *RepoInfo) *Entry {
	e := &Entry{Name: ri.Name, FullName: ri.Name, Kind: KindRepo,
		EditPath: ri.ProjectYml, Hint: ri.Vers,
		OpenHelp: "branches",
		Open:     func(m *Model) tea.Cmd { return m.openBranches(ri) },
		Actions: []Action{
			{Key: "c", Label: "commit history",
				Fn: func(m *Model) tea.Cmd {
					return m.openHistory("history of "+ri.Name, ri.Path, "HEAD")
				}},
			{Key: "f", Label: "fetch",
				Fn: func(m *Model) tea.Cmd { return m.fetchRepo(ri) }},
		},
	}
	if ri.Vers == "" {
		e.Hint = "dependency"
	}
	e.previewFn = func() []string { return repoPreview(ri) }
	return e
}

// repoPreview describes a repo's checkout.
func repoPreview(ri *RepoInfo) []string {
	lines := []string{styleBold.Render(ri.Name), "",
		field("Path", ri.RelPath)}
	if ri.Vers != "" {
		lines = append(lines, field("Version", ri.Vers+
			styleDim.Render("  (project.yml)")))
	} else {
		lines = append(lines, field("Version", styleDim.Render(
			"set by the repos that depend on it")))
	}

	if _, err := os.Stat(ri.Path); err != nil {
		return append(lines, "", styleErr.Render("Not downloaded: "+
			"newt upgrade downloads it."))
	}

	head, err := git(ri.Path, "log", "-1", "--date=short",
		"--format=%h%x09%ad%x09%s")
	if err != nil {
		return append(lines, "", styleErr.Render(err.Error()))
	}
	if f := strings.SplitN(strings.TrimSpace(head), "\t", 3); len(f) == 3 {
		lines = append(lines, field("Commit", f[0]+"  "+styleDim.Render(f[1])),
			field("", f[2]))
	}

	if b, err := git(ri.Path, "symbolic-ref", "-q", "--short",
		"HEAD"); err == nil {
		lines = append(lines, field("Branch", strings.TrimSpace(b)))
	} else if on := headBranches(ri.Path); len(on) > 0 {
		lines = append(lines, field("Branch", strings.Join(on, ", ")+
			styleDim.Render("  (detached, at its tip)")))
	} else {
		lines = append(lines, field("Branch", styleDim.Render("detached")))
	}
	if tags, _ := git(ri.Path, "tag", "--points-at", "HEAD"); strings.
		TrimSpace(tags) != "" {
		lines = append(lines, field("Tags", strings.Join(strings.Fields(tags),
			", ")))
	}
	if st, err := git(ri.Path, "status", "--porcelain"); err == nil {
		if st = strings.TrimRight(st, "\n"); st == "" {
			lines = append(lines, field("State", styleOk.Render("clean")))
		} else {
			lines = append(lines, field("State", styleErr.Render(plural(
				len(strings.Split(st, "\n")), "changed file")+
				"; git may refuse to switch branches")))
		}
	}
	if url, err := git(ri.Path, "remote", "get-url", "origin"); err == nil {
		lines = append(lines, field("Remote", strings.TrimSpace(url)))
	}

	if log, err := git(ri.Path, "log", "-n", "10", "--date=short",
		"--format=%h %ad %s"); err == nil {
		lines = append(lines, section("Recent commits")...)
		lines = append(lines, strings.Split(strings.TrimRight(log, "\n"),
			"\n")...)
	}
	return lines
}

// headBranches returns the remote branches whose tip is checked out.
func headBranches(dir string) []string {
	out, err := git(dir, "branch", "-r", "--points-at", "HEAD",
		"--format=%(refname:short)")
	if err != nil {
		return nil
	}
	names := []string{}
	for _, b := range strings.Fields(out) {
		if b = strings.TrimPrefix(b, "origin/"); b != "HEAD" && b != "origin" {
			names = append(names, b)
		}
	}
	return names
}

type gitBranch struct {
	name    string
	date    string
	hash    string
	subject string
	// Only in this checkout, not on origin.
	local bool
}

// ref is the git name of the branch.
func (b gitBranch) ref() string {
	if b.local {
		return b.name
	}
	return "origin/" + b.name
}

// repoBranches lists the branches a repo can be pinned to, newest first:
// those of origin, which newt knows by name, and local ones that origin
// does not have.
func repoBranches(dir string) ([]gitBranch, error) {
	out, err := git(dir, "for-each-ref", "--sort=-committerdate",
		"--format=%(refname)%09%(committerdate:short)%09%(objectname)"+
			"%09%(subject)", "refs/remotes/origin", "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := []gitBranch{}
	onOrigin := map[string]bool{}
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		f := strings.SplitN(l, "\t", 4)
		if len(f) < 4 {
			continue
		}
		b := gitBranch{name: f[0], date: f[1], hash: f[2], subject: f[3]}
		if n, ok := strings.CutPrefix(b.name, "refs/remotes/origin/"); ok {
			if n == "HEAD" {
				continue
			}
			b.name = n
			onOrigin[n] = true
		} else {
			b.name = strings.TrimPrefix(b.name, "refs/heads/")
			b.local = true
		}
		branches = append(branches, b)
	}
	// A local branch named like one of origin's would pin origin's.
	out2 := branches[:0]
	for _, b := range branches {
		if !b.local || !onOrigin[b.name] {
			out2 = append(out2, b)
		}
	}
	return out2, nil
}

// openBranches lists a repo's branches; Enter pins the repo to one.
func (m *Model) openBranches(ri *RepoInfo) tea.Cmd {
	branches, err := repoBranches(ri.Path)
	if err != nil {
		m.setError(ri.Name + ": " + err.Error())
		return nil
	}
	head, _ := git(ri.Path, "rev-parse", "HEAD")
	head = strings.TrimSpace(head)
	logs := map[int][]string{}

	m.openPanel(&panel{
		title: "branches of " + ri.Name,
		empty: "no branches; f fetches them",
		rows: func(width int) []row {
			rows := []row{}
			for _, b := range branches {
				r := row{text: b.name, hint: b.date,
					style: kindStyle(KindRepo)}
				if b.local {
					r.hint = "local · " + r.hint
				}
				if b.hash == head {
					r.hint = "checked out · " + r.hint
					r.style = r.style.Bold(true)
				}
				if ri.Vers == b.name+"-commit" {
					r.hint = "pinned · " + r.hint
				}
				rows = append(rows, r)
			}
			return rows
		},
		preview: func(i int) []string {
			b := branches[i]
			if logs[i] == nil {
				logs[i] = []string{}
				if out, err := git(ri.Path, "log", "-n", "20", "--date=short",
					"--format=%h %ad %s", b.ref()); err == nil {
					logs[i] = strings.Split(strings.TrimRight(out, "\n"), "\n")
				}
			}
			lines := []string{styleBold.Render(b.name), "",
				field("Updated", b.date), field("Commit", b.hash[:min(
					len(b.hash), 12)]), field("", b.subject)}
			if b.hash == head {
				lines = append(lines, "", styleOk.Render("This is what "+
					ri.RelPath+" has checked out."))
			}
			if b.local {
				lines = append(lines, "", styleDim.Render("A local branch, "+
					"not on origin: a pin to it works only in this checkout."))
			}
			lines = append(lines, section("Recent commits")...)
			return append(lines, logs[i]...)
		},
		enterHelp: "check out this branch and pin it in project.yml",
		enter: func(m *Model, i int) tea.Cmd {
			return m.pinRepo(ri, branches[i])
		},
		keys: []panelKey{
			{key: "c", help: "commit history of this branch",
				// Only with a branch under the cursor.
				when: func(i int) bool { return true },
				fn: func(m *Model, i int) tea.Cmd {
					return m.openHistory("history of "+ri.Name+" "+
						branches[i].name, ri.Path, branches[i].ref())
				}},
			{key: "f", help: "fetch the branches again",
				fn: func(m *Model, i int) tea.Cmd { return m.fetchRepo(ri) }},
		},
	})
	return nil
}

// pinRepo checks out a branch in a repo, and pins the repo to it in
// project.yml.  Only this repo changes; newt upgrade is not run, as it also
// upgrades the repos that this one depends on.
func (m *Model) pinRepo(ri *RepoInfo, b gitBranch) tea.Cmd {
	vers := b.name + "-commit"
	where := ""
	if b.local {
		where = ", a local branch,"
	}
	m.prompt = &prompt{confirm: true,
		label: fmt.Sprintf("Check out %s%s in %s and pin it in project.yml "+
			"(vers: %s)? (y/N) ", b.name, where, ri.Name, vers),
		done: func(m *Model, _ string) tea.Cmd {
			m.setStatus("checking out " + b.name + " in " + ri.Name + "...")
			return func() tea.Msg {
				// Like newt, check out origin's branches detached; a local
				// branch is checked out itself, so work on it continues.
				ref := b.hash
				if b.local {
					ref = b.name
				}
				_, err := git(ri.Path, "checkout", "-q", ref)
				return repoPinnedMsg{ri, b.name, vers, err}
			}
		}}
	return nil
}

type repoPinnedMsg struct {
	repo   *RepoInfo
	branch string
	vers   string
	err    error
}

// repoPinned saves the pin in project.yml once the branch is checked out.
func (m *Model) repoPinned(msg repoPinnedMsg) {
	ri := msg.repo
	if msg.err != nil {
		m.setError(ri.Name + ": " + msg.err.Error())
		return
	}
	yml, err := os.ReadFile(ri.ProjectYml)
	if err == nil {
		var out string
		out, err = setRepoVers(string(yml), ri.Name, msg.vers, ri.Origin)
		if err == nil {
			err = os.WriteFile(ri.ProjectYml, []byte(out), 0644)
		}
	}
	if err != nil {
		m.setError(ri.Name + ": checked out " + msg.branch + ", but " +
			"project.yml is unchanged: " + err.Error())
		return
	}
	m.closePanel()
	m.reload()
	m.setStatus(fmt.Sprintf("%s: checked out %s, project.yml says vers: %s",
		ri.Name, msg.branch, msg.vers))
}

type gitCommit struct {
	hash    string
	short   string
	date    string
	author  string
	subject string
}

// openHistory lists the commits of a repo's branch or HEAD, or only those
// that changed the given paths.
func (m *Model) openHistory(title string, dir string, ref string,
	paths ...string) tea.Cmd {

	args := []string{"log", "-n", "500", "--date=short",
		"--format=%H%x09%h%x09%ad%x09%an%x09%s", ref}
	if len(paths) > 0 {
		args = append(append(args, "--"), paths...)
	}
	out, err := git(dir, args...)
	if err != nil {
		m.setError(title + ": " + err.Error())
		return nil
	}
	commits := []gitCommit{}
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if f := strings.SplitN(l, "\t", 5); len(f) == 5 {
			commits = append(commits, gitCommit{f[0], f[1], f[2], f[3], f[4]})
		}
	}
	shown := map[int][]string{}

	m.openPanel(&panel{
		title: title,
		empty: "no commits",
		rows: func(width int) []row {
			rows := make([]row, len(commits))
			for i, c := range commits {
				rows[i] = row{text: c.short + " " + c.subject,
					hint: c.date, style: kindStyle(KindValue)}
			}
			return rows
		},
		preview: func(i int) []string {
			if shown[i] == nil {
				shown[i] = commitPreview(dir, commits[i])
			}
			return shown[i]
		},
	})
	m.setStatus(fmt.Sprintf("%s: %s", title, plural(len(commits), "commit")))
	return nil
}

// commitPreview shows a commit's message and the files it changed.
func commitPreview(dir string, c gitCommit) []string {
	lines := []string{styleBold.Render("Commit " + c.short), "",
		field("Author", c.author), field("Date", c.date), ""}
	out, err := git(dir, "show", "--stat=60", "--format=%B", c.hash)
	if err != nil {
		return append(lines, styleErr.Render(err.Error()))
	}
	msg, stat, _ := strings.Cut(out, "\n\n\n")
	if stat == "" {
		msg, stat = out, ""
	}
	lines = append(lines, strings.Split(strings.TrimRight(msg, "\n"),
		"\n")...)
	if stat = strings.Trim(stat, "\n"); stat != "" {
		lines = append(lines, section("Files")...)
		lines = append(lines, strings.Split(stat, "\n")...)
	}
	return lines
}

type repoFetchedMsg struct {
	repo *RepoInfo
	err  error
}

// fetchRepo fetches a repo's branches in the background.
func (m *Model) fetchRepo(ri *RepoInfo) tea.Cmd {
	m.setStatus("fetching " + ri.Name + "...")
	return func() tea.Msg {
		_, err := git(ri.Path, "fetch", "--prune", "origin")
		return repoFetchedMsg{ri, err}
	}
}

func (m *Model) repoFetched(msg repoFetchedMsg) tea.Cmd {
	if msg.err != nil {
		m.setError(msg.repo.Name + ": " + msg.err.Error())
		return nil
	}
	m.setStatus("fetched " + msg.repo.Name)
	// Show the new branches if their list is open.
	if m.panel != nil && m.panel.title == "branches of "+msg.repo.Name {
		cur := m.panel.list.cursor
		m.openBranches(msg.repo)
		m.panel.list.cursor = cur
	}
	return nil
}

// repoBlock returns the lines of the "repository.<name>:" block of a
// project.yml: the key's line, and the end of its indented lines.
func repoBlock(lines []string, name string) (int, int) {
	start := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t") == "repository."+name+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end := start + 1
	for ; end < len(lines); end++ {
		trimmed := strings.TrimSpace(lines[end])
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") &&
			indentRe.FindStringSubmatch(lines[end]) == nil {
			break
		}
	}
	return start, end
}

// repoVers returns the version that a project.yml asks for a repo.
func repoVers(yml string, name string) string {
	lines := strings.Split(yml, "\n")
	start, end := repoBlock(lines, name)
	for i := start + 1; start >= 0 && i < end; i++ {
		if v, ok := strings.CutPrefix(strings.TrimSpace(lines[i]),
			"vers:"); ok {
			v, _, _ = strings.Cut(v, "#")
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// setRepoVers sets the version of a repo in the contents of a project.yml,
// changing only its "vers:" line.  A repo that comes with another repo's
// dependencies gets a block of its own, which overrides their version.
func setRepoVers(yml string, name string, vers string,
	origin repoOrigin) (string, error) {

	lines := strings.Split(strings.TrimSuffix(yml, "\n"), "\n")
	start, end := repoBlock(lines, name)

	if start < 0 {
		block := []string{"repository." + name + ":",
			"    type: " + origin.Type, "    vers: " + vers}
		switch origin.Type {
		case "github":
			if origin.Server != "" && origin.Server != "github.com" {
				block = append(block, "    server: "+origin.Server)
			}
			block = append(block, "    user: "+origin.User,
				"    repo: "+origin.Repo)
		case "git":
			block = append(block, "    url: "+origin.Url)
		default:
			return "", util.FmtNewtError("%s is not in %s; add a "+
				"repository.%s block for it by hand", name,
				project.PROJECT_FILE_NAME, name)
		}
		if strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, block...)
		return strings.Join(lines, "\n") + "\n", nil
	}

	indent := "    "
	for i := start + 1; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent = indentRe.FindStringSubmatch(lines[i])[1]
		if strings.HasPrefix(trimmed, "vers:") {
			lines[i] = indent + "vers: " + vers
			return strings.Join(lines, "\n") + "\n", nil
		}
	}
	rest := append([]string{indent + "vers: " + vers}, lines[start+1:]...)
	lines = append(lines[:start+1], rest...)
	return strings.Join(lines, "\n") + "\n", nil
}
