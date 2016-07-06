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
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

type BspPackage struct {
	*LocalPackage
	CompilerName      string
	Arch              string
	LinkerScript      string
	Part2LinkerScript string /* script to link app to second partition */
	DownloadScript    string
	DebugScript       string
}

func (bsp *BspPackage) Reload(features map[string]bool) error {
	bsp.CompilerName = newtutil.GetStringFeatures(bsp.LocalPackage.Viper,
		features, "pkg.compiler")
	bsp.Arch = newtutil.GetStringFeatures(bsp.LocalPackage.Viper,
		features, "pkg.arch")
	bsp.LinkerScript = newtutil.GetStringFeatures(bsp.LocalPackage.Viper,
		features, "pkg.linkerscript")
	bsp.Part2LinkerScript = newtutil.GetStringFeatures(bsp.LocalPackage.Viper,
		features, "pkg.part2linkerscript")
	bsp.DownloadScript = newtutil.GetStringFeatures(bsp.LocalPackage.Viper,
		features, "pkg.downloadscript")
	bsp.DebugScript = newtutil.GetStringFeatures(bsp.LocalPackage.Viper,
		features, "pkg.debugscript")

	if bsp.CompilerName == "" {
		return util.NewNewtError("BSP does not specify a compiler " +
			"(pkg.compiler)")
	}
	if bsp.Arch == "" {
		return util.NewNewtError("BSP does not specify an architecture " +
			"(pkg.arch)")
	}

	return nil
}

func NewBspPackage(lpkg *LocalPackage) *BspPackage {
	bsp := &BspPackage{
		CompilerName:   "",
		LinkerScript:   "",
		DownloadScript: "",
		DebugScript:    "",
	}
	lpkg.Load()
	bsp.LocalPackage = lpkg
	bsp.Reload(nil)
	return bsp
}
