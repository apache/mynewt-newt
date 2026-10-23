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
	"io"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type fakeCmd struct{}

func (fakeCmd) Run() error          { return nil }
func (fakeCmd) SetStdin(io.Reader)  {}
func (fakeCmd) SetStdout(io.Writer) {}
func (fakeCmd) SetStderr(io.Writer) {}

func testCats() []*Category {
	return []*Category{
		{Title: "Targets", Kind: KindTarget, Entries: []*Entry{
			{Name: "blinky", FullName: "targets/blinky", Kind: KindTarget,
				Actions: []Action{{Key: "b", Label: "build", Args: []string{"build", "targets/blinky"}}},
				Links:   []Link{{"a", "app", KindApp, "@core/apps/blinky"}}},
			{Name: "boot", FullName: "targets/boot", Kind: KindTarget},
			{Name: "sim", FullName: "@core/targets/sim", Kind: KindTarget,
				Foreign: true},
		}},
		{Title: "Apps", Kind: KindApp, Entries: []*Entry{
			{Name: "apps/bleprph", FullName: "@nimble/apps/bleprph", Kind: KindApp,
				Foreign: true},
			{Name: "apps/blinky", FullName: "@core/apps/blinky", Kind: KindApp,
				Foreign: true},
		}},
	}
}

