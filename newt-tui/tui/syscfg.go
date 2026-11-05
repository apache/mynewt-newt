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
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mynewt.apache.org/newt/newt-tui/newtinfo"
)

// SettingPoint is one step in the history of a setting's value.
type SettingPoint struct {
	Source string
	Value  string
}

// Setting is one resolved syscfg setting of a target.
type Setting struct {
	Name        string
	Value       string
	Description string
	// Package that defines the setting.
	Package      string
	Type         string
	State        string
	Choices      []string
	Restrictions []string
	// Oldest (the default) first.
	History []SettingPoint
	// Whether the target's syscfg.yml sets the value.
	Overridden bool
	// Constant settings cannot be overridden.
	Const bool
}

// Default returns the value from the setting's definition.
func (s *Setting) Default() string {
	if len(s.History) == 0 {
		return s.Value
	}
	return s.History[0].Value
}

// SetBy names the source of the current value.
func (s *Setting) SetBy() string {
	if len(s.History) <= 1 {
		return "default"
	}
	return s.History[len(s.History)-1].Source
}

// Changed tells whether anything overrides the default value.
func (s *Setting) Changed() bool {
	return len(s.History) > 1
}

func (s *Setting) IsFlag() bool {
	return len(s.Choices) == 0 && (s.Value == "0" || s.Value == "1")
}

// DepReason is a package that depends on another one, and why.
type DepReason struct {
	Pkg string
	// E.g. "if BLE_ROLE_CENTRAL" or "provides API console"; empty when the
	// dependency is unconditional.
	Why string
}

// BuildInfo is what resolving a target tells about its build: the syscfg
// settings and the packages that make it up.
type BuildInfo struct {
	Settings []*Setting
	// Problems that would make "newt build" fail, one per line.
	Errors []string

	// The packages that depend on each package of the build.
	Revdeps map[string][]DepReason
	// The packages the build starts from: app, bsp, target, ...
	Roots []string
	// Size of the flash area the image goes to, and of the MCU's RAM; 0 if
	// unknown.
	FlashSlot int64
	RAMSize   int64
}

// sourceName describes where a setting value comes from, relative to the
// target being configured.
func sourceName(t *newtinfo.Target, source string) string {
	switch source {
	case "newt":
		return "newt"
	case t.Name:
		return "target"
	case t.Vars["app"]:
		return "app " + shortName(source)
	case t.Vars["bsp"]:
		return "bsp " + shortName(source)
	default:
		return shortName(source)
	}
}

func shortName(fullName string) string {
	name, _ := splitName(fullName)
	return name
}

var stateNames = map[string]string{
	"deprecated":   "deprecated",
	"defunct":      "defunct",
	"experimental": "experimental",
}

var typeNames = map[string]string{
	"raw":                "raw",
	"task_priority":      "task priority",
	"interrupt_priority": "interrupt priority",
	"flash_owner":        "flash owner",
}

func restrictionString(r newtinfo.Restriction) string {
	switch r.Type {
	case "notnull":
		return "must be set"
	case "expr":
		return "requires " + r.Expr
	case "range":
		return "range " + r.Expr
	default:
		return strings.TrimSpace(r.Type + " " + r.Expr)
	}
}

// buildInfo turns what "newt info --json" tells about a target's build into
// what the browser shows.
func buildInfo(info *newtinfo.Info, t *newtinfo.Target) *BuildInfo {
	b := &BuildInfo{Revdeps: map[string][]DepReason{}}
	b.Revdeps, b.Roots = revdeps(t)
	b.FlashSlot = flashSlot(info, t)
	for _, line := range t.SyscfgErrors {
		// Newt follows the errors with the history of the settings
		// involved, which the preview shows anyway.
		if strings.HasPrefix(line, "Setting history") {
			break
		}
		b.Errors = append(b.Errors, line)
	}

	for _, e := range t.Syscfg {
		// Newt defines a constant PKG_<name> setting for every package.
		if e.State == "const" && strings.HasPrefix(e.Name, "PKG_") {
			continue
		}

		s := &Setting{
			Name:        e.Name,
			Value:       e.Value,
			Description: e.Description,
			Package:     e.Package,
			Type:        typeNames[e.Type],
			State:       stateNames[e.State],
			Choices:     e.Choices,
			Const:       e.State == "const",
		}
		if s.Package == "newt" {
			s.Package = ""
		}
		for _, r := range e.Restrictions {
			s.Restrictions = append(s.Restrictions, restrictionString(r))
		}
		for i, p := range e.History {
			src := sourceName(t, p.Source)
			if i == 0 {
				src = "default"
			}
			if p.Source == t.Name {
				s.Overridden = true
			}
			s.History = append(s.History, SettingPoint{src, p.Value})
		}
		b.Settings = append(b.Settings, s)
	}

	if s := findSetting(b, "MCU_RAM_SIZE"); s != nil {
		b.RAMSize = numberOf(s.Value)
	}

	// The target's own settings first, then by name.
	sort.Slice(b.Settings, func(i, j int) bool {
		x, y := b.Settings[i], b.Settings[j]
		if x.Overridden != y.Overridden {
			return x.Overridden
		}
		return x.Name < y.Name
	})

	return b
}

