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
	"bufio"
	"fmt"
	"io/ioutil"
	"mynewt.apache.org/newt/newt/cfgv"
	"os"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/apache/mynewt-artifact/flash"
	"github.com/apache/mynewt-artifact/sec"
	"mynewt.apache.org/newt/newt/flashmap"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/resolve"
	"mynewt.apache.org/newt/newt/symbol"
	"mynewt.apache.org/newt/newt/syscfg"
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

	keyFile          string
	injectedSettings *cfgv.Settings

	res *resolve.Resolution
}

func NewTargetTester(target *target.Target,
	testPkg *pkg.LocalPackage) (*TargetBuilder, error) {
	if err := target.Validate(testPkg == nil); err != nil {
		return nil, err
	}

	bspPkg, err := pkg.NewBspPackage(target.Bsp(), target.GetBspYCfgOverride())
	if err != nil {
		return nil, err
	}

	compilerName := bspPkg.CompilerName

	compilerPkg, err := project.GetProject().ResolvePackage(
		bspPkg.Repo(), compilerName)
	if err != nil {
		return nil, err
	}

	t := &TargetBuilder{
		target:           target,
		bspPkg:           bspPkg,
		compilerPkg:      compilerPkg,
		appPkg:           target.App(),
		loaderPkg:        target.Loader(),
		keyFile:          target.KeyFile,
		testPkg:          testPkg,
		injectedSettings: cfgv.NewSettings(nil),
	}

	if err := t.ensureResolved(false); err != nil {
		return nil, err
	}

	if err := t.bspPkg.Reload(t.res.Cfg.SettingValues()); err != nil {
		return nil, err
	}

	t.res.Cfg.DetectErrors(bspPkg.FlashMap)

	return t, nil
}

func NewTargetBuilder(target *target.Target) (*TargetBuilder, error) {
	return NewTargetTester(target, nil)
}

func (t *TargetBuilder) BspPkg() *pkg.BspPackage {
	return t.bspPkg
}

func (t *TargetBuilder) NewCompiler(dstDir string, buildProfile string) (
	*toolchain.Compiler, error) {

	if buildProfile == "" {
		buildProfile = t.target.BuildProfile
	}

	var cfg *cfgv.Settings
	cfg = nil

	if t.AppBuilder != nil {
		cfg = t.AppBuilder.cfg.SettingValues()
	}

	c, err := toolchain.NewCompiler(
		t.compilerPkg.BasePath(), dstDir, buildProfile, cfg)

	return c, err
}

func (t *TargetBuilder) injectNewtSettings() {
	// Indicate that this version of newt supports the generated logcfg header.
	t.InjectSetting("NEWT_FEATURE_LOGCFG", "1")

	// Indicate to the apache-mynewt-core code that this version of newt
	// supports the sysdown mechanism (generated package shutdown functions).
	t.InjectSetting("NEWT_FEATURE_SYSDOWN", "1")
}

func (t *TargetBuilder) injectBuildSettings() {
	t.InjectSetting("ARCH_NAME", "\""+t.bspPkg.Arch+"\"")
	t.InjectSetting("ARCH_"+util.CIdentifier(t.bspPkg.Arch), "1")

	if t.appPkg != nil {
		appName := filepath.Base(t.appPkg.Name())
		t.InjectSetting("APP_NAME", "\""+appName+"\"")
		t.InjectSetting("APP_"+util.CIdentifier(appName), "1")
	}

	bspName := filepath.Base(t.bspPkg.Name())
	t.InjectSetting("BSP_NAME", "\""+bspName+"\"")
	t.InjectSetting("BSP_"+util.CIdentifier(bspName), "1")

	tgtName := filepath.Base(t.target.Name())
	t.InjectSetting("TARGET_NAME", "\""+tgtName+"\"")
	t.InjectSetting("TARGET_"+util.CIdentifier(tgtName), "1")
}

// resolveTransientPkgs replaces packages in a slice with the packages they
// link to.  It has no effect on non-transient packages.
func (t *TargetBuilder) resolveTransientPkgs(lps []*pkg.LocalPackage) {
	for i, lp := range lps {
		resolved := t.target.ResolvePackageName(lp.FullName())
		if resolved != lp {
			resolve.LogTransientWarning(lp)
			lps[i] = resolved
		}
	}
}

