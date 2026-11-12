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
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"mynewt.apache.org/newt/newt-tui/newtinfo"
)

type Kind int

const (
	KindTarget Kind = iota
	KindApp
	KindBsp
	KindLib
	KindUnittest
	// A plain value rather than a package, e.g. a build profile.
	KindValue
	// The target's syscfg settings.
	KindSyscfg
)

// Action is a newt command that can be run on an entry, e.g. "newt build
// <target>".
type Action struct {
	Key   string
	Label string
	Args  []string
	// Optional check before running the command, e.g. that the flash tool
	// is installed.
	Check func() error
	// Ask which connected board to use first.
	PickDevice bool
	// Opens the board's console instead of running newt.
	Console bool
	// Several newt commands to run one after another, instead of Args.
	Steps [][]string
	// Gives the command the terminal itself, e.g. for gdb; its output is
	// then not read for build errors.
	Interactive bool
	// Question to confirm with y before running, if any.
	Confirm string
	// Choose the image to load first.
	ChooseImage bool
	// Runs instead of a newt command.
	Fn func(m *Model) tea.Cmd
}

// runs tells whether the action runs the newt command.
func (a Action) runs(command string) bool {
	return len(a.Args) > 0 && a.Args[0] == command
}

// Link points from one entry to another one, e.g. from a target to its app.
// Links are followed with the "g<Key>" key sequence.
type Link struct {
	Key   string
	Label string
	Kind  Kind
	Name  string
}

// Field is a target variable that can be chosen from a list, e.g.
// "target.bsp".
type Field struct {
	Label string
	Var   string
	Value string
	// Kind of the packages to choose from; KindValue for plain values.
	Kind Kind
	// Whether the variable can be unset.
	Optional bool

	// Preview of the field when it has no list of choices.
	Preview func() []string
	// Shown dimmed next to the value, e.g. "not installed".
	Hint string
	// Description of "l" on this field in the help pane.
	Help string

	// For fields that are not target.yml variables: the choices, and how a
	// choice is saved.  An option with an empty FullName resets the field.
	OptionsFn func() []*Entry
	SetFn     func(value string) error

	// Computes Value and Hint on first use, as that can be slow.
	initFn func(f *Field)
	inited bool

	valuesFn func() []string
	values   []string
}

func (f *Field) ensure() {
	if !f.inited && f.initFn != nil {
		f.inited = true
		f.initFn(f)
	}
}

// Values returns the choices of a KindValue field.
func (f *Field) Values() []string {
	if f.values == nil && f.valuesFn != nil {
		f.values = f.valuesFn()
	}
	return f.values
}

type Entry struct {
	// Displayed name.
	Name string
	// Fully-qualified package name.
	FullName string
	// Secondary text shown dimmed next to the name.
	Hint string
	Kind Kind
	// File opened by the edit action.
	EditPath string
	// What l does instead of opening EditPath, and its help.
	Open     func(m *Model) tea.Cmd
	OpenHelp string
	// Entries from foreign repos are hidden unless the user toggles them.
	Foreign bool
	// The repo the entry comes from; lists group their entries by it.
	Repo string
	// The package's directory; empty for other entries.
	Dir     string
	Actions []Action
	Links   []Link
	Fields  []*Field
	// Sets one of the Fields and saves the entry.
	Set func(variable string, value string) error
	// Bootloader support for targets: whether the app needs a bootloader,
	// the bootloader target to use, and the version of the app's image.
	NeedsBoot bool
	Boot      string
	BootName  string
	Image     string
	ImagePath string
	// Where the browser keeps the target's images, and the app's build.
	ImagesDir string
	ElfPath   string
	App       string
	// For bootloader targets: the targets they boot.
	IsBoot  bool
	BootFor []string

	// Resolves the target's syscfg and changes the target's syscfg.yml.
	Syscfg    func() (*BuildInfo, error)
	SetSyscfg func(name string, value string, remove bool) error

	previewFn func() []string
	preview   []string
}

// Preview returns the lines shown in the right column for this entry.  The
// preview is generated on first use and cached, since reading the yml files
// of every library up front would be slow.
func (e *Entry) Preview() []string {
	if e.preview == nil && e.previewFn != nil {
		e.preview = e.previewFn()
	}
	return e.preview
}

type Category struct {
	Title   string
	Kind    Kind
	Entries []*Entry
}

