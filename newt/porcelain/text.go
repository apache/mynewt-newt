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

package porcelain

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// WritePorcelain writes the information as lines of the form
//
//	<kind> <name> <key> <value>
//
// where kind is "project", "repo", "target" or "package".  The first three
// fields never contain spaces; the value is the rest of the line, with
// backslashes and newlines escaped as "\\" and "\n".  Lines with empty values
// are left out.  Keys of lists are repeated, once per element, in order;
// lists of words, e.g. "choices", are instead one line, with the words
// separated by spaces.  See docs/command_list/newt_info.rst for all keys.
func WritePorcelain(w io.Writer, info *Info) error {
	pw := &porcelainWriter{w: w}

	pw.kind, pw.name = "project", info.Project.Name
	pw.line("format_version", fmt.Sprintf("%d", info.FormatVersion))
	pw.line("newt_version", info.NewtVersion)
	pw.line("path", info.Project.Path)

	for _, r := range info.Repos {
		pw.kind, pw.name = "repo", r.Name
		pw.line("path", r.Path)
		pw.line("local", fmt.Sprintf("%t", r.Local))
		pw.line("origin.type", r.Origin.Type)
		pw.line("origin.url", r.Origin.Url)
		pw.line("origin.server", r.Origin.Server)
		pw.line("origin.user", r.Origin.User)
		pw.line("origin.repo", r.Origin.Repo)
		pw.line("origin.branch", r.Origin.Branch)
		pw.line("requested", r.Requested)
		pw.line("installed", r.Installed)
		pw.line("commit", r.Commit)
		pw.line("dirty", r.Dirty)
		pw.line("needs_upgrade", fmt.Sprintf("%t", r.NeedsUpgrade))
		pw.line("error", r.Error)
	}

	for _, t := range info.Targets {
		writeTarget(pw, t)
	}

	for _, p := range info.Packages {
		writePackage(pw, p)
	}

	return pw.err
}

func writeTarget(pw *porcelainWriter, t Target) {
	pw.kind, pw.name = "target", t.Name
	pw.line("repo", t.Repo)
	pw.line("path", t.Path)
	for _, k := range sortedKeys(t.Vars) {
		pw.line("var."+k, t.Vars[k])
	}
	pw.line("build_profiles", strings.Join(t.BuildProfiles, " "))
	pw.line("artifact.bin_dir", t.Artifacts.BinDir)
	pw.line("artifact.elf", t.Artifacts.Elf)
	pw.line("artifact.img", t.Artifacts.Img)
	pw.line("resolved", fmt.Sprintf("%t", t.Resolved))
	if t.Resolved {
		pw.line("boot_loader", fmt.Sprintf("%t", t.BootLoader))
	}
	pw.line("flash_area", t.FlashArea)

	for _, e := range t.SyscfgErrors {
		pw.line("syscfg_error", e)
	}
	for _, w := range t.SyscfgWarnings {
		pw.line("syscfg_warning", w)
	}

	for _, s := range t.Syscfg {
		key := "syscfg." + s.Name + "."
		pw.line(key+"value", s.Value)
		pw.line(key+"default", s.Default)
		pw.line(key+"description", s.Description)
		pw.line(key+"type", s.Type)
		pw.line(key+"state", s.State)
		pw.line(key+"package", s.Package)
		pw.line(key+"value_ref", s.ValueRef)
		pw.line(key+"choices", strings.Join(s.Choices, " "))
		for _, r := range s.Restrictions {
			pw.line(key+"restriction", strings.TrimSpace(r.Type+" "+r.Expr))
		}
		for _, p := range s.History {
			pw.line(key+"history", p.Source+" "+p.Value)
		}
	}

	for _, d := range t.Deps {
		pw.line("dep", d.Name)
		for _, r := range d.RequiredBy {
			// <depender> [api:<api>[,<api>...]] [<syscfg expression>]
			reason := r.Name
			if len(r.Apis) > 0 {
				reason += " api:" + strings.Join(r.Apis, ",")
			}
			if r.Syscfg != "" {
				reason += " " + r.Syscfg
			}
			pw.line("dep."+d.Name+".required_by", reason)
		}
	}
}

func writePackage(pw *porcelainWriter, p Package) {
	pw.kind, pw.name = "package", p.Name
	pw.line("type", p.Type)
	pw.line("repo", p.Repo)
	pw.line("path", p.Path)
	pw.line("description", p.Description)
	pw.line("author", p.Author)
	pw.line("homepage", p.Homepage)
	pw.line("keywords", strings.Join(p.Keywords, " "))
	for _, d := range p.Deps {
		pw.line("dep", d)
	}
	for _, a := range p.Apis {
		pw.line("api", a)
	}
	for _, a := range p.ReqApis {
		pw.line("req_api", a)
	}
	if p.SyscfgDefs > 0 {
		pw.line("syscfg_defs", fmt.Sprintf("%d", p.SyscfgDefs))
	}

	if p.Bsp != nil {
		pw.line("bsp.arch", p.Bsp.Arch)
		pw.line("bsp.compiler", p.Bsp.Compiler)
		for _, a := range p.Bsp.FlashMap {
			pw.line("bsp.flash_area."+a.Name, fmt.Sprintf(
				"id=%d device=%d offset=0x%x size=%d",
				a.Id, a.Device, a.Offset, a.Size))
		}
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type porcelainWriter struct {
	w   io.Writer
	err error

	// The kind and name of the lines being written.
	kind string
	name string
}

var valueEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`)

func (pw *porcelainWriter) line(key string, value string) {
	if pw.err != nil || value == "" {
		return
	}
	_, pw.err = fmt.Fprintf(pw.w, "%s %s %s %s\n", pw.kind, pw.name, key,
		valueEscaper.Replace(value))
}
