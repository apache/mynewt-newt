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

	"mynewt.apache.org/newt/artifact/mfg"
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

func MfgTargetDir(targetNum int) string {
	return fmt.Sprintf("targets/%d", targetNum)
}

func MfgTargetBinPath(targetNum int) string {
	return fmt.Sprintf("%s/binary.bin", MfgTargetDir(targetNum))
}

func MfgTargetImgPath(targetNum int) string {
	return fmt.Sprintf("%s/image.img", MfgTargetDir(targetNum))
}

func MfgTargetHexPath(targetNum int) string {
	return fmt.Sprintf("%s/image.hex", MfgTargetDir(targetNum))
}

func MfgTargetElfPath(targetNum int) string {
	return fmt.Sprintf("%s/elf.elf", MfgTargetDir(targetNum))
}

func MfgTargetManifestPath(targetNum int) string {
	return fmt.Sprintf("%s/manifest.json", MfgTargetDir(targetNum))
}
