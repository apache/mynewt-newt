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

// Package porcelain describes a project for scripts and other tools rather
// than people, as "newt info" prints it with its machine-readable flags.  The
// output is stable; fields may be added, but existing ones are not renamed or
// removed without increasing FormatVersion.
package porcelain

// FormatVersion is increased whenever a change to the output can break
// existing readers.
const FormatVersion = 1

// Info is everything the output describes.  Paths are relative to the
// project's directory, except for the project's own path.
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
	Name string `json:"name"`
	Path string `json:"path"`
	// Whether this is the project itself rather than a dependency.
	Local  bool       `json:"local"`
	Origin RepoOrigin `json:"origin"`
	// The version project.yml asks for; empty if the repo is only a
	// dependency of another repo.
	Requested string `json:"requested"`
	// The installed version, and its commit; empty if not installed.
	Installed string `json:"installed"`
	Commit    string `json:"commit"`
	// What is modified in the repo's working tree; empty if clean.
	Dirty string `json:"dirty"`
	// Only known when remote repos were queried.
	NeedsUpgrade bool `json:"needs_upgrade"`
	// Why the repo's state could not be read, if it could not.
	Error string `json:"error"`
}

// RepoOrigin is where a repo is downloaded from, as project.yml or the
// depending repo declares it.
type RepoOrigin struct {
	// "github", "git" or "local".
	Type string `json:"type"`
	// The URL to clone, without credentials, or the directory of a local
	// repo.
	Url string `json:"url"`
	// For "github" repos only.
	Server string `json:"server"`
	User   string `json:"user"`
	Repo   string `json:"repo"`
	// The branch the repo is downloaded from, if declared.
	Branch string `json:"branch"`
}

type Target struct {
	Name string `json:"name"`
	Repo string `json:"repo"`
	Path string `json:"path"`
	// The target's variables without the "target." prefix, e.g. "app" and
	// "bsp", and its "cflags", "cxxflags", "lflags" and "aflags".  Lists
	// are joined with spaces; empty variables are left out.
	Vars map[string]string `json:"vars"`
	// The build profiles the target can use, e.g. "debug" and "optimized".
	BuildProfiles []string `json:"build_profiles"`
	// Where "newt build" and "newt create-image" put the target's files.
	Artifacts Artifacts `json:"artifacts"`

	// Whether the target's build was resolved.  Without it, e.g. with
	// "newt info --brief", the fields below are empty.
	Resolved bool `json:"resolved"`
	// Whether the target is a bootloader, i.e. sets BOOT_LOADER.
	BootLoader bool `json:"boot_loader"`
	// The flash area "newt load" writes the target to.
	FlashArea string `json:"flash_area"`

	// All syscfg settings of the target's build, sorted by name, and the
	// problems that would make "newt build" fail or that it warns about.
	Syscfg         []Setting `json:"syscfg"`
	SyscfgErrors   []string  `json:"syscfg_errors"`
	SyscfgWarnings []string  `json:"syscfg_warnings"`
	// The packages of the target's build, sorted by name.
	Deps []Dep `json:"deps"`
}

// Artifacts are the paths of a target's build output.  The files exist only
// once the target is built; the paths are empty for a target without an app.
type Artifacts struct {
	BinDir string `json:"bin_dir"`
	Elf    string `json:"elf"`
	Img    string `json:"img"`
}

// Setting is one resolved syscfg setting, as "newt target config show"
// displays it.
type Setting struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// The value from the setting's definition.
	Default     string `json:"default"`
	Description string `json:"description"`
	// "raw", "task_priority" or "flash_owner".
	Type string `json:"type"`
	// "good", "const", "deprecated", "defunct" or "experimental".
	State string `json:"state"`
	// The package that defines the setting; "newt" for settings newt
	// injects.
	Package string `json:"package"`
	// The setting the value is copied from, if any.
	ValueRef     string        `json:"value_ref"`
	Choices      []string      `json:"choices"`
	Restrictions []Restriction `json:"restrictions"`
	// Where the value comes from, oldest (the definition) first.
	History []SettingPoint `json:"history"`
}

type Restriction struct {
	// "notnull", "expr" or "range".
	Type string `json:"type"`
	// The expression, or the ranges, e.g. "1..10, 16".
	Expr string `json:"expr"`
}

type SettingPoint struct {
	// The package that sets the value; "newt" for values newt sets.
	Source string `json:"source"`
	Value  string `json:"value"`
}

// Dep is a package of a target's build, and the packages that pull it in,
// as "newt target revdep" displays them.
type Dep struct {
	Name string `json:"name"`
	// Empty for the packages the build starts from: the target, its app,
	// its BSP, ...
	RequiredBy []DepReason `json:"required_by"`
}

type DepReason struct {
	// The package that depends on this one.
	Name string `json:"name"`
	// The syscfg expression the dependency depends on; empty if it is
	// unconditional.
	Syscfg string `json:"syscfg"`
	// The APIs the package requires and this one provides, if the
	// dependency is through APIs.
	Apis []string `json:"apis"`
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
	// The pkg.deps, pkg.apis and pkg.req_apis of pkg.yml, including
	// conditional ones.
	Deps    []string `json:"deps"`
	Apis    []string `json:"apis"`
	ReqApis []string `json:"req_apis"`
	// The number of settings the package defines in syscfg.yml.
	SyscfgDefs int `json:"syscfg_defs"`
	// Only for BSP packages.
	Bsp *Bsp `json:"bsp"`
}

type Bsp struct {
	Arch     string `json:"arch"`
	Compiler string `json:"compiler"`
	// The flash areas, sorted by device and offset.
	FlashMap []FlashArea `json:"flash_map"`
}

type FlashArea struct {
	Name   string `json:"name"`
	Id     int    `json:"id"`
	Device int    `json:"device"`
	Offset int    `json:"offset"`
	Size   int    `json:"size"`
}
