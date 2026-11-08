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

// Settings which identify a package and can't be changed by overlays.
var overlayProtectedKeys = []string{
	"pkg.name",
	"pkg.type",
}

// Overlay is a configuration file applied on top of a target's configuration
// files.  It can contain settings from all of them; each setting is applied
// to the file selected by the setting's prefix:
//
//	syscfg.*         syscfg.yml
//	target.*, bsp.*  target.yml
//	pkg.*, app.*     pkg.yml
//
// Overlays support the same operations as local.project.yml (OVERWRITE,
// REMOVE and DELETE suffixes).
type Overlay struct {
	lf *config.LocalFile
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
		if overlayKeyFile(key) == "" {
			return nil, util.FmtNewtError(
				"%s: unrecognized setting \"%s\"; overlays can only "+
					"contain syscfg.*, target.*, bsp.*, pkg.* and app.* "+
					"settings", relPath, key)
		}

		for _, pk := range overlayProtectedKeys {
			if isKeyOrCond(key, pk) {
				return nil, util.FmtNewtError(
					"%s: \"%s\" can't be changed by an overlay", relPath, key)
			}
		}
	}

	return &Overlay{lf: lf}, nil
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

// Overlays returns the overlays applied to the package's configuration.  Only
// targets have overlays.
func (pkg *LocalPackage) Overlays() []*Overlay {
	return pkg.overlays
}

// loadOverlays reads the overlays of a target package.
func (pkg *LocalPackage) loadOverlays() error {
	pkg.overlays = nil

	if pkg.packageType != PACKAGE_TYPE_TARGET || LocalOverlayDisabled {
		return nil
	}

	path := filepath.Join(pkg.BasePath(), LOCAL_OVERLAY_FILENAME)
	if !util.NodeExist(path) {
		return nil
	}

	ov, err := readOverlay(path)
	if err != nil {
		return err
	}
	pkg.overlays = append(pkg.overlays, ov)

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