func newTestModel(t *testing.T) (Model, *[][]string) {
	ran := [][]string{}
	m := NewModel("proj", func() ([]*Category, error) { return testCats(), nil })
	m.newtCmd = func(steps [][]string, env []string) tea.ExecCommand {
		ran = append(ran, steps[0])
		return fakeCmd{}
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
	return m, &ran
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	// Returned commands are run by the tea program, which these tests do not
	// start; actions are recorded by the fake newtCmd instead.
	next, _ := m.Update(msg)
	return next.(Model)
}

func keys(t *testing.T, m Model, ks ...string) Model {
	t.Helper()
	for _, k := range ks {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m = update(t, m, msg)
	}
	return m
}

func selectedName(m Model) string {
	if e := m.Selected(); e != nil {
		return e.FullName
	}
	return ""
}

func TestNavigation(t *testing.T) {
	m, _ := newTestModel(t)

	if m.Selected() != nil {
		t.Fatalf("expected categories level at start")
	}

	m = keys(t, m, "l")
	if got := selectedName(m); got != "targets/blinky" {
		t.Fatalf("got %q", got)
	}

	// Foreign targets are hidden by default, so the list ends at "boot".
	m = keys(t, m, "j", "j", "j")
	if got := selectedName(m); got != "targets/boot" {
		t.Fatalf("got %q", got)
	}

	m = keys(t, m, "g", "g")
	if got := selectedName(m); got != "targets/blinky" {
		t.Fatalf("got %q", got)
	}

	m = keys(t, m, "h", "j", "l", "G")
	if got := selectedName(m); got != "@core/apps/blinky" {
		t.Fatalf("got %q", got)
	}
}

func TestGroupedRunes(t *testing.T) {
	m, _ := newTestModel(t)

	// Key repeat delivers several runes in one message.
	m = keys(t, m, "lj")
	if got := selectedName(m); got != "targets/boot" {
		t.Fatalf("got %q", got)
	}
}

func TestUpAtTop(t *testing.T) {
	// k at the top of a list, also held down, stays on the first entry.
	for _, pre := range []string{"", "l", "ll", "lll"} {
		for _, ks := range []string{"k", "kk", "kkkkk", "kl", "kj"} {
			m, _ := newTestModel(t)
			if pre != "" {
				m = keys(t, m, pre)
			}
			want := selectedName(keys(t, m, ks[1:]))
			m = keys(t, m, ks)
			if got := selectedName(m); got != want {
				t.Errorf("%q then %q: got %q, want %q", pre, ks, got, want)
			}
			_ = m.View()
		}
	}
}

func TestForeignToggle(t *testing.T) {
	m, _ := newTestModel(t)

	m = keys(t, m, "l", ".", "G")
	if got := selectedName(m); got != "@core/targets/sim" {
		t.Fatalf("got %q", got)
	}

	m = keys(t, m, "z", "h", "G")
	if got := selectedName(m); got != "targets/boot" {
		t.Fatalf("got %q", got)
	}
}

func TestRepoGroups(t *testing.T) {
	app := func(repo string, name string) *Entry {
		return &Entry{Name: name, FullName: "@" + repo + "/" + name,
			Hint: "@" + repo, Kind: KindApp, Repo: repo}
	}
	load := func() ([]*Category, error) {
		apps := []*Entry{{Name: "apps/mine", FullName: "apps/mine",
			Kind: KindApp, Repo: "proj"}}
		for i := 0; i < 20; i++ {
			apps = append(apps, app("core", fmt.Sprintf("apps/c%02d", i)))
		}
		apps = append(apps, app("nimble", "apps/btshell"),
			app("nimble", "apps/bleprph"))
		return []*Category{{Title: "Apps", Kind: KindApp, Entries: apps}}, nil
	}
	m := NewModel("proj", load)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = keys(t, m, "l")

	// Each repo gets a header instead of an "@repo" hint on every entry.
	view := ansi.Strip(m.View())
	for _, repo := range []string{"proj", "core"} {
		if !strings.Contains(view, "── "+repo+" ──") {
			t.Errorf("no header for %s:\n%s", repo, view)
		}
	}
	if strings.Contains(view, "@core") {
		t.Errorf("hint shown next to grouped entries:\n%s", view)
	}
	if !strings.Contains(paneText(m), "next / previous repo") {
		t.Errorf("help:\n%s", paneText(m))
	}

	// ] and [ move between repos.
	m = keys(t, m, "]")
	if got := selectedName(m); got != "@core/apps/c00" {
		t.Fatalf("] went to %q", got)
	}
	m = keys(t, m, "]")
	if got := selectedName(m); got != "@nimble/apps/btshell" {
		t.Fatalf("] went to %q", got)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view,
		"── nimble ──") {
		t.Errorf("no header for nimble:\n%s", view)
	}
	m = keys(t, m, "j", "[")
	if got := selectedName(m); got != "@nimble/apps/btshell" {
		t.Fatalf("[ went to %q", got)
	}

	// Going back up shows the header of the repo above the cursor.
	m = keys(t, m, "[")
	if got := selectedName(m); got != "@core/apps/c00" {
		t.Fatalf("[ went to %q", got)
	}
	rows, rowOf := m.entryLayout(0)
	if l := m.entList[0]; l.offset != rowOf[l.cursor]-1 ||
		!rows[l.offset].header {
		t.Errorf("offset %d, cursor row %d", l.offset, rowOf[l.cursor])
	}

	// One repo left after filtering: no headers, and the hints are back.
	m = keys(t, m, "/", "b", "t", "enter")
	view = ansi.Strip(m.View())
	if strings.Contains(view, "── nimble") || !strings.Contains(view,
		"@nimble") {
		t.Errorf("filtered view:\n%s", view)
	}
}

func TestFilter(t *testing.T) {
	m, _ := newTestModel(t)

	m = keys(t, m, "j", "/", "b", "l", "i", "enter")
	if got := selectedName(m); got != "@core/apps/blinky" {
		t.Fatalf("got %q", got)
	}
	if n := len(m.visible(1)); n != 1 {
		t.Fatalf("expected 1 visible entry, got %d", n)
	}

	// After the filter is accepted, keys navigate again.  Clearing the
	// filter keeps the cursor on the selected entry.
	m = keys(t, m, "esc")
	if n := len(m.visible(1)); n != 2 {
		t.Fatalf("expected filter to be cleared, got %d entries", n)
	}
	if got := selectedName(m); got != "@core/apps/blinky" {
		t.Errorf("selection after clearing the filter: %q", got)
	}

}

func TestFollowLinkAndBack(t *testing.T) {
	m, _ := newTestModel(t)

	m = keys(t, m, "l", "g", "a")
	if got := selectedName(m); got != "@core/apps/blinky" {
		t.Fatalf("got %q", got)
	}

	m = keys(t, m, "H")
	if got := selectedName(m); got != "targets/blinky" {
		t.Fatalf("got %q", got)
	}
}

func TestAction(t *testing.T) {
	m, ran := newTestModel(t)

	// No action at the categories level.
	m = keys(t, m, "b")
	if len(*ran) != 0 {
		t.Fatalf("unexpected command %v", *ran)
	}

	m = keys(t, m, "l", "b")
	want := [][]string{{"build", "targets/blinky"}}
	if !reflect.DeepEqual(*ran, want) {
		t.Fatalf("got %v, want %v", *ran, want)
	}
}

func TestReloadKeepsCursor(t *testing.T) {
	m, _ := newTestModel(t)

	m = keys(t, m, "l", "j", "R")
	if got := selectedName(m); got != "targets/boot" {
		t.Fatalf("got %q", got)
	}
}

func TestViewSizes(t *testing.T) {
	m, _ := newTestModel(t)
	m = keys(t, m, "l")

	for _, size := range [][2]int{{100, 20}, {20, 5}, {3, 2}, {1, 1}} {
		m = update(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		v := m.View()
		if lines := strings.Count(v, "\n") + 1; lines > max(size[1], 3) {
			t.Errorf("%v: view has %d lines", size, lines)
		}
	}
}

func TestSplitName(t *testing.T) {
	for _, c := range []struct{ in, name, repo string }{
		{"targets/blinky", "targets/blinky", ""},
		{"@apache-mynewt-core/hw/bsp/native", "hw/bsp/native", "apache-mynewt-core"},
	} {
		name, repo := splitName(c.in)
		if name != c.name || repo != c.repo {
			t.Errorf("splitName(%q) = %q, %q", c.in, name, repo)
		}
	}
}

func TestHumanSize(t *testing.T) {
	for in, want := range map[int64]string{
		12:      "12 B",
		2048:    "2.0 KiB",
		3 << 20: "3.0 MiB",
	} {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSetYmlVar(t *testing.T) {
	yml := "# comment\n\n### Target\ntarget.app: \"apps/blinky\"\n" +
		"target.bsp: \"hw/bsp/native\"\n"

	for _, c := range []struct{ variable, value, want string }{
		{"target.bsp", "@core/hw/bsp/nrf52",
			"# comment\n\n### Target\ntarget.app: \"apps/blinky\"\n" +
				"target.bsp: \"@core/hw/bsp/nrf52\"\n"},
		{"target.loader", "@mcuboot/boot/mynewt",
			yml + "target.loader: \"@mcuboot/boot/mynewt\"\n"},
		{"target.app", "",
			"# comment\n\n### Target\ntarget.bsp: \"hw/bsp/native\"\n"},
		{"target.loader", "", yml},
	} {
		if got := setYmlVar(yml, c.variable, c.value); got != c.want {
			t.Errorf("setYmlVar(%s=%q):\n%s\nwant:\n%s", c.variable, c.value,
				got, c.want)
		}
	}

	// "newt target create" writes an empty target.yml.
	if got := setYmlVar("", "target.app", "apps/blinky"); got !=
		"target.app: \"apps/blinky\"\n" {
		t.Errorf("empty file: %q", got)
	}
}

func TestChooseField(t *testing.T) {
	values := map[string]string{"target.app": "@core/apps/blinky"}
	load := func() ([]*Category, error) {
		cats := testCats()
		e := cats[0].Entries[0]
		e.Fields = []*Field{
			{Label: "app", Var: "target.app", Value: values["target.app"],
				Kind: KindApp},
			{Label: "loader", Var: "target.loader",
				Value: values["target.loader"], Kind: KindApp, Optional: true},
		}
		e.Set = func(variable string, value string) error {
			values[variable] = value
			return nil
		}
		return cats, nil
	}

	m := NewModel("proj", load)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})

	// The choices open with the cursor on the current value.
	m = keys(t, m, "l", "l", "l")
	if o := m.option(); o == nil || o.FullName != "@core/apps/blinky" {
		t.Fatalf("expected cursor on current app, got %v", o)
	}

	m = keys(t, m, "k", "enter")
	if got := values["target.app"]; got != "@nimble/apps/bleprph" {
		t.Fatalf("app = %q", got)
	}
	if m.depth != depthFields || m.field().Value != "@nimble/apps/bleprph" {
		t.Fatalf("expected reloaded field, got depth %d", m.depth)
	}

	// Filter the loader choices, pick one, then unset it again via "none".
	m = keys(t, m, "j", "l", "/", "b", "l", "i", "enter", "enter")
	if got := values["target.loader"]; got != "@core/apps/blinky" {
		t.Fatalf("loader = %q", got)
	}
	m = keys(t, m, "l", "g", "g", "enter")
	if got, ok := values["target.loader"]; !ok || got != "" {
		t.Fatalf("loader = %q, expected unset", got)
	}

	// h cancels without changing anything.
	m = keys(t, m, "k", "l", "j", "h")
	if got := values["target.app"]; got != "@nimble/apps/bleprph" {
		t.Fatalf("app changed on cancel: %q", got)
	}
}

// runOp completes a background target operation the way the tea program
// would: run the command and feed its message back.
func runOp(t *testing.T, m Model, ks ...string) Model {
	t.Helper()
	for _, k := range ks {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, cmd := m.Update(msg)
		m = next.(Model)
		if cmd != nil {
			if done, ok := cmd().(opDoneMsg); ok {
				m = update(t, m, done)
			}
		}
	}
	return m
}

func TestTargetOps(t *testing.T) {
	m, _ := newTestModel(t)
	ran := [][]string{}
	m.newtRun = func(args []string) (string, error) {
		ran = append(ran, args)
		return "", nil
	}

	expect := func(want ...[]string) {
		t.Helper()
		if !reflect.DeepEqual(ran, want) {
			t.Fatalf("ran %v, want %v", ran, want)
		}
		ran = nil
	}

	// New target from the categories level; typed names go to targets/.
	m = runOp(t, m, "n", "foo", "enter")
	expect([]string{"target", "create", "foo"})

	// Keys typed right after "n" arrive in the same message.
	m = runOp(t, m, "esc", "h", "h", "nbar")
	m = runOp(t, m, "enter")
	expect([]string{"target", "create", "bar"})

	m = runOp(t, m, "esc", "h", "h", "l")
	m = runOp(t, m, "y", "enter")
	expect([]string{"target", "copy", "targets/blinky", "blinky_copy"})

	// Rename prefills the current name.
	m = runOp(t, m, "a", "2", "enter")
	expect([]string{"target", "copy", "targets/blinky", "blinky2"},
		[]string{"target", "delete", "-f", "targets/blinky"})

	// Delete asks first; anything but y cancels.
	m = runOp(t, m, "D", "n")
	expect()
	m = runOp(t, m, "D", "y")
	expect([]string{"target", "delete", "-f", "targets/blinky"})

	// Foreign targets cannot be changed.
	m = runOp(t, m, ".", "G", "D", "y")
	expect()
	if !m.statusErr {
		t.Fatalf("expected an error for a foreign target")
	}
}

func TestTargetOpError(t *testing.T) {
	m, _ := newTestModel(t)
	m.newtRun = func(args []string) (string, error) {
		return "Error: Target already exists: targets/boot\n\nUsage:\n" +
			"  newt target create [flags]\n\n  -q, --quiet  Be quiet\n", io.EOF
	}

	m = runOp(t, m, "n", "boot", "enter")
	if !m.statusErr || !strings.Contains(m.status,
		"Target already exists: targets/boot") {
		t.Fatalf("status = %q", m.status)
	}
}

func paneText(m Model) string {
	return ansi.Strip(strings.Join(m.helpPane(), "\n"))
}

func TestHelpPane(t *testing.T) {
	m, _ := newTestModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	contains := func(want ...string) {
		t.Helper()
		text := paneText(m)
		for _, w := range want {
			if !strings.Contains(text, w) {
				t.Errorf("help pane missing %q:\n%s", w, text)
			}
		}
	}
	missing := func(unwanted ...string) {
		t.Helper()
		text := paneText(m)
		for _, w := range unwanted {
			if strings.Contains(text, w) {
				t.Errorf("help pane should not contain %q:\n%s", w, text)
			}
		}
	}

	contains("open category", "new target", "quit")
	missing("build")

	// On a target: its actions, links and target management.
	m = keys(t, m, "l")
	contains("Run on blinky", "b  build", "ga  app", "D  delete")

	// On a package: no target keys.
	m = keys(t, m, "h", "j", "l")
	missing("new target", "Run on")

	// Prompts show only the keys that answer them.
	m = keys(t, m, "h", "k", "l", "D")
	contains("Confirm", "yes", "no, cancel")
	missing("down / up")
	m = keys(t, m, "n", "esc")

	// "?" hides the pane and gives the rows back to the lists.
	h := m.listHeight()
	m = keys(t, m, "?")
	if m.helpPane() != nil || m.listHeight() <= h {
		t.Fatalf("expected the pane to be hidden")
	}
	if !strings.Contains(ansi.Strip(m.statusBar()), "show help") {
		t.Errorf("status bar should say how to show help")
	}
	m = keys(t, m, "?")

	// Narrow terminals use the compact layout; small ones hide the pane.
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 24})
	if n := len(m.helpPane()); n == 0 || n > 12 {
		t.Errorf("compact pane has %d lines", n)
	}
	contains("delete", "quit")
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 20})
	if n := len(m.helpPane()); n == 0 || n > 10 {
		t.Errorf("compact pane has %d lines", n)
	}
	contains("Targets: n new target", "D delete", "q quit")
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 10})
	if m.helpPane() != nil {
		t.Errorf("expected no pane on a small terminal")
	}
}

