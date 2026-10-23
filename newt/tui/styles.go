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

// Colors use the basic 16-color ANSI palette so that the browser follows the
// user's terminal theme, the same way ranger does.
var (
	colorTarget   = lipgloss.Color("2") // green
	colorApp      = lipgloss.Color("6") // cyan
	colorBsp      = lipgloss.Color("5") // magenta
	colorLib      = lipgloss.Color("4") // blue
	colorUnittest = lipgloss.Color("3") // yellow
	colorOther    = lipgloss.Color("7")
	colorDim      = lipgloss.Color("8")
	colorErr      = lipgloss.Color("1")

	styleDim     = lipgloss.NewStyle().Foreground(colorDim)
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleLabel   = lipgloss.NewStyle().Foreground(colorDim).Width(10)
	styleHeading = lipgloss.NewStyle().Bold(true).Underline(true)
	styleErr     = lipgloss.NewStyle().Foreground(colorErr).Bold(true)
	styleOk      = lipgloss.NewStyle().Foreground(colorTarget)
	stylePath    = lipgloss.NewStyle().Foreground(colorApp).Bold(true)
	styleKey     = lipgloss.NewStyle().Bold(true)
	styleSel     = lipgloss.NewStyle().Reverse(true)
	styleSelDim  = lipgloss.NewStyle().Reverse(true).Foreground(colorDim)
	styleSep     = lipgloss.NewStyle().Foreground(colorDim)
	styleGroup   = lipgloss.NewStyle().Foreground(colorDim).Bold(true)
)

func kindColor(k Kind) lipgloss.Color {
	switch k {
	case KindTarget:
		return colorTarget
	case KindApp:
		return colorApp
	case KindBsp:
		return colorBsp
	case KindLib:
		return colorLib
	case KindUnittest:
		return colorUnittest
	default:
		return colorOther
	}
}

func kindStyle(k Kind) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kindColor(k))
}
