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

package pkg

import (
	"path/filepath"
	"strings"

	"mynewt.apache.org/newt/newt/cfgv"
	"mynewt.apache.org/newt/newt/config"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

// Name of a target's local overlay file.  It is applied on top of the
// target's configuration and is intended to be kept out of version control.
const LOCAL_OVERLAY_FILENAME = "local.overlay.yml"

// If set, targets' local overlay files are not applied.
var LocalOverlayDisabled bool

// Overlays specified on the command line.  They are applied to all targets,
// after the local overlay files.
var cliOverlays []*Overlay

// Settings which identify a package and can't be changed by overlays.
var overlayProtectedKeys = []string{
	"pkg.name",
	"pkg.type",
}

// Key of the overlay section with settings of individual packages.
const OVERLAY_PKGS_KEY = "pkgs"

// Settings of individual packages which can be changed by an overlay.
var overlayPkgKeys = []string{
	"pkg.cflags",
	"pkg.cxxflags",
	"pkg.aflags",
}

// Overlay is a configuration file applied on top of a target's configuration
// files.  It can contain settings from all of them; each setting is applied
// to the file selected by the setting's prefix:
//
//	syscfg.*         syscfg.yml
//	target.*, bsp.*  target.yml
//	pkg.*, app.*     pkg.yml
//
// Additionally, the "pkgs" section can change compiler flags of individual
// packages included in the build:
//
//	pkgs:
//	    "@apache-mynewt-nimble/nimble/host":
//	        pkg.cflags:
//	            - -O0
//
// Overlays support the same operations as local.project.yml (OVERWRITE,
// REMOVE and DELETE suffixes).
type Overlay struct {
	lf *config.LocalFile

	// Settings of individual packages; package name -> settings.
	pkgs map[string]*config.LocalFile
}

// isKeyOrCond reports whether key is the specified setting or its
// conditional value (e.g., "pkg.cflags.FOO" for "pkg.cflags").
func isKeyOrCond(key string, setting string) bool {
	return key == setting || strings.HasPrefix(key, setting+".")
}

// overlayKeyFile returns the name of the target's configuration file which a
// setting in an overlay applies to, or "" if the setting is not recognized.
func overlayKeyFile(key string) string {
	prefix, _, _ := strings.Cut(key, ".")
	switch prefix {
	case "syscfg":
		return SYSCFG_YAML_FILENAME
	case "target", "bsp":
		return TARGET_YAML_FILENAME
	case "pkg", "app":
		return PACKAGE_FILE_NAME
	default:
		return ""
	}
}

// readOverlay reads an overlay file and checks that all its settings can be
// applied to a target.
func readOverlay(path string) (*Overlay, error) {
	lf, err := config.ReadLocalFile(path)
	if err != nil {
		return nil, err
	}

	relPath := filepath.ToSlash(newtutil.ProjRelPath(lf.Path()))

	for _, key := range lf.Keys() {
		if key == OVERLAY_PKGS_KEY {
			continue
		}

		if overlayKeyFile(key) == "" {
			return nil, util.FmtNewtError(
				"%s: unrecognized setting \"%s\"; overlays can only "+
					"contain syscfg.*, target.*, bsp.*, pkg.* and app.* "+
					"settings and the %s section", relPath, key,
				OVERLAY_PKGS_KEY)
		}

		for _, pk := range overlayProtectedKeys {
			if isKeyOrCond(key, pk) {
				return nil, util.FmtNewtError(
					"%s: \"%s\" can't be changed by an overlay", relPath, key)
			}
		}
	}

	pkgs, err := lf.Sections(OVERLAY_PKGS_KEY)
	if err != nil {
		return nil, err
	}

	for name, sec := range pkgs {
		for _, key := range sec.Keys() {
			supported := false
			for _, pk := range overlayPkgKeys {
				if isKeyOrCond(key, pk) {
					supported = true
					break
				}
			}

			if !supported {
				return nil, util.FmtNewtError(
					"%s: \"%s\" can't be changed for package \"%s\"; "+
						"only %s are supported", relPath, key, name,
					strings.Join(overlayPkgKeys, ", "))
			}
		}
	}

	return &Overlay{lf: lf, pkgs: pkgs}, nil
}

// Path returns the absolute path of the overlay file.
func (ov *Overlay) Path() string {
	return ov.lf.Path()
}

// RelPath returns the path of the overlay file relative to the project, with
// forward slashes, for use in messages.
func (ov *Overlay) RelPath() string {
	return filepath.ToSlash(newtutil.ProjRelPath(ov.Path()))
}

// PkgSections returns the settings of individual packages specified in the
// overlay, indexed by package name.
func (ov *Overlay) PkgSections() map[string]*config.LocalFile {
	return ov.pkgs
}

// ApplyOverlayPkgSection applies settings from a package section of an
// overlay to the package's values of the specified setting (e.g.,
// "pkg.cflags"), and returns the resulting values.
func ApplyOverlayPkgSection(sec *config.LocalFile, key string, vals []string,
	settings *cfgv.Settings) ([]string, error) {

	yc := ycfg.NewYCfg(sec.Path())
	if len(vals) > 0 {
		itfs := make([]interface{}, len(vals))
		for i, v := range vals {
			itfs[i] = v
		}
		yc.Replace(key, itfs)
	}

	if err := sec.Apply(&yc, func(k string) bool {
		return isKeyOrCond(k, key)
	}); err != nil {
		return nil, err
	}

	return yc.GetValStringSlice(key, settings)
}

// ReadOverlays reads the overlays specified on the command line.  They are
// read before the project is loaded, so relative imports are resolved
// relative to the importing file.
func ReadOverlays(paths []string) error {
	cliOverlays = nil

	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return util.ChildNewtError(err)
		}

		ov, err := readOverlay(absPath)
		if err != nil {
			return err
		}
		cliOverlays = append(cliOverlays, ov)
	}

	return nil
}

