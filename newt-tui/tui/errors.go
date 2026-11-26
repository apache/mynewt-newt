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
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// BuildIssue is an error or warning found in the output of a build.
type BuildIssue struct {
	// Source location; File is empty for e.g. linker or newt errors.
	File string
	Line int
	Col  int
	// "error" or "warning".
	Severity string
	Msg      string
	// Compiler notes that follow the message.
	Notes []string
}

func (b BuildIssue) Location() string {
	switch {
	case b.File == "":
		return ""
	case b.Col > 0:
		return fmt.Sprintf("%s:%d:%d", b.File, b.Line, b.Col)
	default:
		return fmt.Sprintf("%s:%d", b.File, b.Line)
	}
}

var (
	gccIssueRe = regexp.MustCompile(
		`^(\S[^:]*):(\d+):(?:(\d+):)?\s+(fatal error|error|warning|note):\s+(.*)$`)
	linkRefRe = regexp.MustCompile(
		`^(\S[^:]*):(\d+):(?:\(\S+\):)?\s*(undefined reference to .*)$`)
	linkerRe  = regexp.MustCompile(`(?:^|[/-])ld(?:\.bfd)?:\s+(.*)$`)
	newtErrRe = regexp.MustCompile(`^(Error|Warning): (.*)$`)
	// Compiler context lines such as "foo.c: In function 'main':".
	contextRe = regexp.MustCompile(`^\S[^:]*: In (function|file included|member)`)
)

// parseBuildIssues finds compiler, linker and newt errors and warnings in
// build output.
func parseBuildIssues(out string) []BuildIssue {
	issues := []BuildIssue{}
	seen := map[string]bool{}

	add := func(b BuildIssue) {
		key := b.Location() + " " + b.Severity + " " + b.Msg
		if !seen[key] {
			seen[key] = true
			issues = append(issues, b)
		}
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")

		// newt prefixes the first line of a compiler's output with
		// "Error: ".
		if rest, ok := strings.CutPrefix(line, "Error: "); ok &&
			(gccIssueRe.MatchString(rest) || contextRe.MatchString(rest)) {
			line = rest
		}
		if contextRe.MatchString(line) {
			continue
		}
		// A list that follows a newt message, e.g. "[mbedtls]".
		if strings.HasPrefix(line, "[") && len(issues) > 0 {
			last := &issues[len(issues)-1]
			if last.File == "" && strings.HasSuffix(last.Msg, ":") {
				last.Msg += " " + strings.Trim(line, "[]")
				continue
			}
		}

		if m := gccIssueRe.FindStringSubmatch(line); m != nil {
			ln, _ := strconv.Atoi(m[2])
			col, _ := strconv.Atoi(m[3])
			if m[4] == "note" {
				if len(issues) > 0 {
					last := &issues[len(issues)-1]
					last.Notes = append(last.Notes, fmt.Sprintf("%s:%s: %s",
						m[1], m[2], m[5]))
				}
				continue
			}
			sev := "error"
			if m[4] == "warning" {
				sev = "warning"
			}
			add(BuildIssue{File: m[1], Line: ln, Col: col, Severity: sev,
				Msg: m[5]})
			continue
		}
		if m := linkRefRe.FindStringSubmatch(line); m != nil {
			ln, _ := strconv.Atoi(m[2])
			add(BuildIssue{File: m[1], Line: ln, Severity: "error",
				Msg: m[3]})
			continue
		}
		if m := linkerRe.FindStringSubmatch(line); m != nil &&
			!strings.Contains(line, "undefined reference") {
			if msg, ok := strings.CutPrefix(m[1], "warning: "); ok {
				add(BuildIssue{Severity: "warning", Msg: "linker: " + msg})
			} else {
				add(BuildIssue{Severity: "error", Msg: "linker: " + m[1]})
			}
			continue
		}
		if m := newtErrRe.FindStringSubmatch(line); m != nil {
			sev := "error"
			if m[1] == "Warning" {
				sev = "warning"
			}
			add(BuildIssue{Severity: sev, Msg: m[2]})
		}
	}
	return issues
}