// Load builds the browser contents from the description of the project
// that "newt info --json" prints.  Targets whose build is not resolved in
// it are resolved with resolve when their syscfg is needed.
func Load(info *newtinfo.Info,
	resolve func(name string) (*newtinfo.Target, error)) []*Category {

	p := &project{info: info, local: map[string]bool{}, resolve: resolve}
	for _, r := range info.Repos {
		p.local[r.Name] = r.Local
	}

	users := map[string][]string{}
	targets := targetEntries(p, users)

	cats := []*Category{
		{Title: "Targets", Kind: KindTarget, Entries: targets},
		{Title: "Apps", Kind: KindApp},
		{Title: "BSPs", Kind: KindBsp},
		{Title: "Libraries", Kind: KindLib},
		{Title: "Unit tests", Kind: KindUnittest},
	}
	kinds := map[string]*Category{
		"app":      cats[1],
		"bsp":      cats[2],
		"lib":      cats[3],
		"unittest": cats[4],
	}

	for i := range info.Packages {
		pkg := &info.Packages[i]
		if cat := kinds[pkg.Type]; cat != nil {
			cat.Entries = append(cat.Entries,
				packageEntry(p, pkg, cat.Kind, users[pkg.Name]))
		}
	}
	for _, cat := range cats[1:] {
		sortEntries(cat.Entries)
	}

	return cats
}

// project is the description of the project, with what the browser looks
// up in it often.
type project struct {
	info *newtinfo.Info
	// Whether each repo is the project itself.
	local map[string]bool
	// Reads the resolved build of a target.
	resolve func(name string) (*newtinfo.Target, error)
}

// path returns the absolute path of a path in the description, which is
// relative to the project.
func (p *project) path(rel string) string {
	if rel == "" || filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(p.info.Project.Path, filepath.FromSlash(rel))
}

func sortEntries(entries []*Entry) {
	// Local packages first, then by repo and name.
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Foreign != b.Foreign {
			return !a.Foreign
		}
		return a.FullName < b.FullName
	})
}

// splitName splits "@repo/path/to/pkg" into "path/to/pkg" and "repo".
func splitName(fullName string) (string, string) {
	if !strings.HasPrefix(fullName, "@") {
		return fullName, ""
	}
	slash := strings.Index(fullName, "/")
	if slash < 0 {
		return fullName, ""
	}
	return fullName[slash+1:], fullName[1:slash]
}

func targetEntries(p *project, users map[string][]string) []*Entry {
	entries := []*Entry{}
	targets := map[string]*newtinfo.Target{}

	for i := range p.info.Targets {
		t := &p.info.Targets[i]
		full := t.Name
		short, repo := splitName(full)
		short = strings.TrimPrefix(short, "targets/")
		dir := p.path(t.Path)

		e := &Entry{
			Name:     short,
			FullName: full,
			Kind:     KindTarget,
			EditPath: filepath.Join(dir, "target.yml"),
			Foreign:  !p.local[t.Repo],
			Repo:     t.Repo,
			Actions: []Action{
				{Key: "b", Label: "build", Args: []string{"build", full}},
				{Key: "r", Label: "run", Args: []string{"run", full},
					Interactive: true},
				{Key: "L", Label: "load", Args: []string{"load", full},
					PickDevice: true},
				{Key: "d", Label: "debug", Args: []string{"debug", full},
					PickDevice: true, Interactive: true},
				{Key: "C", Label: "console", Args: []string{"console"},
					PickDevice: true, Console: true},
				{Key: "s", Label: "size", Args: []string{"size", full}},
				{Key: "c", Label: "clean", Args: []string{"clean", full},
					Confirm: "Delete the build output of " + short + "?"},
			},
		}
		if repo != "" {
			e.Hint = "@" + repo
		}

		link := func(key string, label string, kind Kind, name string) {
			if name == "" || p.info.Package(name) == nil {
				return
			}
			e.Links = append(e.Links, Link{key, label, kind, name})
			users[name] = append(users[name], full)
		}
		link("a", "app", KindApp, t.Vars["app"])
		link("b", "bsp", KindBsp, t.Vars["bsp"])
		link("l", "loader", KindApp, t.Vars["loader"])

		profiles := t.BuildProfiles
		e.Fields = []*Field{
			{Label: "app", Var: "target.app", Value: t.Vars["app"],
				Kind: KindApp},
			{Label: "bsp", Var: "target.bsp", Value: t.Vars["bsp"],
				Kind: KindBsp},
			{Label: "profile", Var: "target.build_profile",
				Value: t.Vars["build_profile"], Kind: KindValue,
				valuesFn: func() []string { return profiles }},
		}
		flash := downloaderField(e)
		e.Fields = append(e.Fields, flash)
		for i := range e.Actions {
			if e.Actions[i].runs("load") {
				e.Actions[i].Check = func() error { return loadCheck(flash) }
			}
		}

		// Loaders are only used for split images, which are rare; only
		// offer the setting on targets that already use one.
		if t.Vars["loader"] != "" {
			e.Fields = append(e.Fields, &Field{Label: "loader",
				Var: "target.loader", Value: t.Vars["loader"], Kind: KindApp,
				Optional: true})
		}
		targetYml := e.EditPath
		e.Set = func(variable string, value string) error {
			return setTargetVar(targetYml, variable, value)
		}

		overrides := targetOverrides(t)
		e.Fields = append(e.Fields, &Field{Label: "syscfg",
			Value: overrideCount(len(overrides)), Kind: KindSyscfg,
			Preview: func() []string { return overridesPreview(overrides) }})
		// Converted at most once per load of the project.
		var info *BuildInfo
		e.Syscfg = func() (*BuildInfo, error) {
			if info != nil {
				return info, nil
			}
			rt := t
			if !t.Resolved && p.resolve != nil {
				var err error
				if rt, err = p.resolve(full); err != nil {
					return nil, err
				}
			}
			info = buildInfo(p.info, rt)
			return info, nil
		}
		syscfgYml := filepath.Join(dir, "syscfg.yml")
		e.SetSyscfg = func(name string, value string, remove bool) error {
			return setTargetSyscfg(syscfgYml, name, value, remove)
		}

		e.previewFn = func() []string { return targetPreview(p, t, e) }
		entries = append(entries, e)
		targets[full] = t
	}

	addBootSupport(p, entries, targets)
	sortEntries(entries)
	for _, list := range users {
		sort.Strings(list)
	}

	return entries
}