func TestSetSyscfgVal(t *testing.T) {
	yml := "# Target settings.\nsyscfg.vals:\n  # Radio\n  BLE_FOO: 1\n" +
		"  BLE_BAR: 2\n\nsyscfg.defs:\n  X: {value: 0}\n"

	for _, c := range []struct {
		name, value string
		remove      bool
		in, want    string
	}{
		// Replace in place, keeping comments and indentation.
		{"BLE_FOO", "4", false, yml, strings.Replace(yml, "BLE_FOO: 1",
			"BLE_FOO: 4", 1)},
		// Add at the end of the block, before the next key.
		{"BLE_BAZ", "3", false, yml, strings.Replace(yml, "BLE_BAR: 2\n",
			"BLE_BAR: 2\n  BLE_BAZ: 3\n", 1)},
		{"BLE_BAR", "", true, yml, strings.Replace(yml, "  BLE_BAR: 2\n", "",
			1)},
		{"BLE_NONE", "", true, yml, yml},
		// Empty and missing files.
		{"BLE_FOO", "1", false, "", "syscfg.vals:\n    BLE_FOO: 1\n"},
		{"BLE_FOO", "1", false, "# hello\n",
			"# hello\n\nsyscfg.vals:\n    BLE_FOO: 1\n"},
		// Removing the last value drops the emptied map.
		{"BLE_FOO", "", true, "syscfg.vals:\n    BLE_FOO: 1\n", ""},
	} {
		got := setSyscfgVal(c.in, c.name, c.value, c.remove)
		if got != c.want {
			t.Errorf("setSyscfgVal(%s=%q, remove=%v):\n%s\nwant:\n%s",
				c.name, c.value, c.remove, got, c.want)
		}
	}
}

