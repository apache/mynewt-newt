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
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/config"
	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

// Options select what Gather collects.
type Options struct {
	// Query the repos' remotes to tell whether they can be upgraded.
	Remote bool
	// Don't resolve the targets' builds, which takes most of the time:
	// leave out their syscfg, dependencies and everything derived from
	// them.
	Brief bool
	// Only describe these targets; all of them if nil.
	Targets []*target.Target
}

// Gather collects the information about a project.
func Gather(proj *project.Project, opts Options) (*Info, error) {
	info := &Info{
		FormatVersion: FormatVersion,
		NewtVersion:   newtutil.NewtVersionStr,
		Project: Project{
			Name: proj.Name(),
			Path: proj.Path(),
		},
		Repos:    []Repo{},
		Targets:  []Target{},
		Packages: []Package{},
	}

	repos, err := gatherRepos(proj, opts.Remote)
	if err != nil {
		return nil, err
	}
	info.Repos = repos

	targets := opts.Targets
	if targets == nil {
		for name, t := range target.GetTargets() {
			// The unittest target is used internally by newt.
			if !strings.HasSuffix(name, "/unittest") {
				targets = append(targets, t)
			}
		}
	}
	for _, t := range targets {
		info.Targets = append(info.Targets,
			gatherTarget(proj, t, !opts.Brief))
	}
	sort.Slice(info.Targets, func(i, j int) bool {
		return info.Targets[i].Name < info.Targets[j].Name
	})

	for _, pkgs := range proj.PackageList() {
		for _, p := range *pkgs {
			if lpkg, ok := p.(*pkg.LocalPackage); ok {
				info.Packages = append(info.Packages,
					gatherPackage(proj, lpkg))
			}
		}
	}
	sort.Slice(info.Packages, func(i, j int) bool {
		return info.Packages[i].Name < info.Packages[j].Name
	})

	return info, nil
}

func gatherRepos(proj *project.Project, remote bool) ([]Repo, error) {
	all := func(r *repo.Repo) bool { return true }

	repoInfos, err := proj.RepoInfos(all, remote)
	if err != nil {
		return nil, err
	}

	repos := []Repo{}
	for _, r := range proj.SelectRepos(all) {
		ri := repoInfos[r.Name()]
		if r.IsLocal() && ri.CommitHash == "" {
			// The project is not a git repo, which is not an error.
			ri.ErrorText = ""
		}

		pr := Repo{
			Name:         r.Name(),
			Path:         relPath(proj, r.Path()),
			Local:        r.IsLocal(),
			Origin:       repoOrigin(r),
			Commit:       ri.CommitHash,
			Dirty:        ri.DirtyState,
			NeedsUpgrade: ri.NeedsUpgrade,
			Error:        ri.ErrorText,
		}
		if req := proj.RepoRequirement(r.Name()); req != nil {
			pr.Requested = req.String()
		}
		if ri.InstalledVer != nil {
			pr.Installed = ri.InstalledVer.String()
		}
		repos = append(repos, pr)
	}

	return repos, nil
}

// repoOrigin returns where the repo is downloaded from, without any
// credentials.
func repoOrigin(r *repo.Repo) RepoOrigin {
	switch dl := r.Downloader().(type) {
	case *downloader.GithubDownloader:
		server := dl.Server
		if server == "" {
			server = "github.com"
		}
		return RepoOrigin{
			Type:   "github",
			Url:    fmt.Sprintf("https://%s/%s/%s.git", server, dl.User, dl.Repo),
			Server: dl.Server,
			User:   dl.User,
			Repo:   dl.Repo,
			Branch: dl.Branch,
		}
	case *downloader.GitDownloader:
		return RepoOrigin{Type: "git", Url: dl.Url, Branch: dl.Branch}
	case *downloader.LocalDownloader:
		return RepoOrigin{Type: "local", Url: dl.Path}
	default:
		return RepoOrigin{}
	}
}