func packageEntry(p *project, pkg *newtinfo.Package, kind Kind,
	users []string) *Entry {

	full := pkg.Name
	short, repo := splitName(full)
	dir := p.path(pkg.Path)

	e := &Entry{
		Name:     short,
		FullName: full,
		Kind:     kind,
		EditPath: filepath.Join(dir, "pkg.yml"),
		Foreign:  !p.local[pkg.Repo],
		Repo:     pkg.Repo,
		Dir:      dir,
	}
	if repo != "" {
		e.Hint = "@" + repo
	}
	if kind == KindUnittest {
		e.Actions = []Action{{Key: "t", Label: "test", Args: []string{"test", full}}}
	}

	e.previewFn = func() []string {
		return packagePreview(p, pkg, kind, users)
	}
	return e
}

func field(label string, value string) string {
	if value == "" {
		value = styleDim.Render("-")
	}
	return styleLabel.Render(label) + value
}

func section(title string) []string {
	return []string{"", styleHeading.Render(title)}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func targetPreview(p *project, t *newtinfo.Target, e *Entry) []string {
	lines := []string{
		field("Target", styleBold.Render(t.Name)),
		field("App", t.Vars["app"]),
		field("BSP", t.Vars["bsp"]),
	}
	if t.Vars["loader"] != "" {
		lines = append(lines, field("Loader", t.Vars["loader"]))
	}
	lines = append(lines,
		field("Profile", t.Vars["build_profile"]),
		field("Path", t.Path))

	built := styleDim.Render("not built")
	if elf := t.Artifacts.Elf; elf != "" {
		if fi, err := os.Stat(p.path(elf)); err == nil {
			built = styleOk.Render("built") + styleDim.Render(fmt.Sprintf(
				" %s, %s", fi.ModTime().Format(time.DateTime),
				humanSize(fi.Size())))
		}
	}
	lines = append(lines, field("Status", built))
	lines = append(lines, bootPreview(e)...)

	flags := []string{}
	for _, f := range []string{"aflags", "cflags", "cxxflags", "lflags"} {
		if v := t.Vars[f]; v != "" {
			flags = append(flags, field(f, v))
		}
	}
	if len(flags) > 0 {
		lines = append(lines, section("Flags")...)
		lines = append(lines, flags...)
	}

	vals := targetOverrides(t)
	if len(vals) > 0 {
		lines = append(lines, section("Settings")...)
		for _, k := range sortedKeys(vals) {
			lines = append(lines, k+styleDim.Render(": ")+vals[k])
		}
	}

	return lines
}

func packagePreview(p *project, pkg *newtinfo.Package, kind Kind,
	users []string) []string {

	lines := []string{
		field("Package", styleBold.Render(pkg.Name)),
		field("Type", pkg.Type),
		field("Repo", pkg.Repo),
		field("Path", pkg.Path),
	}
	if pkg.Author != "" {
		lines = append(lines, field("Author", pkg.Author))
	}
	if pkg.Homepage != "" {
		lines = append(lines, field("Homepage", pkg.Homepage))
	}
	if len(pkg.Keywords) > 0 {
		lines = append(lines, field("Keywords", strings.Join(pkg.Keywords, ", ")))
	}
	if pkg.Description != "" {
		lines = append(lines, "", pkg.Description)
	}

	if kind == KindBsp {
		lines = append(lines, bspPreview(pkg)...)
	}

	if kind == KindApp || kind == KindBsp {
		lines = append(lines, section("Used by targets")...)
		if len(users) == 0 {
			lines = append(lines, styleDim.Render("none"))
		}
		for _, u := range users {
			lines = append(lines, targetRef(u))
		}
	}

	for _, s := range []struct {
		vals  []string
		title string
	}{
		{pkg.Deps, "Dependencies"},
		{pkg.Apis, "Provides APIs"},
		{pkg.ReqApis, "Requires APIs"},
	} {
		if len(s.vals) == 0 {
			continue
		}
		lines = append(lines, section(s.title)...)
		lines = append(lines, s.vals...)
	}

	if pkg.SyscfgDefs > 0 {
		lines = append(lines, "", field("Settings",
			fmt.Sprintf("%d defined in syscfg.yml", pkg.SyscfgDefs)))
	}

	return lines
}

func targetRef(name string) string {
	short, _ := splitName(name)
	return kindStyle(KindTarget).Render(strings.TrimPrefix(short, "targets/"))
}

func bspPreview(pkg *newtinfo.Package) []string {
	if pkg.Bsp == nil {
		return nil
	}

	lines := section("Hardware")
	lines = append(lines, field("Arch", pkg.Bsp.Arch))
	lines = append(lines, field("Compiler", pkg.Bsp.Compiler))

	if areas := pkg.Bsp.FlashMap; len(areas) > 0 {
		lines = append(lines, section("Flash map")...)
		for _, a := range areas {
			lines = append(lines, fmt.Sprintf("%-26s %s",
				strings.TrimPrefix(a.Name, "FLASH_AREA_"), styleDim.Render(
					fmt.Sprintf("0x%06x  %s", a.Offset, humanSize(int64(a.Size))))))
		}
	}

	return lines
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}

// setTargetVar sets a variable in the target's target.yml; an empty value
// deletes the variable.  Unlike "newt target set", which regenerates the
// target's files, only the variable's line is changed so that comments and
// formatting are kept.
func setTargetVar(path string, variable string, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	data = []byte(setYmlVar(string(data), variable, value))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	return nil
}

// setYmlVar replaces, adds or (for an empty value) removes the top-level
// "<variable>: <value>" line of a yml file.
func setYmlVar(yml string, variable string, value string) string {
	newLine := variable + ": " + strconv.Quote(value)

	out := []string{}
	found := false
	lines := strings.Split(strings.TrimSuffix(yml, "\n"), "\n")
	if yml == "" {
		// "newt target create" writes an empty target.yml.
		lines = nil
	}
	for _, line := range lines {
		if strings.HasPrefix(line, variable+":") {
			if !found && value != "" {
				out = append(out, newLine)
			}
			found = true
			continue
		}
		out = append(out, line)
	}
	if !found && value != "" {
		out = append(out, newLine)
	}

	return strings.Join(out, "\n") + "\n"
}

func overrideCount(n int) string {
	switch n {
	case 0:
		return "no overrides"
	case 1:
		return "1 override"
	default:
		return fmt.Sprintf("%d overrides", n)
	}
}

func overridesPreview(overrides map[string]string) []string {
	lines := []string{styleBold.Render("Settings set by this target"), ""}
	if len(overrides) == 0 {
		lines = append(lines, styleDim.Render("none yet"))
	}
	for _, k := range sortedKeys(overrides) {
		lines = append(lines, k+styleDim.Render(": ")+overrides[k])
	}
	return append(lines, "", styleDim.Render(
		"Press l to see and change all settings of this build; for "+
			"NimBLE apps, B there shows the Bluetooth essentials. Values "+
			"are saved in the target's syscfg.yml, which overrides the "+
			"app, the bsp and all packages."))
}
