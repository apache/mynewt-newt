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
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"mynewt.apache.org/newt/util"
)

// "newt load" runs the BSP's download script.  For most BSPs that is
// apache-mynewt-core's hw/scripts/download.sh, which flashes with the tool
// named by the MYNEWT_DOWNLOADER syscfg setting.  The BSP picks a default,
// and a target can override it.

const downloaderSetting = "MYNEWT_DOWNLOADER"

// Flash tools offered in the browser, in this order.  apache-mynewt-core
// supports more (openocd, pyocd, ST and Renesas tools, ...), but these are
// the ones for Nordic boards.
var flashTools = []string{"nrfutil", "nrfjprog", "jlink"}

// Programs run by download.sh for each tool.
var downloaderPrograms = map[string]string{
	"jlink":                "JLinkExe",
	"stflash":              "st-flash",
	"stm32_programmer_cli": "STM32_Programmer_CLI",
	"openocd":              "openocd",
	"pyocd":                "pyocd",
	"nrfjprog":             "nrfjprog",
	"nrfutil":              "nrfutil",
	"ezflashcli":           "ezFlashCLI",
}

// Settings that configure each tool.
var downloaderSettings = map[string][]string{
	"jlink":          {"JLINK_TARGET", "MYNEWT_DEBUGGER_SN"},
	"openocd":        {"MYNEWT_DOWNLOADER_OPENOCD_CFG", "MYNEWT_DOWNLOADER_OPENOCD_BOARD", "MYNEWT_DOWNLOADER_OPENOCD_INTERFACE"},
	"pyocd":          {"PYOCD_TARGET", "MYNEWT_DEBUGGER_SN"},
	"nrfjprog":       {"NRFJPROG_COPROCESSOR", "MYNEWT_DEBUGGER_SN"},
	"nrfutil":        {"NRFUTIL_TRAITS", "NRFUTIL_DFU_MODE", "NRFUTIL_DFU_SN", "MYNEWT_DEBUGGER_SN"},
	"bsp_downloader": {"MYNEWT_DOWNLOADER_SCRIPT"},
}

// lookPath finds a program; replaced in tests.
var lookPath = exec.LookPath

func downloaderProgram(tool string) string {
	if tool == "jlink" && runtime.GOOS == "windows" {
		return "JLink.exe"
	}
	return downloaderPrograms[tool]
}

// toolInstalled tells whether the program of a tool is in PATH; known is
// false for tools that are not a single program, e.g. a BSP script.
func toolInstalled(tool string) (installed bool, known bool) {
	prog := downloaderProgram(tool)
	if prog == "" {
		return false, false
	}
	_, err := lookPath(prog)
	return err == nil, true
}

// How each tool tells its version, and where the version is in what it
// prints.  "JLinkExe -version" prints its banner and stops at the unknown
// option, before it looks for a probe.
var toolVersionArgs = map[string][]string{
	"nrfutil":  {"--version"},
	"nrfjprog": {"--version"},
	"jlink":    {"-NoGui", "1", "-version"},
}

var toolVersionRe = map[string]*regexp.Regexp{
	"nrfutil":  regexp.MustCompile(`nrfutil (\S+)`),
	"nrfjprog": regexp.MustCompile(`nrfjprog version: (\S+)`),
	"jlink":    regexp.MustCompile(`J-Link Commander V(\S+)`),
}

// Versions by program path, read once.
var toolVersions sync.Map

// toolVersion returns the version of an installed tool, or "" when it
// cannot tell.
func toolVersion(tool string) string {
	args, re := toolVersionArgs[tool], toolVersionRe[tool]
	if re == nil {
		return ""
	}
	path, err := lookPath(downloaderProgram(tool))
	if err != nil {
		return ""
	}
	if v, ok := toolVersions.Load(path); ok {
		return v.(string)
	}
	out, _ := runTool("", path, args...)
	v := ""
	if m := re.FindSubmatch(out); m != nil {
		v = string(m[1])
	}
	toolVersions.Store(path, v)
	return v
}

func toolHint(tool string) string {
	installed, known := toolInstalled(tool)
	switch {
	case !known:
		return ""
	case !installed:
		return "not installed"
	case toolVersion(tool) != "":
		return toolVersion(tool)
	default:
		return "installed"
	}
}