func (t *TargetBuilder) ensureResolved(detectErr bool) error {
	if t.res != nil {
		return nil
	}

	t.injectNewtSettings()
	t.injectBuildSettings()

	// When populating seed lists, resolve transient packages as a separate
	// step (i.e., fill the list with the "Yml()" version of each package, then
	// call `resolveTransientPkgs` on the list).  This is done so that we can
	// detect and warn the user if transient packages are being used.

	var loaderSeeds []*pkg.LocalPackage
	if t.loaderPkg != nil {
		loaderSeeds = []*pkg.LocalPackage{
			t.target.LoaderYml(),
			t.target.BspYml(),
			t.compilerPkg,
			t.target.Package(),
		}
		t.resolveTransientPkgs(loaderSeeds)

		// For split images, inject the SPLIT_[...] settings into the
		// corresponding app packages.  This ensures that:
		//     * The app packages know they are part of a split image during
		//       dependency resolution.
		//     * The app source files receive "-DSPLIT_[...]=1" command line
		//       arguments during compilation.
		t.loaderPkg.InjectedSettings().Set("SPLIT_LOADER", "1")
		if t.appPkg != nil {
			t.appPkg.InjectedSettings().Set("SPLIT_APPLICATION", "1")
		}

		// Inject the SPLIT_IMAGE setting into the entire target.  All packages
		// now know that they are part of a split image build.
		t.InjectSetting("SPLIT_IMAGE", "1")
	}

	appSeeds := []*pkg.LocalPackage{
		t.target.BspYml(),
		t.compilerPkg,
		t.target.Package(),
	}
	t.resolveTransientPkgs(appSeeds)

	if t.appPkg != nil {
		appSeeds = append(appSeeds, t.target.AppYml())
	}

	if t.testPkg != nil {
		// A few features are automatically supported when the test command is
		// used:
		//     * TEST:      lets packages know that this is a test app
		//     * SELFTEST:  indicates that the "newt test" command is used;
		//                  causes a package to define a main() function.
		t.InjectSetting("TEST", "1")
		t.InjectSetting("SELFTEST", "1")

		appSeeds = append(appSeeds, t.testPkg)
	}

	var err error
	t.res, err = resolve.ResolveFull(
		loaderSeeds, appSeeds, t.injectedSettings, t.bspPkg.FlashMap, detectErr)
	if err != nil {
		return err
	}

	// Configure the basic set of environment variables in the current process.
	env := BasicEnvVars("", t.bspPkg)
	keys := make([]string, 0, len(env))
	for k, _ := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	log.Debugf("exporting environment variables:")
	for _, k := range keys {
		v := env[k]
		log.Debugf("    %s=%s", k, env[k])

		err := os.Setenv(k, v)
		if err != nil {
			return util.FmtNewtError(
				"failed to set env var %s=%s: %s", k, v, err.Error())
		}
	}

	return nil
}

func (t *TargetBuilder) Resolve() (*resolve.Resolution, error) {
	if err := t.ensureResolved(true); err != nil {
		return nil, err
	}

	return t.res, nil
}

