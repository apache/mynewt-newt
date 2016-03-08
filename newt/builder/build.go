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

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type Builder struct {
	Packages map[*pkg.LocalPackage]*BuildPackage
	features map[string]bool
	apis     map[string]*BuildPackage

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
			cli.StatusMessage(util.VERBOSITY_QUIET,
				"Warning: API conflict: %s <-> %s\n", curBpkg.Name(),
				bpkg.Name())
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
			resolved, err := bpkg.Resolve(b)
			if err != nil {
				return err
			}

			if !resolved {
				reprocess = true
			}
		}

		if !reprocess {
			break
		}
	}

	b.logFeatures()

	return nil
}

// Recursively compiles all the .c and .s files in the specified directory.
// Architecture-specific files are also compiled.
func buildDir(srcDir string, c *toolchain.Compiler, arch string,
	ignDirs []string) error {

	// Quietly succeed if the source directory doesn't exist.
	if cli.NodeNotExist(srcDir) {
		return nil
	}

	cli.StatusMessage(util.VERBOSITY_VERBOSE,
		"compiling src in base directory: %s\n", srcDir)

	// Start from the source directory.
	if err := os.Chdir(srcDir); err != nil {
		return util.NewNewtError(err.Error())
	}

	// Don't recurse into destination directories.
	ignDirs = append(ignDirs, "obj")
	ignDirs = append(ignDirs, "bin")

	// Ignore architecture-specific source files for now.  Use a temporary
	// string slice here so that the "arch" directory is not ignored in the
	// subsequent architecture-specific compile phase.
	if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_C,
		append(ignDirs, "arch")); err != nil {

		return err
	}

	archDir := srcDir + "/arch/" + arch + "/"
	cli.StatusMessage(util.VERBOSITY_VERBOSE,
		"compiling architecture specific src pkgs in directory: %s\n",
		archDir)

	if cli.NodeExist(archDir) {
		if err := os.Chdir(archDir); err != nil {
			return util.NewNewtError(err.Error())
		}
		if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_C,
			ignDirs); err != nil {

			return err
		}

		// compile assembly sources in recursive compile as well
		if err := c.RecursiveCompile(toolchain.COMPILER_TYPE_ASM,
			ignDirs); err != nil {

			return err
		}
	}

	return nil
}

// Compiles and archives a package.
func (b *Builder) buildPackage(bpkg *BuildPackage) error {
	srcDir := bpkg.BasePath() + "/src"
	if cli.NodeNotExist(srcDir) {
		// Nothing to compile.
		return nil
	}

	c, err := toolchain.NewCompiler(b.compilerPkg.BasePath(),
		b.pkgBinDir(bpkg.Name()), "default") // XXX Use correct compiler def
	if err != nil {
		return err
	}
	c.AddInfo(b.compilerInfo)

	ci, err := bpkg.CompilerInfo(b)
	if err != nil {
		return err
	}
	c.AddInfo(ci)

	// Build the package source in two phases:
	// 1. Non-test code.
	// 2. Test code (if the "test" feature is enabled).
	//
	// This is done in two passes because the structure of
	// architecture-specific directories is different for normal code and test
	// code, and not easy to generalize into a single operation:
	//     * src/arch/<target-arch>
	//     * src/test/arch/<target-arch>
	if err = buildDir(srcDir, c, b.target.Arch, []string{"test"}); err != nil {
		return err
	}
	if b.features["test"] {
		testSrcDir := srcDir + "/test"
		if err = buildDir(testSrcDir, c, b.target.Arch, nil); err != nil {
			return err
		}
	}

	// Create a static library ("archive").
	if err := os.Chdir(bpkg.BasePath() + "/"); err != nil {
		return util.NewNewtError(err.Error())
	}
	archiveFile := b.archivePath(bpkg.Name())
	if err = c.CompileArchive(archiveFile); err != nil {
		return err
	}

	return nil
}

