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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/flash"
	"mynewt.apache.org/newt/newt/image"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
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

	res *resolve.Resolution
}

func NewTargetTester(target *target.Target,
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

func NewTargetBuilder(target *target.Target) (*TargetBuilder, error) {
	return NewTargetTester(target, nil)
}

func (t *TargetBuilder) NewCompiler(dstDir string) (
	*toolchain.Compiler, error) {

	c, err := toolchain.NewCompiler(
		t.compilerPkg.BasePath(),
		dstDir,
		t.target.BuildProfile)

	return c, err
}

func (t *TargetBuilder) ensureResolved() error {
	if t.res != nil {
		return nil
	}

	var loaderSeeds []*pkg.LocalPackage
	if t.loaderPkg != nil {
		loaderSeeds = []*pkg.LocalPackage{
			t.loaderPkg,
			t.bspPkg.LocalPackage,
			t.compilerPkg,
			t.target.Package(),
		}

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
	}

	appSeeds := []*pkg.LocalPackage{
		t.bspPkg.LocalPackage,
		t.compilerPkg,
		t.target.Package(),
	}

	if t.appPkg != nil {
		appSeeds = append(appSeeds, t.appPkg)
	}

	if t.testPkg != nil {
		// A few features are automatically supported when the test command is
		// used:
		//     * TEST:      lets packages know that this is a test app
		//     * SELFTEST:  indicates that the "newt test" command is used;
		//                  causes a package to define a main() function.
		t.injectedSettings["TEST"] = "1"
		t.injectedSettings["SELFTEST"] = "1"

		appSeeds = append(appSeeds, t.testPkg)
	}

	var err error
	t.res, err = resolve.ResolveFull(
		loaderSeeds, appSeeds, t.injectedSettings, t.bspPkg.FlashMap)
	if err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) Resolve() (*resolve.Resolution, error) {
	if err := t.ensureResolved(); err != nil {
		return nil, err
	}

	return t.res, nil
}

func (t *TargetBuilder) validateAndWriteCfg() error {
	if err := t.ensureResolved(); err != nil {
		return err
	}

	if errText := t.res.ErrorText(); errText != "" {
		return util.NewNewtError(errText)
	}

	warningText := strings.TrimSpace(t.res.WarningText())
	if warningText != "" {
		for _, line := range strings.Split(warningText, "\n") {
			log.Debugf(line)
		}
	}

	if err := syscfg.EnsureWritten(t.res.Cfg,
		GeneratedIncludeDir(t.target.Name())); err != nil {

		return err
	}

	return nil
}

func (t *TargetBuilder) generateSysinit() error {
	if err := t.ensureResolved(); err != nil {
		return err
	}

	srcDir := GeneratedSrcDir(t.target.Name())

	if t.res.LoaderSet != nil {
		lpkgs := resolve.RpkgSliceToLpkgSlice(t.res.LoaderSet.Rpkgs)
		sysinit.EnsureWritten(lpkgs, srcDir,
			pkg.ShortName(t.target.Package()), true)
	}

	lpkgs := resolve.RpkgSliceToLpkgSlice(t.res.AppSet.Rpkgs)
	sysinit.EnsureWritten(lpkgs, srcDir,
		pkg.ShortName(t.target.Package()), false)

	return nil
}

func (t *TargetBuilder) generateFlashMap() error {
	return t.bspPkg.FlashMap.EnsureWritten(
		GeneratedSrcDir(t.target.Name()),
		GeneratedIncludeDir(t.target.Name()),
		pkg.ShortName(t.target.Package()))
}

func (t *TargetBuilder) generateCode() error {
	if err := t.generateSysinit(); err != nil {
		return err
	}

	if err := t.generateFlashMap(); err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) PrepBuild() error {
	if err := t.ensureResolved(); err != nil {
		return err
	}

	flashErrText := t.bspPkg.FlashMap.ErrorText()
	if flashErrText != "" {
		return util.NewNewtError(flashErrText)
	}

	if err := t.validateAndWriteCfg(); err != nil {
		return err
	}

	var err error
	if t.res.LoaderSet != nil {
		t.LoaderBuilder, err = NewBuilder(t, BUILD_NAME_LOADER,
			t.res.LoaderSet.Rpkgs, t.res.ApiMap, t.res.Cfg)
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

	t.AppBuilder, err = NewBuilder(t, BUILD_NAME_APP, t.res.AppSet.Rpkgs,
		t.res.ApiMap, t.res.Cfg)
	if err != nil {
		return err
	}
	if err := t.AppBuilder.PrepBuild(); err != nil {
		return err
	}

	if t.res.LoaderSet != nil {
		appFlags := toolchain.NewCompilerInfo()
		appFlags.Cflags = append(appFlags.Cflags, "-DSPLIT_APPLICATION")
		t.AppBuilder.AddCompilerInfo(appFlags)
	}

	t.AppList = project.ResetDeps(nil)

	logDepInfo(t.res)

	if err := t.generateCode(); err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) buildLoader() error {
	/* Tentatively link the app (using the normal single image linker script) */
	if err := t.AppBuilder.TentativeLink(t.bspPkg.LinkerScripts); err != nil {
		return err
	}

	/* rebuild the loader */
	project.ResetDeps(t.LoaderList)

	if err := t.bspPkg.Reload(t.LoaderBuilder.cfg.Features()); err != nil {
		return err
	}

	if err := t.LoaderBuilder.Build(); err != nil {
		return err
	}

	/* Tentatively link the loader */
	if err := t.LoaderBuilder.TentativeLink(t.bspPkg.LinkerScripts); err != nil {
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

	return nil

}

func (t *TargetBuilder) Build() error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	/* Build the Apps */
	project.ResetDeps(t.AppList)

	if err := t.bspPkg.Reload(t.AppBuilder.cfg.Features()); err != nil {
		return err
	}

	if err := t.AppBuilder.Build(); err != nil {
		return err
	}

	var linkerScripts []string
	if t.LoaderBuilder == nil {
		linkerScripts = t.bspPkg.LinkerScripts
	} else {
		if err := t.buildLoader(); err != nil {
			return err
		}
		linkerScripts = t.bspPkg.Part2LinkerScripts
	}

	/* Link the app. */
	if err := t.AppBuilder.Link(linkerScripts); err != nil {
		return err
	}

	/* Create manifest. */
	if err := t.createManifest(); err != nil {
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
	for _, rpkg := range t.LoaderBuilder.sortedRpkgs() {
		log.Debugf("    * %s", rpkg.Lpkg.Name())
	}
	log.Debugf("App packages:")
	for _, rpkg := range t.AppBuilder.sortedRpkgs() {
		log.Debugf("    * %s", rpkg.Lpkg.Name())
	}
	err, appLibSym := t.AppBuilder.ExtractSymbolInfo()
	if err != nil {
		return err, nil, nil
	}

	/* fetch the symbol list from the app temporary elf */
	err, appElfSym := t.AppBuilder.ParseObjectElf(t.AppBuilder.AppTentativeElfPath())
	if err != nil {
		return err, nil, nil
	}

	/* extract the library symbols and elf symbols from the loader */
	err, loaderLibSym := t.LoaderBuilder.ExtractSymbolInfo()
	if err != nil {
		return err, nil, nil
	}

	err, loaderElfSym := t.LoaderBuilder.ParseObjectElf(
		t.LoaderBuilder.AppTentativeElfPath())
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
	delete(commonPkgs, t.AppBuilder.appPkg.rpkg.Lpkg.Name())
	uncommonPkgs[t.AppBuilder.appPkg.rpkg.Lpkg.Name()] = true
	ma := smMatch.FilterPkg(t.AppBuilder.appPkg.rpkg.Lpkg.Name())
	smMatch.RemoveMap(ma)

	delete(commonPkgs, t.LoaderBuilder.appPkg.rpkg.Lpkg.Name())
	uncommonPkgs[t.LoaderBuilder.appPkg.rpkg.Lpkg.Name()] = true
	ml := smMatch.FilterPkg(t.LoaderBuilder.appPkg.rpkg.Lpkg.Name())
	smMatch.RemoveMap(ml)

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Putting %d symbols from %d packages into loader\n",
		len(*smMatch), len(commonPkgs))

	var badpkgs []string
	var symbolStr string
	for v, _ := range uncommonPkgs {
		if t.AppBuilder.appPkg != nil &&
			t.AppBuilder.appPkg.rpkg.Lpkg.Name() != v &&
			t.LoaderBuilder.appPkg != nil &&
			t.LoaderBuilder.appPkg.rpkg.Lpkg.Name() != v {

			trouble := smNomatch.FilterPkg(v)

			var found bool
			for _, sym := range *trouble {
				if !sym.IsLocal() {
					found = true
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
			"Common packages with different implementation\n %s\n",
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

	err = t.LoaderBuilder.KeepLink(t.bspPkg.LinkerScripts, preserveElf)

	if err != nil {
		return err, nil, nil
	}
	return err, commonPkgs, smMatch
}

func (t *TargetBuilder) GetTarget() *target.Target {
	return t.target
}

func (t *TargetBuilder) GetTestPkg() *pkg.LocalPackage {
	return t.testPkg
}

func (t *TargetBuilder) InjectSetting(key string, value string) {
	t.injectedSettings[key] = value
}

func readManifest(path string) (*image.ImageManifest, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, util.ChildNewtError(err)
	}

	manifest := &image.ImageManifest{}
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, util.FmtNewtError(
			"Failure decoding manifest with path \"%s\": %s", err.Error())
	}

	return manifest, nil
}

func (t *TargetBuilder) createManifest() error {
	manifest := &image.ImageManifest{
		Date: time.Now().Format(time.RFC3339),
		Name: t.GetTarget().FullName(),
	}

	rm := image.NewRepoManager()
	for _, rpkg := range t.AppBuilder.sortedRpkgs() {
		manifest.Pkgs = append(manifest.Pkgs,
			rm.GetImageManifestPkg(rpkg.Lpkg))
	}

	if t.LoaderBuilder != nil {
		for _, rpkg := range t.LoaderBuilder.sortedRpkgs() {
			manifest.LoaderPkgs = append(manifest.LoaderPkgs,
				rm.GetImageManifestPkg(rpkg.Lpkg))
		}
	}

	manifest.Repos = rm.AllRepos()

	vars := t.GetTarget().Vars
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		manifest.TgtVars = append(manifest.TgtVars, k+"="+vars[k])
	}
	syscfgKV := t.GetTarget().Package().SyscfgV.GetStringMapString("syscfg.vals")
	if len(syscfgKV) > 0 {
		tgtSyscfg := fmt.Sprintf("target.syscfg=%s",
			syscfg.KeyValueToStr(syscfgKV))
		manifest.TgtVars = append(manifest.TgtVars, tgtSyscfg)
	}

	c, err := t.AppBuilder.PkgSizes()
	if err == nil {
		manifest.PkgSizes = c.Pkgs
	}
	if t.LoaderBuilder != nil {
		c, err = t.LoaderBuilder.PkgSizes()
		if err == nil {
			manifest.LoaderPkgSizes = c.Pkgs
		}
	}
	file, err := os.Create(t.AppBuilder.ManifestPath())
	if err != nil {
		return util.FmtNewtError("Cannot create manifest file %s: %s",
			t.AppBuilder.ManifestPath(), err.Error())
	}
	defer file.Close()

	buffer, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return util.FmtNewtError("Cannot encode manifest: %s", err.Error())
	}
	_, err = file.Write(buffer)
	if err != nil {
		return util.FmtNewtError("Cannot write manifest file: %s",
			err.Error())
	}

	return nil
}

// Reads an existing manifest file and augments it with image fields:
//     * Image version
//     * App image path
//     * App image hash
//     * Loader image path
//     * Loader image hash
//     * Build ID
func (t *TargetBuilder) augmentManifest(
	appImg *image.Image,
	loaderImg *image.Image,
	buildId []byte) error {

	manifest, err := readManifest(t.AppBuilder.ManifestPath())
	if err != nil {
		return err
	}

	manifest.Version = appImg.Version.String()
	manifest.ImageHash = fmt.Sprintf("%x", appImg.Hash)
	manifest.Image = filepath.Base(appImg.TargetImg)

	if loaderImg != nil {
		manifest.Loader = filepath.Base(loaderImg.TargetImg)
		manifest.LoaderHash = fmt.Sprintf("%x", loaderImg.Hash)
	}

	manifest.BuildID = fmt.Sprintf("%x", buildId)

	file, err := os.Create(t.AppBuilder.ManifestPath())
	if err != nil {
		return util.FmtNewtError("Cannot create manifest file %s: %s",
			t.AppBuilder.ManifestPath(), err.Error())
	}
	defer file.Close()

	buffer, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return util.FmtNewtError("Cannot encode manifest: %s", err.Error())
	}
	_, err = file.Write(buffer)
	if err != nil {
		return util.FmtNewtError("Cannot write manifest file: %s",
			err.Error())
	}

	return nil
}

// Calculates the size of a single boot trailer.  This is the amount of flash
// that must be reserved at the end of each image slot.
func (t *TargetBuilder) bootTrailerSize() int {
	var minWriteSz int

	entry, ok := t.res.Cfg.Settings["MCU_FLASH_MIN_WRITE_SIZE"]
	if !ok {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"* Warning: target does not define MCU_FLASH_MIN_WRITE_SIZE "+
				"setting; assuming a value of 1.\n")
		minWriteSz = 1
	} else {
		val, err := util.AtoiNoOct(entry.Value)
		if err != nil {
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"* Warning: target specifies invalid non-integer "+
					"MCU_FLASH_MIN_WRITE_SIZE setting; assuming a "+
					"value of 1.\n")
			minWriteSz = 1
		} else {
			minWriteSz = val
		}
	}

	/* Mynewt boot trailer format:
	 *
	 *  0                   1                   2                   3
	 *  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * ~                       MAGIC (16 octets)                       ~
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * ~                                                               ~
	 * ~             Swap status (128 * min-write-size * 3)            ~
	 * ~                                                               ~
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |   Copy done   |     0xff padding (up to min-write-sz - 1)     |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 * |   Image OK    |     0xff padding (up to min-write-sz - 1)     |
	 * +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	 */

	tsize := 16 + // Magic.
		128*minWriteSz*3 + // Swap status.
		minWriteSz + // Copy done.
		minWriteSz // Image Ok.

	log.Debugf("Min-write-size=%d; boot-trailer-size=%d", minWriteSz, tsize)

	return tsize
}

