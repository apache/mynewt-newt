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
	"os"
	"path/filepath"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/symbol"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/sysinit"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type Builder struct {
	Packages         map[*pkg.LocalPackage]*BuildPackage
	apis             map[string]*BuildPackage
	appPkg           *BuildPackage
	BspPkg           *pkg.LocalPackage
	compilerInfo     *toolchain.CompilerInfo
	target           *TargetBuilder
	linkerScript     string
	buildName        string
	LinkElf          string
	injectedSettings map[string]string

	Cfg syscfg.Cfg
}

func NewBuilder(t *TargetBuilder, buildName string) (*Builder, error) {
	b := &Builder{
		Packages:         map[*pkg.LocalPackage]*BuildPackage{},
		buildName:        buildName,
		apis:             map[string]*BuildPackage{},
		LinkElf:          "",
		target:           t,
		injectedSettings: map[string]string{},
		Cfg:              syscfg.NewCfg(),
	}

	return b, nil
}

func (b *Builder) AddPackage(npkg *pkg.LocalPackage) *BuildPackage {
	// Don't allow nil entries to the map
	if npkg == nil {
		panic("Cannot add nil package builder map")
	}

	bpkg := b.Packages[npkg]
	if bpkg == nil {
		bpkg = NewBuildPackage(npkg)
		b.Packages[npkg] = bpkg
	}

	return bpkg
}

// @return bool                 true if this is a new API.
func (b *Builder) AddApi(apiString string, bpkg *BuildPackage) bool {
	curBpkg := b.apis[apiString]
	if curBpkg == nil {
		b.apis[apiString] = bpkg
		return true
	} else {
		if curBpkg != bpkg {
			util.StatusMessage(util.VERBOSITY_QUIET,
				"Warning: API conflict: %s (%s <-> %s)\n", apiString,
				curBpkg.Name(), bpkg.Name())
		}
		return false
	}
}

func (b *Builder) ApiNames() []string {
	apiNames := make([]string, len(b.apis), len(b.apis))

	i := 0
	for api, _ := range b.apis {
		apiNames[i] = api
		i += 1
	}

	return apiNames
}

// @return                      changed,err
func (b *Builder) reloadCfg() (bool, error) {
	apis := make([]string, len(b.apis))
	i := 0
	for api, _ := range b.apis {
		apis[i] = api
		i++
	}

	// Determine which features have been detected so far.  The feature map is
	// required for reloading syscfg, as features may unlock additional
	// settings.
	features := b.Cfg.Features()
	cfg, err := syscfg.Read(b.sortedLocalPackages(), apis, b.injectedSettings,
		features)
	if err != nil {
		return false, err
	}

	changed := false
	for k, v := range cfg.Settings {
		oldval, ok := b.Cfg.Settings[k]
		if !ok || len(oldval.History) != len(v.History) {
			b.Cfg = cfg
			changed = true
		}
	}

	return changed, nil
}

func (b *Builder) loadDepsOnce() (bool, error) {
	// Circularly resolve dependencies, APIs, and required APIs until no new
	// ones exist.
	newDeps := false
	for {
		reprocess := false
		for _, bpkg := range b.Packages {
			newDeps, err := bpkg.Resolve(b, b.Cfg)
			if err != nil {
				return false, err
			}

			if newDeps {
				// The new dependencies need to be processed.  Iterate again
				// after this iteration completes.
				reprocess = true
			}
		}

		if !reprocess {
			break
		}
	}

	return newDeps, nil
}

func (b *Builder) loadDeps() error {
	if _, err := b.loadDepsOnce(); err != nil {
		return err
	}

	for {
		cfgChanged, err := b.reloadCfg()
		if err != nil {
			return err
		}
		if cfgChanged {
			// A new supported feature was discovered.  It is impossible
			// to determine what new dependency and API requirements are
			// generated as a result.  All packages need to be
			// reprocessed.
			for _, bpkg := range b.Packages {
				bpkg.depsResolved = false
				bpkg.apisSatisfied = false
			}
		}

		newDeps, err := b.loadDepsOnce()
		if err != nil {
			return err
		}

		if !newDeps && !cfgChanged {
			break
		}
	}

	// Log the final syscfg.
	b.Cfg.Log()

	// Report syscfg errors.
	if err := b.Cfg.DetectErrors(); err != nil {
		return err
	}

	return nil
}