func (t *TargetBuilder) validateAndWriteCfg() error {
	if err := t.ensureResolved(true); err != nil {
		return err
	}

	if errText := t.res.ErrorText(); errText != "" {
		return util.NewNewtError(errText)
	}

	warningText := strings.TrimSpace(t.res.WarningText())
	if warningText != "" {
		log.Debug(warningText)
	}

	for _, line := range t.res.DeprecatedWarning() {
		log.Warn(line)
	}

	for _, line := range t.res.CfgExperimentalWarning() {
		log.Warn(line)
	}

	for _, line := range t.res.PkgExperimentalWarning() {
		log.Warn(line)
	}

	incDir := GeneratedIncludeDir(t.target.FullName())
	srcDir := GeneratedSrcDir(t.target.FullName())

	lpkgs := resolve.RpkgSliceToLpkgSlice(t.res.MasterSet.Rpkgs)
	apis := []string{}
	for api := range t.res.ApiMap {
		apis = append(apis, api)
	}
	if err := syscfg.EnsureWritten(t.res.Cfg, incDir, lpkgs, apis); err != nil {
		return err
	}

	if err := t.res.LCfg.EnsureWritten(incDir, srcDir, pkg.ShortName(t.target.Package())); err != nil {
		return err
	}

	// Generate loader sysinit.
	if t.res.LoaderSet != nil {
		lpkgs = resolve.RpkgSliceToLpkgSlice(t.res.LoaderSet.Rpkgs)
		if err := t.res.SysinitCfg.EnsureWritten(lpkgs, srcDir,
			pkg.ShortName(t.target.Package()), true); err != nil {

			return err
		}
	}

	// Generate app sysinit.
	lpkgs = resolve.RpkgSliceToLpkgSlice(t.res.AppSet.Rpkgs)
	if err := t.res.SysinitCfg.EnsureWritten(lpkgs, srcDir,
		pkg.ShortName(t.target.Package()), false); err != nil {

		return err
	}

	// Generate loader sysinit.
	if t.res.LoaderSet != nil {
		lpkgs = resolve.RpkgSliceToLpkgSlice(t.res.LoaderSet.Rpkgs)
		if err := t.res.SysdownCfg.EnsureWritten(lpkgs, srcDir,
			pkg.ShortName(t.target.Package()), true); err != nil {

			return err
		}
	}

	// XXX: Generate loader sysdown.

	// Generate app sysdown.
	lpkgs = resolve.RpkgSliceToLpkgSlice(t.res.AppSet.Rpkgs)
	if err := t.res.SysdownCfg.EnsureWritten(lpkgs, srcDir,
		pkg.ShortName(t.target.Package()), false); err != nil {

		return err
	}

	// Generate flash map.
	if err := flashmap.EnsureFlashMapWritten(
		t.bspPkg.FlashMap,
		srcDir,
		incDir,
		pkg.ShortName(t.target.Package())); err != nil {

		return err
	}

	return nil
}

// extraADirs returns a slice of extra directories that should be used at link
// time.  .a files in these directores are used as input to the link (in
// addition to .a files produced by building packages).
func (t *TargetBuilder) extraADirs() []string {
	return []string{
		// Artifacts generated by pre-link user scripts.
		UserPreLinkSrcDir(t.target.FullName()),
	}

	// Note: we don't include the pre-build source directory in this list.  At
	// compile time, newt copies .a files from package source directories to
	// their respective binary directories.  Since pre-link scripts run after
	// compile time, this copy never happens for pre-link artifacts, so we need
	// to tell newt where to look.
}

func (t *TargetBuilder) PrepBuild() error {
	if err := t.ensureResolved(true); err != nil {
		return err
	}

	flashErrText := t.bspPkg.FlashMap.ErrorText()
	if flashErrText != "" {
		return util.NewNewtError(flashErrText)
	}

	if err := t.validateAndWriteCfg(); err != nil {
		return err
	}

	// Create directories where user scripts can write artifacts to incorporate
	// into the build.

	err := os.MkdirAll(UserPreBuildSrcDir(t.target.FullName()), 0755)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	err = os.MkdirAll(UserPreBuildIncludeDir(t.target.FullName()), 0755)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	err = os.MkdirAll(UserPreLinkSrcDir(t.target.FullName()), 0755)
	if err != nil {
		return util.NewNewtError(err.Error())
	}

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

	return nil
}

