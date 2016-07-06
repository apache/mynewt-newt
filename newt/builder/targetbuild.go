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

	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/symbol"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type TargetBuilder struct {
	compilerPkg *pkg.LocalPackage
	Bsp         *pkg.BspPackage
	target      *target.Target

	App     *Builder
	AppList interfaces.PackageList

	Loader     *Builder
	LoaderList interfaces.PackageList
}

func NewTargetBuilder(target *target.Target) (*TargetBuilder, error) {
	t := &TargetBuilder{}

	/* TODO */
	t.target = target

	return t, nil
}

func (t *TargetBuilder) NewCompiler(dstDir string) (*toolchain.Compiler, error) {
	c, err := toolchain.NewCompiler(t.compilerPkg.BasePath(), dstDir,
		t.target.BuildProfile)

	return c, err
}

func (t *TargetBuilder) PrepBuild() error {

	if t.Bsp != nil {
		// Already prepped
		return nil
	}
	// Collect the seed packages.
	bspPkg := t.target.Bsp()
	if bspPkg == nil {
		if t.target.BspName == "" {
			return util.NewNewtError("BSP package not specified by target")
		} else {
			return util.NewNewtError("BSP package not found: " +
				t.target.BspName)
		}
	}
	t.Bsp = pkg.NewBspPackage(bspPkg)

	compilerPkg := t.resolveCompiler()
	if compilerPkg == nil {
		if t.Bsp.CompilerName == "" {
			return util.NewNewtError("Compiler package not specified by BSP")
		} else {
			return util.NewNewtError("Compiler package not found: " +
				t.Bsp.CompilerName)
		}
	}
	t.compilerPkg = compilerPkg

	appPkg := t.target.App()
	targetPkg := t.target.Package()

	app, err := NewBuilder(t, "app")

	if err == nil {
		t.App = app
	} else {
		return err
	}

	loaderPkg := t.target.Loader()

	if loaderPkg != nil {
		loader, err := NewBuilder(t, "loader")

		if err == nil {
			t.Loader = loader
		} else {
			return err
		}

		err = t.Loader.PrepBuild(loaderPkg, bspPkg, targetPkg)
		if err != nil {
			return err
		}

		loader_flag := toolchain.NewCompilerInfo()
		loader_flag.Cflags = append(loader_flag.Cflags, "-DSPLIT_LOADER")
		t.Loader.AddCompilerInfo(loader_flag)

		t.LoaderList = project.ResetDeps(nil)
	}

	bsp_pkg := t.target.Bsp()

	err = t.App.PrepBuild(appPkg, bsp_pkg, targetPkg)
	if err != nil {
		return err

	}
	if loaderPkg != nil {
		app_flag := toolchain.NewCompilerInfo()
		app_flag.Cflags = append(app_flag.Cflags, "-DSPLIT_APPLICATION")
		t.App.AddCompilerInfo(app_flag)
	}

	t.AppList = project.ResetDeps(nil)

	return nil
}

