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
	Packages   map[*pkg.LocalPackage]*BuildPackage
	identities map[string]bool
	apis       map[string]*BuildPackage

	target *target.Target
}

func (b *Builder) Identities() map[string]bool {
	return b.identities
}

func (b *Builder) AddIdentity(identity string) {
	b.identities[identity] = true
}

func (b *Builder) AddPackage(pkg *pkg.LocalPackage) {
	// Don't allow nil entries to the map
	if pkg == nil {
		panic("Cannot add nil package builder map")
	}

	b.Packages[pkg] = NewBuildPackage(pkg)
}

func (b *Builder) AddApi(apiString string, bpkg *BuildPackage) {
	curBpkg := b.apis[apiString]
	if curBpkg != nil {
		cli.StatusMessage(util.VERBOSITY_QUIET,
			"Warning: API conflict: %s <-> %s\n", curBpkg.Name(), bpkg.Name())
	} else {
		b.apis[apiString] = bpkg
	}
}

func (b *Builder) populateApis() {
	// Satisfy API requirements.
	for _, bpkg := range b.Packages {
		for _, api := range bpkg.Apis() {
			b.AddApi(api, bpkg)
		}
	}
}

// Returns true if at least one new API was satisfied.
func (b *Builder) satisfyApis() (bool, error) {
	moreWork := false

	for _, bpkg := range b.Packages {
		for _, reqApi := range bpkg.ReqApis() {
			apiBpkg := b.apis[reqApi]
			if apiBpkg == nil {
				return false, util.NewNewtError(fmt.Sprintf("Could not "+
					"satisfy API requirement; package=%s api=%s",
					bpkg.Name(), reqApi))
			}

			dep := &pkg.Dependency{
				Name: apiBpkg.Name(),
				Repo: apiBpkg.Repo().Name(),
			}
			bpkg.AddDep(dep)
			bpkg.DelReqApi(reqApi)
			moreWork = true
		}
	}

	return moreWork, nil
}

func (b *Builder) loadDeps() error {
	for {
		reprocess := false
		for _, bpkg := range b.Packages {
			loaded, err := bpkg.Load(b)
			if err != nil {
				return err
			}

			if !loaded {
				reprocess = true
			}
		}

		if !reprocess {
			break
		}
	}
	return nil
}

func (b *Builder) unloadAllPackages() {
	for _, bpkg := range b.Packages {
		bpkg.loaded = false
	}
}

func (b *Builder) FinalizePackages() error {
	keepGoing := true
	for keepGoing {
		var err error

		keepGoing, err = b.satisfyApis()
		if err != nil {
			return err
		}

		if keepGoing {
			b.unloadAllPackages()
		}

		err = b.loadDeps()
		if err != nil {
			return err
		}
	}

	return nil
}

// Recursively compiles all the .c and .s files in the specified directory.
// Architecture-specific files are also compiled.
func buildDir(srcDir string, c *toolchain.Compiler, t *target.Target,
	ignDirs []string) error {

	var err error

	cli.StatusMessage(util.VERBOSITY_VERBOSE,
		"compiling src in base directory: %s\n", srcDir)

	// First change into the pkg src directory, and build all the objects
	// there
	err = os.Chdir(srcDir)
	if err != nil {
		return util.NewNewtError(err.Error())
	}

	// Don't recurse into destination directories.
	ignDirs = append(ignDirs, "obj")
	ignDirs = append(ignDirs, "bin")

	// Ignore architecture-specific source files for now.  Use a temporary
	// string array here so that the "arch" directory is not ignored in the
	// subsequent architecture-specific compile phase.
	baseIgnDirs := append(ignDirs, "arch")

	if err = c.RecursiveCompile(toolchain.COMPILER_TYPE_C,
		baseIgnDirs); err != nil {
		return err
	}

	archDir := srcDir + "/arch/" + t.Arch + "/"
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
		if err = c.RecursiveCompile(toolchain.COMPILER_TYPE_ASM,
			ignDirs); err != nil {
			return err
		}
	}

	return nil
}

func (b *Builder) archivePath(bpkg *BuildPackage) string {
	binDir := bpkg.BasePath() + "/bin/" + b.target.Package().Name()
	archiveFile := binDir + "/lib" + filepath.Base(bpkg.Name()) + ".a"
	return archiveFile
}

