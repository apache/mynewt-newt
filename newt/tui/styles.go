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
	"github.com/charmbracelet/lipgloss"

	"mynewt.apache.org/newt/util"
)

// By default the colors are the basic 16-color ANSI palette, so that the
// browser follows the user's terminal theme, the same way ranger does.
// "theme: hal" in tui.yml uses the hal theme's colors instead.

// Theme is the browser's colors.  An empty color is the terminal's
// default text color.
type Theme struct {
	Target, App, Bsp, Lib, Unittest lipgloss.TerminalColor
	Text, Dim, Err, Ok              lipgloss.TerminalColor
	// The project name in the title bar, and the keys in the help pane.
	Title, Key lipgloss.TerminalColor
	// Lines between and around the columns; the git pane, the boards pane
	// and the output and console panes have their own.
	Border, GitBorder, BoardsBorder, DockBorder lipgloss.TerminalColor
	// The selected row; without a background it is shown reversed.
	SelBg, SelFg lipgloss.TerminalColor
}

var terminalTheme = Theme{
	Target:       lipgloss.Color("2"), // green
	App:          lipgloss.Color("6"), // cyan
	Bsp:          lipgloss.Color("5"), // magenta
	Lib:          lipgloss.Color("4"), // blue
	Unittest:     lipgloss.Color("3"), // yellow
	Text:         lipgloss.Color("7"),
	Dim:          lipgloss.Color("8"),
	Err:          lipgloss.Color("1"),
	Ok:           lipgloss.Color("2"),
	Title:        lipgloss.NoColor{},
	Key:          lipgloss.NoColor{},
	Border:       lipgloss.Color("8"),
	GitBorder:    lipgloss.Color("8"),
	BoardsBorder: lipgloss.Color("8"),
	DockBorder:   lipgloss.Color("8"),
	SelBg:        lipgloss.NoColor{},
	SelFg:        lipgloss.NoColor{},
}

var (
	theme = terminalTheme

	styleDim       lipgloss.Style
	styleBold      lipgloss.Style
	styleLabel     lipgloss.Style
	styleHeading   lipgloss.Style
	styleErr       lipgloss.Style
	styleOk        lipgloss.Style
	stylePath      lipgloss.Style
	styleTitle     lipgloss.Style
	styleKey       lipgloss.Style
	styleSel       lipgloss.Style
	styleSelDim    lipgloss.Style
	styleSep       lipgloss.Style
	styleGitSep    lipgloss.Style
	styleBoardsSep lipgloss.Style
	styleDockSep   lipgloss.Style
	styleGroup     lipgloss.Style
	styleOverride  lipgloss.Style
	styleChanged   lipgloss.Style
)

func init() {
	setTheme(terminalTheme)
}

// setTheme makes the styles use a theme's colors.
func setTheme(t Theme) {
	theme = t
	fg := func(c lipgloss.TerminalColor) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(c)
	}
	styleDim = fg(t.Dim)
	styleBold = lipgloss.NewStyle().Bold(true)
	styleLabel = fg(t.Dim).Width(10)
	styleHeading = fg(t.Title).Bold(true).Underline(true)
	styleErr = fg(t.Err).Bold(true)
	styleOk = fg(t.Ok)
	stylePath = fg(t.App).Bold(true)
	styleTitle = fg(t.Title).Bold(true)
	styleKey = fg(t.Key).Bold(true)
	styleSep = fg(t.Border)
	styleGitSep = fg(t.GitBorder)
	styleBoardsSep = fg(t.BoardsBorder)
	styleDockSep = fg(t.DockBorder)
	styleGroup = fg(t.Dim).Bold(true)
	if _, none := t.SelBg.(lipgloss.NoColor); none {
		styleSel = lipgloss.NewStyle().Reverse(true)
		styleSelDim = lipgloss.NewStyle().Reverse(true).Foreground(t.Dim)
	} else {
		styleSel = lipgloss.NewStyle().Background(t.SelBg).Foreground(t.SelFg).
			Bold(true)
		styleSelDim = lipgloss.NewStyle().Background(t.SelBg).Foreground(t.Dim)
	}
	styleOverride = kindStyle(KindTarget).Bold(true)
	styleChanged = fg(t.Text)
}

func kindColor(k Kind) lipgloss.TerminalColor {
	switch k {
	case KindTarget:
		return theme.Target
	case KindApp:
		return theme.App
	case KindBsp:
		return theme.Bsp
	case KindLib:
		return theme.Lib
	case KindUnittest:
		return theme.Unittest
	default:
		return theme.Text
	}
}

func kindStyle(k Kind) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kindColor(k))
}

// halTheme is a dark theme of muted greens, golds and blues.
var halTheme = Theme{
	Target:       lipgloss.Color("#87af87"),
	App:          lipgloss.Color("#87afaf"),
	Bsp:          lipgloss.Color("#af8787"),
	Lib:          lipgloss.Color("#87afd7"),
	Unittest:     lipgloss.Color("#afaf87"),
	Text:         lipgloss.Color("#a8a8a8"),
	Dim:          lipgloss.Color("#6c6c6c"),
	Err:          lipgloss.Color("#d75f5f"),
	Ok:           lipgloss.Color("#87af87"),
	Title:        lipgloss.Color("#d7af87"),
	Key:          lipgloss.Color("#87af87"),
	Border:       lipgloss.Color("#5f87af"),
	GitBorder:    lipgloss.Color("#afaf87"),
	BoardsBorder: lipgloss.Color("#af8787"),
	DockBorder:   lipgloss.Color("#6c6c6c"),
	SelBg:        lipgloss.Color("#2a332a"),
	SelFg:        lipgloss.Color("#d7af5f"),
}

// themes are the themes "theme" in tui.yml can name.
var themes = map[string]Theme{
	"terminal": terminalTheme,
	"hal":      halTheme,
}

// loadTheme returns the theme a tui.yml setting names; empty is the
// terminal's colors.
func loadTheme(setting string) (Theme, error) {
	if setting == "" {
		return terminalTheme, nil
	}
	if t, ok := themes[setting]; ok {
		return t, nil
	}
	return terminalTheme, util.FmtNewtError(
		"unknown theme %q in tui.yml; the themes are terminal and hal",
		setting)
}
