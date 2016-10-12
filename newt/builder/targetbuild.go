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

package builder

import (
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/resolve"
	"mynewt.apache.org/newt/newt/symbol"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/sysinit"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type TargetBuilder struct {
	target      *target.Target
	bspPkg      *pkg.BspPackage
	compilerPkg *pkg.LocalPackage
	appPkg      *pkg.LocalPackage
	loaderPkg   *pkg.LocalPackage
	testPkg     *pkg.LocalPackage

	AppBuilder *Builder
	AppList    interfaces.PackageList

	LoaderBuilder *Builder
	LoaderList    interfaces.PackageList

	injectedSettings map[string]string
}

func NewTargetBuilder(target *target.Target,
	testPkg *pkg.LocalPackage) (*TargetBuilder, error) {

	if err := target.Validate(testPkg == nil); err != nil {
		return nil, err
	}

	bspPkg, err := pkg.NewBspPackage(target.Bsp())
	if err != nil {
		return nil, err
	}

	compilerPkg, err := project.GetProject().ResolvePackage(
		bspPkg.Repo(), bspPkg.CompilerName)
	if err != nil {
		return nil, err
	}

	t := &TargetBuilder{
		target:           target,
		bspPkg:           bspPkg,
		compilerPkg:      compilerPkg,
		appPkg:           target.App(),
		loaderPkg:        target.Loader(),
		testPkg:          testPkg,
		injectedSettings: map[string]string{},
	}

	return t, nil
}

func (t *TargetBuilder) NewCompiler(dstDir string) (*toolchain.Compiler, error) {
	c, err := toolchain.NewCompiler(t.compilerPkg.BasePath(), dstDir,
		t.target.BuildProfile)

	return c, err
}

func (t *TargetBuilder) ExportCfg() (resolve.CfgResolution, error) {
	seeds := []*pkg.LocalPackage{
		t.bspPkg.LocalPackage,
		t.compilerPkg,
		t.target.Package(),
	}

	if t.loaderPkg != nil {
		// For split images, inject the SPLIT_[...] settings into the
		// corresponding app packages.  This ensures that:
		//     * The app packages know they are part of a split image during
		//       dependency resolution.
		//     * The app source files receive "-DSPLIT_[...]=1" command line
		//       arguments during compilation.
		t.loaderPkg.InjectedSettings()["SPLIT_LOADER"] = "1"
		if t.appPkg != nil {
			t.appPkg.InjectedSettings()["SPLIT_APPLICATION"] = "1"
		}

		// Inject the SPLIT_IMAGE setting into the entire target.  All packages
		// now know that they are part of a split image build.
		t.injectedSettings["SPLIT_IMAGE"] = "1"

		seeds = append(seeds, t.loaderPkg)
	}

	if t.appPkg != nil {
		seeds = append(seeds, t.appPkg)
	}

	if t.testPkg != nil {
		// A few features are automatically supported when the test command is
		// used:
		//     * TEST:      lets packages know that this is a test app
		//     * SELFTEST:  indicates that the "newt test" command is used;
		//                  causes a package to define a main() function.
		t.injectedSettings["TEST"] = "1"
		t.injectedSettings["SELFTEST"] = "1"

		seeds = append(seeds, t.testPkg)
	}

	cfgResolution, err := resolve.ResolveCfg(seeds, t.injectedSettings,
		t.bspPkg.FlashMap)
	if err != nil {
		return cfgResolution, err
	}

	return cfgResolution, nil
}

func (t *TargetBuilder) validateAndWriteCfg(
	cfgResolution resolve.CfgResolution) error {

	if errText := cfgResolution.ErrorText(); errText != "" {
		return util.NewNewtError(errText)
	}

	if err := syscfg.EnsureWritten(cfgResolution.Cfg,
		GeneratedIncludeDir(t.target.Name())); err != nil {

		return err
	}

	return nil
}

func (t *TargetBuilder) resolvePkgs(cfgResolution resolve.CfgResolution) (
	[]*pkg.LocalPackage, []*pkg.LocalPackage, error) {

	var appPkg *pkg.LocalPackage
	if t.appPkg != nil {
		appPkg = t.appPkg
	} else {
		appPkg = t.testPkg
	}

	return resolve.ResolveSplitPkgs(cfgResolution,
		t.loaderPkg,
		appPkg,
		t.bspPkg.LocalPackage,
		t.compilerPkg,
		t.target.Package())
}

func (t *TargetBuilder) generateSysinit(
	cfgResolution resolve.CfgResolution) error {

	loaderPkgs, appPkgs, err := t.resolvePkgs(cfgResolution)
	if err != nil {
		return err
	}

	srcDir := GeneratedSrcDir(t.target.Name())

	if loaderPkgs != nil {
		sysinit.EnsureWritten(loaderPkgs, srcDir,
			pkg.ShortName(t.target.Package()), true)
	}

	sysinit.EnsureWritten(appPkgs, srcDir,
		pkg.ShortName(t.target.Package()), false)

	return nil
}