// Calculates the size of the largest image that can be written to each image
// slot.
func (t *TargetBuilder) maxImgSizes() []int {
	sz0 := t.bspPkg.FlashMap.Areas[flash.FLASH_AREA_NAME_IMAGE_0].Size
	sz1 := t.bspPkg.FlashMap.Areas[flash.FLASH_AREA_NAME_IMAGE_1].Size
	trailerSz := t.bootTrailerSize()

	return []int{
		sz0 - trailerSz,
		sz1 - trailerSz,
	}
}

// Verifies that each already-built image leaves enough room for a boot trailer
// a the end of its slot.
func (t *TargetBuilder) verifyImgSizes(li *image.Image, ai *image.Image) error {
	maxSizes := t.maxImgSizes()

	errLines := []string{}
	if li != nil {
		if overflow := int(li.TotalSize) - maxSizes[0]; overflow > 0 {
			errLines = append(errLines,
				fmt.Sprintf("loader overflows slot-0 by %d bytes "+
					"(image=%d max=%d)",
					overflow, li.TotalSize, maxSizes[0]))
		}
		if overflow := int(ai.TotalSize) - maxSizes[1]; overflow > 0 {
			errLines = append(errLines,
				fmt.Sprintf("app overflows slot-1 by %d bytes "+
					"(image=%d max=%d)",
					overflow, ai.TotalSize, maxSizes[1]))

		}
	} else {
		if overflow := int(ai.TotalSize) - maxSizes[0]; overflow > 0 {
			errLines = append(errLines,
				fmt.Sprintf("app overflows slot-0 by %d bytes "+
					"(image=%d max=%d)",
					overflow, ai.TotalSize, maxSizes[0]))
		}
	}

	if len(errLines) > 0 {
		if !newtutil.NewtForce {
			return util.NewNewtError(strings.Join(errLines, "; "))
		} else {
			for _, e := range errLines {
				util.StatusMessage(util.VERBOSITY_QUIET,
					"* Warning: %s (ignoring due to force flag)\n", e)
			}
		}
	}

	return nil
}