func (b *Builder) link(elfName string) error {
	c, err := toolchain.NewCompiler(b.compilerPkg.BasePath(),
		b.pkgBinDir(elfName), "default") // XXX
	if err != nil {
		return err
	}

	pkgNames := []string{}
	for _, bpkg := range b.Packages {
		archivePath := b.archivePath(bpkg.Name())
		if cli.NodeExist(archivePath) {
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

// Populates the builder with all the packages that need to be built, and
// configures each package's build settings.  After this function executes,
// packages are ready to be built.
func (b *Builder) PrepBuild() error {
	// Collect the seed packages.
	bspPkg := b.target.Bsp()
	if bspPkg == nil {
		return util.NewNewtError("bsp package not specified")
	}

	compilerPkg := b.target.Compiler()
	if compilerPkg == nil {
		return util.NewNewtError("compiler package not specified")
	}

	// An app package is not required (e.g., unit tests).
	appPkg := b.target.App()

	// Seed the builder with the app (if present), bsp, compiler, and target
	// packages.

	var appBpkg *BuildPackage
	if appPkg != nil {
		appBpkg = b.Packages[appPkg]
		if appBpkg == nil {
			appBpkg = b.AddPackage(appPkg)
		}
	}

	bspBpkg := b.Packages[bspPkg]
	if bspBpkg == nil {
		bspBpkg = b.AddPackage(bspPkg)
	}

	compilerBpkg := b.Packages[compilerPkg]
	if compilerBpkg == nil {
		compilerBpkg = b.AddPackage(compilerPkg)
	}

	targetBpkg := b.AddPackage(b.target.Package())

	// Populate the full set of packages to be built and resolve the feature
	// set.
	if err := b.loadDeps(); err != nil {
		return err
	}

	// Terminate if any package has an unmet API requirement.
	if err := b.verifyApisSatisfied(); err != nil {
		return err
	}

	// Populate the base set of compiler flags.  Flags from the following
	// packages get applied to every source file:
	//     * app (if present)
	//     * bsp
	//     * compiler
	//     * target

	baseCi := toolchain.NewCompilerInfo()

	if appBpkg != nil {
		appCi, err := appBpkg.CompilerInfo(b)
		if err != nil {
			return err
		}
		baseCi.AddCompilerInfo(appCi)
	}

	bspCi, err := bspBpkg.CompilerInfo(b)
	if err != nil {
		return err
	}
	baseCi.AddCompilerInfo(bspCi)

	targetCi, err := targetBpkg.CompilerInfo(b)
	if err != nil {
		return err
	}
	baseCi.AddCompilerInfo(targetCi)

	// Read the BSP configuration.  These settings are necessary for the link
	// step.
	b.Bsp = pkg.NewBspPackage(bspPkg, b.Features())

	b.compilerPkg = compilerPkg
	b.compilerInfo = baseCi

	return nil
}

func (b *Builder) Build() error {
	if b.target.App() == nil {
		fmt.Println(b.target)
		return util.NewNewtError("app package not specified")
	}

	// Populate the package and feature sets and calculate the base compiler
	// flags.
	err := b.PrepBuild()
	if err != nil {
		return err
	}

	// XXX: If any yml files have changed, a full rebuild is required.  We
	// don't currently check this.

	for _, bpkg := range b.Packages {
		err = b.buildPackage(bpkg)
		if err != nil {
			return err
		}
	}

	err = b.link(b.AppElfPath())
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) Test(p *pkg.LocalPackage) error {
	// Seed the builder with the package under test.
	testBpkg := b.AddPackage(p)

	// A few features are automatically supported when the test command is
	// used:
	//     * test:      ensures that the test code gets compiled.
	//     * selftest:  indicates that there is no app.
	b.AddFeature("test")
	b.AddFeature("selftest")

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
	testPkgCi.Cflags = append(testPkgCi.Cflags, "-DPKG_TEST")

	// XXX: If any yml files have changed, a full rebuild is required.  We
	// don't currently check this.

	for _, bpkg := range b.Packages {
		err = b.buildPackage(bpkg)
		if err != nil {
			return err
		}
	}

	testFilename := b.testExePath(p.Name())
	err = b.link(testFilename)
	if err != nil {
		return err
	}

	// Run the tests.
	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Testing package %s\n", p.Name())

	if err := os.Chdir(filepath.Dir(testFilename)); err != nil {
		return err
	}

	o, err := cli.ShellCommand(testFilename)
	if err != nil {
		cli.StatusMessage(cli.VERBOSITY_DEFAULT, "%s", string(o))

		// Always terminate on test failure since only one test is being run.
		return util.NewtErrorNoTrace("Test failed: " + testFilename)
	}

	cli.StatusMessage(cli.VERBOSITY_VERBOSE, "%s", string(o))
	cli.StatusMessage(cli.VERBOSITY_DEFAULT, "Test %s ok!\n", testFilename)

	return nil
}

func (b *Builder) Clean() error {
	path := b.binDir()
	cli.StatusMessage(util.VERBOSITY_VERBOSE, "Cleaning directory %s\n", path)
	err := os.RemoveAll(path)
	return err
}
