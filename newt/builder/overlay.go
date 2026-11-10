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
	"sort"

	"mynewt.apache.org/newt/newt/config"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/util"
)

// pkgSections returns the package sections of the target's pkg.yml and of
// its overlays which apply to the specified package, in the order they are
// applied.  Package names are resolved the same way as the target's
// dependencies; a package which doesn't exist is an error.  The sections are
// only applied to this build, so the packages' configuration is not changed.
func (t *TargetBuilder) pkgSections(
	lpkg *pkg.LocalPackage) ([]*config.LocalFile, error) {

	if t.targetPkgSections == nil {
		m := map[*pkg.LocalPackage][]*config.LocalFile{}

		for _, ps := range t.target.Package().AllPkgSections() {
			names := make([]string, 0, len(ps.Sections))
			for name := range ps.Sections {
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				p := t.target.ResolvePackageName(name)
				if p == nil {
					return nil, util.FmtNewtError(
						"%s: unknown package \"%s\" in \"%s\"",
						ps.RelPath, name, pkg.OVERLAY_PKGS_KEY)
				}
				m[p] = append(m[p], ps.Sections[name])
			}
		}

		t.targetPkgSections = m
	}

	return t.targetPkgSections[lpkg], nil
}
