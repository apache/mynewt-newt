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

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/image"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/newt/symbol"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type Builder struct {
	PkgMap map[*pkg.LocalPackage]*BuildPackage

	apiMap           map[string]*BuildPackage
	appPkg           *BuildPackage
	bspPkg           *BuildPackage
	compilerPkg      *BuildPackage
	targetPkg        *BuildPackage
	compilerInfo     *toolchain.CompilerInfo
	targetBuilder    *TargetBuilder
	cfg              syscfg.Cfg
	linkerScripts    []string
	buildName        string
	linkElf          string
	injectedSettings map[string]string
}

func NewBuilder(t *TargetBuilder, buildName string, lpkgs []*pkg.LocalPackage,
	apiMap map[string]*pkg.LocalPackage, cfg syscfg.Cfg) (*Builder, error) {

	b := &Builder{
		PkgMap: make(map[*pkg.LocalPackage]*BuildPackage, len(lpkgs)),
		cfg:    cfg,

		buildName:        buildName,
		apiMap:           make(map[string]*BuildPackage, len(apiMap)),
		linkElf:          "",
		targetBuilder:    t,
		injectedSettings: map[string]string{},
	}

	for _, lpkg := range lpkgs {
		if _, err := b.addPackage(lpkg); err != nil {
			return nil, err
		}
	}

	// Create a pseudo build package for the generated sysinit code.
	if _, err := b.addSysinitBpkg(); err != nil {
		return nil, err
	}

	for api, lpkg := range apiMap {
		bpkg := b.PkgMap[lpkg]
		if bpkg == nil {
			for _, lpkg := range b.sortedLocalPackages() {
				log.Debugf("    * %s", lpkg.Name())
			}
			return nil, util.FmtNewtError(
				"Unexpected unsatisfied API: %s; required by: %s", api,
				lpkg.Name())
		}

		b.apiMap[api] = bpkg
	}

	return b, nil
}

func (b *Builder) addPackage(npkg *pkg.LocalPackage) (*BuildPackage, error) {
	// Don't allow nil entries to the map
	if npkg == nil {
		panic("Cannot add nil package builder map")
	}

	bpkg := b.PkgMap[npkg]
	if bpkg == nil {
		bpkg = NewBuildPackage(npkg)

		switch bpkg.Type() {
		case pkg.PACKAGE_TYPE_APP:
			if b.appPkg != nil {
				return nil, pkgTypeConflictErr(b.appPkg, bpkg)
			}
			b.appPkg = bpkg

		case pkg.PACKAGE_TYPE_BSP:
			if b.bspPkg != nil {
				return nil, pkgTypeConflictErr(b.bspPkg, bpkg)
			}
			b.bspPkg = bpkg

		case pkg.PACKAGE_TYPE_COMPILER:
			if b.compilerPkg != nil {
				return nil, pkgTypeConflictErr(b.compilerPkg, bpkg)
			}
			b.compilerPkg = bpkg

		case pkg.PACKAGE_TYPE_TARGET:
			if b.targetPkg != nil {
				return nil, pkgTypeConflictErr(b.targetPkg, bpkg)
			}
			b.targetPkg = bpkg
		}

		b.PkgMap[npkg] = bpkg
	}

	return bpkg, nil
}

func pkgTypeConflictErr(p1 *BuildPackage, p2 *BuildPackage) error {
	return util.FmtNewtError("Two %s packages in build: %s, %s",
		pkg.PackageTypeNames[p1.Type()], p1.Name(), p2.Name())
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

	// Compile CPP files
	if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_CPP,
		append(ignDirs, "arch")); err != nil {
		return err
	}

	// Copy in pre-compiled library files
	if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_ARCHIVE,
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

		// Compile CPP source
		if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_CPP,
			ignDirs); err != nil {
			return err
		}

		// Compile assembly source (only architecture-specific).
		if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_ASM,
			ignDirs); err != nil {

			return err
		}

		// Copy in pre-compiled library files
		if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_ARCHIVE,
			ignDirs); err != nil {
			return err
		}
	}

	return nil
}

