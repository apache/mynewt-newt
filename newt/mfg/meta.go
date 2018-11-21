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
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/newt/flashmap"
	"mynewt.apache.org/newt/util"
)

// The "manufacturing meta region" is located at the end of the boot loader
// flash area.  This region has the following structure.
//
//  0                   1                   2                   3
//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |Version (0x01) |                  0xff padding                 |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |   TLV type    |   TLV size    | TLV data ("TLV size" bytes)   ~
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               ~
// ~                                                               ~
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |   TLV type    |   TLV size    | TLV data ("TLV size" bytes)   ~
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               ~
// ~                                                               ~
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |   Region size                 |         0xff padding          |
// +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
// |                       Magic (0x3bb2a269)                      |
// +-+-+-+-+-+--+-+-+-+-end of boot loader area+-+-+-+-+-+-+-+-+-+-+
//
// The number of TLVs is variable; two are shown above for illustrative
// purposes.
//
// Fields:
// <Header>
// 1. Version: Manufacturing meta version number; always 0x01.
//
// <TLVs>
// 2. TLV type: Indicates the type of data to follow.
// 3. TLV size: The number of bytes of data to follow.
// 4. TLV data: TLV-size bytes of data.
//
// <Footer>
// 5. Region size: The size, in bytes, of the entire manufacturing meta region;
//    includes header, TLVs, and footer.
// 6. Magic: indicates the presence of the manufacturing meta region.

const META_MAGIC = 0x3bb2a269
const META_VERSION = 1
const META_TLV_CODE_HASH = 0x01
const META_TLV_CODE_FLASH_AREA = 0x02

const META_HASH_SZ = 32
const META_FOOTER_SZ = 8
const META_TLV_HASH_SZ = META_HASH_SZ
const META_TLV_FLASH_AREA_SZ = 12

type metaHeader struct {
	version uint8  // 1
	pad8    uint8  // 0xff
	pad16   uint16 // 0xffff
}

type metaFooter struct {
	size  uint16 // Includes header, TLVs, and footer.
	pad16 uint16 // 0xffff
	magic uint32 // META_MAGIC
}

type metaTlvHeader struct {
	typ  uint8 // Indicates the type of data to follow.
	size uint8 // The number of bytes of data to follow.
}

type metaTlvFlashArea struct {
	header   metaTlvHeader
	areaId   uint8  // Unique value identifying this flash area.
	deviceId uint8  // Indicates host flash device (aka section number).
	pad16    uint16 // 0xffff
	offset   uint32 // The byte offset within the flash device.
	size     uint32 // Size, in bytes, of entire flash area.
}

type metaTlvHash struct {
	header metaTlvHeader
	hash   [META_HASH_SZ]byte
}

func writeElem(elem interface{}, buf *bytes.Buffer) error {
	/* XXX: Assume target platform uses little endian. */
	if err := binary.Write(buf, binary.LittleEndian, elem); err != nil {
		return util.ChildNewtError(err)
	}
	return nil
}

func writeHeader(buf *bytes.Buffer) error {
	hdr := metaHeader{
		version: META_VERSION,
		pad8:    0xff,
		pad16:   0xffff,
	}
	return writeElem(hdr, buf)
}

func writeFooter(buf *bytes.Buffer) error {
	ftr := metaFooter{
		size:  uint16(buf.Len() + META_FOOTER_SZ),
		pad16: 0xffff,
		magic: META_MAGIC,
	}
	return writeElem(ftr, buf)
}

func writeTlvHeader(typ uint8, size uint8, buf *bytes.Buffer) error {
	tlvHdr := metaTlvHeader{
		typ:  typ,
		size: size,
	}
	return writeElem(tlvHdr, buf)
}

// Writes a single entry of the flash map TLV.
func writeFlashMapEntry(area flash.FlashArea, buf *bytes.Buffer) error {
	tlv := metaTlvFlashArea{
		header: metaTlvHeader{
			typ:  META_TLV_CODE_FLASH_AREA,
			size: META_TLV_FLASH_AREA_SZ,
		},
		areaId:   uint8(area.Id),
		deviceId: uint8(area.Device),
		pad16:    0xffff,
		offset:   uint32(area.Offset),
		size:     uint32(area.Size),
	}
	return writeElem(tlv, buf)
}

// Writes a zeroed-out hash TLV.  The hash's original value must be zero for
// the actual hash to be calculated later.  After the actual value is
// calculated, it replaces the zeros in the TLV.
func writeZeroHash(buf *bytes.Buffer) error {
	tlv := metaTlvHash{
		header: metaTlvHeader{
			typ:  META_TLV_CODE_HASH,
			size: META_TLV_HASH_SZ,
		},
		hash: [META_HASH_SZ]byte{},
	}
	return writeElem(tlv, buf)
}

// @return						meta-offset, hash-offset, error
func insertMeta(section0Data []byte, flashMap flashmap.FlashMap) (
	int, int, error) {

	buf := &bytes.Buffer{}

	if err := writeHeader(buf); err != nil {
		return 0, 0, err
	}

	for _, area := range flashMap.SortedAreas() {
		if err := writeFlashMapEntry(area, buf); err != nil {
			return 0, 0, err
		}
	}

	if err := writeZeroHash(buf); err != nil {
		return 0, 0, err
	}
	hashSubOff := buf.Len() - META_HASH_SZ

	if err := writeFooter(buf); err != nil {
		return 0, 0, err
	}

	// The meta region gets placed at the very end of the boot loader slot.
	bootArea, ok := flashMap.Areas[flash.FLASH_AREA_NAME_BOOTLOADER]
	if !ok {
		return 0, 0,
			util.NewNewtError("Required boot loader flash area missing")
	}

	if bootArea.Size < buf.Len() {
		return 0, 0, util.FmtNewtError(
			"Boot loader flash area too small to accommodate meta region; "+
				"boot=%d meta=%d", bootArea.Size, buf.Len())
	}

	metaOff := bootArea.Offset + bootArea.Size - buf.Len()
	for i := metaOff; i < bootArea.Size; i++ {
		if section0Data[i] != 0xff {
			return 0, 0, util.FmtNewtError(
				"Boot loader extends into meta region; "+
					"meta region starts at offset %d", metaOff)
		}
	}

	// Copy the meta region into the manufacturing image.  The meta hash is
	// still zeroed.
	copy(section0Data[metaOff:], buf.Bytes())

	return metaOff, metaOff + hashSubOff, nil
}

// Calculates the SHA256 hash, using the full manufacturing image as input.
// Hash-calculation algorithm is as follows:
// 1. Concatenate sections in ascending order of index.
// 2. Zero out the 32 bytes that will contain the hash.
// 3. Apply SHA256 to the result.
//
// This function assumes that the 32 bytes of hash data have already been
// zeroed.
func calcMetaHash(sections [][]byte) []byte {
	// Concatenate all sections.
	blob := []byte{}
	for _, section := range sections {
		blob = append(blob, section...)
	}

	// Calculate hash.
	hash := sha256.Sum256(blob)

	return hash[:]
}