func countIssues(issues []BuildIssue) (int, int) {
	errs, warns := 0, 0
	for _, b := range issues {
		if b.Severity == "error" {
			errs++
		} else {
			warns++
		}
	}
	return errs, warns
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// sourceContext returns the lines around a source location, with the line
// itself marked.
func sourceContext(path string, line int, around int) []string {
	data, err := os.ReadFile(path)
	if err != nil || line <= 0 {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	out := []string{}
	for i := max(line-around, 1); i <= min(line+around, len(lines)); i++ {
		text := strings.ReplaceAll(lines[i-1], "\t", "    ")
		num := fmt.Sprintf("%5d ", i)
		if i == line {
			out = append(out, styleErr.Render(num+"▶ ")+text)
		} else {
			out = append(out, styleDim.Render(num+"  ")+text)
		}
	}
	return out
}

// editorArgs builds the arguments to open a file at a line.  Most terminal
// editors take "+<line> <file>"; VS Code and Helix take "<file>:<line>".
func editorArgs(editor []string, path string, line int) []string {
	name := filepath.Base(editor[0])
	args := append([]string{}, editor[1:]...)
	switch {
	case line <= 0:
		return append(args, path)
	case name == "code" || name == "codium" || name == "code-insiders":
		return append(args, "-g", fmt.Sprintf("%s:%d", path, line))
	case name == "hx" || name == "helix":
		return append(args, fmt.Sprintf("%s:%d", path, line))
	default:
		return append(args, fmt.Sprintf("+%d", line), path)
	}
}

// issuesPanel lists the errors and warnings of a build.
func (m *Model) issuesPanel(target string, issues []BuildIssue) *panel {
	errs, warns := countIssues(issues)
	title := "build of " + target + ": " + plural(errs, "error")
	if warns > 0 {
		title += ", " + plural(warns, "warning")
	}

	// Errors first, keeping the build's order otherwise.
	sorted := []BuildIssue{}
	for _, sev := range []string{"error", "warning"} {
		for _, b := range issues {
			if b.Severity == sev {
				sorted = append(sorted, b)
			}
		}
	}

	return &panel{
		title: title,
		empty: "no errors or warnings",
		rows: func(width int) []row {
			rows := make([]row, len(sorted))
			for i, b := range sorted {
				mark, style := "✗ ", styleErr
				if b.Severity == "warning" {
					mark, style = "! ", kindStyle(KindUnittest)
				}
				// The location comes first so that long messages do not
				// hide it.
				text := mark + b.Msg
				if b.File != "" {
					text = fmt.Sprintf("%s%s:%d  %s", mark,
						filepath.Base(b.File), b.Line, b.Msg)
				}
				rows[i] = row{text: text, style: style}
			}
			return rows
		},
		preview: func(i int) []string {
			b := sorted[i]
			title := "Error"
			if b.Severity == "warning" {
				title = "Warning"
			}
			lines := []string{styleBold.Render(title),
				"", b.Msg}
			if b.File != "" {
				lines = append(lines, "", field("File", b.Location()))
			}
			for _, n := range b.Notes {
				lines = append(lines, styleDim.Render("note: ")+n)
			}
			if ctx := sourceContext(b.File, b.Line, 6); len(ctx) > 0 {
				lines = append(lines, "")
				lines = append(lines, ctx...)
			}
			return lines
		},
		enterHelp: "open the file at this line in $EDITOR",
		enter: func(m *Model, i int) tea.Cmd {
			b := sorted[i]
			if b.File == "" {
				m.setStatus("this message has no source location")
				return nil
			}
			c, err := editorExec(b.File, b.Line)
			if err != nil {
				m.setError(err.Error())
				return nil
			}
			return tea.ExecProcess(c, func(err error) tea.Msg {
				return execDoneMsg{label: "edit " + b.File, err: err}
			})
		},
	}
}