// targetOverrides returns the syscfg values the target's syscfg.yml sets.
func targetOverrides(t *newtinfo.Target) map[string]string {
	overrides := map[string]string{}
	for _, s := range t.Syscfg {
		for _, p := range s.History {
			if p.Source == t.Name {
				overrides[s.Name] = p.Value
			}
		}
	}
	return overrides
}

// setTargetSyscfg sets (or, with remove, deletes) a value in the target's
// syscfg.yml.  Only the setting's line is changed, so that comments and
// formatting are kept.
func setTargetSyscfg(path string, name string, value string,
	remove bool) error {

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	data = []byte(setSyscfgVal(string(data), name, value, remove))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return nil
}

// Characters such as "*", "!" or "|" have a special meaning at the start of
// a yml value.
var plainYmlValue = regexp.MustCompile(
	`^(-?[0-9]|[A-Za-z_(])[A-Za-z0-9_.()+\-*/<>|&!~ ]*$`)

// ymlValue formats a syscfg value for a yml file.  Simple values (numbers,
// identifiers, expressions) are written as they are; anything else, such as a
// C string literal, is quoted so that yml keeps it intact.
func ymlValue(value string) string {
	if plainYmlValue.MatchString(value) &&
		strings.TrimSpace(value) == value {
		return value
	}
	return strconv.Quote(value)
}

var indentRe = regexp.MustCompile(`^(\s+)\S`)

// setSyscfgVal changes the "<name>: <value>" entry of the syscfg.vals map in
// the contents of a syscfg.yml file.
func setSyscfgVal(yml string, name string, value string, remove bool) string {
	lines := strings.Split(strings.TrimSuffix(yml, "\n"), "\n")
	if yml == "" {
		lines = nil
	}
	entry := name + ": " + ymlValue(value)

	// Find the syscfg.vals block: the indented lines after its key.
	start := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t") == "syscfg.vals:" {
			start = i
			break
		}
	}

	if start < 0 {
		if remove {
			return yml
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "syscfg.vals:", "    "+entry)
		return strings.Join(lines, "\n") + "\n"
	}

	end := start + 1
	indent := "    "
	last := start
	found := -1
	for ; end < len(lines); end++ {
		l := lines[end]
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		m := indentRe.FindStringSubmatch(l)
		if m == nil {
			break
		}
		if last == start {
			indent = m[1]
		}
		last = end
		if strings.HasPrefix(trimmed, name+":") {
			found = end
		}
	}

	switch {
	case found >= 0 && remove:
		lines = append(lines[:found], lines[found+1:]...)
		if last == found {
			last--
		}
		// Drop the key of an emptied map, as yml would read it as null.
		if last == start {
			lines = append(lines[:start], lines[start+1:]...)
		}
	case found >= 0:
		lines[found] = indent + entry
	case !remove:
		rest := append([]string{indent + entry}, lines[last+1:]...)
		lines = append(lines[:last+1], rest...)
	}

	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// settingPreview returns the right column for a setting.
