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
	"encoding/binary"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/target"
)

const MFG_YAML_FILENAME string = "mfg.yml"

const MFG_IMAGE_VERSION = 1

type mfgImageHeader struct {
	Version    uint8 // 0x01
	_          uint8
	_          uint16
	HashOffset uint32
}

type mfgImageSectionHeader struct {
	DeviceId uint8
	_        uint8
	_        uint16
	Offset   uint32 // Offset within flash device.
	Size     uint32 // Does not include header.
}

type MfgRawSection struct {
	offset   int
	filename string
	data     []byte
}

// A chunk of data in the manufacturing image.  Can be a firmware image or a
// raw section (contents of a data file).
type mfgPart struct {
	offset int
	data   []byte
	name   string
}

type MfgImage struct {
	basePkg *pkg.LocalPackage

	bsp *pkg.BspPackage

	boot        *target.Target
	images      []*target.Target
	rawSections []MfgRawSection
}

var MFG_IMAGE_HEADER_SIZE = binary.Size(mfgImageHeader{})
var MFG_IMAGE_SECTION_HEADER_SIZE = binary.Size(mfgImageSectionHeader{})