func (b *Builder) newCompiler(bpkg *BuildPackage,
	dstDir string) (*toolchain.Compiler, error) {

	c, err := b.targetBuilder.NewCompiler(dstDir)
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
	c, err := b.newCompiler(bpkg, b.PkgBinDir(bpkg))
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

	// Make sure we restore the current working dir to whatever it was when this function was called
	cwd, err := os.Getwd()
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Unable to determine current working directory: %v", err))
	}
	defer os.Chdir(cwd)

	for _, dir := range srcDirs {
		if err = buildDir(dir, c, b.targetBuilder.bspPkg.Arch, nil); err != nil {
			return err
		}
	}

	// Create a static library ("archive").
	if err := os.Chdir(bpkg.BasePath() + "/"); err != nil {
		return util.NewNewtError(err.Error())
	}
	archiveFile := b.ArchivePath(bpkg)
	if err = c.CompileArchive(archiveFile); err != nil {
		return err
	}

	return nil
}

func (b *Builder) RemovePackages(cmn map[string]bool) error {
	for pkgName, _ := range cmn {
		for lp, bpkg := range b.PkgMap {
			if bpkg.Name() == pkgName {
				delete(b.PkgMap, lp)
			}
		}
	}
	return nil
}

func (b *Builder) ExtractSymbolInfo() (error, *symbol.SymbolMap) {
	syms := symbol.NewSymbolMap()
	for _, bpkg := range b.PkgMap {
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

func (b *Builder) link(elfName string, linkerScripts []string,
	keepSymbols []string) error {

	c, err := b.newCompiler(b.appPkg, b.FileBinDir(elfName))
	if err != nil {
		return err
	}

	/* Always used the trimmed archive files. */
	pkgNames := []string{}

	for _, bpkg := range b.PkgMap {
		archiveNames, _ := filepath.Glob(b.PkgBinDir(bpkg) + "/*.a")
		pkgNames = append(pkgNames, archiveNames...)
	}

	c.LinkerScripts = linkerScripts
	err = c.CompileElf(elfName, pkgNames, keepSymbols, b.linkElf)
	if err != nil {
		return err
	}

	return nil
}

// Populates the builder with all the packages that need to be built and
// configures each package's build settings.  After this function executes,
// packages are ready to be built.
func (b *Builder) PrepBuild() error {
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
		b.targetPkg.FullName())
	targetCi, err := b.targetPkg.CompilerInfo(b)
	if err != nil {
		return err
	}
	baseCi.AddCompilerInfo(targetCi)

	// App flags.
	if b.appPkg != nil {
		log.Debugf("Generating build flags for app %s", b.appPkg.FullName())
		appCi, err := b.appPkg.CompilerInfo(b)
		if err != nil {
			return err
		}

		baseCi.AddCompilerInfo(appCi)
	}

	// Bsp flags.
	log.Debugf("Generating build flags for bsp %s", b.bspPkg.FullName())
	bspCi, err := b.bspPkg.CompilerInfo(b)
	if err != nil {
		return err
	}

	// Define a cpp symbol indicating the BSP architecture, name of the
	// BSP and app.

	archName := b.targetBuilder.bspPkg.Arch
	bspCi.Cflags = append(bspCi.Cflags, "-DARCH_"+util.CIdentifier(archName))
	bspCi.Cflags = append(bspCi.Cflags, "-DARCH_NAME=\""+archName+"\"")

	if b.appPkg != nil {
		appName := filepath.Base(b.appPkg.Name())
		bspCi.Cflags = append(bspCi.Cflags, "-DAPP_"+util.CIdentifier(appName))
		bspCi.Cflags = append(bspCi.Cflags, "-DAPP_NAME=\""+appName+"\"")
	}

	bspName := filepath.Base(b.bspPkg.Name())
	bspCi.Cflags = append(bspCi.Cflags, "-DBSP_"+util.CIdentifier(bspName))
	bspCi.Cflags = append(bspCi.Cflags, "-DBSP_NAME=\""+bspName+"\"")

	baseCi.AddCompilerInfo(bspCi)

	// All packages have access to the generated code header directory.
	baseCi.Cflags = append(baseCi.Cflags,
		"-I"+GeneratedIncludeDir(b.targetPkg.Name()))

	// Note: Compiler flags get added at the end, after the flags for library
	// package being built are calculated.
	b.compilerInfo = baseCi

	return nil
}

func (b *Builder) AddCompilerInfo(info *toolchain.CompilerInfo) {
	b.compilerInfo.AddCompilerInfo(info)
}

func (b *Builder) addSysinitBpkg() (*BuildPackage, error) {
	lpkg := pkg.NewLocalPackage(b.targetPkg.Repo().(*repo.Repo),
		GeneratedBaseDir(b.targetPkg.Name()))
	lpkg.SetName(pkg.ShortName(b.targetPkg) + "-sysinit-" + b.buildName)
	lpkg.SetType(pkg.PACKAGE_TYPE_GENERATED)

	return b.addPackage(lpkg)
}

func (b *Builder) Build() error {
	b.CleanArtifacts()

	// Build the packages alphabetically to ensure a consistent order.
	bpkgs := b.sortedBuildPackages()

	for _, bpkg := range bpkgs {
		if err := b.buildPackage(bpkg); err != nil {
			return err
		}
	}

	return nil
}

func (b *Builder) Link(linkerScripts []string) error {
	if err := b.link(b.AppElfPath(), linkerScripts, nil); err != nil {
		return err
	}
	return nil
}

func (b *Builder) KeepLink(
	linkerScripts []string, keepMap *symbol.SymbolMap) error {

	keepSymbols := make([]string, 0)

	if keepMap != nil {
		for _, info := range *keepMap {
			keepSymbols = append(keepSymbols, info.Name)
		}
	}
	if err := b.link(b.AppElfPath(), linkerScripts, keepSymbols); err != nil {
		return err
	}
	return nil
}

func (b *Builder) TestLink(linkerScripts []string) error {
	if err := b.link(b.AppTempElfPath(), linkerScripts, nil); err != nil {
		return err
	}
	return nil
}

func (b *Builder) pkgWithPath(path string) *BuildPackage {
	for _, p := range b.PkgMap {
		if p.BasePath() == path {
			return p
		}
	}

	return nil
}

func (b *Builder) FetchSymbolMap() (error, *symbol.SymbolMap) {
	loader_sm := symbol.NewSymbolMap()

	for _, value := range b.PkgMap {
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
	return b.targetBuilder.GetTarget()
}

func (b *Builder) buildRomElf(common *symbol.SymbolMap) error {

	/* check dependencies on the ROM ELF.  This is really dependent on
	 * all of the .a files, but since we already depend on the loader
	 * .as to build the initial elf, we only need to check the app .a */
	c, err := b.targetBuilder.NewCompiler(b.AppElfPath())
	d := toolchain.NewDepTracker(c)
	if err != nil {
		return err
	}

	archNames := []string{}

	/* build the set of archive file names */
	for _, bpkg := range b.PkgMap {
		archiveNames, _ := filepath.Glob(b.PkgBinDir(bpkg) + "/*.a")
		archNames = append(archNames, archiveNames...)
	}

	bld, err := d.RomElfBuildRequired(b.AppLinkerElfPath(),
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

func (b *Builder) CreateImage(version string,
	keystr string, keyId uint8, loaderImg *image.Image) (*image.Image, error) {

	img, err := image.NewImage(b.AppBinPath(), b.AppImgPath())
	if err != nil {
		return nil, err
	}

	err = img.SetVersion(version)
	if err != nil {
		return nil, err
	}

	if keystr != "" {
		err = img.SetSigningKey(keystr, keyId)
		if err != nil {
			return nil, err
		}
	}

	err = img.Generate(loaderImg)
	if err != nil {
		return nil, err
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"App image succesfully generated: %s\n", img.TargetImg)

	return img, nil
}

// Deletes files that should never be reused for a subsequent build.  This
// list includes:
//     <app>.img
//     <app>.elf.bin
//     manifest.json
func (b *Builder) CleanArtifacts() {
	if b.appPkg == nil {
		return
	}

	paths := []string{
		b.AppImgPath(),
		b.AppBinPath(),
		b.ManifestPath(),
	}

	// Attempt to delete each artifact, ignoring errors.
	for _, p := range paths {
		os.Remove(p)
	}
}
