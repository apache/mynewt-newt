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
	"mynewt.apache.org/newt/newt/symbol"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type Builder struct {
	Packages         map[*pkg.LocalPackage]*BuildPackage
	features         map[string]bool
	apis             map[string]*BuildPackage
	appPkg           *BuildPackage
	BspPkg           *pkg.LocalPackage
	compilerInfo     *toolchain.CompilerInfo
	featureWhiteList []map[string]interface{}
	featureBlackList []map[string]interface{}
	target           *TargetBuilder
	linkerScript     string
	buildName        string
	LinkElf          string
}

func NewBuilder(t *TargetBuilder, buildName string) (*Builder, error) {
	b := &Builder{}

	b.buildName = buildName
	/* TODO */
	b.Init(t)

	return b, nil
}

func (b *Builder) Init(t *TargetBuilder) error {

	b.target = t
	b.Packages = map[*pkg.LocalPackage]*BuildPackage{}
	b.features = map[string]bool{}
	b.apis = map[string]*BuildPackage{}
	b.LinkElf = ""

	return nil
}

func (b *Builder) AllFeatures() map[string]bool {
	return b.features
}

func (b *Builder) Features(pkg pkg.Package) map[string]bool {
	featureList := map[string]bool{}

	for fname, _ := range b.features {
		if b.CheckValidFeature(pkg, fname) {
			featureList[fname] = true
		}
	}

	return featureList
}

func (b *Builder) AddFeature(feature string) {
	b.features[feature] = true
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

func (b *Builder) loadDeps() error {
	// Circularly resolve dependencies, identities, APIs, and required APIs
	// until no new ones exist.
	for {
		reprocess := false
		for _, bpkg := range b.Packages {
			newDeps, newFeatures, err := bpkg.Resolve(b)
			if err != nil {
				return err
			}

			if newFeatures {
				// A new supported feature was discovered.  It is impossible
				// to determine what new dependency and API requirements are
				// generated as a result.  All packages need to be
				// reprocessed.
				for _, bpkg := range b.Packages {
					bpkg.depsResolved = false
					bpkg.apisSatisfied = false
				}
				reprocess = true
				break
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

	// Specify all the source yml files as dependencies.  If a yml file has
	// changed, a full rebuild is required.
	for _, bp := range b.Packages {
		c.AddDeps(bp.CfgFilenames()...)
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

	// Build the package source in two phases:
	// 1. Non-test code.
	// 2. Test code (if the "test" feature is enabled).
	//
	// This is done in two passes because the structure of
	// architecture-specific directories is different for normal code and test
	// code, and not easy to generalize into a single operation:
	//     * src/arch/<target-arch>
	//     * src/test/arch/<target-arch>
	for _, dir := range srcDirs {
		if err = buildDir(dir, c, b.target.Bsp.Arch, []string{"test"}); err != nil {
			return err
		}
		if b.features["TEST"] {
			testSrcDir := dir + "/test"
			if err = buildDir(testSrcDir, c, b.target.Bsp.Arch, nil); err != nil {
				return err
			}
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
		c.LinkerScript = b.target.Bsp.BasePath() + linkerScript
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

	b.featureBlackList = []map[string]interface{}{}
	b.featureWhiteList = []map[string]interface{}{}

	// Seed the builder with the app (if present), bsp, and target packages.

	b.BspPkg = bspPkg

	var appBpkg *BuildPackage
	if appPkg != nil {
		appBpkg = b.Packages[appPkg]
		if appBpkg == nil {
			appBpkg = b.AddPackage(appPkg)
		}
		b.appPkg = appBpkg

		b.featureBlackList = append(b.featureBlackList, appBpkg.FeatureBlackList())
		b.featureWhiteList = append(b.featureWhiteList, appBpkg.FeatureWhiteList())
	}

	bspBpkg := b.Packages[bspPkg]

	if bspBpkg == nil {
		bspBpkg = b.AddPackage(bspPkg)
	}

	b.featureBlackList = append(b.featureBlackList, bspPkg.FeatureBlackList())
	b.featureWhiteList = append(b.featureWhiteList, bspPkg.FeatureWhiteList())

	targetBpkg := b.AddPackage(targetPkg)

	b.featureBlackList = append(b.featureBlackList, targetBpkg.FeatureBlackList())
	b.featureWhiteList = append(b.featureWhiteList, targetBpkg.FeatureWhiteList())

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
	log.Debugf("Generating build flags for target %s", b.target.target.FullName())
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

func (b *Builder) CheckValidFeature(pkg pkg.Package,
	feature string) bool {

	// If the feature is not in the blacklist, automatically valid
	if match := b.matchFeature(b.featureBlackList, pkg, feature); !match {
		return true
	}

	// If it is in the blacklist, check if its in the whitelist
	// if not, override the feature definition.
	if match := b.matchFeature(b.featureWhiteList, pkg, feature); match {
		return true
	} else {
		return false
	}
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

func (b *Builder) Test(p *pkg.LocalPackage) error {

	// Seed the builder with the package under test.
	testBpkg := b.AddPackage(p)

	// Define the PKG_TEST symbol while the package under test is being
	// compiled.  This symbol enables the appropriate main function that
	// usually comes from an app.
	testPkgCi, err := testBpkg.CompilerInfo(b)
	if err != nil {
		return err
	}
	testPkgCi.Cflags = append(testPkgCi.Cflags, "-DMYNEWT_SELFTEST")

	// Build the packages alphabetically to ensure a consistent order.
	bpkgs := b.sortedBuildPackages()
	for _, bpkg := range bpkgs {
		err = b.buildPackage(bpkg)
		if err != nil {
			return err
		}
	}

	testFilename := b.TestExePath(p.Name())
	err = b.link(testFilename, "", nil)
	if err != nil {
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