func TestYmlValue(t *testing.T) {
	for in, want := range map[string]string{
		"4":              "4",
		"-1":             "-1",
		"0x20000":        "0x20000",
		"ram":            "ram",
		"MYNEWT_VAL(X)":  "MYNEWT_VAL(X)",
		`"btshell"`:      `"\"btshell\""`,
		"*alias":         `"*alias"`,
		"":               `""`,
		"a: b":           `"a: b"`,
		" padded":        `" padded"`,
		"(1 << 4) | 0x1": "(1 << 4) | 0x1",
	} {
		if got := ymlValue(in); got != want {
			t.Errorf("ymlValue(%q) = %s, want %s", in, got, want)
		}
	}
}

func syscfgModel(t *testing.T, foreign bool) (Model, map[string]string) {
	saved := map[string]string{"BLE_FOO": "4"}
	settings := func() *BuildInfo {
		mk := func(name, def string, choices ...string) *Setting {
			s := &Setting{Name: name, Value: def, Choices: choices,
				Description: "about " + name,
				History:     []SettingPoint{{"default", def}}}
			if v, ok := saved[name]; ok {
				s.Value, s.Overridden = v, true
				s.History = append(s.History, SettingPoint{"target", v})
			}
			return s
		}
		info := &BuildInfo{Settings: []*Setting{
			mk("BLE_FOO", "1"),
			mk("BLE_FLAG", "0"),
			mk("CONSOLE_HISTORY", "none", "log", "none", "ram"),
			{Name: "PKG_CONST", Value: "1", Const: true},
		}}
		if saved["BLE_FOO"] == "99" {
			info.Errors = []string{"Syscfg restriction violations:",
				"* BLE_FOO must be at most 8"}
		}
		return info
	}

	load := func() ([]*Category, error) {
		cats := testCats()
		e := cats[0].Entries[0]
		e.Foreign = foreign
		e.Fields = []*Field{{Label: "syscfg", Kind: KindSyscfg,
			Preview: func() []string { return []string{"overrides"} }}}
		e.Syscfg = func() (*BuildInfo, error) { return settings(), nil }
		e.SetSyscfg = func(name string, value string, remove bool) error {
			if remove {
				delete(saved, name)
			} else {
				saved[name] = value
			}
			return nil
		}
		if foreign {
			cats[0].Entries = cats[0].Entries[:1]
		}
		return cats, nil
	}

	m := NewModel("proj", load)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	if foreign {
		m = keys(t, m, "l", ".")
	} else {
		m = keys(t, m, "l")
	}
	m = keys(t, m, "l", "l")
	if m.depth != depthSyscfg {
		t.Fatalf("expected the syscfg view, depth %d (%s)", m.depth, m.status)
	}
	return m, saved
}

