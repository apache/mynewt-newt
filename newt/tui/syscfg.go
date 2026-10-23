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

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
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

// BuildInfo is what resolving a target tells about its build: its syscfg
// settings.
type BuildInfo struct {
	Settings []*Setting
	// Problems that would make "newt build" fail, one per line.
	Errors []string
}

// sourceName describes where a setting value comes from, relative to the
// target being configured.
func sourceName(t *target.Target, lpkg *pkg.LocalPackage) string {
	switch {
	case lpkg == nil:
		return "command line"
	case lpkg == t.Package():
		return "target"
	case lpkg == t.App():
		return "app " + shortName(lpkg.FullName())
	case lpkg == t.Bsp():
		return "bsp " + shortName(lpkg.FullName())
	default:
		return shortName(lpkg.FullName())
	}
}

func shortName(fullName string) string {
	name, _ := splitName(fullName)
	return name
}

var stateNames = map[syscfg.CfgSettingState]string{
	syscfg.CFG_SETTING_STATE_DEPRECATED:   "deprecated",
	syscfg.CFG_SETTING_STATE_DEFUNCT:      "defunct",
	syscfg.CFG_SETTING_STATE_EXPERIMENTAL: "experimental",
}

var typeNames = map[syscfg.CfgSettingType]string{
	syscfg.CFG_SETTING_TYPE_RAW:            "raw",
	syscfg.CFG_SETTING_TYPE_TASK_PRIO:      "task priority",
	syscfg.CFG_SETTING_TYPE_INTERRUPT_PRIO: "interrupt priority",
	syscfg.CFG_SETTING_TYPE_FLASH_OWNER:    "flash owner",
}

func restrictionString(r syscfg.CfgRestriction) string {
	switch r.Code {
	case syscfg.CFG_RESTRICTION_CODE_NOTNULL:
		return "must be set"
	case syscfg.CFG_RESTRICTION_CODE_EXPR:
		return "requires " + r.Expr
	case syscfg.CFG_RESTRICTION_CODE_RANGE:
		ranges := []string{}
		for _, rr := range r.Ranges {
			if rr.RExpr == "" {
				ranges = append(ranges, rr.LExpr)
			} else {
				ranges = append(ranges, rr.LExpr+".."+rr.RExpr)
			}
		}
		return "range " + strings.Join(ranges, ", ")
	default:
		return r.Code.String() + " " + r.Expr
	}
}

// resolveSyscfg resolves the target's configuration the way "newt target
// config show" does.
func resolveSyscfg(t *target.Target) (*BuildInfo, error) {
	if t.App() == nil || t.Bsp() == nil {
		return nil, util.NewNewtError("pick an app and a bsp first")
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		return nil, err
	}
	res, err := b.Resolve()
	if err != nil {
		return nil, err
	}

	overrides, _ := t.Package().SyscfgY.GetValStringMapString(
		"syscfg.vals", nil)

	info := &BuildInfo{}
	for _, line := range strings.Split(res.ErrorText(), "\n") {
		line = strings.TrimSpace(line)
		// Newt follows the errors with the history of the settings
		// involved, which the preview shows anyway.
		if strings.HasPrefix(line, "Setting history") {
			break
		}
		if line != "" {
			info.Errors = append(info.Errors, line)
		}
	}

	for _, e := range res.Cfg.Settings {
		// Newt defines a constant PKG_<name> setting for every package.
		if e.State == syscfg.CFG_SETTING_STATE_CONST &&
			strings.HasPrefix(e.Name, "PKG_") {
			continue
		}

		s := &Setting{
			Name:        e.Name,
			Value:       e.Value,
			Description: e.Description,
			Type:        typeNames[e.SettingType],
			State:       stateNames[e.State],
			Choices:     e.ValidChoices,
			Const:       e.State == syscfg.CFG_SETTING_STATE_CONST,
		}
		if e.PackageDef != nil {
			s.Package = e.PackageDef.FullName()
		}
		_, s.Overridden = overrides[e.Name]
		for _, r := range e.Restrictions {
			// Choices are listed separately.
			if r.Code != syscfg.CFG_RESTRICTION_CODE_CHOICE {
				s.Restrictions = append(s.Restrictions, restrictionString(r))
			}
		}
		for i, p := range e.History {
			src := sourceName(t, p.Source)
			if i == 0 {
				src = "default"
			}
			s.History = append(s.History, SettingPoint{src, p.Value})
		}
		info.Settings = append(info.Settings, s)
	}

	// The target's own settings first, then by name.
	sort.Slice(info.Settings, func(i, j int) bool {
		a, b := info.Settings[i], info.Settings[j]
		if a.Overridden != b.Overridden {
			return a.Overridden
		}
		return a.Name < b.Name
	})

	return info, nil
}

// setTargetSyscfg sets (or, with remove, deletes) a value in the target's
// syscfg.yml.  Only the setting's line is changed, so that comments and
// formatting are kept.
func setTargetSyscfg(t *target.Target, name string, value string,
	remove bool) error {

	path := t.Package().SyscfgYamlPath()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return util.ChildNewtError(err)
	}

	data = []byte(setSyscfgVal(string(data), name, value, remove))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return util.ChildNewtError(err)
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
