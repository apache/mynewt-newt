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

// imgprod - Manifest generation.

package manifest

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/artifact/manifest"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/util"
)

type ManifestSizeCollector struct {
	Pkgs []*manifest.ManifestSizePkg
}

type ManifestCreateOpts struct {
	TgtBldr    *builder.TargetBuilder
	LoaderHash []byte
	AppHash    []byte
	Version    image.ImageVersion
	BuildID    string
	Syscfg     map[string]string
}

type RepoManager struct {
	repos map[string]manifest.ManifestRepo
}

func NewRepoManager() *RepoManager {
	return &RepoManager{
		repos: make(map[string]manifest.ManifestRepo),
	}
}

func (r *RepoManager) AllRepos() []*manifest.ManifestRepo {
	keys := make([]string, 0, len(r.repos))
	for k := range r.repos {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	repos := make([]*manifest.ManifestRepo, 0, len(keys))
	for _, key := range keys {
		r := r.repos[key]
		repos = append(repos, &r)
	}

	return repos
}

func (c *ManifestSizeCollector) AddPkg(pkg string) *manifest.ManifestSizePkg {
	p := &manifest.ManifestSizePkg{
		Name: pkg,
	}
	c.Pkgs = append(c.Pkgs, p)

	return p
}

func AddSymbol(p *manifest.ManifestSizePkg, file string, sym string, area string,
	symSz uint32) {

	f := addFile(p, file)
	s := addSym(f, sym)
	addArea(s, area, symSz)
}

func addFile(p *manifest.ManifestSizePkg, file string) *manifest.ManifestSizeFile {
	for _, f := range p.Files {
		if f.Name == file {
			return f
		}
	}
	f := &manifest.ManifestSizeFile{
		Name: file,
	}
	p.Files = append(p.Files, f)

	return f
}

func addSym(f *manifest.ManifestSizeFile, sym string) *manifest.ManifestSizeSym {
	s := &manifest.ManifestSizeSym{
		Name: sym,
	}
	f.Syms = append(f.Syms, s)

	return s
}

func addArea(s *manifest.ManifestSizeSym, area string, areaSz uint32) {
	a := &manifest.ManifestSizeArea{
		Name: area,
		Size: areaSz,
	}
	s.Areas = append(s.Areas, a)
}

func (r *RepoManager) GetManifestPkg(
	lpkg *pkg.LocalPackage) *manifest.ManifestPkg {

	ip := &manifest.ManifestPkg{
		Name: lpkg.FullName(),
	}

	var path string
	if lpkg.Repo().IsLocal() {
		ip.Repo = lpkg.Repo().Name()
		path = lpkg.BasePath()
	} else {
		ip.Repo = lpkg.Repo().Name()
		path = lpkg.BasePath()
	}

	if _, present := r.repos[ip.Repo]; present {
		return ip
	}

	repo := manifest.ManifestRepo{
		Name: ip.Repo,
	}

	// Make sure we restore the current working dir to whatever it was when
	// this function was called
	cwd, err := os.Getwd()
	if err != nil {
		log.Debugf("Unable to determine current working directory: %v", err)
		return ip
	}
	defer os.Chdir(cwd)

	if err := os.Chdir(path); err != nil {
		return ip
	}

	var res []byte

	res, err = util.ShellCommand([]string{
		"git",
		"rev-parse",
		"HEAD",
	}, nil)
	if err != nil {
		log.Debugf("Unable to determine commit hash for %s: %v", path, err)
		repo.Commit = "UNKNOWN"
	} else {
		repo.Commit = strings.TrimSpace(string(res))
		res, err = util.ShellCommand([]string{
			"git",
			"status",
			"--porcelain",
		}, nil)
		if err != nil {
			log.Debugf("Unable to determine dirty state for %s: %v", path, err)
		} else {
			if len(res) > 0 {
				repo.Dirty = true
			}
		}
		res, err = util.ShellCommand([]string{
			"git",
			"config",
			"--get",
			"remote.origin.url",
		}, nil)
		if err != nil {
			log.Debugf("Unable to determine URL for %s: %v", path, err)
		} else {
			repo.URL = strings.TrimSpace(string(res))
		}
	}
	r.repos[ip.Repo] = repo

	return ip
}

func ManifestPkgSizes(b *builder.Builder) (ManifestSizeCollector, error) {
	msc := ManifestSizeCollector{}

	libs, err := builder.ParseMapFileSizes(b.AppMapPath())
	if err != nil {
		return msc, err
	}

	// Order libraries by name.
	pkgSizes := make(builder.PkgSizeArray, len(libs))
	i := 0
	for _, es := range libs {
		pkgSizes[i] = es
		i++
	}
	sort.Sort(pkgSizes)

	for _, es := range pkgSizes {
		p := msc.AddPkg(b.FindPkgNameByArName(es.Name))

		// Order symbols by name.
		symbols := make(builder.SymbolDataArray, len(es.Syms))
		i := 0
		for _, sym := range es.Syms {
			symbols[i] = sym
			i++
		}
		sort.Sort(symbols)
		for _, sym := range symbols {
			for area, areaSz := range sym.Sizes {
				if areaSz != 0 {
					AddSymbol(p, sym.ObjName, sym.Name, area, areaSz)
				}
			}
		}
	}

	return msc, nil
}

func OptsForNonImage(t *builder.TargetBuilder) (ManifestCreateOpts, error) {
	res, err := t.Resolve()
	if err != nil {
		return ManifestCreateOpts{}, err
	}

	return ManifestCreateOpts{
		TgtBldr: t,
		Syscfg:  res.Cfg.SettingValues(),
	}, nil
}

func OptsForImage(t *builder.TargetBuilder, ver image.ImageVersion,
	appHash []byte, loaderHash []byte) (ManifestCreateOpts, error) {

	res, err := t.Resolve()
	if err != nil {
		return ManifestCreateOpts{}, err
	}

	return ManifestCreateOpts{
		TgtBldr:    t,
		AppHash:    appHash,
		LoaderHash: loaderHash,
		Version:    ver,
		BuildID:    fmt.Sprintf("%x", appHash),
		Syscfg:     res.Cfg.SettingValues(),
	}, nil
}

func CreateManifest(opts ManifestCreateOpts) (manifest.Manifest, error) {
	t := opts.TgtBldr

	m := manifest.Manifest{
		Name:      t.GetTarget().FullName(),
		Date:      time.Now().Format(time.RFC3339),
		Version:   opts.Version.String(),
		BuildID:   opts.BuildID,
		Image:     t.AppBuilder.AppImgPath(),
		ImageHash: fmt.Sprintf("%x", opts.AppHash),
		Syscfg:    opts.Syscfg,
	}

	rm := NewRepoManager()
	for _, rpkg := range t.AppBuilder.SortedRpkgs() {
		m.Pkgs = append(m.Pkgs, rm.GetManifestPkg(rpkg.Lpkg))
	}

	m.Repos = rm.AllRepos()

	vars := t.GetTarget().TargetY.AllSettingsAsStrings()
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		m.TgtVars = append(m.TgtVars, k+"="+vars[k])
	}
	syscfgKV := t.GetTarget().Package().SyscfgY.GetValStringMapString(
		"syscfg.vals", nil)
	if len(syscfgKV) > 0 {
		tgtSyscfg := fmt.Sprintf("target.syscfg=%s",
			syscfg.KeyValueToStr(syscfgKV))
		m.TgtVars = append(m.TgtVars, tgtSyscfg)
	}

	c, err := ManifestPkgSizes(t.AppBuilder)
	if err == nil {
		m.PkgSizes = c.Pkgs
	}

	if t.LoaderBuilder != nil {
		m.Loader = t.LoaderBuilder.AppImgPath()
		m.LoaderHash = fmt.Sprintf("%x", opts.LoaderHash)

		for _, rpkg := range t.LoaderBuilder.SortedRpkgs() {
			m.LoaderPkgs = append(m.LoaderPkgs, rm.GetManifestPkg(rpkg.Lpkg))
		}

		c, err = ManifestPkgSizes(t.LoaderBuilder)
		if err == nil {
			m.LoaderPkgSizes = c.Pkgs
		}
	}

	return m, nil
}
