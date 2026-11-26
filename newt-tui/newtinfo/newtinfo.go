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

// Package newtinfo reads a newt project through "newt info --json", the
// stable description of a project that newt prints for other tools.  It is
// the only way the browser learns about the project: it does not use newt's
// packages, so newt does not depend on the browser and the browser depends
// only on the documented format.
package newtinfo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// FormatVersion is the version of the "newt info --json" format this
// package reads.
const FormatVersion = 1

// The types mirror the JSON objects; see the documentation of "newt info"
// for the meaning of each field.

type Info struct {
	FormatVersion int       `json:"format_version"`
	NewtVersion   string    `json:"newt_version"`
	Project       Project   `json:"project"`
	Repos         []Repo    `json:"repos"`
	Targets       []Target  `json:"targets"`
	Packages      []Package `json:"packages"`
}

type Project struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Repo struct {
	Name         string     `json:"name"`
	Path         string     `json:"path"`
	Local        bool       `json:"local"`
	Origin       RepoOrigin `json:"origin"`
	Requested    string     `json:"requested"`
	Installed    string     `json:"installed"`
	Commit       string     `json:"commit"`
	Dirty        string     `json:"dirty"`
	NeedsUpgrade bool       `json:"needs_upgrade"`
	Error        string     `json:"error"`
}

type RepoOrigin struct {
	Type   string `json:"type"`
	Url    string `json:"url"`
	Server string `json:"server"`
	User   string `json:"user"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

type Target struct {
	Name          string            `json:"name"`
	Repo          string            `json:"repo"`
	Path          string            `json:"path"`
	Vars          map[string]string `json:"vars"`
	BuildProfiles []string          `json:"build_profiles"`
	Artifacts     Artifacts         `json:"artifacts"`

	// Whether the fields below were filled in; not with --brief.
	Resolved   bool   `json:"resolved"`
	BootLoader bool   `json:"boot_loader"`
	FlashArea  string `json:"flash_area"`

	Syscfg         []Setting `json:"syscfg"`
	SyscfgErrors   []string  `json:"syscfg_errors"`
	SyscfgWarnings []string  `json:"syscfg_warnings"`
	Deps           []Dep     `json:"deps"`
}

type Artifacts struct {
	BinDir string `json:"bin_dir"`
	Elf    string `json:"elf"`
	Img    string `json:"img"`
}

type Setting struct {
	Name         string         `json:"name"`
	Value        string         `json:"value"`
	Default      string         `json:"default"`
	Description  string         `json:"description"`
	Type         string         `json:"type"`
	State        string         `json:"state"`
	Package      string         `json:"package"`
	ValueRef     string         `json:"value_ref"`
	Choices      []string       `json:"choices"`
	Restrictions []Restriction  `json:"restrictions"`
	History      []SettingPoint `json:"history"`
}

type Restriction struct {
	Type string `json:"type"`
	Expr string `json:"expr"`
}

type SettingPoint struct {
	Source string `json:"source"`
	Value  string `json:"value"`
}

type Dep struct {
	Name       string      `json:"name"`
	RequiredBy []DepReason `json:"required_by"`
}

type DepReason struct {
	Name   string   `json:"name"`
	Syscfg string   `json:"syscfg"`
	Apis   []string `json:"apis"`
}

type Package struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Repo        string   `json:"repo"`
	Path        string   `json:"path"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Homepage    string   `json:"homepage"`
	Keywords    []string `json:"keywords"`
	Deps        []string `json:"deps"`
	Apis        []string `json:"apis"`
	ReqApis     []string `json:"req_apis"`
	SyscfgDefs  int      `json:"syscfg_defs"`
	Bsp         *Bsp     `json:"bsp"`
}

type Bsp struct {
	Arch     string      `json:"arch"`
	Compiler string      `json:"compiler"`
	FlashMap []FlashArea `json:"flash_map"`
}

type FlashArea struct {
	Name   string `json:"name"`
	Id     int    `json:"id"`
	Device int    `json:"device"`
	Offset int    `json:"offset"`
	Size   int    `json:"size"`
}

// Load runs "newt info --json" in dir with the given newt binary, adding
// args, e.g. "--brief" or target names.
func Load(newt string, dir string, args ...string) (*Info, error) {
	cmd := exec.Command(newt, append([]string{"info", "--json"}, args...)...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, fmt.Errorf("cannot run %s: %v", newt, err)
		}
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "unknown flag: --") {
			return nil, fmt.Errorf("%s is too old: newt-tui needs "+
				"\"newt info --json --brief\"", newt)
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("newt info --json: %s",
			strings.TrimPrefix(msg, "Error: "))
	}

	return Parse(stdout.Bytes())
}

// Parse reads the output of "newt info --json".
func Parse(data []byte) (*Info, error) {
	// Check the version first, as other changes could make the rest
	// unreadable.
	var version struct {
		FormatVersion int `json:"format_version"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return nil, fmt.Errorf("cannot read newt info --json: %v", err)
	}
	if version.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("newt info --json has format version %d; "+
			"this browser reads version %d", version.FormatVersion,
			FormatVersion)
	}

	info := &Info{}
	if err := json.Unmarshal(data, info); err != nil {
		return nil, fmt.Errorf("cannot read newt info --json: %v", err)
	}
	return info, nil
}

// Target returns the target with the given full name, or nil.
func (info *Info) Target(name string) *Target {
	for i := range info.Targets {
		if info.Targets[i].Name == name {
			return &info.Targets[i]
		}
	}
	return nil
}

// Package returns the package with the given full name, or nil.
func (info *Info) Package(name string) *Package {
	for i := range info.Packages {
		if info.Packages[i].Name == name {
			return &info.Packages[i]
		}
	}
	return nil
}

// Repo returns the repo with the given name, or nil.
func (info *Info) Repo(name string) *Repo {
	for i := range info.Repos {
		if info.Repos[i].Name == name {
			return &info.Repos[i]
		}
	}
	return nil
}

// Setting returns the target's syscfg setting with the given name, or nil.
func (t *Target) Setting(name string) *Setting {
	for i := range t.Syscfg {
		if t.Syscfg[i].Name == name {
			return &t.Syscfg[i]
		}
	}
	return nil
}

// FlashArea returns the BSP's flash area with the given name, or nil.
func (b *Bsp) FlashArea(name string) *FlashArea {
	for i := range b.FlashMap {
		if b.FlashMap[i].Name == name {
			return &b.FlashMap[i]
		}
	}
	return nil
}
