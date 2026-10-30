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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"mynewt.apache.org/newt/util"
)

// Device is a debug probe / board connected over USB.
type Device struct {
	// Serial number as reported by the probe, e.g. "001050012345".
	Serial string
	// Human friendly names, any of which may be empty.
	Product string
	Board   string
	Chip    string
	Ports   []string
	// Tool that found the device.
	Source string
}

// Title is the device's name in the board picker.
func (d Device) Title() string {
	parts := []string{}
	for _, p := range []string{d.Board, d.Product} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "device")
	}
	return strings.Join(parts, " · ")
}

// The flash and debug scripts of apache-mynewt-core pick a probe by the
// JLINK_SN environment variable, falling back to the MYNEWT_DEBUGGER_SN
// syscfg setting.
const (
	deviceEnv     = "JLINK_SN"
	deviceSetting = "MYNEWT_DEBUGGER_SN"
)

// deviceSN formats a serial number for the given flash tool.  J-Link and
// nrfjprog read it as a number, while nrfutil, openocd and pyocd match the
// USB serial string with its leading zeros.
func deviceSN(tool string, serial string) string {
	switch tool {
	case "jlink", "nrfjprog":
		if s := strings.TrimLeft(serial, "0"); s != "" {
			return s
		}
	}
	return serial
}

// sameDevice compares serial numbers regardless of leading zeros.
func sameDevice(a string, b string) bool {
	return a != "" && b != "" &&
		strings.TrimLeft(a, "0") == strings.TrimLeft(b, "0")
}

// runTool runs a device discovery program; replaced in tests.
var runTool = func(stdin string, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.CombinedOutput()
}

// findDevices lists connected probes with nrfutil, or with J-Link's own
// tool when nrfutil is not installed.
func findDevices() ([]Device, error) {
	if _, err := lookPath("nrfutil"); err == nil {
		out, err := runTool("", "nrfutil", "device", "list", "--json")
		if err != nil {
			return nil, util.FmtNewtError("nrfutil device list: %s",
				errorLine(string(out), err))
		}
		return parseNrfutilDevices(out), nil
	}

	jlink := downloaderProgram("jlink")
	if _, err := lookPath(jlink); err == nil {
		out, err := runTool("ShowEmuList\nexit\n", jlink, "-NoGui", "1")
		if err != nil && len(out) == 0 {
			return nil, util.FmtNewtError("%s: %s", jlink, err.Error())
		}
		return parseJLinkDevices(out), nil
	}

	return nil, util.NewNewtError(
		"cannot look for boards: neither nrfutil nor J-Link is installed")
}

func str(v interface{}, path ...string) string {
	for _, p := range path {
		m, ok := v.(map[string]interface{})
		if !ok {
			return ""
		}
		v = m[p]
	}
	s, _ := v.(string)
	return s
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseNrfutilDevices reads the device list from the JSON lines printed by
// "nrfutil device list --json".  Fields are looked up loosely, since their
// layout differs between nrfutil versions and device types.
func parseNrfutilDevices(out []byte) []Device {
	devices := []Device{}
	seen := map[string]bool{}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		var msg struct {
			Type string `json:"type"`
			Data struct {
				Devices []map[string]interface{} `json:"devices"`
			} `json:"data"`
		}
		if json.Unmarshal(scanner.Bytes(), &msg) != nil || msg.Type != "info" {
			continue
		}

		for _, d := range msg.Data.Devices {
			dev := Device{
				Serial: first(str(d, "serialNumber"), str(d, "id"),
					str(d, "usb", "serialNumber"),
					str(d, "usb", "device", "serialNumber")),
				Product: first(str(d, "usb", "device", "product"),
					str(d, "usb", "product")),
				Board: first(str(d, "devkit", "boardVersion"),
					str(d, "jlink", "boardVersion")),
				Chip: first(str(d, "jlink", "deviceVersion"),
					str(d, "devkit", "deviceFamily"),
					str(d, "jlink", "deviceFamily")),
				Source: "nrfutil",
			}
			if ports, ok := d["serialPorts"].([]interface{}); ok {
				for _, p := range ports {
					if name := first(str(p, "comName"), str(p, "path")); name != "" {
						dev.Ports = append(dev.Ports, name)
					}
				}
			}
			if dev.Serial == "" || seen[dev.Serial] {
				continue
			}
			seen[dev.Serial] = true
			devices = append(devices, dev)
		}
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Serial < devices[j].Serial
	})
	return devices
}

var jlinkEmuRe = regexp.MustCompile(
	`J-Link\[\d+\]:.*Serial number: (\d+)(?:, ProductName: (.*))?`)

// parseJLinkDevices reads the output of J-Link Commander's ShowEmuList.
func parseJLinkDevices(out []byte) []Device {
	devices := []Device{}
	for _, line := range strings.Split(string(out), "\n") {
		m := jlinkEmuRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		devices = append(devices, Device{Serial: m[1],
			Product: strings.TrimSpace(m[2]), Source: "J-Link"})
	}
	return devices
}

