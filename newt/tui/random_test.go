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
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// randomKeys are the keys the random test presses: every key the browser
// uses, and a few it does not.
var randomKeys = []tea.KeyMsg{
	{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeySpace},
	{Type: tea.KeyUp}, {Type: tea.KeyDown}, {Type: tea.KeyLeft},
	{Type: tea.KeyRight}, {Type: tea.KeyHome}, {Type: tea.KeyEnd},
	{Type: tea.KeyPgUp}, {Type: tea.KeyPgDown}, {Type: tea.KeyTab},
	{Type: tea.KeyShiftTab}, {Type: tea.KeyBackspace},
	{Type: tea.KeyCtrlD}, {Type: tea.KeyCtrlU}, {Type: tea.KeyCtrlF},
	{Type: tea.KeyCtrlB}, {Type: tea.KeyCtrlC}, {Type: tea.KeyCtrlCloseBracket},
}

const randomRunes = "hjklggGHLzz/.?[]=abcdefinoprstuvxyBCDEFMORVX0123456789 "

// randomWorld is a fake project with a target that has settings, syscfg
// and a bootloader, and fake boards, tools and files.
func randomWorld(t *testing.T) Model {
	values := map[string]string{"target.app": "@core/apps/blinky",
		"target.bsp": "@core/hw/bsp/nrf52"}
	saved := map[string]string{"BLE_FOO": "4"}
	images := t.TempDir()
	os.WriteFile(images+"/blinky-1.0.0.0.img", []byte("img"), 0644)
	yml := t.TempDir() + "/project.yml"
	os.WriteFile(yml, []byte(testProjectYml), 0644)
	repo := &RepoInfo{Name: "apache-mynewt-core", Path: images,
		RelPath: "repos/apache-mynewt-core", ProjectYml: yml,
		Origin: repoOrigin{Type: "github", User: "apache",
			Repo: "mynewt-core"}}

	load := func() ([]*Category, error) {
		cats := testCats()
		cats = append(cats,
			&Category{Title: "BSPs", Kind: KindBsp, Entries: []*Entry{
				{Name: "hw/bsp/nrf52", FullName: "@core/hw/bsp/nrf52",
					Kind: KindBsp, Repo: "core", Foreign: true},
				{Name: "hw/bsp/native", FullName: "@core/hw/bsp/native",
					Kind: KindBsp, Repo: "core", Foreign: true}}},
			&Category{Title: "Unit tests", Kind: KindUnittest},
			&Category{Title: "Repos", Kind: KindRepo,
				Entries: []*Entry{repoEntry(repo)}})
		for _, e := range cats[1].Entries {
			e.Repo = strings.TrimPrefix(strings.SplitN(e.FullName, "/",
				2)[0], "@")
			e.RepoDir, e.Dir = images, images+"/"+e.Name
		}

		e := cats[0].Entries[0]
		e.Fields = []*Field{
			{Label: "app", Var: "target.app", Value: values["target.app"],
				Kind: KindApp},
			{Label: "bsp", Var: "target.bsp", Value: values["target.bsp"],
				Kind: KindBsp},
			{Label: "profile", Var: "target.build_profile", Kind: KindValue,
				valuesFn: func() []string { return []string{"debug", "optimized"} }},
			{Label: "syscfg", Kind: KindSyscfg,
				Preview: func() []string { return []string{"overrides"} }},
		}
		e.Set = func(variable string, value string) error {
			values[variable] = value
			return nil
		}
		e.Syscfg = func() (*BuildInfo, error) {
			mk := func(name, def string, choices ...string) *Setting {
				s := &Setting{Name: name, Value: def, Choices: choices,
					History: []SettingPoint{{"default", def}}}
				if v, ok := saved[name]; ok {
					s.Value, s.Overridden = v, true
				}
				return s
			}
			return &BuildInfo{Revdeps: map[string][]DepReason{},
				Settings: []*Setting{mk("BLE_FOO", "1"), mk("BLE_FLAG", "0"),
					mk("CONSOLE_UART", "1"), mk("CONSOLE_UART_BAUD", "115200"),
					mk("CONSOLE_HISTORY", "none", "log", "none", "ram"),
					{Name: "PKG_CONST", Value: "1", Const: true}}}, nil
		}
		e.SetSyscfg = func(name string, value string, remove bool) error {
			if remove {
				delete(saved, name)
			} else {
				saved[name] = value
			}
			return nil
		}
		e.NeedsBoot, e.Boot, e.BootName = true, "targets/boot", "boot"
		e.Image, e.ImagesDir = "1.0.0.0", images
		e.ImagePath = images + "/blinky.img"
		e.Actions = append(e.Actions,
			Action{Key: "L", Label: "load", Args: []string{"load",
				e.FullName}, PickDevice: true, ChooseImage: true},
			Action{Key: "C", Label: "console", Args: []string{"console"},
				PickDevice: true, Console: true},
			Action{Key: "c", Label: "clean", Args: []string{"clean",
				e.FullName}, Confirm: "Delete?"})
		cats[0].Entries[1].Syscfg = func() (*BuildInfo, error) {
			return &BuildInfo{Settings: []*Setting{{Name: "BLE_FOO",
				Value: "8"}, {Name: "BOOT_ONLY", Value: "1"}}}, nil
		}
		e.Links = append(e.Links, Link{"B", "bootloader", KindTarget,
			"targets/boot"})
		cats[0].Entries[1].IsBoot = true
		cats[0].Entries[1].BootFor = []string{"blinky"}
		return cats, nil
	}

	m := NewModel("proj", load)
	m.newtCmd = func(steps [][]string, env []string) tea.ExecCommand {
		return fakeCmd{}
	}
	m.newtRun = func(args []string) (string, error) {
		return "", errors.New("Error: fake newt")
	}
	m.runSteps = func(ctx context.Context, steps [][]string, env []string,
		out io.Writer) error {
		fmt.Fprintln(out, "main.c:1:1: error: fake")
		return errors.New("exit status 1")
	}
	m.findDevices = func() ([]Device, error) {
		return []Device{{Serial: "1", Board: "PCA10056",
			Ports: []string{"/dev/fake0", "/dev/fake1"}, Source: "J-Link"}}, nil
	}
	m.readChip = func(serial string) (*ChipInfo, error) {
		return nil, errors.New("not powered")
	}
	m.openPort = func(path string, baud int) (io.ReadWriteCloser, error) {
		return newFakePort(), nil
	}
	m.editor = "vim"
	return m
}