// Recursively compiles all the .c and .s files in the specified directory.
// Architecture-specific files are also compiled.
func buildDir(srcDir string, c *toolchain.Compiler, arch string,
	ignDirs []string) error {

	// Quietly succeed if the source directory doesn't exist.
	if util.NodeNotExist(srcDir) {
		return nil
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Compiling src in base directory: %s\n", srcDir)

	// Start from the source directory.
	if err := os.Chdir(srcDir); err != nil {
		return util.NewNewtError(err.Error())
	}

	// Ignore architecture-specific source files for now.  Use a temporary
	// string slice here so that the "arch" directory is not ignored in the
	// subsequent architecture-specific compile phase.
	if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_C,
		append(ignDirs, "arch")); err != nil {

		return err
	}

	archDir := srcDir + "/arch/" + arch + "/"
	if util.NodeExist(archDir) {
		util.StatusMessage(util.VERBOSITY_VERBOSE,
			"Compiling architecture specific src pkgs in directory: %s\n",
			archDir)
		if err := os.Chdir(archDir); err != nil {
			return util.NewNewtError(err.Error())
		}

		// Compile C source.
		if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_C,
			ignDirs); err != nil {

			return err
		}

		// Compile assembly source (only architecture-specific).
		if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_ASM,
			ignDirs); err != nil {

			return err
		}
	}

	return nil
}

func (b *Builder) newCompiler(bpkg *BuildPackage,
	dstDir string) (*toolchain.Compiler, error) {

	c, err := b.target.NewCompiler(dstDir)
	if err != nil {
		return nil, err
	}

	c.AddInfo(b.compilerInfo)

	if bpkg != nil {
		log.Debugf("Generating build flags for package %s", bpkg.FullName())
		ci, err := bpkg.CompilerInfo(b)
		if err != nil {
			return nil, err
		}
		c.AddInfo(ci)
	}

	return c, nil
}

// Compiles and archives a package.
func (b *Builder) buildPackage(bpkg *BuildPackage) error {
	c, err := b.newCompiler(bpkg, b.PkgBinDir(bpkg.Name()))
	if err != nil {
		return err
	}

	srcDirs := []string{}

	if len(bpkg.SourceDirectories) > 0 {
		for _, relDir := range bpkg.SourceDirectories {
			dir := bpkg.BasePath() + "/" + relDir
			if util.NodeNotExist(dir) {
				return util.NewNewtError(fmt.Sprintf(
					"Specified source directory %s, does not exist.",
					dir))
			}
			srcDirs = append(srcDirs, dir)
		}
	} else {
		srcDir := bpkg.BasePath() + "/src"
		if util.NodeNotExist(srcDir) {
			// Nothing to compile.
			return nil
		}

		srcDirs = append(srcDirs, srcDir)
	}

	for _, dir := range srcDirs {
		if err = buildDir(dir, c, b.target.Bsp.Arch, nil); err != nil {
			return err
		}
	}

	// Create a static library ("archive").
	if err := os.Chdir(bpkg.BasePath() + "/"); err != nil {
		return util.NewNewtError(err.Error())
	}
	archiveFile := b.ArchivePath(bpkg.Name())
	if err = c.CompileArchive(archiveFile); err != nil {
		return err
	}

	return nil
}

func (b *Builder) RemovePackages(cmn map[string]bool) error {

	for pkgName, _ := range cmn {
		for lp, bpkg := range b.Packages {
			if bpkg.Name() == pkgName {
				delete(b.Packages, lp)
			}
		}
	}
	return nil
}

func (b *Builder) ExtractSymbolInfo() (error, *symbol.SymbolMap) {
	syms := symbol.NewSymbolMap()
	for _, bpkg := range b.Packages {
		err, sm := b.ParseObjectLibrary(bpkg)
		if err == nil {
			syms, err = (*syms).Merge(sm)
			if err != nil {
				return err, nil
			}
		}
	}
	return nil, syms
}

func (b *Builder) link(elfName string, linkerScript string,
	keepSymbols []string) error {
	c, err := b.newCompiler(b.appPkg, b.PkgBinDir(elfName))
	if err != nil {
		return err
	}

	/* always used the trimmed archive files */
	pkgNames := []string{}

	for _, bpkg := range b.Packages {
		if util.NodeExist(b.ArchivePath(bpkg.Name())) {
			pkgNames = append(pkgNames, b.ArchivePath(bpkg.Name()))
		}
	}

	if linkerScript != "" {
		c.LinkerScript = b.target.Bsp.BasePath() + "/" + linkerScript
	}
	err = c.CompileElf(elfName, pkgNames, keepSymbols, b.LinkElf)
	if err != nil {
		return err
	}

	return nil
}