func (t *TargetBuilder) generateFlashMap() error {
	return t.bspPkg.FlashMap.EnsureWritten(
		GeneratedSrcDir(t.target.Name()),
		GeneratedIncludeDir(t.target.Name()),
		pkg.ShortName(t.target.Package()))
}

func (t *TargetBuilder) generateCode(
	cfgResolution resolve.CfgResolution) error {

	if err := t.generateSysinit(cfgResolution); err != nil {
		return err
	}

	if err := t.generateFlashMap(); err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) PrepBuild() error {
	cfgResolution, err := t.ExportCfg()
	if err != nil {
		return err
	}

	flashErrText := t.bspPkg.FlashMap.ErrorText()
	if flashErrText != "" {
		return util.NewNewtError(flashErrText)
	}

	if err := t.validateAndWriteCfg(cfgResolution); err != nil {
		return err
	}

	loaderPkgs, appPkgs, err := t.resolvePkgs(cfgResolution)
	if err != nil {
		return err
	}

	if loaderPkgs != nil {
		t.LoaderBuilder, err = NewBuilder(t, "loader", loaderPkgs,
			cfgResolution.ApiMap, cfgResolution.Cfg)
		if err != nil {
			return err
		}
		if err := t.LoaderBuilder.PrepBuild(); err != nil {
			return err
		}

		loaderFlags := toolchain.NewCompilerInfo()
		loaderFlags.Cflags = append(loaderFlags.Cflags, "-DSPLIT_LOADER")
		t.LoaderBuilder.AddCompilerInfo(loaderFlags)

		t.LoaderList = project.ResetDeps(nil)
	}

	t.AppBuilder, err = NewBuilder(t, "app", appPkgs,
		cfgResolution.ApiMap, cfgResolution.Cfg)
	if err != nil {
		return err
	}
	if err := t.AppBuilder.PrepBuild(); err != nil {
		return err
	}

	if loaderPkgs != nil {
		appFlags := toolchain.NewCompilerInfo()
		appFlags.Cflags = append(appFlags.Cflags, "-DSPLIT_APPLICATION")
		t.AppBuilder.AddCompilerInfo(appFlags)
	}

	t.AppList = project.ResetDeps(nil)

	if err := t.generateCode(cfgResolution); err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) Build() error {
	var err error
	var linkerScript string

	if err = t.PrepBuild(); err != nil {
		return err
	}

	/* Build the Apps */
	project.ResetDeps(t.AppList)

	if err := t.bspPkg.Reload(t.AppBuilder.cfg.Features()); err != nil {
		return err
	}

	err = t.AppBuilder.Build()
	if err != nil {
		return err
	}

	/* if we have no loader, we are done here.  All of the rest of this
	 * function is for split images */
	if t.LoaderBuilder == nil {
		err = t.AppBuilder.Link(t.bspPkg.LinkerScript)
		return err
	}

	/* Link the app as a test (using the normal single image linker script) */
	err = t.AppBuilder.TestLink(t.bspPkg.LinkerScript)
	if err != nil {
		return err
	}

	/* rebuild the loader */
	project.ResetDeps(t.LoaderList)

	if err = t.bspPkg.Reload(t.LoaderBuilder.cfg.Features()); err != nil {
		return err
	}

	err = t.LoaderBuilder.Build()

	if err != nil {
		return err
	}

	/* perform a test link of the loader */
	err = t.LoaderBuilder.TestLink(t.bspPkg.LinkerScript)

	if err != nil {
		return err
	}

	/* re-link the loader with app dependencies */
	err, commonPkgs, commonSyms := t.RelinkLoader()
	if err != nil {
		return err
	}

	/* The app can ignore these packages next time */
	delete(commonPkgs, t.bspPkg.Name())
	t.AppBuilder.RemovePackages(commonPkgs)

	/* create the special elf to link the app against */
	/* its just the elf with a set of symbols removed and renamed */
	err = t.LoaderBuilder.buildRomElf(commonSyms)
	if err != nil {
		return err
	}

	/* set up the linker elf and linker script for the app */
	t.AppBuilder.linkElf = t.LoaderBuilder.AppLinkerElfPath()
	linkerScript = t.bspPkg.Part2LinkerScript

	if linkerScript == "" {
		return util.NewNewtError("BSP must specify linker script ")
	}

	/* link the app */
	err = t.AppBuilder.Link(linkerScript)
	if err != nil {
		return err
	}

	return nil
}

/*
 * This function re-links the loader adding symbols from libraries
 * shared with the app. Returns a list of the common packages shared
 * by the app and loader
 */
