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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

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

func init() {
	// Never run real newt commands from the tests.
	runSteps = func(ctx context.Context, steps [][]string, env []string,
		out io.Writer) error {
		return nil
	}
}

// settle finishes a background job, as the tea program would.
func settle(t *testing.T, m Model) Model {
	t.Helper()
	for m.jobRunning() {
		select {
		case msg := <-m.jobCh:
			next, _ := m.Update(msg)
			m = next.(Model)
		case <-time.After(2 * time.Second):
			t.Fatalf("job did not finish")
		}
	}
	return m
}

func newTestModel(t *testing.T) (Model, *[][]string) {
	ran := [][]string{}
	m := NewModel("proj", func() ([]*Category, error) { return testCats(), nil })
	m.newtCmd = func(steps [][]string, env []string) tea.ExecCommand {
		ran = append(ran, steps[0])
		return fakeCmd{}
	}
	m.runSteps = func(ctx context.Context, steps [][]string, env []string,
		out io.Writer) error {
		ran = append(ran, steps[0])
		return nil
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
	return m, &ran
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	// Returned commands are run by the tea program, which these tests do not
	// start; actions are recorded by the fake newtCmd and runSteps instead.
	next, _ := m.Update(msg)
	return settle(t, next.(Model))
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

	// Ctrl-C does not quit.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil || !strings.Contains(next.(Model).status, "q quits") {
		t.Errorf("Ctrl-C should not quit")
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
		if m.jobRunning() {
			m = settle(t, m)
			continue
		}
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

func TestFlashTool(t *testing.T) {
	installed := map[string]bool{"JLinkExe": true}
	lookPath = func(prog string) (string, error) {
		if installed[prog] {
			return "/usr/bin/" + prog, nil
		}
		return "", io.EOF
	}
	defer func() { lookPath = exec.LookPath }()
	defaultRunTool := runTool
	runTool = func(stdin string, name string, args ...string) ([]byte,
		error) {
		if name == "/usr/bin/JLinkExe" && strings.Join(args, " ") ==
			"-NoGui 1 -version" {
			return []byte("SEGGER J-Link Commander V7.94e (Compiled Jan 1 " +
				"2025)\nDLL version V7.94e\n\nUnknown command line " +
				"option -version.\n"), errors.New("exit status 1")
		}
		return nil, errors.New("not here")
	}
	defer func() { runTool = defaultRunTool }()
	toolVersions.Clear()
	defer toolVersions.Clear()

	saved := map[string]string{}
	load := func() ([]*Category, error) {
		cats := testCats()
		e := cats[0].Entries[0]
		e.Syscfg = func() (*BuildInfo, error) {
			s := &Setting{Name: downloaderSetting, Value: "nrfutil",
				Choices: []string{"jlink", "nrfutil", "bsp_downloader"},
				History: []SettingPoint{{"default", ""},
					{"bsp nordic_pca10056", "nrfutil"}}}
			if v, ok := saved[downloaderSetting]; ok {
				s.Value, s.Overridden = v, true
				s.History = append(s.History, SettingPoint{"target", v})
			}
			return &BuildInfo{Settings: []*Setting{s,
				{Name: "JLINK_TARGET", Value: "NRF52"}}}, nil
		}
		e.SetSyscfg = func(name string, value string, remove bool) error {
			if remove {
				delete(saved, name)
			} else {
				saved[name] = value
			}
			return nil
		}
		flash := downloaderField(e)
		e.Fields = []*Field{flash}
		e.Actions = []Action{{Key: "L", Label: "load",
			Args:  []string{"load", e.FullName},
			Check: func() error { return loadCheck(flash) }}}
		return cats, nil
	}

	m := NewModel("proj", load)
	m.newtCmd = func(steps [][]string, env []string) tea.ExecCommand { return fakeCmd{} }
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	// Loading with a missing tool explains what to do instead.
	m = keys(t, m, "l", "L")
	if !m.statusErr || !strings.Contains(m.status, "nrfutil is not installed") ||
		!strings.Contains(m.status, "installed: jlink") {
		t.Fatalf("status = %q", m.status)
	}

	m = keys(t, m, "l")
	rows := m.fieldRows(m.Selected())
	if !strings.Contains(rows[0].text, "nrfutil") ||
		rows[0].hint != "not installed" {
		t.Fatalf("flash row = %+v", rows[0])
	}

	// The choices start at the current tool and say what is installed.
	m = keys(t, m, "l")
	if o := m.option(); o == nil || o.FullName != "nrfutil" {
		t.Fatalf("expected cursor on nrfutil, got %v", o)
	}
	// Only the Nordic tools are offered, in a fixed order.
	names := []string{}
	for _, o := range m.visibleOpts() {
		names = append(names, o.Name)
	}
	if !reflect.DeepEqual(names, []string{"nrfutil", "jlink"}) {
		t.Fatalf("choices = %v", names)
	}
	if o := m.visibleOpts()[1]; o.Hint != "7.94e" {
		t.Errorf("jlink hint = %q", o.Hint)
	}
	if p := ansi.Strip(strings.Join(m.visibleOpts()[1].Preview(), "\n")); !strings.Contains(p, "JLINK_TARGET: NRF52") ||
		!strings.Contains(p, "JLinkExe 7.94e (installed)") {
		t.Errorf("jlink preview:\n%s", p)
	}

	m = keys(t, m, "j", "enter")
	if saved[downloaderSetting] != "jlink" {
		t.Fatalf("MYNEWT_DOWNLOADER = %q", saved[downloaderSetting])
	}
	if f := m.field(); f.Value != "jlink" || f.Hint != "7.94e" {
		t.Fatalf("flash field = %q %q", f.Value, f.Hint)
	}

	// Now load runs.
	m = keys(t, m, "h")
	if cmd := m.run(m.Selected().Actions[0]); cmd == nil {
		t.Fatalf("load should run with jlink installed: %q", m.status)
	}

	// "bsp default" removes the target's choice.
	m = keys(t, m, "l", "l", "g", "g", "enter")
	if _, ok := saved[downloaderSetting]; ok {
		t.Fatalf("expected the override to be removed")
	}
	if !strings.Contains(m.status, "back to the default (nrfutil)") {
		t.Errorf("status = %q", m.status)
	}
}

func TestParseDevices(t *testing.T) {
	out := `{"type":"task_begin","data":{"task":{"name":"list_devices"}}}
{"type":"info","data":{"devices":[{"id":"1050099999","serialNumber":"001050099999","traits":{"jlink":true,"devkit":true},"devkit":{"boardVersion":"PCA10056","deviceFamily":"NRF52_FAMILY"},"jlink":{"deviceVersion":"NRF52840_xxAA_REV3"},"usb":{"device":{"product":"J-Link"}},"serialPorts":[{"comName":"/dev/ttyACM0"},{"path":"/dev/ttyACM1"}]},{"id":"683000001","serialNumber":"000683000001","traits":{"jlink":true}}]}}
{"type":"task_end","data":{"devices":[]}}`

	got := parseNrfutilDevices([]byte(out))
	want := []Device{
		{Serial: "000683000001", Source: "nrfutil"},
		{Serial: "001050099999", Product: "J-Link", Board: "PCA10056",
			Chip:  "NRF52840_xxAA_REV3",
			Ports: []string{"/dev/ttyACM0", "/dev/ttyACM1"}, Source: "nrfutil"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNrfutilDevices:\n%+v\nwant:\n%+v", got, want)
	}

	jl := "J-Link>ShowEmuList\n" +
		"J-Link[0]: Connection: USB, Serial number: 1050099999, ProductName: J-Link OB-nRF5340-NordicSemi\n" +
		"J-Link[1]: Connection: USB, Serial number: 683000001, ProductName: J-Link\n"
	got = parseJLinkDevices([]byte(jl))
	if len(got) != 2 || got[0].Serial != "1050099999" ||
		got[0].Product != "J-Link OB-nRF5340-NordicSemi" {
		t.Errorf("parseJLinkDevices: %+v", got)
	}

	// The serial format follows the tool.
	if s := deviceSN("jlink", "001050099999"); s != "1050099999" {
		t.Errorf("jlink SN = %q", s)
	}
	if s := deviceSN("nrfutil", "001050099999"); s != "001050099999" {
		t.Errorf("nrfutil SN = %q", s)
	}
}

// step sends a key and runs a returned command if it is a board scan.
func step(t *testing.T, m Model, k string) Model {
	t.Helper()
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
	if m.jobRunning() {
		return settle(t, m)
	}
	if cmd != nil {
		if found, ok := cmd().(devicesMsg); ok {
			m = update(t, m, found)
		}
	}
	return m
}

func TestDevicePicker(t *testing.T) {
	saved := map[string]string{"MYNEWT_DEBUGGER_SN": "1050022222"}
	boards := []Device{
		{Serial: "001050011111", Board: "PCA10056", Source: "nrfutil"},
		{Serial: "001050022222", Board: "PCA10040", Source: "nrfutil"},
	}

	load := func() ([]*Category, error) {
		cats := testCats()
		e := cats[0].Entries[0]
		e.Actions = []Action{{Key: "L", Label: "load",
			Args: []string{"load", e.FullName}, PickDevice: true}}
		e.Syscfg = func() (*BuildInfo, error) {
			return &BuildInfo{Settings: []*Setting{
				{Name: downloaderSetting, Value: "jlink"},
				{Name: deviceSetting, Value: saved[deviceSetting]},
			}}, nil
		}
		e.SetSyscfg = func(name string, value string, remove bool) error {
			saved[name] = value
			return nil
		}
		flash := downloaderField(e)
		e.Fields = []*Field{flash}
		return cats, nil
	}

	ran := []string{}
	m := NewModel("proj", load)
	m.newtCmd = func(steps [][]string, env []string) tea.ExecCommand {
		ran = append(ran, strings.Join(append(env, steps[0]...), " "))
		return fakeCmd{}
	}
	m.runSteps = func(ctx context.Context, steps [][]string, env []string,
		out io.Writer) error {
		ran = append(ran, strings.Join(append(env, steps[0]...), " "))
		return nil
	}
	m.findDevices = func() ([]Device, error) { return boards, nil }
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	// The picker starts on the target's default board.
	m = step(t, m, "l")
	m = step(t, m, "L")
	if m.depth != depthDevice {
		t.Fatalf("expected the board picker, depth %d", m.depth)
	}
	if d := m.pickedDevice(); d == nil || d.Board != "PCA10040" {
		t.Fatalf("expected the default board, got %v", d)
	}
	if !strings.Contains(paneText(m), "load on this board") {
		t.Errorf("help pane:\n%s", paneText(m))
	}

	// Enter runs on the chosen board; jlink gets the number without zeros.
	m = step(t, m, "k")
	m = step(t, m, "enter")
	if len(ran) != 1 || ran[0] != "JLINK_SN=1050011111 load targets/blinky" {
		t.Fatalf("ran %q", ran)
	}
	if m.depth != depthEntries {
		t.Errorf("expected to return to the targets, depth %d", m.depth)
	}

	// s also makes it the default.
	m = step(t, m, "L")
	m = step(t, m, "k")
	m = step(t, m, "s")
	if saved[deviceSetting] != "1050011111" {
		t.Errorf("default board = %q", saved[deviceSetting])
	}

	// "let the tool pick" runs without JLINK_SN.
	m = step(t, m, "L")
	m = step(t, m, "G")
	m = step(t, m, "enter")
	if last := ran[len(ran)-1]; last != "load targets/blinky" {
		t.Errorf("ran %q", last)
	}

	// No boards: say so, and allow scanning again; Esc cancels.
	boards = nil
	m = step(t, m, "L")
	if !m.statusErr || !strings.Contains(m.status, "no boards found") {
		t.Errorf("status = %q", m.status)
	}
	boards = []Device{{Serial: "001050033333", Source: "nrfutil"}}
	m = step(t, m, "r")
	if len(m.picker.devices) != 1 {
		t.Errorf("rescan found %d boards", len(m.picker.devices))
	}
	n := len(ran)
	m = step(t, m, "esc")
	if m.depth != depthEntries || len(ran) != n {
		t.Errorf("Esc should cancel, depth %d", m.depth)
	}
}

func TestParseChipInfo(t *testing.T) {
	out := `{"type":"task_begin","data":{"task":{"name":"device_info"}}}
{"type":"task_end","data":{"result":"success","data":{"devices":[{"serialNumber":"000683281753","deviceInfo":{"jlink":{"boardVersion":"PCA10056","deviceFamily":"NRF52_FAMILY","deviceVersion":"NRF52840_xxAA_REV2","jlinkObFirmwareVersion":"J-Link OB-SAM3U128-V2-NordicSemi compiled Nov  7 2022","serialNumber":"000683281753"},"hwInfo":{"romSize":1048576,"ramSize":262144,"romPageSize":4096}},"protectionStatus":"NRFDL_PROTECTION_STATUS_NONE"}]}}}`

	got := parseChipInfo([]byte(out))
	want := &ChipInfo{Version: "NRF52840_xxAA_REV2", Part: "NRF52840",
		Variant: "xxAA", Revision: "2", Family: "NRF52", Board: "PCA10056",
		ProbeFirmware: "J-Link OB-SAM3U128-V2-NordicSemi compiled Nov  7 2022",
		Flash:         1 << 20, RAM: 256 << 10, Protection: "none"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseChipInfo:\n%+v\nwant:\n%+v", got, want)
	}
	if got.Protected() {
		t.Errorf("chip should not be protected")
	}

	locked := parseChipInfo([]byte(
		`{"type":"info","data":{"protectionStatus":"NRFDL_PROTECTION_STATUS_ALL"}}`))
	if !locked.Protected() || locked.Protection != "readback protected" {
		t.Errorf("protection = %q", locked.Protection)
	}

	for _, c := range []struct {
		part, target string
		ok           bool
	}{
		{"NRF52840", "nRF52840", true},
		{"NRF52832", "nRF52840", false},
		{"NRF5340", "nRF5340_app", true},
		{"NRF52840", "", true},
	} {
		if got := chipMatches(c.part, c.target); got != c.ok {
			t.Errorf("chipMatches(%q, %q) = %v", c.part, c.target, got)
		}
	}

	errOut := `{"type":"task_end","data":{"result":"fail","error":{"code":4,"description":"Failed to open probe"}}}`
	if msg := nrfutilError([]byte(errOut), io.EOF); msg != "Failed to open probe" {
		t.Errorf("nrfutilError = %q", msg)
	}
}

func TestDevicePickerChip(t *testing.T) {
	boards := []Device{
		{Serial: "000682222222", Board: "PCA10040", Source: "nrfutil"},
		{Serial: "001050011111", Board: "PCA10056", Source: "nrfutil"},
	}
	chips := map[string]*ChipInfo{
		"001050011111": {Part: "NRF52840", Revision: "3", Protection: "none"},
		"000682222222": {Part: "NRF52832", Protection: "readback protected"},
	}

	load := func() ([]*Category, error) {
		cats := testCats()
		e := cats[0].Entries[0]
		e.Actions = []Action{{Key: "L", Label: "load",
			Args: []string{"load", e.FullName}, PickDevice: true}}
		e.Syscfg = func() (*BuildInfo, error) {
			return &BuildInfo{Settings: []*Setting{
				{Name: downloaderSetting, Value: "nrfutil"},
				{Name: "MCU_TARGET", Value: "nRF52840"},
			}}, nil
		}
		e.Fields = []*Field{downloaderField(e)}
		return cats, nil
	}

	ran := 0
	m := NewModel("proj", load)
	m.newtCmd = func(steps [][]string, env []string) tea.ExecCommand {
		ran++
		return fakeCmd{}
	}
	m.runSteps = func(ctx context.Context, steps [][]string, env []string,
		out io.Writer) error {
		ran++
		return nil
	}
	m.findDevices = func() ([]Device, error) { return boards, nil }
	m.readChip = func(serial string) (*ChipInfo, error) {
		return chips[serial], nil
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = step(t, m, "l")
	m = step(t, m, "L")

	// The chips are read after the scan.
	for _, d := range boards {
		info, _ := m.readChip(d.Serial)
		m = update(t, m, chipMsg{d.Serial, info, nil})
	}

	// The first board has the wrong chip, and it is locked.
	text := ansi.Strip(strings.Join(m.devicePreviewLines(), "\n"))
	for _, want := range []string{"NRF52832", "built for nRF52840",
		"will not run", "readback protected", "nrfutil device recover"} {
		if !strings.Contains(text, want) {
			t.Errorf("preview missing %q:\n%s", want, text)
		}
	}

	// Loading on the wrong chip asks first.
	m = step(t, m, "enter")
	if m.prompt == nil || !m.prompt.confirm || ran != 0 {
		t.Fatalf("expected a confirmation, ran %d", ran)
	}
	m = step(t, m, "n")
	if ran != 0 {
		t.Fatalf("n should cancel")
	}

	// The matching board loads straight away.
	m = step(t, m, "j")
	text = ansi.Strip(strings.Join(m.devicePreviewLines(), "\n"))
	if !strings.Contains(text, "matches this board") {
		t.Errorf("preview:\n%s", text)
	}
	m = step(t, m, "enter")
	if ran != 1 {
		t.Fatalf("expected load to run, ran %d", ran)
	}
}

func TestFlashToolOutsideList(t *testing.T) {
	// A BSP that uses a tool outside the list still shows it.
	e := &Entry{Name: "stm", FullName: "targets/stm"}
	e.Syscfg = func() (*BuildInfo, error) {
		return &BuildInfo{Settings: []*Setting{{Name: downloaderSetting,
			Value:   "openocd",
			Choices: []string{"jlink", "openocd", "nrfutil", "stflash"}}}}, nil
	}
	names := []string{}
	for _, o := range downloaderField(e).OptionsFn() {
		names = append(names, o.Name)
	}
	if !reflect.DeepEqual(names, []string{"nrfutil", "jlink", "openocd"}) {
		t.Errorf("choices = %v", names)
	}
}

func TestParseBuildIssues(t *testing.T) {
	out := `Warning: Following external repos are modified or missing, which might be causing build errors:
[mbedtls]
Compiling repos/apache-mynewt-nimble/apps/btshell/src/main.c
Error: repos/apache-mynewt-nimble/apps/btshell/src/main.c: In function 'main':
repos/apache-mynewt-nimble/apps/btshell/src/main.c:120:5: error: 'foo' undeclared (first use in this function)
repos/apache-mynewt-nimble/apps/btshell/src/main.c:120:5: note: each undeclared identifier is reported only once
apps/blinky/src/main.c:7:10: warning: unused variable 'x' [-Wunused-variable]
apps/blinky/src/main.c:7:10: warning: unused variable 'x' [-Wunused-variable]
/usr/lib/gcc/arm-none-eabi/14/../../../../arm-none-eabi/bin/ld: btshell.elf section ` + "`.text' will not fit in region `FLASH'" + `
repos/x/src/a.c:33:(.text.f+0x6): undefined reference to ` + "`bar'" + `
/usr/bin/arm-none-eabi-ld: warning: mynewt.elf has a LOAD segment with RWX permissions
Error: Syscfg restriction violations detected`

	got := parseBuildIssues(out)
	want := []BuildIssue{
		{Severity: "warning", Msg: "Following external repos are modified " +
			"or missing, which might be causing build errors: mbedtls"},
		{File: "repos/apache-mynewt-nimble/apps/btshell/src/main.c", Line: 120,
			Col: 5, Severity: "error",
			Msg: "'foo' undeclared (first use in this function)",
			Notes: []string{"repos/apache-mynewt-nimble/apps/btshell/src/main.c:120: " +
				"each undeclared identifier is reported only once"}},
		{File: "apps/blinky/src/main.c", Line: 7, Col: 10, Severity: "warning",
			Msg: "unused variable 'x' [-Wunused-variable]"},
		{Severity: "error", Msg: "linker: btshell.elf section `.text' will " +
			"not fit in region `FLASH'"},
		{File: "repos/x/src/a.c", Line: 33, Severity: "error",
			Msg: "undefined reference to `bar'"},
		{Severity: "warning", Msg: "linker: mynewt.elf has a LOAD segment " +
			"with RWX permissions"},
		{Severity: "error", Msg: "Syscfg restriction violations detected"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBuildIssues:\n%+v\nwant:\n%+v", got, want)
	}

	for _, c := range []struct {
		editor []string
		want   []string
	}{
		{[]string{"vim"}, []string{"+7", "a.c"}},
		{[]string{"code", "--wait"}, []string{"--wait", "-g", "a.c:7"}},
		{[]string{"/usr/bin/hx"}, []string{"a.c:7"}},
	} {
		if got := editorArgs(c.editor, "a.c", 7); !reflect.DeepEqual(got, c.want) {
			t.Errorf("editorArgs(%v) = %v", c.editor, got)
		}
	}
}

func TestBuildErrorsPanel(t *testing.T) {
	m, _ := newTestModel(t)
	out := "apps/blinky/src/main.c:3:1: error: expected ';' before '}' token\n" +
		"apps/blinky/src/main.c:9:5: warning: unused variable 'y'\n"

	// What runWith reports once the tea program has run the build.
	m = keys(t, m, "l")
	m = update(t, m, execDoneMsg{label: "newt build targets/blinky",
		err: io.EOF, command: "build", target: "blinky", output: out})

	if m.panel == nil || !strings.Contains(m.panel.title, "1 error, 1 warning") {
		t.Fatalf("expected the errors panel, got %+v (%s)", m.panel, m.status)
	}
	if !strings.Contains(paneText(m), "open the file at this line") {
		t.Errorf("help pane:\n%s", paneText(m))
	}
	if r := m.panelRow(); r != 0 {
		t.Fatalf("cursor row = %d", r)
	}

	// h closes it; E reopens it from the target.
	m = keys(t, m, "h")
	if m.panel != nil {
		t.Fatalf("h should close the panel")
	}
	m = keys(t, m, "E")
	if m.panel == nil {
		t.Fatalf("E should reopen the errors")
	}
	m = keys(t, m, "h")

	// A successful build with warnings only reports them.
	m = update(t, m, execDoneMsg{label: "newt build targets/blinky",
		command: "build", target: "blinky",
		output: "apps/blinky/src/main.c:9:5: warning: unused variable 'y'\n"})
	if m.panel != nil || !strings.Contains(m.status, "1 warning (E to see them)") {
		t.Errorf("status = %q", m.status)
	}
}

func TestSizeAndWhy(t *testing.T) {
	out := `Size of Application Image: app
   BOOT   FLASH     RAM STACK_RAM 
      0      28       0       0 *fill*
      0    5900     120       0 @mcuboot_boot_bootutil.a
      0    1024       0       0 @apache-mynewt-core_crypto_mbedtls.a
      0      72       0       0 nrf52_boot-sysinit-app.a

objsize
   text	   data	    bss	    dec	    hex	filename
  12980	    580	   6704	  20264	   4f28	/p/bin/targets/nrf52_boot/app/@mcuboot/boot/mynewt/mynewt.elf
`
	size := parseSize(out)
	if size.Flash != 13560 || size.RAM != 7284 || len(size.Pkgs) != 4 {
		t.Fatalf("parseSize = %+v", size)
	}

	info := &BuildInfo{
		Roots: []string{"targets/nrf52_boot", "@mcuboot/boot/mynewt"},
		Revdeps: map[string][]DepReason{
			"@mcuboot/boot/mynewt":   nil,
			"@mcuboot/boot/bootutil": {{"@mcuboot/boot/mynewt", "provides API bootloader"}},
			"@apache-mynewt-core/crypto/mbedtls": {
				{"@mcuboot/boot/bootutil", "if BOOTUTIL_USE_MBED_TLS"}},
		},
	}
	size.matchPackages(info)
	names := []string{}
	for _, p := range size.Pkgs {
		names = append(names, p.displayName())
	}
	want := []string{"(alignment padding)", "boot/bootutil", "crypto/mbedtls",
		"(generated sysinit)"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("names = %v", names)
	}

	chain := info.whyIncluded("@apache-mynewt-core/crypto/mbedtls")
	wantChain := []DepReason{
		{"@mcuboot/boot/mynewt", "provides API bootloader"},
		{"@mcuboot/boot/bootutil", "if BOOTUTIL_USE_MBED_TLS"},
		{"@apache-mynewt-core/crypto/mbedtls", ""},
	}
	if !reflect.DeepEqual(chain, wantChain) {
		t.Errorf("whyIncluded = %+v", chain)
	}
}

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

type interactiveRecorder struct {
	fakeCmd
	interactive *bool
}

func (r interactiveRecorder) setInteractive() { *r.interactive = true }

func TestInteractiveCommands(t *testing.T) {
	// A copied output is not a terminal; gdb needs the terminal itself.
	term := os.Stdout
	p := newtExec([][]string{{"debug", "t"}}, nil).(*pausedCmd)
	p.setInteractive()
	p.SetStdout(term)
	p.SetStderr(term)
	if p.cmds[0].Stdout != term || p.cmds[0].Stderr != term {
		t.Errorf("an interactive command should get the terminal")
	}
	p = newtExec([][]string{{"build", "t"}}, nil).(*pausedCmd)
	p.SetStdout(term)
	if p.cmds[0].Stdout == term {
		t.Errorf("build output should be copied for finding errors")
	}

	// Debug and run are interactive, build is not.
	m, _ := newTestModel(t)
	for _, c := range []struct {
		a    Action
		want bool
	}{
		{Action{Args: []string{"debug", "t"}, Interactive: true}, true},
		{Action{Args: []string{"build", "t"}}, false},
	} {
		got := false
		m.newtCmd = func(steps [][]string, env []string) tea.ExecCommand {
			return interactiveRecorder{interactive: &got}
		}
		m.runWith(c.a, nil)
		if got != c.want {
			t.Errorf("%v: interactive = %v", c.a.Args, got)
		}
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

func TestJobOutputPane(t *testing.T) {
	m, _ := newTestModel(t)
	m = keys(t, m, "l")

	written := make(chan struct{})
	m.runSteps = func(ctx context.Context, steps [][]string, env []string,
		out io.Writer) error {
		io.WriteString(out, "$ newt build targets/blinky\nCompiling main.c\n")
		io.WriteString(out, "progress 10%\rprogress 50%")
		close(written)
		<-ctx.Done()
		return io.ErrUnexpectedEOF
	}

	full := m.listHeight()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(Model)
	<-written
	if !m.jobRunning() || m.listHeight() >= full {
		t.Fatalf("expected a running job with its pane")
	}
	dock := ansi.Strip(strings.Join(m.dockView(m.width, m.dockHeight()), "\n"))
	for _, want := range []string{"newt build targets/blinky", "Compiling main.c",
		"progress 50%", "running", "Ctrl-C stops it"} {
		if !strings.Contains(dock, want) {
			t.Errorf("pane missing %q:\n%s", want, dock)
		}
	}
	if strings.Contains(dock, "progress 10%") {
		t.Errorf("a carriage return should replace the line:\n%s", dock)
	}
	if !strings.Contains(paneText(m), "stop newt build") {
		t.Errorf("help:\n%s", paneText(m))
	}

	// The browser stays usable, but another job has to wait.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(Model)
	if !strings.Contains(m.status, "wait for newt build") {
		t.Errorf("status = %q", m.status)
	}

	// q asks first while a job runs.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = next.(Model)
	if m.prompt == nil || !strings.Contains(m.prompt.label, "and quit?") {
		t.Fatalf("q should ask, prompt %+v", m.prompt)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)

	// Ctrl-C stops the job instead of quitting.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(Model)
	if cmd != nil {
		t.Fatalf("Ctrl-C should not quit while a job runs")
	}
	m = settle(t, m)
	if !m.statusErr || !strings.Contains(ansi.Strip(strings.Join(m.dockView(m.width, m.dockHeight()),
		"\n")), "stopped") {
		t.Errorf("status %q", m.status)
	}

	// Esc hides the finished output, O shows it again.
	m = keys(t, m, "esc")
	if m.dockHeight() != 0 || len(m.bottomView()) != 0 {
		t.Errorf("Esc should hide the pane")
	}
	m = keys(t, m, "O")
	if m.dockHeight() == 0 {
		t.Errorf("O should show the pane")
	}
}