// Populates the builder with all the packages that need to be built and
// configures each package's build settings.  After this function executes,
// packages are ready to be built.
func (b *Builder) PrepBuild(appPkg *pkg.LocalPackage,
	bspPkg *pkg.LocalPackage, targetPkg *pkg.LocalPackage) error {

	// Seed the builder with the app (if present), bsp, and target packages.

	b.BspPkg = bspPkg

	var appBpkg *BuildPackage
	if appPkg != nil {
		appBpkg = b.Packages[appPkg]
		if appBpkg == nil {
			appBpkg = b.AddPackage(appPkg)
		}
		b.appPkg = appBpkg
	}

	bspBpkg := b.Packages[bspPkg]

	if bspBpkg == nil {
		bspBpkg = b.AddPackage(bspPkg)
	}

	targetBpkg := b.AddPackage(targetPkg)

	// Populate the full set of packages to be built and resolve the feature
	// set.
	if err := b.loadDeps(); err != nil {
		return err
	}

	b.logDepInfo()

	// Terminate if any package has an unmet API requirement.
	if err := b.verifyApisSatisfied(); err != nil {
		return err
	}

	// Populate the base set of compiler flags.  Flags from the following
	// packages get applied to every source file:
	//     * target
	//     * app (if present)
	//     * bsp
	//     * compiler (not added here)
	//
	// In the case of conflicting flags, the higher priority package's flag
	// wins.  Package priorities are assigned as follows (highest priority
	// first):
	//     * target
	//     * app (if present)
	//     * bsp
	//     * <library package>
	//     * compiler

	baseCi := toolchain.NewCompilerInfo()

	// Target flags.
	log.Debugf("Generating build flags for target %s",
		b.target.target.FullName())
	targetCi, err := targetBpkg.CompilerInfo(b)
	if err != nil {
		return err
	}
	baseCi.AddCompilerInfo(targetCi)

	// App flags.
	if appBpkg != nil {
		log.Debugf("Generating build flags for app %s", appPkg.FullName())
		appCi, err := appBpkg.CompilerInfo(b)
		if err != nil {
			return err
		}

		baseCi.AddCompilerInfo(appCi)
	}

	// Bsp flags.
	log.Debugf("Generating build flags for bsp %s", bspPkg.FullName())
	bspCi, err := bspBpkg.CompilerInfo(b)
	if err != nil {
		return err
	}

	// Define a cpp symbol indicating the BSP architecture, name of the
	// BSP and app.
	bspCi.Cflags = append(bspCi.Cflags, "-DARCH_"+b.target.Bsp.Arch)
	bspCi.Cflags = append(bspCi.Cflags,
		"-DBSP_NAME=\""+filepath.Base(b.target.Bsp.Name())+"\"")
	if appPkg != nil {
		bspCi.Cflags = append(bspCi.Cflags,
			"-DAPP_NAME=\""+filepath.Base(appPkg.Name())+"\"")
	}
	baseCi.AddCompilerInfo(bspCi)

	// Note: Compiler flags get added at the end, after the flags for library
	// package being built are calculated.
	b.compilerInfo = baseCi

	lpkgs := b.sortedLocalPackages()
	if err := syscfg.EnsureWritten(b.Cfg, lpkgs, b.ApiNames(),
		targetPkg.BasePath()); err != nil {
		return err
	}

	if err := sysinit.EnsureWritten(lpkgs, targetPkg.BasePath()); err != nil {
		return err
	}

	b.compilerInfo = baseCi

	return nil
}

func (b *Builder) matchFeature(flist []map[string]interface{},
	pkg pkg.Package, featureName string) bool {

	fullName := ""
	if pkg != nil {
		fullName = pkg.FullName()
	}

	for _, matchList := range flist {
		for pkgDesc, featureDesc := range matchList {
			re, _ := regexp.Compile(pkgDesc)
			if re.MatchString(fullName) {
				if strings.Compare(featureDesc.(string), featureName) == 0 {
					return true
				}
			}
		}
	}

	return false
}

func (b *Builder) AddCompilerInfo(info *toolchain.CompilerInfo) {
	b.compilerInfo.AddCompilerInfo(info)
}

func (b *Builder) Build() error {
	// Build the packages alphabetically to ensure a consistent order.
	bpkgs := b.sortedBuildPackages()
	for _, bpkg := range bpkgs {
		if err := b.buildPackage(bpkg); err != nil {
			return err
		}
	}

	return nil
}

func (b *Builder) Link(linkerScript string) error {
	if err := b.link(b.AppElfPath(), linkerScript, nil); err != nil {
		return err
	}
	return nil
}

func (b *Builder) KeepLink(linkerScript string, keepMap *symbol.SymbolMap) error {
	keepSymbols := make([]string, 0)

	if keepMap != nil {
		for _, info := range *keepMap {
			keepSymbols = append(keepSymbols, info.Name)
		}
	}
	if err := b.link(b.AppElfPath(), linkerScript, keepSymbols); err != nil {
		return err
	}
	return nil
}

func (b *Builder) TestLink(linkerScript string) error {
	if err := b.link(b.AppTempElfPath(), linkerScript, nil); err != nil {
		return err
	}
	return nil
}

func (b *Builder) pkgWithPath(path string) *BuildPackage {
	for _, p := range b.Packages {
		if p.BasePath() == path {
			return p
		}
	}

	return nil
}