// @return                      app-image, loader-image, error
func (t *TargetBuilder) CreateImages(version string,
	keystr string, keyId uint8) (*image.Image, *image.Image, error) {

	if err := t.Build(); err != nil {
		return nil, nil, err
	}

	var err error
	var appImg *image.Image
	var loaderImg *image.Image

	c, err := t.NewCompiler("")
	if err != nil {
		return nil, nil, err
	}

	if t.LoaderBuilder != nil {
		loaderImg, err = t.LoaderBuilder.CreateImage(version, keystr, keyId,
			nil)
		if err != nil {
			return nil, nil, err
		}
		tgtArea := t.bspPkg.FlashMap.Areas[flash.FLASH_AREA_NAME_IMAGE_0]
		log.Debugf("Convert %s -> %s at offset 0x%x",
			t.LoaderBuilder.AppImgPath(),
			t.LoaderBuilder.AppHexPath(),
			tgtArea.Offset)
		err = c.ConvertBinToHex(t.LoaderBuilder.AppImgPath(),
				t.LoaderBuilder.AppHexPath(), tgtArea.Offset)
		if err != nil {
			log.Errorf("Can't convert to hexfile %s\n", err.Error())
		}
	}

	appImg, err = t.AppBuilder.CreateImage(version, keystr, keyId, loaderImg)
	if err != nil {
		return nil, nil, err
	}

	flashTargetArea := ""
	if t.LoaderBuilder == nil {
		flashTargetArea = flash.FLASH_AREA_NAME_IMAGE_0
	} else  {
		flashTargetArea = flash.FLASH_AREA_NAME_IMAGE_1
	}
	tgtArea := t.bspPkg.FlashMap.Areas[flashTargetArea]
	if tgtArea.Name != "" {
		log.Debugf("Convert %s -> %s at offset 0x%x",
			t.AppBuilder.AppImgPath(),
			t.AppBuilder.AppHexPath(),
			tgtArea.Offset)
		err = c.ConvertBinToHex(t.AppBuilder.AppImgPath(),
				t.AppBuilder.AppHexPath(), tgtArea.Offset)
		if err != nil {
			log.Errorf("Can't convert to hexfile %s\n", err.Error())
		}
	}
	buildId := image.CreateBuildId(appImg, loaderImg)
	if err := t.augmentManifest(appImg, loaderImg, buildId); err != nil {
		return nil, nil, err
	}

	if err := t.verifyImgSizes(loaderImg, appImg); err != nil {
		return nil, nil, err
	}

	return appImg, loaderImg, nil
}

func (t *TargetBuilder) CreateDepGraph() (DepGraph, error) {
	if err := t.ensureResolved(); err != nil {
		return nil, err
	}

	return depGraph(t.res.MasterSet)
}

func (t *TargetBuilder) CreateRevdepGraph() (DepGraph, error) {
	if err := t.ensureResolved(); err != nil {
		return nil, err
	}

	return revdepGraph(t.res.MasterSet)
}