// downloaderField is the target's "flash" setting.
func downloaderField(e *Entry) *Field {
	var setting *Setting
	var info *BuildInfo

	f := &Field{
		Label: "flash",
		Var:   downloaderSetting,
		Kind:  KindValue,
		Help:  "choose the flash tool for newt load",
	}

	f.initFn = func(f *Field) {
		var err error
		info, err = e.Syscfg()
		if err != nil {
			f.Hint = "pick an app and a bsp first"
			return
		}
		setting = findSetting(info, downloaderSetting)
		if setting == nil {
			f.Hint = "set by the bsp's own script"
			return
		}
		f.Value = setting.Value
		if f.Value != "" {
			f.Hint = toolHint(f.Value)
		}
	}

	f.OptionsFn = func() []*Entry {
		f.ensure()
		if setting == nil {
			return nil
		}

		opts := []*Entry{}
		if setting.Overridden && len(setting.History) >= 2 {
			// Undo the target's choice.
			dflt := setting.History[len(setting.History)-2]
			opts = append(opts, &Entry{Name: "bsp default",
				Hint: dflt.Value, Kind: KindValue,
				previewFn: func() []string {
					return []string{
						field("Tool", styleBold.Render(dflt.Value)),
						field("Set by", dflt.Source),
						"",
						"Removes " + downloaderSetting + " from the " +
							"target's syscfg.yml.",
					}
				}})
		}
		offered := map[string]bool{}
		for _, c := range setting.Choices {
			offered[c] = true
		}
		tools := []string{}
		for _, c := range flashTools {
			if offered[c] {
				tools = append(tools, c)
			}
		}
		// Keep a tool from outside the list visible while it is in use.
		if v := setting.Value; v != "" && offered[v] && !contains(tools, v) {
			tools = append(tools, v)
		}

		for _, c := range tools {
			c := c
			opts = append(opts, &Entry{Name: c, FullName: c, Kind: KindValue,
				Hint:      toolHint(c),
				previewFn: func() []string { return toolPreview(c, info) }})
		}
		return opts
	}

	f.SetFn = func(value string) error {
		return e.SetSyscfg(downloaderSetting, value, value == "")
	}

	return f
}

func toolPreview(tool string, info *BuildInfo) []string {
	lines := []string{field("Tool", styleBold.Render(tool))}

	prog := downloaderProgram(tool)
	installed, known := toolInstalled(tool)
	switch {
	case !known:
		lines = append(lines, field("Program", styleDim.Render(
			"set by MYNEWT_DOWNLOADER_SCRIPT")))
	case installed:
		if v := toolVersion(tool); v != "" {
			prog += " " + v
		}
		lines = append(lines, field("Program", styleOk.Render(prog+
			" (installed)")))
	default:
		lines = append(lines, field("Program", styleErr.Render(prog+
			" not found in PATH")))
	}

	if names := downloaderSettings[tool]; len(names) > 0 {
		lines = append(lines, section("Settings used by "+tool)...)
		for _, name := range names {
			val := styleDim.Render("(not set)")
			if s := findSetting(info, name); s != nil && s.Value != "" {
				val = s.Value
			}
			lines = append(lines, name+styleDim.Render(": ")+val)
		}
		lines = append(lines, "", styleDim.Render(
			"Change these in the target's syscfg settings."))
	}

	return append(lines, "", styleDim.Render(
		"Choosing a tool sets "+downloaderSetting+
			" in the target's syscfg.yml."))
}

// loadCheck stops "newt load" early, with a helpful message, if the flash
// tool is not installed.
func loadCheck(f *Field) error {
	f.ensure()
	if f.Value == "" {
		return nil
	}
	installed, known := toolInstalled(f.Value)
	if !known || installed {
		return nil
	}

	others := []string{}
	if f.OptionsFn != nil {
		for _, o := range f.OptionsFn() {
			if ok, _ := toolInstalled(o.FullName); ok {
				others = append(others, o.FullName)
			}
		}
	}

	msg := fmt.Sprintf("flash tool %s is not installed (%s not found)",
		f.Value, downloaderProgram(f.Value))
	if len(others) > 0 {
		msg += "; installed: " + strings.Join(others, ", ") +
			" — pick one under settings › flash"
	}
	return util.NewNewtError(msg)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
