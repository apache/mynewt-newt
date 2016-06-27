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

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type Builder struct {
	Packages map[*pkg.LocalPackage]*BuildPackage
	features map[string]bool
	apis     map[string]*BuildPackage

	appPkg       *BuildPackage
	Bsp          *pkg.BspPackage
	compilerPkg  *pkg.LocalPackage
	compilerInfo *toolchain.CompilerInfo

	target *target.Target
}

func NewBuilder(target *target.Target) (*Builder, error) {
	b := &Builder{}

	if err := b.Init(target); err != nil {
		return nil, err
	}

	return b, nil
}

func (b *Builder) Init(target *target.Target) error {
	b.target = target

	b.Packages = map[*pkg.LocalPackage]*BuildPackage{}
	b.features = map[string]bool{}
	b.apis = map[string]*BuildPackage{}

	return nil
}

func (b *Builder) Features() map[string]bool {
	return b.features
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
				// A new supported feature was discovered.  It is impossible to
				// determine what new dependency and API requirements are
				// generated as a result.  All packages need to be reprocessed.
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

	c, err := toolchain.NewCompiler(b.compilerPkg.BasePath(), dstDir,
		b.target.BuildProfile)
	if err != nil {
		return nil, err
	}
	c.AddInfo(b.compilerInfo)

	if bpkg != nil {
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
	srcDir := bpkg.BasePath() + "/src"
	if util.NodeNotExist(srcDir) {
		// Nothing to compile.
		return nil
	}

	c, err := b.newCompiler(bpkg, b.PkgBinDir(bpkg.Name()))
	if err != nil {
		return err
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
	if err = buildDir(srcDir, c, b.Bsp.Arch, []string{"test"}); err != nil {
		return err
	}
	if b.features["TEST"] {
		testSrcDir := srcDir + "/test"
		if err = buildDir(testSrcDir, c, b.Bsp.Arch, nil); err != nil {
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

func (b *Builder) link(elfName string) error {
	c, err := b.newCompiler(b.appPkg, b.PkgBinDir(elfName))
	if err != nil {
		return err
	}

	pkgNames := []string{}
	for _, bpkg := range b.Packages {
		archivePath := b.ArchivePath(bpkg.Name())
		if util.NodeExist(archivePath) {
			pkgNames = append(pkgNames, archivePath)
		}
	}

	if b.Bsp.LinkerScript != "" {
		c.LinkerScript = b.Bsp.BasePath() + b.Bsp.LinkerScript
	}
	err = c.CompileElf(elfName, pkgNames)
	if err != nil {
		return err
	}

	return nil
}

// Populates the builder with all the packages that need to be built and
// configures each package's build settings.  After this function executes,
// packages are ready to be built.
func (b *Builder) PrepBuild() error {
	if b.Bsp != nil {
		// Already prepped
		return nil
	}

	// Collect the seed packages.
	bspPkg := b.target.Bsp()
	if bspPkg == nil {
		if b.target.BspName == "" {
			return util.NewNewtError("BSP package not specified by target")
		} else {
			return util.NewNewtError("BSP package not found: " +
				b.target.BspName)
		}
	}

	b.Bsp = pkg.NewBspPackage(bspPkg)
	compilerPkg := b.resolveCompiler()
	if compilerPkg == nil {
		if b.Bsp.CompilerName == "" {
			return util.NewNewtError("Compiler package not specified by BSP")
		} else {
			return util.NewNewtError("Compiler package not found: " +
				b.Bsp.CompilerName)
		}
	}

	// An app package is not required (e.g., unit tests).
	appPkg := b.target.App()

	// Seed the builder with the app (if present), bsp, and target packages.

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

	targetBpkg := b.AddPackage(b.target.Package())

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
	//     * app (if present)
	//     * bsp
	//     * compiler (not added here)
	//     * target

	baseCi := toolchain.NewCompilerInfo()

	// App flags.
	if appBpkg != nil {
		appCi, err := appBpkg.CompilerInfo(b)
		if err != nil {
			return err
		}
		baseCi.AddCompilerInfo(appCi)
	}

	// Bsp flags.
	bspCi, err := bspBpkg.CompilerInfo(b)
	if err != nil {
		return err
	}
	// Define a cpp symbol indicating the BSP architecture, name of the BSP and app.
	bspCi.Cflags = append(bspCi.Cflags, "-DARCH_"+b.Bsp.Arch)
	bspCi.Cflags = append(bspCi.Cflags,
		"-DBSP_NAME=\""+filepath.Base(b.Bsp.Name())+"\"");
	if appPkg != nil {
		bspCi.Cflags = append(bspCi.Cflags,
			"-DAPP_NAME=\""+filepath.Base(appPkg.Name())+"\"");
	}
	baseCi.AddCompilerInfo(bspCi)

	// Target flags.
	targetCi, err := targetBpkg.CompilerInfo(b)
	if err != nil {
		return err
	}

	baseCi.AddCompilerInfo(targetCi)

	// Note: Compiler flags get added when compiler is created.

	// Read the BSP configuration.  These settings are necessary for the link
	// step.
	if err := b.Bsp.Reload(b.Features()); err != nil {
		return err
	}

	b.compilerPkg = compilerPkg
	b.compilerInfo = baseCi

	return nil
}

func (b *Builder) Build() error {
	if err := b.target.Validate(true); err != nil {
		return err
	}

	// Populate the package and feature sets and calculate the base compiler
	// flags.
	if err := b.PrepBuild(); err != nil {
		return err
	}

	// Build the packages alphabetically to ensure a consistent order.
	bpkgs := b.sortedBuildPackages()
	for _, bpkg := range bpkgs {
		if err := b.buildPackage(bpkg); err != nil {
			return err
		}
	}

	if err := b.link(b.AppElfPath()); err != nil {
		return err
	}

	return nil
}

func (b *Builder) Test(p *pkg.LocalPackage) error {
	if err := b.target.Validate(false); err != nil {
		return err
	}

	// Seed the builder with the package under test.
	testBpkg := b.AddPackage(p)

	// A few features are automatically supported when the test command is
	// used:
	//     * TEST:      ensures that the test code gets compiled.
	//     * SELFTEST:  indicates that there is no app.
	b.AddFeature("TEST")
	b.AddFeature("SELFTEST")

	// Populate the package and feature sets and calculate the base compiler
	// flags.
	err := b.PrepBuild()
	if err != nil {
		return err
	}

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
	err = b.link(testFilename)
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
	util.StatusMessage(util.VERBOSITY_VERBOSE, "Cleaning directory %s\n", path)
	err := os.RemoveAll(path)
	return err
}