func (t *TargetBuilder) Build() error {
	var err error
	var linkerScript string

	if err = t.target.Validate(true); err != nil {
		return err
	}

	if err = t.PrepBuild(); err != nil {
		return err
	}

	/* Build the Apps */
	project.ResetDeps(t.AppList)

	if err := t.Bsp.Reload(t.App.Features(t.App.BspPkg)); err != nil {
		return err
	}

	err = t.App.Build()
	if err != nil {
		return err
	}

	/* if we have no loader, we are done here.  All of the rest of this
	 * function is for split images */
	if t.Loader == nil {
		err = t.App.Link(t.Bsp.LinkerScript)
		return err
	}

	/* Link the app as a test (using the normal single image linker script) */
	err = t.App.TestLink(t.Bsp.LinkerScript)
	if err != nil {
		return err
	}

	/* rebuild the loader */
	project.ResetDeps(t.LoaderList)

	if err = t.Bsp.Reload(t.Loader.Features(t.Loader.BspPkg)); err != nil {
		return err
	}

	err = t.Loader.Build()

	if err != nil {
		return err
	}

	/* perform a test link of the loader */
	err = t.Loader.TestLink(t.Bsp.LinkerScript)

	if err != nil {
		return err
	}

	/* re-link the loader with app dependencies */
	err, common_pkgs, common_syms := t.RelinkLoader()
	if err != nil {
		return err
	}

	/* The app can ignore these packages next time */
	t.App.RemovePackages(common_pkgs)
	/* add back the BSP package which needs linking in both */
	t.App.AddPackage(t.Bsp.LocalPackage)

	/* create the special elf to link the app against */
	/* its just the elf with a set of symbols removed and renamed */
	err = t.Loader.buildRomElf(common_syms)
	if err != nil {
		return err
	}

	/* set up the linker elf and linker script for the app */
	t.App.LinkElf = t.Loader.AppLinkerElfPath()
	linkerScript = t.Bsp.Part2LinkerScript

	if linkerScript == "" {
		return util.NewNewtError("BSP Must specify Linker script ")
	}

	/* link the app */
	err = t.App.Link(linkerScript)
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
func (t *TargetBuilder) RelinkLoader() (error, map[string]bool, *symbol.SymbolMap) {

	/* fetch symbols from the elf and from the libraries themselves */
	err, appLibSym := t.App.ExtractSymbolInfo()
	if err != nil {
		return err, nil, nil
	}

	/* fetch the symbol list from the app temporary elf */
	err, appElfSym := t.App.ParseObjectElf(t.App.AppTempElfPath())
	if err != nil {
		return err, nil, nil
	}

	/* extract the library symbols and elf symbols from the loader */
	err, loaderLibSym := t.Loader.ExtractSymbolInfo()
	if err != nil {
		return err, nil, nil
	}

	err, loaderElfSym := t.Loader.ParseObjectElf(t.Loader.AppTempElfPath())
	if err != nil {
		return err, nil, nil
	}

	/* create the set of matching and non-matching symbols */
	err, sm_match, sm_nomatch := symbol.IdenticalUnion(appLibSym,
		loaderLibSym, true, false)

	/* which packages are shared between the two */
	common_pkgs := sm_match.Packages()
	uncommon_pkgs := sm_nomatch.Packages()

	/* ensure that the loader and app packages are never shared */
	delete(common_pkgs, t.App.appPkg.Name())
	uncommon_pkgs[t.App.appPkg.Name()] = true
	ma := sm_match.FilterPkg(t.App.appPkg.Name())
	sm_match.RemoveMap(ma)

	delete(common_pkgs, t.Loader.appPkg.Name())
	uncommon_pkgs[t.Loader.appPkg.Name()] = true
	ml := sm_match.FilterPkg(t.Loader.appPkg.Name())
	sm_match.RemoveMap(ml)

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Putting %d symbols from %d packages into Loader\n",
		len(*sm_match), len(common_pkgs))

	/* This is worth a special comment.  We are building both apps as
	 * stand-alone apps against the normal linker file, so they will both
	 * have a Reset_Handler symbol.  We need to ignore this here.  When
	 * we build the split app, we use a special linker file which
	 * uses a different entry point */
	special_sm := symbol.NewSymbolMap()
	special_sm.Add(*symbol.NewElfSymbol("Reset_Handler(app)"))
	special_sm.Add(*symbol.NewElfSymbol("Reset_Handler(loader)"))

	var badpkgs []string
	var symbol_str string
	for v, _ := range uncommon_pkgs {
		if t.App.appPkg != nil && t.App.appPkg.Name() != v &&
			t.Loader.appPkg != nil && t.Loader.appPkg.Name() != v {
			trouble := sm_nomatch.FilterPkg(v)

			var found bool
			for _, sym := range *trouble {
				if _, ok := special_sm.Find(sym.Name); !ok {
					if !sym.IsLocal() {
						found = true
					}
				}
			}

			if found {
				symbol_str = (*trouble).String("Non Matching Symbols")
				badpkgs = append(badpkgs, v)
				delete(common_pkgs, v)
			}
		}
	}

	if len(badpkgs) > 0 {
		errStr := fmt.Sprintf("Common packages with different implementaiton\n %s \n",
			strings.Join(badpkgs, "\n "))
		errStr += symbol_str
		return util.NewNewtError(errStr), nil, nil
	}

	/* for each symbol in the elf of the app, if that symbol is in
	 * a common package, keep that symbol in the loader */
	preserve_elf := symbol.NewSymbolMap()

	/* go through each symbol in the app */
	for _, elfsym := range *appElfSym {
		name := elfsym.Name
		if libsym, ok := (*appLibSym)[name]; ok {
			if _, ok := common_pkgs[libsym.Bpkg]; ok {
				/* if its not in the loader elf, add it as undefined */
				if _, ok := (*loaderElfSym)[name]; !ok {
					preserve_elf.Add(elfsym)
				}
			}
		}
	}

	/* re-link loader */
	project.ResetDeps(t.LoaderList)

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Migrating %d unused symbols into Loader\n", len(*preserve_elf))

	err = t.Loader.KeepLink(t.Bsp.LinkerScript, preserve_elf)

	if err != nil {
		return err, nil, nil
	}
	return err, common_pkgs, sm_match
}

func (t *TargetBuilder) Test(p *pkg.LocalPackage) error {
	if err := t.target.Validate(false); err != nil {
		return err
	}

	if t.Bsp != nil {
		// Already prepped
		return nil
	}
	// Collect the seed packages.
	bspPkg := t.target.Bsp()
	if bspPkg == nil {
		if t.target.BspName == "" {
			return util.NewNewtError("BSP package not specified by target")
		} else {
			return util.NewNewtError("BSP package not found: " +
				t.target.BspName)
		}
	}
	t.Bsp = pkg.NewBspPackage(bspPkg)

	compilerPkg := t.resolveCompiler()
	if compilerPkg == nil {
		if t.Bsp.CompilerName == "" {
			return util.NewNewtError("Compiler package not specified by BSP")
		} else {
			return util.NewNewtError("Compiler package not found: " +
				t.Bsp.CompilerName)
		}
	}
	t.compilerPkg = compilerPkg

	targetPkg := t.target.Package()

	app, err := NewBuilder(t, "test")

	if err == nil {
		t.App = app
	} else {
		return err
	}

	// A few features are automatically supported when the test command is
	// used:
	//     * TEST:      ensures that the test code gets compiled.
	//     * SELFTEST:  indicates that there is no app.
	t.App.AddFeature("TEST")
	t.App.AddFeature("SELFTEST")

	err = t.App.PrepBuild(p, bspPkg, targetPkg)

	if err != nil {
		return err
	}

	err = t.App.Test(p)

	return err
}

func (t *TargetBuilder) resolveCompiler() *pkg.LocalPackage {
	if t.Bsp.CompilerName == "" {
		return nil
	}
	dep, _ := pkg.NewDependency(t.Bsp.Repo(), t.Bsp.CompilerName)
	mypkg := project.GetProject().ResolveDependency(dep).(*pkg.LocalPackage)
	return mypkg
}

func (t *TargetBuilder) Clean() error {
	var err error

	err = t.PrepBuild()

	if err == nil && t.App != nil {
		err = t.App.Clean()
	}
	if err == nil && t.Loader != nil {
		err = t.Loader.Clean()
	}
	return err
}

func (t *TargetBuilder) GetTarget() *target.Target {
	return (*t).target
}
