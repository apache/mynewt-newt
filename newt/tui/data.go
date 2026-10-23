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

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/config"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
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
	// Entries from foreign repos are hidden unless the user toggles them.
	Foreign bool
	// The repo the entry comes from; lists group their entries by it.
	Repo    string
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

// Load builds the browser contents from the current newt project.
func Load(proj *project.Project) []*Category {
	users := map[string][]string{}
	targets := targetEntries(proj, users)

	cats := []*Category{
		{Title: "Targets", Kind: KindTarget, Entries: targets},
		{Title: "Apps", Kind: KindApp},
		{Title: "BSPs", Kind: KindBsp},
		{Title: "Libraries", Kind: KindLib},
		{Title: "Unit tests", Kind: KindUnittest},
	}
	pkgTypes := []interfaces.PackageType{
		pkg.PACKAGE_TYPE_APP,
		pkg.PACKAGE_TYPE_BSP,
		pkg.PACKAGE_TYPE_LIB,
		pkg.PACKAGE_TYPE_UNITTEST,
	}

	for i, pt := range pkgTypes {
		cat := cats[i+1]
		for _, p := range proj.PackagesOfType(pt) {
			lpkg := p.(*pkg.LocalPackage)
			cat.Entries = append(cat.Entries,
				packageEntry(proj, lpkg, cat.Kind, users[lpkg.FullName()]))
		}
		sortEntries(cat.Entries)
	}

	return cats
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

func relPath(proj *project.Project, path string) string {
	if rel, err := filepath.Rel(proj.Path(), path); err == nil {
		return rel
	}
	return path
}

func targetEntries(proj *project.Project, users map[string][]string) []*Entry {
	entries := []*Entry{}
	targets := map[string]*target.Target{}

	for name, t := range target.GetTargets() {
		// The unittest target is used internally by "newt test".
		if strings.HasSuffix(name, "/unittest") {
			continue
		}

		t := t
		full := t.FullName()
		short, repo := splitName(full)
		short = strings.TrimPrefix(short, "targets/")

		e := &Entry{
			Name:     short,
			FullName: full,
			Kind:     KindTarget,
			EditPath: t.TargetYamlPath(),
			Foreign:  !t.Package().Repo().IsLocal(),
			Repo:     t.Package().Repo().Name(),
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
			if name == "" {
				return
			}
			e.Links = append(e.Links, Link{key, label, kind, name})
			users[name] = append(users[name], full)
		}
		if t.App() != nil {
			link("a", "app", KindApp, t.App().FullName())
		}
		if t.Bsp() != nil {
			link("b", "bsp", KindBsp, t.Bsp().FullName())
		}
		if t.Loader() != nil {
			link("l", "loader", KindApp, t.Loader().FullName())
		}

		e.Fields = []*Field{
			{Label: "app", Var: "target.app", Value: t.AppName,
				Kind: KindApp},
			{Label: "bsp", Var: "target.bsp", Value: t.BspName,
				Kind: KindBsp},
			{Label: "profile", Var: "target.build_profile",
				Value: t.BuildProfile, Kind: KindValue,
				valuesFn: func() []string { return buildProfiles(proj, t) }},
		}
		flash := downloaderField(e)
		e.Fields = append(e.Fields, flash)
		for i := range e.Actions {
			if e.Actions[i].Args[0] == "load" {
				e.Actions[i].Check = func() error { return loadCheck(flash) }
			}
		}

		// Loaders are only used for split images, which are rare; only
		// offer the setting on targets that already use one.
		if t.LoaderName != "" {
			e.Fields = append(e.Fields, &Field{Label: "loader",
				Var: "target.loader", Value: t.LoaderName, Kind: KindApp,
				Optional: true})
		}
		e.Set = func(variable string, value string) error {
			return setTargetVar(t, variable, value)
		}

		overrides, _ := t.Package().SyscfgY.GetValStringMapString(
			"syscfg.vals", nil)
		e.Fields = append(e.Fields, &Field{Label: "syscfg",
			Value: overrideCount(len(overrides)), Kind: KindSyscfg,
			Preview: func() []string { return overridesPreview(overrides) }})
		// Resolved at most once per load of the project.
		var info *BuildInfo
		e.Syscfg = func() (*BuildInfo, error) {
			if info != nil {
				return info, nil
			}
			var err error
			info, err = resolveSyscfg(t)
			return info, err
		}
		e.SetSyscfg = func(name string, value string, remove bool) error {
			return setTargetSyscfg(t, name, value, remove)
		}

		e.previewFn = func() []string { return targetPreview(proj, t, e) }
		entries = append(entries, e)
		targets[full] = t
	}

	addBootSupport(proj, entries, targets)
	sortEntries(entries)
	for _, list := range users {
		sort.Strings(list)
	}

	return entries
}

func packageEntry(proj *project.Project, lpkg *pkg.LocalPackage, kind Kind,
	users []string) *Entry {

	full := lpkg.FullName()
	short, repo := splitName(full)

	e := &Entry{
		Name:     short,
		FullName: full,
		Kind:     kind,
		EditPath: lpkg.PkgYamlPath(),
		Foreign:  !lpkg.Repo().IsLocal(),
		Repo:     lpkg.Repo().Name(),
	}
	if repo != "" {
		e.Hint = "@" + repo
	}
	if kind == KindUnittest {
		e.Actions = []Action{{Key: "t", Label: "test", Args: []string{"test", full}}}
	}

	e.previewFn = func() []string {
		return packagePreview(proj, lpkg, kind, users)
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

func targetPreview(proj *project.Project, t *target.Target, e *Entry) []string {
	lines := []string{
		field("Target", styleBold.Render(t.FullName())),
		field("App", t.AppName),
		field("BSP", t.BspName),
	}
	if t.LoaderName != "" {
		lines = append(lines, field("Loader", t.LoaderName))
	}
	lines = append(lines,
		field("Profile", t.BuildProfile),
		field("Path", relPath(proj, t.Package().BasePath())))

	built := styleDim.Render("not built")
	if t.App() != nil {
		elf := builder.AppElfPath(t.FullName(), builder.BUILD_NAME_APP,
			t.App().FullName())
		if fi, err := os.Stat(elf); err == nil {
			built = styleOk.Render("built") + styleDim.Render(fmt.Sprintf(
				" %s, %s", fi.ModTime().Format(time.DateTime),
				humanSize(fi.Size())))
		}
	}
	lines = append(lines, field("Status", built))
	lines = append(lines, bootPreview(e)...)

	flags := []string{}
	for _, f := range []string{"aflags", "cflags", "cxxflags", "lflags"} {
		vals, _ := t.Package().PkgY.GetValStringSlice("pkg."+f, nil)
		if len(vals) > 0 {
			flags = append(flags, field(f, strings.Join(vals, " ")))
		}
	}
	if len(flags) > 0 {
		lines = append(lines, section("Flags")...)
		lines = append(lines, flags...)
	}

	vals, _ := t.Package().SyscfgY.GetValStringMapString("syscfg.vals", nil)
	if len(vals) > 0 {
		lines = append(lines, section("Settings")...)
		for _, k := range sortedKeys(vals) {
			lines = append(lines, k+styleDim.Render(": ")+vals[k])
		}
	}

	return lines
}

func packagePreview(proj *project.Project, lpkg *pkg.LocalPackage, kind Kind,
	users []string) []string {

	_, repo := splitName(lpkg.FullName())
	if repo == "" {
		repo = proj.LocalRepo().Name()
	}

	desc := lpkg.Desc()
	lines := []string{
		field("Package", styleBold.Render(lpkg.FullName())),
		field("Type", pkg.PackageTypeNames[lpkg.Type()]),
		field("Repo", repo),
		field("Path", relPath(proj, lpkg.BasePath())),
	}
	if desc.Author != "" {
		lines = append(lines, field("Author", desc.Author))
	}
	if desc.Homepage != "" {
		lines = append(lines, field("Homepage", desc.Homepage))
	}
	if len(desc.Keywords) > 0 {
		lines = append(lines, field("Keywords", strings.Join(desc.Keywords, ", ")))
	}
	if desc.Description != "" {
		lines = append(lines, "", desc.Description)
	}

	if kind == KindBsp {
		lines = append(lines, bspPreview(lpkg)...)
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

	for _, s := range []struct{ key, title string }{
		{"pkg.deps", "Dependencies"},
		{"pkg.apis", "Provides APIs"},
		{"pkg.req_apis", "Requires APIs"},
	} {
		vals, _ := lpkg.PkgY.GetValStringSlice(s.key, nil)
		if len(vals) == 0 {
			continue
		}
		sort.Strings(vals)
		lines = append(lines, section(s.title)...)
		lines = append(lines, vals...)
	}

	defs, _ := lpkg.SyscfgY.GetValStringMap("syscfg.defs", nil)
	if len(defs) > 0 {
		lines = append(lines, "", field("Settings",
			fmt.Sprintf("%d defined in syscfg.yml", len(defs))))
	}

	return lines
}

func targetRef(name string) string {
	short, _ := splitName(name)
	return kindStyle(KindTarget).Render(strings.TrimPrefix(short, "targets/"))
}

func bspPreview(lpkg *pkg.LocalPackage) []string {
	yc, err := config.ReadFile(filepath.Join(lpkg.BasePath(),
		pkg.BSP_YAML_FILENAME))
	if err != nil {
		return nil
	}

	lines := section("Hardware")
	for _, f := range []struct{ key, label string }{
		{"bsp.arch", "Arch"},
		{"bsp.compiler", "Compiler"},
	} {
		val, _ := yc.GetValString(f.key, nil)
		lines = append(lines, field(f.label, val))
	}

	if areas := bspFlashAreas(lpkg); len(areas) > 0 {
		names := make([]string, 0, len(areas))
		for k := range areas {
			names = append(names, k)
		}
		sort.Slice(names, func(i, j int) bool {
			return areas[names[i]].Offset < areas[names[j]].Offset
		})
		lines = append(lines, section("Flash map")...)
		for _, n := range names {
			a := areas[n]
			lines = append(lines, fmt.Sprintf("%-26s %s",
				strings.TrimPrefix(n, "FLASH_AREA_"), styleDim.Render(
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
func setTargetVar(t *target.Target, variable string, value string) error {
	path := t.TargetYamlPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return util.ChildNewtError(err)
	}

	data = []byte(setYmlVar(string(data), variable, value))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return util.ChildNewtError(err)
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

// buildProfiles returns the build profiles defined by the compiler of the
// target's BSP, i.e. the <profile> parts of its compiler.flags.<profile>
// settings.
func buildProfiles(proj *project.Project, t *target.Target) []string {
	found := map[string]bool{
		target.DEFAULT_BUILD_PROFILE: true,
		t.BuildProfile:               true,
	}

	if bsp := t.Bsp(); bsp != nil {
		bspY, _ := config.ReadFile(filepath.Join(bsp.BasePath(),
			pkg.BSP_YAML_FILENAME))
		compName, _ := bspY.GetValString("bsp.compiler", nil)
		// The compiler name is relative to the BSP's repo.
		var comp *pkg.LocalPackage
		if compName != "" {
			comp, _ = proj.ResolvePackage(bsp.Repo(), compName)
		}
		if comp != nil {
			compY, _ := config.ReadFile(filepath.Join(comp.BasePath(),
				toolchain.COMPILER_FILENAME))
			for k := range compY.AllSettings() {
				parts := strings.Split(k, ".")
				if len(parts) >= 3 && parts[0] == "compiler" &&
					parts[1] == "flags" && parts[2] != "base" {
					found[parts[2]] = true
				}
			}
		}
	}

	profiles := []string{}
	for p := range found {
		if p != "" {
			profiles = append(profiles, p)
		}
	}
	sort.Strings(profiles)
	return profiles
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
