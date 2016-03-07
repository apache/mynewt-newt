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
	"bytes"
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
	features   map[string]bool
	apis       map[string]*BuildPackage

	target *target.Target
}

func (b *Builder) Features() map[string]bool {
	return b.features
}

func (b *Builder) AddFeature(feature string) {
	b.features[feature] = true
}

func (b *Builder) AddPackage(pkg *pkg.LocalPackage) {
	// Don't allow nil entries to the map
	if pkg == nil {
		panic("Cannot add nil package builder map")
	}

	b.Packages[pkg] = NewBuildPackage(pkg)
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
	return nil
}

// Makes sure all packages with required APIs have been augmented a dependency
// which satisfies that requirement.  If there are any unsatisfied
// requirements, an error is returned.
func (b *Builder) verifyApisSatisfied() error {
	unsatisfied := map[*BuildPackage][]string{}

	for _, bpkg := range b.Packages {
		for api, status := range bpkg.reqApiMap {
			if status == REQ_API_STATUS_UNSATISFIED {
				slice := unsatisfied[bpkg]
				if slice == nil {
					unsatisfied[bpkg] = []string{api}
				} else {
					slice = append(slice, api)
				}
			}
		}
	}

	if len(unsatisfied) != 0 {
		var buffer bytes.Buffer
		for bpkg, apis := range unsatisfied {
			buffer.WriteString("Package " + bpkg.Name() +
				" has unsatisfied required APIs: ")
			for i, api := range apis {
				if i != 0 {
					buffer.WriteString(", ")
				}
				buffer.WriteString(api)
			}
			buffer.WriteString("\n")
		}
		return util.NewNewtError(buffer.String())
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

// Generates the path+filename of the specified package's .a file.
func (b *Builder) archivePath(bpkg *BuildPackage) string {
	binDir := bpkg.BasePath() + "/bin/" + b.target.Package().Name()
	archiveFile := binDir + "/lib" + filepath.Base(bpkg.Name()) + ".a"
	return archiveFile
}

// Compiles and archives a package.
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

	ci, err := bpkg.CompilerInfo(b)
	if err != nil {
		return err
	}
	c.AddInfo(ci)

	// For now, ignore test code.  Tests get built later if the test
	// feature is in effect.
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

	ci, err := appPackage.CompilerInfo(b)
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

	if err := b.loadDeps(); err != nil {
		return err
	}

	if err := b.verifyApisSatisfied(); err != nil {
		return err
	}

	bspPackage := b.Packages[b.target.Bsp()]
	if bspPackage == nil {
		return util.NewNewtError("BSP package not found!")
	}

	bspCi, err := bspPackage.CompilerInfo(b)
	if err != nil {
		return err
	}

	appPackage := b.Packages[b.target.App()]
	if appPackage == nil {
		return util.NewNewtError("App package not found")
	}
	appCi, err := appPackage.CompilerInfo(b)
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
	b.features = map[string]bool{}
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