// HasCliOverlays reports whether any overlays were specified on the command
// line.
func HasCliOverlays() bool {
	return len(cliOverlays) > 0
}

// Overlays returns the overlays applied to the package's configuration.  Only
// targets have overlays.
func (pkg *LocalPackage) Overlays() []*Overlay {
	return pkg.overlays
}

// loadOverlays reads the overlays of a target package: its local overlay
// file followed by the overlays specified on the command line.
func (pkg *LocalPackage) loadOverlays() error {
	pkg.overlays = nil

	if pkg.packageType != PACKAGE_TYPE_TARGET {
		return nil
	}

	path := filepath.Join(pkg.BasePath(), LOCAL_OVERLAY_FILENAME)
	if !LocalOverlayDisabled && util.NodeExist(path) {
		ov, err := readOverlay(path)
		if err != nil {
			return err
		}
		pkg.overlays = append(pkg.overlays, ov)
	}

	pkg.overlays = append(pkg.overlays, cliOverlays...)

	return nil
}

// ApplyOverlays applies the settings of the package's overlays which belong to
// the specified configuration file (e.g., "syscfg.yml") to yc.
func (pkg *LocalPackage) ApplyOverlays(yc *ycfg.YCfg, filename string) error {
	for _, ov := range pkg.overlays {
		if err := ov.lf.Apply(yc, func(key string) bool {
			return overlayKeyFile(key) == filename
		}); err != nil {
			return err
		}
	}

	return nil
}

var reportedOverlays = map[string]struct{}{}

// ReportOverlays displays the overlays applied to the specified package.  Each
// overlay is reported only once.
func ReportOverlays(lpkg *LocalPackage) {
	for _, ov := range lpkg.overlays {
		if _, ok := reportedOverlays[ov.Path()]; !ok {
			reportedOverlays[ov.Path()] = struct{}{}
			util.ErrorMessage(util.VERBOSITY_DEFAULT, "Using overlay %s\n",
				ov.RelPath())
		}
	}
}