func TestSyscfgEdit(t *testing.T) {
	m, saved := syscfgModel(t, false)

	if s := m.setting(); s == nil || s.Name != "BLE_FOO" {
		t.Fatalf("expected cursor on BLE_FOO, got %v", s)
	}
	if !strings.Contains(paneText(m), "remove this target's value") {
		t.Errorf("help should offer removing the override:\n%s", paneText(m))
	}

	// Enter on a plain setting asks for the value, prefilled.
	m = runOp(t, m, "enter")
	if m.prompt == nil || m.prompt.input != "4" {
		t.Fatalf("expected a prefilled prompt, got %+v", m.prompt)
	}
	if !strings.Contains(paneText(m), "save to the target's syscfg.yml") {
		t.Errorf("prompt help missing:\n%s", paneText(m))
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	m = runOp(t, m, "6", "enter")
	if saved["BLE_FOO"] != "6" {
		t.Fatalf("BLE_FOO = %q", saved["BLE_FOO"])
	}
	if s := m.setting(); s == nil || s.Name != "BLE_FOO" || s.Value != "6" {
		t.Fatalf("cursor should stay on the saved setting, got %v", s)
	}

	// A value that breaks the build is saved, with a warning.
	m = runOp(t, m, "enter")
	for range "6" {
		m = update(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m = runOp(t, m, "99", "enter")
	if !m.statusErr || !strings.Contains(m.status, "must be at most 8") {
		t.Errorf("expected a build warning, status %q", m.status)
	}

	// Space toggles flags.
	m = keys(t, m, "/", "flag", "enter", " ")
	if saved["BLE_FLAG"] != "1" {
		t.Fatalf("BLE_FLAG = %q", saved["BLE_FLAG"])
	}

	// Choices are picked from a list starting at the current value.
	m = keys(t, m, "esc", "/", "history", "enter", "enter")
	if m.depth != depthChoice || m.choiceList.cursor != 1 {
		t.Fatalf("expected choice list on current value, depth %d cursor %d",
			m.depth, m.choiceList.cursor)
	}
	m = keys(t, m, "j", "enter")
	if saved["CONSOLE_HISTORY"] != "ram" {
		t.Fatalf("CONSOLE_HISTORY = %q", saved["CONSOLE_HISTORY"])
	}

	// x removes the target's value; "o" hides unchanged settings.
	m = keys(t, m, "esc", "o")
	if n := len(m.visibleSettings()); n != 3 {
		t.Fatalf("expected 3 changed settings, got %d", n)
	}
	m = keys(t, m, "/", "flag", "enter", "x")
	if _, ok := saved["BLE_FLAG"]; ok {
		t.Fatalf("BLE_FLAG should be removed")
	}
	if !strings.Contains(m.status, "back to 0") {
		t.Errorf("status = %q", m.status)
	}

	// Constants cannot be changed.
	m = keys(t, m, "esc", "o", "G", "enter")
	if m.prompt != nil || !m.statusErr {
		t.Errorf("constant should not be editable")
	}

	m = keys(t, m, "h")
	if m.depth != depthFields {
		t.Errorf("h should go back to the target settings")
	}
}

func TestSyscfgReadOnly(t *testing.T) {
	m, saved := syscfgModel(t, true)

	if !strings.Contains(paneText(m), "read-only") {
		t.Errorf("help should say read-only:\n%s", paneText(m))
	}
	m = keys(t, m, "enter", "x", " ")
	if m.prompt != nil || saved["BLE_FOO"] != "4" || !m.statusErr {
		t.Errorf("foreign target must not be changed: %v %q", saved, m.status)
	}
}

// step sends a key and runs a returned command if it is a board scan.
func TestBluetoothPage(t *testing.T) {
	m, _ := syscfgModel(t, false)
	m = keys(t, m, "B")
	if m.bluetoothOnly || !strings.Contains(m.status, "does not include NimBLE") {
		t.Errorf("B without NimBLE: %q", m.status)
	}

	m.syscfg.Settings = append(m.syscfg.Settings,
		&Setting{Name: "BLE_SM_SC", Value: "0"},
		&Setting{Name: "BLE_ROLE_PERIPHERAL", Value: "1"},
		&Setting{Name: "BLE_UNLISTED", Value: "1"})
	if !strings.Contains(paneText(m), "Bluetooth essentials only") {
		t.Errorf("help:\n%s", paneText(m))
	}
	m = keys(t, m, "B")
	names := []string{}
	for _, s := range m.visibleSettings() {
		names = append(names, s.Name)
	}
	// Roles come before security, unlisted settings are left out.
	if !reflect.DeepEqual(names, []string{"BLE_ROLE_PERIPHERAL", "BLE_SM_SC"}) {
		t.Errorf("Bluetooth page = %v", names)
	}
	p := ansi.Strip(strings.Join(settingPreview(m.setting(), nil), "\n"))
	if !strings.Contains(p, "Bluetooth › Roles") {
		t.Errorf("preview:\n%s", p)
	}
}

func TestProjectTitle(t *testing.T) {
	for _, c := range []struct {
		dir, name, title, note string
	}{
		// "newt new" names every project my_project; the directory says more.
		{"/home/u/nimbleproj", "my_project", "nimbleproj", ""},
		{"/home/u/nimbleproj", "nimbleproj", "nimbleproj", ""},
		{"/home/u/nimbleproj", "", "nimbleproj", ""},
		{"/home/u/work", "sensor_hub", "work", "sensor_hub"},
	} {
		title, note := ProjectTitle(c.dir, c.name)
		if title != c.title || note != c.note {
			t.Errorf("ProjectTitle(%q, %q) = %q, %q", c.dir, c.name, title, note)
		}
	}

	m, _ := newTestModel(t)
	m.note = "sensor_hub"
	if bar := ansi.Strip(m.titleBar()); !strings.HasPrefix(bar,
		"newt proj (sensor_hub) › Targets") {
		t.Errorf("title bar = %q", bar)
	}
}

func TestCleanAsksFirst(t *testing.T) {
	m, ran := newTestModel(t)
	m.cats[0].Entries[0].Actions = append(m.cats[0].Entries[0].Actions,
		Action{Key: "c", Label: "clean", Args: []string{"clean", "targets/blinky"},
			Confirm: "Delete the build output of blinky?"})

	m = keys(t, m, "l", "c")
	if m.prompt == nil || !strings.Contains(m.prompt.label, "build output") ||
		len(*ran) != 0 {
		t.Fatalf("clean should ask first, ran %v", *ran)
	}
	m = keys(t, m, "n")
	if len(*ran) != 0 {
		t.Fatalf("n should not clean")
	}
	m = keys(t, m, "c", "y")
	if len(*ran) != 1 || (*ran)[0][0] != "clean" {
		t.Fatalf("y should clean, ran %v", *ran)
	}
}
