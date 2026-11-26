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
)

// By default the colors are the basic 16-color ANSI palette, so that the
// browser follows the user's terminal theme, the same way ranger does.

// Theme is the browser's colors.  An empty color is the terminal's
// default text color.
type Theme struct {
	Target, App, Bsp, Lib, Unittest lipgloss.TerminalColor
	Text, Dim, Err, Ok              lipgloss.TerminalColor
	// The project name in the title bar, and the keys in the help pane.
	Title, Key lipgloss.TerminalColor
	// Lines between and around the columns; the boards pane and the output
	// and console panes have their own.
	Border, BoardsBorder, DockBorder lipgloss.TerminalColor
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
