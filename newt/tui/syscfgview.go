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
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The syscfg view lists the resolved settings of the selected target, i.e.
// what "newt target config show" prints, and changes them in the target's
// syscfg.yml.

func (m *Model) visibleSettings() []*Setting {
	if m.syscfg == nil {
		return nil
	}

	settings := m.syscfg.Settings
	if m.bluetoothOnly && m.hasBluetooth() {
		settings = m.bluetoothSettings()
	}

	filter := strings.ToLower(m.setList.filter)
	out := []*Setting{}
	for _, s := range settings {
		if m.changedOnly && !s.Changed() && !s.Overridden {
			continue
		}
		if filter != "" && !strings.Contains(
			strings.ToLower(s.Name+" "+s.Description), filter) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (m *Model) setting() *Setting {
	if m.depth != depthSyscfg && m.depth != depthChoice {
		return nil
	}
	settings := m.visibleSettings()
	if c := m.setList.cursor; c >= 0 && c < len(settings) {
		return settings[c]
	}
	return nil
}

// errorSummary returns the first actual error, skipping headings such as
// "Syscfg restriction violations:".
func errorSummary(errors []string) string {
	for _, e := range errors {
		if !strings.HasSuffix(e, ":") {
			return strings.TrimSpace(strings.TrimPrefix(e, "*"))
		}
	}
	return strings.Join(errors, " ")
}

func (m *Model) openSyscfg() {
	e := m.Selected()
	if e == nil || e.Syscfg == nil {
		return
	}

	info, err := e.Syscfg()
	if err != nil {
		m.setError("syscfg: " + err.Error())
		return
	}

	m.syscfg = info
	m.setList = list{}
	m.choiceList = list{}
	m.depth = depthSyscfg

	if len(info.Errors) > 0 {
		m.setError("build would fail: " + errorSummary(info.Errors))
		return
	}
	n := 0
	for _, s := range info.Settings {
		if s.Overridden {
			n++
		}
	}
	m.setStatus(fmt.Sprintf("%d settings, %d set by this target",
		len(info.Settings), n))
}

// refreshSyscfg resolves the syscfg again after a change and puts the cursor
// back on the named setting.
func (m *Model) refreshSyscfg(name string) {
	e := m.Selected()
	if e == nil || e.Syscfg == nil {
		m.syscfg = nil
		return
	}
	info, err := e.Syscfg()
	if err != nil {
		m.setError("syscfg: " + err.Error())
		m.syscfg = nil
		return
	}
	m.syscfg = info

	for i, s := range m.visibleSettings() {
		if s.Name == name {
			m.setList.cursor = i
			m.setList.offset = i - m.listHeight()/2
			break
		}
	}
}

func (m *Model) saveSetting(name string, value string, remove bool) {
	e := m.Selected()
	if e == nil || e.SetSyscfg == nil {
		return
	}
	if err := e.SetSyscfg(name, value, remove); err != nil {
		m.setError(err.Error())
		return
	}

	if !m.reload() {
		return
	}
	m.depth = depthSyscfg
	m.refreshSyscfg(name)
	if m.syscfg == nil {
		return
	}

	related := []string{}
	for _, err := range m.syscfg.Errors {
		if strings.Contains(err, name) {
			related = append(related, err)
		}
	}

	switch {
	case len(related) > 0:
		m.setError("saved, but the build would fail: " +
			errorSummary(related))
	case remove:
		m.setStatus("removed this target's value of " + name)
		for _, s := range m.syscfg.Settings {
			if s.Name == name {
				m.setStatus(fmt.Sprintf("removed this target's value; "+
					"%s is back to %s (%s)", name, s.Value, s.SetBy()))
			}
		}
	default:
		m.setStatus(fmt.Sprintf("%s = %s, saved in %s/syscfg.yml", name,
			value, e.Name))
	}
}

// editSetting starts changing the setting under the cursor: from a list if
// it has choices, otherwise by typing the value.
func (m *Model) editSetting() tea.Cmd {
	s := m.setting()
	if s == nil || m.localTarget() == nil {
		return nil
	}
	if s.Const {
		m.setError(s.Name + " is a constant and cannot be changed")
		return nil
	}

	if len(s.Choices) > 0 {
		cur := 0
		for i, c := range s.Choices {
			if c == s.Value {
				cur = i
			}
		}
		m.choiceList = list{cursor: cur}
		m.depth = depthChoice
		return nil
	}

	name := s.Name
	m.prompt = &prompt{
		label: name + " = ",
		input: s.Value,
		title: "New value for " + name,
		help: [][2]string{
			{"type", "the new value (current value is filled in)"},
			{"Enter", "save to the target's syscfg.yml"},
			{"Esc", "cancel"},
		},
		done: func(m *Model, value string) tea.Cmd {
			m.saveSetting(name, value, false)
			return nil
		},
	}
	return nil
}

func (m *Model) chooseSetting() {
	s := m.setting()
	c := m.choiceList.cursor
	m.depth = depthSyscfg
	if s == nil || c < 0 || c >= len(s.Choices) {
		return
	}
	if s.Choices[c] == s.Value {
		m.setStatus(s.Name + " unchanged")
		return
	}
	m.saveSetting(s.Name, s.Choices[c], false)
}

// syscfgKey handles the keys of the settings list; it returns false if the
// key is not one of them.
func (m *Model) syscfgKey(k string) (tea.Cmd, bool) {
	if m.depth != depthSyscfg {
		return nil, false
	}

	switch k {
	case "B":
		if !m.hasBluetooth() {
			m.setStatus("this build does not include NimBLE")
			break
		}
		m.bluetoothOnly = !m.bluetoothOnly
		m.setList = list{}
		if m.bluetoothOnly {
			m.setStatus(fmt.Sprintf("Bluetooth essentials: %d settings",
				len(m.bluetoothSettings())))
		} else {
			m.setStatus("showing all settings")
		}

	case "o":
		m.changedOnly = !m.changedOnly
		m.setList = list{}
		if m.changedOnly {
			m.setStatus("showing only settings changed from their default")
		} else {
			m.setStatus("showing all settings")
		}

	case " ":
		s := m.setting()
		if s == nil || m.localTarget() == nil {
			break
		}
		if !s.IsFlag() || s.Const {
			m.setError("space only toggles 0/1 settings; " +
				"press Enter to change " + s.Name)
			break
		}
		value := "1"
		if s.Value == "1" {
			value = "0"
		}
		m.saveSetting(s.Name, value, false)

	case "x":
		s := m.setting()
		if s == nil || m.localTarget() == nil {
			break
		}
		if !s.Overridden {
			m.setStatus(s.Name + " is not set by this target; " +
				"nothing to remove")
			break
		}
		m.saveSetting(s.Name, "", true)

	default:
		return nil, false
	}

	return nil, true
}

var (
	styleOverride = kindStyle(KindTarget).Bold(true)
	styleChanged  = lipgloss.NewStyle()
)

func settingRows(settings []*Setting) []row {
	rows := make([]row, len(settings))
	for i, s := range settings {
		r := row{text: "  " + s.Name, hint: clip(s.Value, 18)}
		switch {
		case s.Overridden:
			r.text = "● " + s.Name
			r.style = styleOverride
		case s.Changed() && !s.Const:
			r.style = styleChanged
		default:
			r.style = styleDim
		}
		rows[i] = r
	}
	return rows
}

func choiceRows(s *Setting) []row {
	rows := make([]row, len(s.Choices))
	for i, c := range s.Choices {
		rows[i] = row{text: c, style: kindStyle(KindValue)}
		if c == s.Value {
			rows[i].style = rows[i].style.Bold(true)
			rows[i].hint = "current"
		}
	}
	return rows
}

// The Bluetooth page of the syscfg view: the NimBLE settings that matter
// most day to day, in groups.
var bleEssentials = []struct {
	group string
	names []string
}{
	{"Roles", []string{"BLE_ROLE_CENTRAL", "BLE_ROLE_PERIPHERAL",
		"BLE_ROLE_BROADCASTER", "BLE_ROLE_OBSERVER"}},
	{"Connections", []string{"BLE_MAX_CONNECTIONS", "BLE_ATT_PREFERRED_MTU",
		"BLE_GATT_MAX_PROCS", "BLE_L2CAP_COC_MAX_NUM",
		"BLE_LL_MAX_PKT_SIZE", "BLE_LL_CONN_INIT_MAX_TX_BYTES"}},
	{"Advertising", []string{"BLE_EXT_ADV", "BLE_EXT_ADV_MAX_SIZE",
		"BLE_MULTI_ADV_INSTANCES", "BLE_PERIODIC_ADV"}},
	{"Version and PHY", []string{"BLE_VERSION", "BLE_PHY_2M",
		"BLE_PHY_CODED", "BLE_LL_CFG_FEAT_LE_2M_PHY",
		"BLE_LL_CFG_FEAT_LE_CODED_PHY", "BLE_LL_TX_PWR_DBM"}},
	{"Security", []string{"BLE_SM_LEGACY", "BLE_SM_SC", "BLE_SM_BONDING",
		"BLE_SM_MITM", "BLE_SM_IO_CAP", "BLE_STORE_CONFIG_PERSIST",
		"BLE_STORE_MAX_BONDS"}},
	{"Identity", []string{"BLE_SVC_GAP_DEVICE_NAME"}},
	{"Buffers", []string{"BLE_TRANSPORT_ACL_COUNT",
		"BLE_TRANSPORT_EVT_COUNT"}},
	{"Logging", []string{"BLE_HS_LOG_LVL", "BLE_HS_DEBUG"}},
}

// bleGroup returns the Bluetooth page group of a setting, if it has one.
func bleGroup(name string) string {
	for _, g := range bleEssentials {
		for _, n := range g.names {
			if n == name {
				return g.group
			}
		}
	}
	return ""
}

// hasBluetooth tells whether the build includes NimBLE.
func (m *Model) hasBluetooth() bool {
	return m.syscfg != nil && findSetting(m.syscfg, "BLE_ROLE_PERIPHERAL") != nil
}

// bluetoothSettings returns the settings of the Bluetooth page, in order.
func (m *Model) bluetoothSettings() []*Setting {
	out := []*Setting{}
	for _, g := range bleEssentials {
		for _, n := range g.names {
			if s := findSetting(m.syscfg, n); s != nil {
				out = append(out, s)
			}
		}
	}
	return out
}