func (t *TargetBuilder) buildLoader() error {
	/* Tentatively link the app (using the normal single image linker
	 * script)
	 */
	if err := t.AppBuilder.TentativeLink(t.bspPkg.LinkerScripts,
		t.extraADirs()); err != nil {

		return err
	}

	/* rebuild the loader */
	project.ResetDeps(t.LoaderList)

	if err := t.bspPkg.Reload(t.LoaderBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	if err := t.LoaderBuilder.Build(); err != nil {
		return err
	}

	/* Tentatively link the loader */
	if err := t.LoaderBuilder.TentativeLink(t.bspPkg.LinkerScripts,
		t.extraADirs()); err != nil {

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

/// Generates a .c source file with public key information required by the
/// bootloader.
///
/// The input filename should be supplied by the user in the target.yml file,
/// using the `target.key_file` option. This file can be either a private key
/// in PEM format, an extracted public key in PEM format or a DER file.
///
/// To extract a PEM public key from the private key:
///   `openssl ec -in ec_pk.pem -pubout -out pubkey.pub`
///   `openssl rsa -in rsa_pk.pem -RSAPublicKey_out -out pubkey.pub`
func (t *TargetBuilder) autogenKeys() error {
	keyBytes, err := ioutil.ReadFile(t.keyFile)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Error reading key file: %s", err))
	}

	var pubKey sec.PubSignKey
	privKey, err := sec.ParsePrivSignKey(keyBytes)
	if err != nil {
		pubKey, err = sec.ParsePubSignKey(keyBytes)
		if err != nil {
			return err
		}
	} else {
		pubKey = privKey.PubKey()
	}

	pubBytes, err := pubKey.Bytes()
	if err != nil {
		return err
	}

	srcDir := GeneratedSrcDir(t.target.FullName())

	f, _ := os.Create(srcDir + "/pubkey-autogen.c")
	w := bufio.NewWriter(f)

	fmt.Fprintln(w, "/* Autogenerated, do not edit. */")
	fmt.Fprintln(w, "#include <bootutil/sign_key.h>")
	fmt.Fprintf(w, "const unsigned char key[] = {")
	for count, b := range pubBytes {
		if count%8 == 0 {
			fmt.Fprintf(w, "\n    ")
		} else {
			fmt.Fprintf(w, " ")
		}
		fmt.Fprintf(w, "0x%02x,", b)
	}
	fmt.Fprintf(w, "\n};\n")
	fmt.Fprintf(w, "const unsigned int key_len = %v;\n", len(pubBytes))
	fmt.Fprintln(w, "const struct bootutil_key bootutil_keys[] = {")
	fmt.Fprintln(w, "    [0] = {")
	fmt.Fprintln(w, "        .key = key,")
	fmt.Fprintln(w, "        .len = &key_len,")
	fmt.Fprintln(w, "    },")
	fmt.Fprintln(w, "};")
	fmt.Fprintln(w, "const int bootutil_key_cnt = 1;")
	w.Flush()

	return nil
}

func (t *TargetBuilder) Build() error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	project.ResetDeps(t.AppList)

	if err := t.bspPkg.Reload(t.AppBuilder.cfg.SettingValues()); err != nil {
		return err
	}

	if t.keyFile != "" {
		err := t.autogenKeys()
		if err != nil {
			return err
		}
	}

	workDir, err := makeUserWorkDir()
	if err != nil {
		return err
	}
	defer func() {
		log.Debugf("removing user work dir: %s", workDir)
		os.RemoveAll(workDir)
	}()

	t.generateLinkTables()

	// Execute the set of pre-build user scripts.
	if err := t.execPreBuildCmds(workDir); err != nil {
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

	// Execute the set of pre-link user scripts.
	if err := t.execPreLinkCmds(workDir); err != nil {
		return err
	}

	/* Link the app. */
	if err := t.AppBuilder.Link(linkerScripts, t.extraADirs()); err != nil {
		return err
	}

	// Execute the set of post-build user scripts.
	if err := t.execPostLinkCmds(workDir); err != nil {
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
	for _, rpkg := range t.LoaderBuilder.SortedRpkgs() {
		log.Debugf("    * %s", rpkg.Lpkg.Name())
	}
	log.Debugf("App packages:")
	for _, rpkg := range t.AppBuilder.SortedRpkgs() {
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

	err = t.LoaderBuilder.KeepLink(t.bspPkg.LinkerScripts, preserveElf,
		t.extraADirs())
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
	t.injectedSettings.Set(key, value)
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
func (t *TargetBuilder) MaxImgSizes() []int {
	sz0 := t.bspPkg.FlashMap.Areas[flash.FLASH_AREA_NAME_IMAGE_0].Size
	sz1 := t.bspPkg.FlashMap.Areas[flash.FLASH_AREA_NAME_IMAGE_1].Size
	trailerSz := t.bootTrailerSize()

	return []int{
		sz0 - trailerSz,
		sz1 - trailerSz,
	}
}

func (t *TargetBuilder) CreateDepGraph() (DepGraph, error) {
	if err := t.ensureResolved(true); err != nil {
		return nil, err
	}

	return depGraph(t.res.MasterSet)
}

func (t *TargetBuilder) CreateRevdepGraph() (DepGraph, error) {
	if err := t.ensureResolved(true); err != nil {
		return nil, err
	}

	return revdepGraph(t.res.MasterSet)
}