// devicePreview describes a device and the command that will use it.
func devicePreview(d Device, command string) []string {
	lines := []string{styleBold.Render(d.Title()), ""}
	lines = append(lines, field("Serial", d.Serial))
	if d.Board != "" {
		lines = append(lines, field("Board", d.Board))
	}
	if d.Chip != "" {
		lines = append(lines, field("Chip", d.Chip))
	}
	if d.Product != "" {
		lines = append(lines, field("Probe", d.Product))
	}
	if len(d.Ports) > 0 {
		lines = append(lines, field("Ports", strings.Join(d.Ports, " ")))
	}
	lines = append(lines, field("Found by", d.Source))
	lines = append(lines, section("Runs")...)
	return append(lines, command)
}

// ChipInfo describes the chip on a board, read over the debug port.
type ChipInfo struct {
	// From e.g. "NRF52840_xxAA_REV2".
	Version  string
	Part     string
	Variant  string
	Revision string

	Family        string
	Board         string
	ProbeFirmware string
	Flash         int64
	RAM           int64
	// "none" or what is protected against readback.
	Protection string
}

// Protected tells whether debug access, and so loading, is blocked.
func (c *ChipInfo) Protected() bool {
	return c.Protection != "" && c.Protection != "none"
}

// readChipInfo asks nrfutil about the chip on a board; replaced in tests.
// This connects to the chip and reads its ID registers; nothing is written.
var readChipInfo = func(serial string) (*ChipInfo, error) {
	out, err := runTool("", "nrfutil", "device", "device-info",
		"--serial-number", serial, "--json")
	if err != nil {
		return nil, util.NewNewtError(nrfutilError(out, err))
	}
	return parseChipInfo(out), nil
}

// nrfutilError finds the error message in nrfutil's JSON output.
func nrfutilError(out []byte, err error) string {
	msg := ""
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		var v interface{}
		if json.Unmarshal(scanner.Bytes(), &v) != nil {
			continue
		}
		for _, key := range []string{"description", "message"} {
			if s, ok := findKey(v, key).(string); ok && s != "" &&
				s != "Completed" {
				msg = s
			}
		}
	}
	if msg == "" {
		msg = errorLine(string(out), err)
	}
	return msg
}

// findKey returns the value of the first key with the given name anywhere
// in a decoded JSON value.
func findKey(v interface{}, key string) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		if val, ok := t[key]; ok && val != nil {
			return val
		}
		for _, child := range t {
			if val := findKey(child, key); val != nil {
				return val
			}
		}
	case []interface{}:
		for _, child := range t {
			if val := findKey(child, key); val != nil {
				return val
			}
		}
	}
	return nil
}

func numberOf(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case string:
		if i, err := strconv.ParseInt(n, 0, 64); err == nil {
			return i
		}
	}
	return 0
}

// parseChipInfo reads the output of "nrfutil device device-info --json".
// Keys are searched for anywhere in the output, since the layout differs
// between nrfutil versions.
func parseChipInfo(out []byte) *ChipInfo {
	info := &ChipInfo{}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		var v interface{}
		if json.Unmarshal(scanner.Bytes(), &v) != nil {
			continue
		}
		get := func(dst *string, key string) {
			if s, ok := findKey(v, key).(string); ok && *dst == "" {
				*dst = s
			}
		}
		get(&info.Version, "deviceVersion")
		get(&info.Family, "deviceFamily")
		get(&info.Board, "boardVersion")
		get(&info.ProbeFirmware, "jlinkObFirmwareVersion")
		get(&info.Protection, "protectionStatus")
		for _, key := range []string{"codeSize", "romSize"} {
			if info.Flash == 0 {
				info.Flash = numberOf(findKey(v, key))
			}
		}
		if info.RAM == 0 {
			info.RAM = numberOf(findKey(v, "ramSize"))
		}
	}

	parts := strings.Split(info.Version, "_")
	info.Part = parts[0]
	if len(parts) >= 3 {
		info.Variant = parts[1]
		info.Revision = strings.TrimPrefix(parts[len(parts)-1], "REV")
	}
	info.Family = strings.TrimSuffix(info.Family, "_FAMILY")
	info.Protection = protectionName(info.Protection)
	return info
}

func protectionName(status string) string {
	switch strings.TrimPrefix(status, "NRFDL_PROTECTION_STATUS_") {
	case "":
		return ""
	case "NONE":
		return "none"
	case "ALL":
		return "readback protected"
	case "SECURE_REGIONS":
		return "secure regions protected"
	case "REGION0":
		return "region 0 protected"
	default:
		return strings.ToLower(status)
	}
}

func alnumUpper(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') {
			return unicode.ToUpper(r)
		}
		return -1
	}, s)
}

// chipMatches compares the chip on a board, e.g. "NRF52840", with the one a
// target is built for, e.g. MCU_TARGET "nRF52840".
func chipMatches(part string, mcuTarget string) bool {
	p, t := alnumUpper(part), alnumUpper(mcuTarget)
	if p == "" || t == "" {
		return true
	}
	return strings.HasPrefix(p, t) || strings.HasPrefix(t, p)
}
