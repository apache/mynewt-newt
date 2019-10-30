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

package mfg

import (
	"fmt"

	"github.com/apache/mynewt-artifact/mfg"
	"mynewt.apache.org/newt/newt/builder"
)

// Filename containing a manufacturing image definition.
const YAML_FILENAME string = "mfg.yml"

func MfgBinDir(mfgPkgName string) string {
	return builder.BinRoot() + "/" + mfgPkgName
}

func MfgBinPath(mfgPkgName string) string {
	return MfgBinDir(mfgPkgName) + "/" + mfg.MFG_BIN_IMG_FILENAME
}

func MfgHexPath(mfgPkgName string) string {
	return MfgBinDir(mfgPkgName) + "/" + mfg.MFG_HEX_IMG_FILENAME
}

func MfgManifestPath(mfgPkgName string) string {
	return MfgBinDir(mfgPkgName) + "/" + mfg.MANIFEST_FILENAME
}

func MfgTargetDir(mfgPkgName string, targetNum int) string {
	return fmt.Sprintf("%s/targets/%d", MfgBinDir(mfgPkgName), targetNum)
}

func MfgRawDir(mfgPkgName string, rawNum int) string {
	return fmt.Sprintf("%s/raws/%d", MfgBinDir(mfgPkgName), rawNum)
}

func MfgTargetBinPath(mfgPkgName string, targetNum int) string {
	return fmt.Sprintf("%s/binary.bin", MfgTargetDir(mfgPkgName, targetNum))
}

func MfgTargetImgPath(mfgPkgName string, targetNum int) string {
	return fmt.Sprintf("%s/image.img", MfgTargetDir(mfgPkgName, targetNum))
}

func MfgTargetHexPath(mfgPkgName string, targetNum int) string {
	return fmt.Sprintf("%s/image.hex", MfgTargetDir(mfgPkgName, targetNum))
}

func MfgTargetElfPath(mfgPkgName string, targetNum int) string {
	return fmt.Sprintf("%s/elf.elf", MfgTargetDir(mfgPkgName, targetNum))
}

func MfgTargetManifestPath(mfgPkgName string, targetNum int) string {
	return fmt.Sprintf("%s/manifest.json", MfgTargetDir(mfgPkgName, targetNum))
}

func MfgRawBinPath(mfgPkgName string, rawNum int) string {
	return fmt.Sprintf("%s/raw.bin", MfgRawDir(mfgPkgName, rawNum))
}