func (t *TargetBuilder) RelinkLoader() (error, map[string]bool,
	*symbol.SymbolMap) {

	/* fetch symbols from the elf and from the libraries themselves */
	log.Debugf("Loader packages:")
	for _, lpkg := range t.LoaderBuilder.sortedLocalPackages() {
		log.Debugf("    * %s", lpkg.Name())
	}
	log.Debugf("App packages:")
	for _, lpkg := range t.AppBuilder.sortedLocalPackages() {
		log.Debugf("    * %s", lpkg.Name())
	}
	err, appLibSym := t.AppBuilder.ExtractSymbolInfo()
	if err != nil {
		return err, nil, nil
	}

	/* fetch the symbol list from the app temporary elf */
	err, appElfSym := t.AppBuilder.ParseObjectElf(t.AppBuilder.AppTempElfPath())
	if err != nil {
		return err, nil, nil
	}

	/* extract the library symbols and elf symbols from the loader */
	err, loaderLibSym := t.LoaderBuilder.ExtractSymbolInfo()
	if err != nil {
		return err, nil, nil
	}

	err, loaderElfSym := t.LoaderBuilder.ParseObjectElf(
		t.LoaderBuilder.AppTempElfPath())
	if err != nil {
		return err, nil, nil
	}

	/* create the set of matching and non-matching symbols */
	err, smMatch, smNomatch := symbol.IdenticalUnion(appLibSym,
		loaderLibSym, true, false)

	/* which packages are shared between the two */
	commonPkgs := smMatch.Packages()
	uncommonPkgs := smNomatch.Packages()

	/* ensure that the loader and app packages are never shared */
	delete(commonPkgs, t.AppBuilder.appPkg.Name())
	uncommonPkgs[t.AppBuilder.appPkg.Name()] = true
	ma := smMatch.FilterPkg(t.AppBuilder.appPkg.Name())
	smMatch.RemoveMap(ma)

	delete(commonPkgs, t.LoaderBuilder.appPkg.Name())
	uncommonPkgs[t.LoaderBuilder.appPkg.Name()] = true
	ml := smMatch.FilterPkg(t.LoaderBuilder.appPkg.Name())
	smMatch.RemoveMap(ml)

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Putting %d symbols from %d packages into loader\n",
		len(*smMatch), len(commonPkgs))

	/* This is worth a special comment.  We are building both apps as
	 * stand-alone apps against the normal linker file, so they will both
	 * have a Reset_Handler symbol.  We need to ignore this here.  When
	 * we build the split app, we use a special linker file which
	 * uses a different entry point */
	specialSm := symbol.NewSymbolMap()
	specialSm.Add(*symbol.NewElfSymbol("Reset_Handler(app)"))
	specialSm.Add(*symbol.NewElfSymbol("Reset_Handler(loader)"))

	var badpkgs []string
	var symbolStr string
	for v, _ := range uncommonPkgs {
		if t.AppBuilder.appPkg != nil &&
			t.AppBuilder.appPkg.Name() != v &&
			t.LoaderBuilder.appPkg != nil &&
			t.LoaderBuilder.appPkg.Name() != v {

			trouble := smNomatch.FilterPkg(v)

			var found bool
			for _, sym := range *trouble {
				if _, ok := specialSm.Find(sym.Name); !ok {
					if !sym.IsLocal() {
						found = true
					}
				}
			}

			if found {
				symbolStr = (*trouble).String("Non Matching Symbols")
				badpkgs = append(badpkgs, v)
				delete(commonPkgs, v)
			}
		}
	}

	if len(badpkgs) > 0 {
		errStr := fmt.Sprintf(
			"Common packages with different implementaiton\n %s \n",
			strings.Join(badpkgs, "\n "))
		errStr += symbolStr
		return util.NewNewtError(errStr), nil, nil
	}

	/* for each symbol in the elf of the app, if that symbol is in
	 * a common package, keep that symbol in the loader */
	preserveElf := symbol.NewSymbolMap()

	/* go through each symbol in the app */
	for _, elfsym := range *appElfSym {
		name := elfsym.Name
		if libsym, ok := (*appLibSym)[name]; ok {
			if _, ok := commonPkgs[libsym.Bpkg]; ok {
				/* if its not in the loader elf, add it as undefined */
				if _, ok := (*loaderElfSym)[name]; !ok {
					preserveElf.Add(elfsym)
				}
			}
		}
	}

	/* re-link loader */
	project.ResetDeps(t.LoaderList)

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Migrating %d unused symbols into Loader\n", len(*preserveElf))

	err = t.LoaderBuilder.KeepLink(t.bspPkg.LinkerScript, preserveElf)

	if err != nil {
		return err, nil, nil
	}
	return err, commonPkgs, smMatch
}

func (t *TargetBuilder) Test() error {
	cfgResolution, err := t.ExportCfg()
	if err != nil {
		return err
	}

	if err := t.validateAndWriteCfg(cfgResolution); err != nil {
		return err
	}

	_, appPkgs, err := t.resolvePkgs(cfgResolution)
	if err != nil {
		return err
	}

	t.AppBuilder, err = NewBuilder(t, "test", appPkgs,
		cfgResolution.ApiMap, cfgResolution.Cfg)
	if err != nil {
		return err
	}
	if err := t.AppBuilder.PrepBuild(); err != nil {
		return err
	}

	if err := t.generateCode(cfgResolution); err != nil {
		return err
	}

	if err := t.AppBuilder.Test(t.testPkg); err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) GetTarget() *target.Target {
	return (*t).target
}