func (b *Builder) buildPackage(bpkg *BuildPackage,
	baseCi *toolchain.CompilerInfo, compilerPkg *pkg.LocalPackage) error {

	srcDir := bpkg.BasePath() + "/src"
	if cli.NodeNotExist(srcDir) {
		// Nothing to compile.
		return nil
	}

	c, err := toolchain.NewCompiler(compilerPkg.BasePath(),
		"default", // XXX
		b.target.Package().Name())
	if err != nil {
		return err
	}
	c.AddInfo(baseCi)

	ci, err := bpkg.FullCompilerInfo(b)
	if err != nil {
		return err
	}
	c.AddInfo(ci)

	// For now, ignore test code.  Tests get built later if the test
	// identity is in effect.
	ignDirs := []string{"test"}

	// Compile the source files.
	err = buildDir(srcDir, c, b.target, ignDirs)
	if err != nil {
		return err
	}

	// Archive everything into a static library, which can be linked with a
	// main program
	if err := os.Chdir(bpkg.BasePath() + "/"); err != nil {
		return util.NewNewtError(err.Error())
	}

	archiveFile := b.archivePath(bpkg)
	if err := os.MkdirAll(filepath.Dir(archiveFile), 0755); err != nil {
		return util.NewNewtError(err.Error())
	}
	if err = c.CompileArchive(archiveFile); err != nil {
		return err
	}

	return nil
}

func (b *Builder) linkApp(appPackage *BuildPackage,
	baseCi *toolchain.CompilerInfo, compilerPkg *pkg.LocalPackage) error {

	c, err := toolchain.NewCompiler(compilerPkg.BasePath(),
		"default", // XXX
		b.target.Package().Name())
	if err != nil {
		return err
	}

	c.AddInfo(baseCi)
	binDir := appPackage.BasePath() + "/bin/" + b.target.Package().Name()
	if cli.NodeNotExist(binDir) {
		os.MkdirAll(binDir, 0755)
	}

	ci, err := appPackage.FullCompilerInfo(b)
	if err != nil {
		return err
	}
	c.AddInfo(ci)

	pkgNames := []string{}
	for _, bpkg := range b.Packages {
		archivePath := b.archivePath(bpkg)
		if cli.NodeExist(archivePath) {
			pkgNames = append(pkgNames, archivePath)
		}
	}

	elfFile := binDir + "/" + appPackage.Name()
	err = c.CompileElf(elfFile, pkgNames)
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) Build() error {
	compilerPkg := b.target.Compiler()
	if compilerPkg == nil {
		return util.NewNewtError("compiler package not found")
	}

	b.AddPackage(b.target.Bsp())
	b.AddPackage(b.target.App())
	b.AddPackage(b.target.Compiler())

	if err := b.FinalizePackages(); err != nil {
		return err
	}

	bspPackage := b.Packages[b.target.Bsp()]
	if bspPackage == nil {
		return util.NewNewtError("BSP package not found!")
	}

	bspCi, err := bspPackage.FullCompilerInfo(b)
	if err != nil {
		return err
	}

	appPackage := b.Packages[b.target.App()]
	if appPackage == nil {
		return util.NewNewtError("App package not found")
	}
	appCi, err := appPackage.FullCompilerInfo(b)
	if err != nil {
		return err
	}

	baseCi := toolchain.NewCompilerInfo()
	baseCi.AddCompilerInfo(bspCi)
	baseCi.AddCompilerInfo(appCi)

	for _, bpkg := range b.Packages {
		err = b.buildPackage(bpkg, baseCi, compilerPkg)
		if err != nil {
			return err
		}
	}

	b.linkApp(appPackage, baseCi, compilerPkg)

	return nil
}

func (b *Builder) Init(target *target.Target) error {
	b.target = target

	b.Packages = map[*pkg.LocalPackage]*BuildPackage{}
	b.identities = map[string]bool{}
	b.apis = map[string]*BuildPackage{}

	return nil
}

func NewBuilder(target *target.Target) (*Builder, error) {
	b := &Builder{}

	if err := b.Init(target); err != nil {
		return nil, err
	}

	return b, nil
}