func settingPreview(s *Setting, errors []string) []string {
	lines := []string{styleBold.Render(s.Name), ""}

	val := s.Value
	if val == "" {
		val = styleDim.Render("(empty)")
	}
	lines = append(lines, field("Value", styleBold.Render(val)))
	if s.Overridden {
		lines = append(lines, field("Set by", kindStyle(KindTarget).
			Render("this target")))
	} else {
		lines = append(lines, field("Set by", s.SetBy()))
	}
	if s.Changed() {
		lines = append(lines, field("Default", s.Default()))
	}
	lines = append(lines, field("Defined", shortName(s.Package)))
	if g := bleGroup(s.Name); g != "" {
		lines = append(lines, field("Group", "Bluetooth › "+g))
	}
	if s.Type != "" && s.Type != "raw" {
		lines = append(lines, field("Type", s.Type))
	}
	if len(s.Choices) > 0 {
		lines = append(lines, field("Choices", strings.Join(s.Choices, ", ")))
	}
	for _, r := range s.Restrictions {
		lines = append(lines, field("Rule", r))
	}
	if s.State != "" {
		lines = append(lines, field("State", styleErr.Render(s.State)))
	}
	if s.Const {
		lines = append(lines, field("Note", "constant, cannot be changed"))
	}

	for _, e := range errors {
		if strings.Contains(e, s.Name) {
			lines = append(lines, "", styleErr.Render("Build would fail: ")+e)
		}
	}

	if s.Description != "" {
		lines = append(lines, "", s.Description)
	}

	if s.Changed() {
		lines = append(lines, section("History, newest first")...)
		for i := len(s.History) - 1; i >= 0; i-- {
			p := s.History[i]
			lines = append(lines, fmt.Sprintf("%-12s %s",
				clip(p.Value, 12), styleDim.Render(p.Source)))
		}
	}

	return lines
}

func clip(s string, w int) string {
	if r := []rune(s); len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s
}

// revdeps returns, for each package of the target's build, the packages
// that depend on it and why, and the packages the build starts from.
func revdeps(t *newtinfo.Target) (map[string][]DepReason, []string) {
	out := map[string][]DepReason{}
	roots := []string{}
	for _, d := range t.Deps {
		out[d.Name] = nil
		if len(d.RequiredBy) == 0 {
			roots = append(roots, d.Name)
		}
		for _, r := range d.RequiredBy {
			whys := []string{}
			if r.Syscfg != "" {
				whys = append(whys, "if "+r.Syscfg)
			}
			for _, api := range r.Apis {
				whys = append(whys, "provides API "+api)
			}
			out[d.Name] = append(out[d.Name], DepReason{r.Name,
				strings.Join(whys, ", ")})
		}
	}
	return out, roots
}

// flashSlot returns the size of the flash area the target's image goes to,
// or 0 if unknown.
func flashSlot(info *newtinfo.Info, t *newtinfo.Target) int64 {
	bsp := info.Package(t.Vars["bsp"])
	if bsp == nil || bsp.Bsp == nil {
		return 0
	}
	if area := bsp.Bsp.FlashArea(t.FlashArea); area != nil {
		return int64(area.Size)
	}
	return 0
}

// whyIncluded returns the shortest chain of dependencies from one of the
// build's roots to a package, root first; each step says why it depends on
// the next one.
func (info *BuildInfo) whyIncluded(pkgName string) []DepReason {
	if info == nil || info.Revdeps == nil {
		return nil
	}
	isRoot := map[string]bool{}
	for _, r := range info.Roots {
		isRoot[r] = true
	}

	// Breadth first search up the reverse dependencies.
	next := map[string]DepReason{}
	seen := map[string]bool{pkgName: true}
	queue := []string{pkgName}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if isRoot[cur] {
			chain := []DepReason{}
			for cur != pkgName {
				step := next[cur]
				chain = append(chain, DepReason{cur, step.Why})
				cur = step.Pkg
			}
			return append(chain, DepReason{Pkg: pkgName})
		}
		for _, d := range info.Revdeps[cur] {
			if !seen[d.Pkg] {
				seen[d.Pkg] = true
				next[d.Pkg] = DepReason{cur, d.Why}
				queue = append(queue, d.Pkg)
			}
		}
	}
	return nil
}

func findSetting(info *BuildInfo, name string) *Setting {
	if info == nil {
		return nil
	}
	for _, s := range info.Settings {
		if s.Name == name {
			return s
		}
	}
	return nil
}