// fakePort is a serial port that echoes what is written to it.
type fakePort struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func newFakePort() *fakePort {
	r, w := io.Pipe()
	return &fakePort{r, w}
}

func (p *fakePort) Read(b []byte) (int, error) { return p.r.Read(b) }

func (p *fakePort) Write(b []byte) (int, error) {
	go p.w.Write(append([]byte{}, b...))
	return len(b), nil
}

func (p *fakePort) Close() error {
	p.w.Close()
	return p.r.Close()
}

// randomStarts are where the random runs begin, so that they also reach
// the deeper screens: the target's settings, syscfg, a choice list, the
// board list, the console and the guide.
var randomStarts = [][]string{{}, {"l"}, {"l", "l"},
	{"l", "l", "j", "j", "j", "l"}, {"l", "l", "l"}, {"l", "L", "j"},
	{"l", "C", "enter"}, {"g", "?"}, {"j", "l"}, {"G", "l", "l"},
	{"G", "l", "c"}}

// TestRandomKeys presses random keys everywhere and checks that the
// browser never panics.
func TestRandomKeys(t *testing.T) {
	defaultLookPath := lookPath
	lookPath = func(prog string) (string, error) { return "/bin/" + prog, nil }
	fakeGit(t)
	cfg := t.TempDir() + "/tui.yml"
	defaultPath := tuiConfigPath
	tuiConfigPath = func() string { return cfg }
	defer func() {
		lookPath = defaultLookPath
		tuiConfigPath = defaultPath
	}()

	// TUI_RANDOM_RUNS=n runs it longer, to look for rarer problems.
	runs, presses := 100, 300
	if n, err := strconv.Atoi(os.Getenv("TUI_RANDOM_RUNS")); err == nil {
		runs = n
	} else if testing.Short() {
		runs = 5
	}
	for seed := int64(1); seed <= int64(runs); seed++ {
		rng := rand.New(rand.NewSource(seed))
		m := randomWorld(t)
		m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
		pressed := []string{}
		for _, k := range randomStarts[rng.Intn(len(randomStarts))] {
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
			if k == "enter" {
				msg = tea.KeyMsg{Type: tea.KeyEnter}
			}
			pressed = append(pressed, k)
			next, cmd := m.Update(msg)
			m = settle(t, next.(Model))
			for _, reply := range runCmd(cmd) {
				next, _ = m.Update(reply)
				m = settle(t, next.(Model))
			}
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("seed %d: panic %v after keys %q\n%s", seed, r,
						pressed, debug.Stack())
				}
			}()
			for i := 0; i < presses; i++ {
				var msg tea.Msg
				switch n := rng.Intn(100); {
				case n < 2:
					msg = tea.WindowSizeMsg{Width: 20 + rng.Intn(140),
						Height: 3 + rng.Intn(50)}
				case n < 25:
					msg = randomKeys[rng.Intn(len(randomKeys))]
				case n < 30:
					// Several runes at once, as with key repeat.
					rs := []rune{}
					for j := 0; j < 2+rng.Intn(4); j++ {
						rs = append(rs, rune(randomRunes[rng.Intn(
							len(randomRunes))]))
					}
					msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: rs}
				default:
					msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{
						rune(randomRunes[rng.Intn(len(randomRunes))])}}
				}
				pressed = append(pressed, fmt.Sprint(msg))

				next, cmd := m.Update(msg)
				m = settle(t, next.(Model))
				for _, reply := range runCmd(cmd) {
					next, _ = m.Update(reply)
					m = settle(t, next.(Model))
				}
				_ = m.View()
			}
		}()
	}
}