func (b *Builder) testOwner(p *BuildPackage) *BuildPackage {
	if p.Type() != pkg.PACKAGE_TYPE_UNITTEST {
		panic("Expected unittest package; got: " + p.Name())
	}

	curPath := p.BasePath()

	for {
		parentPath := filepath.Dir(curPath)
		if parentPath == project.GetProject().BasePath || parentPath == "." {
			return nil
		}

		parentPkg := b.pkgWithPath(parentPath)
		if parentPkg != nil && parentPkg.Type() != pkg.PACKAGE_TYPE_UNITTEST {
			return parentPkg
		}

		curPath = parentPath
	}
}

func (b *Builder) Test(p *pkg.LocalPackage) error {
	// Build the packages alphabetically to ensure a consistent order.
	bpkgs := b.sortedBuildPackages()
	for _, bpkg := range bpkgs {
		if err := b.buildPackage(bpkg); err != nil {
			return err
		}
	}

	testFilename := b.TestExePath(p.Name())
	if err := b.link(testFilename, "", nil); err != nil {
		return err
	}

	// Run the tests.
	if err := os.Chdir(filepath.Dir(testFilename)); err != nil {
		return err
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Executing test: %s\n",
		testFilename)
	if _, err := util.ShellCommand(testFilename); err != nil {
		newtError := err.(*util.NewtError)
		newtError.Text = fmt.Sprintf("Test failure (%s):\n%s", p.Name(),
			newtError.Text)
		return newtError
	}

	return nil
}

func (b *Builder) Clean() error {
	path := b.BinDir()
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Cleaning directory %s\n",
		path)
	err := os.RemoveAll(path)
	return err
}

func (b *Builder) FetchSymbolMap() (error, *symbol.SymbolMap) {
	loader_sm := symbol.NewSymbolMap()

	for _, value := range b.Packages {
		err, sm := b.ParseObjectLibrary(value)
		if err == nil {
			util.StatusMessage(util.VERBOSITY_VERBOSE,
				"Size of %s Loader Map %d\n", value.Name(), len(*sm))
			loader_sm, err = loader_sm.Merge(sm)
			if err != nil {
				return err, nil
			}
		}
	}

	return nil, loader_sm
}

func (b *Builder) GetTarget() *target.Target {
	return b.target.GetTarget()
}

func (b *Builder) buildRomElf(common *symbol.SymbolMap) error {

	/* check dependencies on the ROM ELF.  This is really dependent on
	 * all of the .a files, but since we already depend on the loader
	 * .as to build the initial elf, we only need to check the app .a */
	c, err := b.target.NewCompiler(b.AppElfPath())
	d := toolchain.NewDepTracker(c)
	if err != nil {
		return err
	}

	archNames := []string{}

	/* build the set of archive file names */
	for _, bpkg := range b.Packages {
		archivePath := b.ArchivePath(bpkg.Name())
		if util.NodeExist(archivePath) {
			archNames = append(archNames, archivePath)
		}
	}

	bld, err := d.RomElfBuldRequired(b.AppLinkerElfPath(),
		b.AppElfPath(), archNames)
	if err != nil {
		return err
	}

	if !bld {
		return nil
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Generating ROM elf \n")

	/* the linker needs these symbols kept for the split app
	 * to initialize the loader data and bss */
	common.Add(*symbol.NewElfSymbol("__HeapBase"))
	common.Add(*symbol.NewElfSymbol("__bss_start__"))
	common.Add(*symbol.NewElfSymbol("__bss_end__"))
	common.Add(*symbol.NewElfSymbol("__etext"))
	common.Add(*symbol.NewElfSymbol("__data_start__"))
	common.Add(*symbol.NewElfSymbol("__data_end__"))

	/* the split app may need this to access interrupts */
	common.Add(*symbol.NewElfSymbol("__vector_tbl_reloc__"))
	common.Add(*symbol.NewElfSymbol("__isr_vector"))

	err = b.CopySymbols(common)
	if err != nil {
		return err
	}

	/* These symbols are needed by the split app so it can zero
	 * bss and copy data from the loader app before it restarts,
	 * but we have to rename them since it has its own copies of
	 * these special linker symbols  */
	tmp_sm := symbol.NewSymbolMap()
	tmp_sm.Add(*symbol.NewElfSymbol("__HeapBase"))
	tmp_sm.Add(*symbol.NewElfSymbol("__bss_start__"))
	tmp_sm.Add(*symbol.NewElfSymbol("__bss_end__"))
	tmp_sm.Add(*symbol.NewElfSymbol("__etext"))
	tmp_sm.Add(*symbol.NewElfSymbol("__data_start__"))
	tmp_sm.Add(*symbol.NewElfSymbol("__data_end__"))
	err = c.RenameSymbols(tmp_sm, b.AppLinkerElfPath(), "_loader")

	if err != nil {
		return err
	}
	return nil
}