func gatherTarget(proj *project.Project, t *target.Target,
	resolve bool) Target {

	pt := Target{
		Name:           t.FullName(),
		Repo:           t.Package().Repo().Name(),
		Path:           relPath(proj, t.Package().BasePath()),
		Vars:           targetVars(t),
		BuildProfiles:  []string{},
		Syscfg:         []Setting{},
		SyscfgErrors:   []string{},
		SyscfgWarnings: []string{},
		Deps:           []Dep{},
	}

	if app := t.App(); app != nil {
		pt.Artifacts = Artifacts{
			BinDir: relPath(proj, builder.TargetBinDir(t.FullName())),
			Elf: relPath(proj, builder.AppElfPath(t.FullName(),
				builder.BUILD_NAME_APP, app.FullName())),
			Img: relPath(proj, builder.AppImgPath(t.FullName(),
				builder.BUILD_NAME_APP, app.FullName())),
		}
	}

	// Resolving the build tells everything else, as "newt target config
	// show" and "newt target revdep" do.  A target that cannot be resolved,
	// e.g. because it has no app, gets the reason as its only error.
	var bsp *pkg.BspPackage
	if resolve {
		pt.Resolved = true
		b, err := builder.NewTargetBuilder(t)
		if err == nil {
			err = resolveTarget(b, &pt)
			bsp = b.BspPkg()
		}
		if err != nil {
			pt.SyscfgErrors = append(pt.SyscfgErrors, err.Error())
		}
	}

	if bsp == nil && t.Bsp() != nil {
		bsp, _ = pkg.NewBspPackage(t.Bsp(), nil)
	}
	pt.BuildProfiles = buildProfiles(proj, t, bsp)

	return pt
}

func resolveTarget(b *builder.TargetBuilder, pt *Target) error {
	res, err := b.Resolve()
	if err != nil {
		return err
	}

	for _, e := range res.Cfg.Settings {
		pt.Syscfg = append(pt.Syscfg, setting(e))
		if e.Name == "BOOT_LOADER" {
			pt.BootLoader = parse.ValueIsTrue(e.Value)
		}
	}
	sort.Slice(pt.Syscfg, func(i, j int) bool {
		return pt.Syscfg[i].Name < pt.Syscfg[j].Name
	})

	// The same choice "newt load" makes.
	if pt.BootLoader {
		pt.FlashArea = "FLASH_AREA_BOOTLOADER"
	} else {
		pt.FlashArea = "FLASH_AREA_IMAGE_0"
	}

	pt.SyscfgErrors = append(pt.SyscfgErrors, textLines(res.ErrorText())...)
	pt.SyscfgWarnings = append(pt.SyscfgWarnings,
		textLines(res.WarningText())...)

	graph, err := b.CreateRevdepGraph()
	if err != nil {
		return err
	}
	for name, dependers := range graph {
		dep := Dep{Name: name, RequiredBy: []DepReason{}}
		for _, d := range dependers {
			reason := DepReason{
				Name:   d.PkgName,
				Syscfg: d.DepExprs.Disjunction().String(),
				Apis:   []string{},
			}
			for api := range d.ReqApiExprs {
				reason.Apis = append(reason.Apis, api)
			}
			sort.Strings(reason.Apis)
			dep.RequiredBy = append(dep.RequiredBy, reason)
		}
		pt.Deps = append(pt.Deps, dep)
	}
	sort.Slice(pt.Deps, func(i, j int) bool {
		return pt.Deps[i].Name < pt.Deps[j].Name
	})

	return nil
}

func setting(e syscfg.CfgEntry) Setting {
	s := Setting{
		Name:         e.Name,
		Value:        e.Value,
		Description:  strings.TrimSpace(e.Description),
		Type:         e.SettingType.String(),
		State:        e.State.String(),
		Package:      pkgName(e.PackageDef),
		ValueRef:     e.ValueRefName,
		Choices:      e.ValidChoices,
		Restrictions: []Restriction{},
		History:      []SettingPoint{},
	}
	if s.Choices == nil {
		s.Choices = []string{}
	}

	for _, r := range e.Restrictions {
		switch r.Code {
		case syscfg.CFG_RESTRICTION_CODE_NOTNULL:
			s.Restrictions = append(s.Restrictions,
				Restriction{Type: "notnull"})
		case syscfg.CFG_RESTRICTION_CODE_EXPR:
			s.Restrictions = append(s.Restrictions,
				Restriction{Type: "expr", Expr: r.Expr})
		case syscfg.CFG_RESTRICTION_CODE_RANGE:
			s.Restrictions = append(s.Restrictions,
				Restriction{Type: "range", Expr: r.Expr})
		}
		// Choices are listed separately.
	}

	for _, p := range e.History {
		s.History = append(s.History,
			SettingPoint{Source: pkgName(p.Source), Value: p.Value})
	}
	if len(e.History) > 0 {
		s.Default = e.History[0].Value
	}

	return s
}

func targetVars(t *target.Target) map[string]string {
	vars := map[string]string{}
	for k, v := range t.TargetY.AllSettings() {
		vars[strings.TrimPrefix(k, "target.")] = valueString(v)
	}

	// The build flags come from the target's package rather than target.yml.
	for _, name := range []string{"cflags", "cxxflags", "lflags", "aflags"} {
		flags, err := t.Package().PkgY.GetValStringSlice("pkg."+name, nil)
		util.OneTimeWarningError(err)
		vars[name] = strings.Join(flags, " ")
	}

	for k, v := range vars {
		if v == "" {
			delete(vars, k)
		}
	}

	return vars
}

// buildProfiles returns the build profiles the compiler of the target's BSP
// defines flags for, along with the default profile and the one the target
// uses.
func buildProfiles(proj *project.Project, t *target.Target,
	bsp *pkg.BspPackage) []string {

	found := map[string]bool{
		target.DEFAULT_BUILD_PROFILE: true,
	}
	if t.BuildProfile != "" {
		found[t.BuildProfile] = true
	}

	if bsp != nil && bsp.CompilerName != "" {
		// The compiler name is relative to the repo of the package that
		// names it.
		var comp *pkg.LocalPackage
		if bsp.CompilerNamePkg != nil {
			comp, _ = proj.ResolvePackage(bsp.CompilerNamePkg.Repo(),
				bsp.CompilerName)
		}
		if comp != nil {
			compY, err := config.ReadFile(filepath.Join(comp.BasePath(),
				toolchain.COMPILER_FILENAME))
			if err == nil {
				for k := range compY.AllSettings() {
					parts := strings.Split(k, ".")
					if len(parts) >= 3 && parts[0] == "compiler" &&
						parts[1] == "flags" && parts[2] != "base" {
						found[parts[2]] = true
					}
				}
			}
		}
	}

	profiles := make([]string, 0, len(found))
	for p := range found {
		profiles = append(profiles, p)
	}
	sort.Strings(profiles)
	return profiles
}

func gatherPackage(proj *project.Project, lpkg *pkg.LocalPackage) Package {
	pp := Package{
		Name:     lpkg.FullName(),
		Type:     pkg.PackageTypeNames[lpkg.Type()],
		Repo:     lpkg.Repo().Name(),
		Path:     relPath(proj, lpkg.BasePath()),
		Keywords: []string{},
		Deps:     pkgYmlList(lpkg, "pkg.deps"),
		Apis:     pkgYmlList(lpkg, "pkg.apis"),
		ReqApis:  pkgYmlList(lpkg, "pkg.req_apis"),
	}
	if desc := lpkg.Desc(); desc != nil {
		pp.Description = strings.TrimSpace(desc.Description)
		pp.Author = desc.Author
		pp.Homepage = desc.Homepage
		if desc.Keywords != nil {
			pp.Keywords = desc.Keywords
		}
	}

	defs, _ := lpkg.SyscfgY.GetValStringMap("syscfg.defs", nil)
	pp.SyscfgDefs = len(defs)

	if lpkg.Type() == pkg.PACKAGE_TYPE_BSP {
		pp.Bsp = gatherBsp(lpkg)
	}

	return pp
}

func pkgYmlList(lpkg *pkg.LocalPackage, key string) []string {
	vals, _ := lpkg.PkgY.GetValStringSliceNonempty(key, nil)
	if vals == nil {
		vals = []string{}
	}
	sort.Strings(vals)
	return vals
}

// gatherBsp reads a BSP the way the builder does; nil if it cannot be read.
func gatherBsp(lpkg *pkg.LocalPackage) *Bsp {
	bsp, err := pkg.NewBspPackage(lpkg, nil)
	if err != nil {
		return nil
	}

	pb := &Bsp{
		Arch:     bsp.Arch,
		Compiler: bsp.CompilerName,
		FlashMap: []FlashArea{},
	}
	for _, a := range bsp.FlashMap.Areas {
		pb.FlashMap = append(pb.FlashMap, FlashArea{
			Name:   a.Name,
			Id:     a.Id,
			Device: a.Device,
			Offset: a.Offset,
			Size:   a.Size,
		})
	}
	sort.Slice(pb.FlashMap, func(i, j int) bool {
		a, b := pb.FlashMap[i], pb.FlashMap[j]
		if a.Device != b.Device {
			return a.Device < b.Device
		}
		if a.Offset != b.Offset {
			return a.Offset < b.Offset
		}
		return a.Name < b.Name
	})

	return pb
}

func pkgName(lpkg *pkg.LocalPackage) string {
	if lpkg == nil {
		return "newt"
	}
	return lpkg.FullName()
}

// valueString formats a yml value, joining lists with spaces.
func valueString(v interface{}) string {
	if list, ok := v.([]interface{}); ok {
		strs := make([]string, len(list))
		for i, e := range list {
			strs[i] = cast.ToString(e)
		}
		return strings.Join(strs, " ")
	}
	return cast.ToString(v)
}

// textLines splits newt's multi-line error or warning text into its
// non-empty lines.
func textLines(text string) []string {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func relPath(proj *project.Project, path string) string {
	rel, err := filepath.Rel(proj.Path(), path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
