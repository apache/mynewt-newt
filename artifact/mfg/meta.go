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
	"encoding/binary"
	"io"
	"io/ioutil"

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
const META_VERSION = 2
const META_TLV_TYPE_HASH = 0x01
const META_TLV_TYPE_FLASH_AREA = 0x02
const META_TLV_TYPE_MMR_REF = 0x04

const META_HASH_SZ = 32
const META_FOOTER_SZ = 8
const META_TLV_HEADER_SZ = 2
const META_TLV_HASH_SZ = META_HASH_SZ
const META_TLV_FLASH_AREA_SZ = 10
const META_TLV_MMR_REF_SZ = 1

type MetaFooter struct {
	Size    uint16 // Includes header, TLVs, and footer.
	Version uint8
	Pad8    uint8  // 0xff
	Magic   uint32 // META_MAGIC
}

type MetaTlvHeader struct {
	Type uint8 // Indicates the type of data to follow.
	Size uint8 // The number of bytes of data to follow.
}

type MetaTlvBodyFlashArea struct {
	Area   uint8  // Unique value identifying this flash area.
	Device uint8  // Indicates host flash device (aka section number).
	Offset uint32 // The byte offset within the flash device.
	Size   uint32 // Size, in bytes, of entire flash area.
}

type MetaTlvBodyHash struct {
	Hash [META_HASH_SZ]byte
}

type MetaTlvBodyMmrRef struct {
	Area uint8
}

type MetaTlv struct {
	Header MetaTlvHeader
	Data   []byte
}

type Meta struct {
	Tlvs   []MetaTlv
	Footer MetaFooter
}

type MetaOffsets struct {
	Tlvs      []int
	Footer    int
	TotalSize int
}

var metaTlvTypeNameMap = map[uint8]string{
	META_TLV_TYPE_HASH:       "hash",
	META_TLV_TYPE_FLASH_AREA: "flash_area",
	META_TLV_TYPE_MMR_REF:    "mmr_ref",
}

func MetaTlvTypeName(typ uint8) string {
	name := metaTlvTypeNameMap[typ]
	if name == "" {
		name = "???"
	}
	return name
}

func writeElem(elem interface{}, w io.Writer) error {
	/* XXX: Assume target platform uses little endian. */
	if err := binary.Write(w, binary.LittleEndian, elem); err != nil {
		return util.ChildNewtError(err)
	}
	return nil
}

func (tlv *MetaTlv) Write(w io.Writer) (int, error) {
	sz := 0

	if err := writeElem(tlv.Header, w); err != nil {
		return sz, err
	}
	sz += META_TLV_HEADER_SZ

	if err := writeElem(tlv.Data, w); err != nil {
		return sz, err
	}
	sz += len(tlv.Data)

	return sz, nil
}

func (meta *Meta) WritePlusOffsets(w io.Writer) (MetaOffsets, error) {
	mo := MetaOffsets{}
	sz := 0

	for _, tlv := range meta.Tlvs {
		tlvSz, err := tlv.Write(w)
		if err != nil {
			return mo, err
		}
		mo.Tlvs = append(mo.Tlvs, sz)
		sz += tlvSz
	}

	if err := writeElem(meta.Footer, w); err != nil {
		return mo, err
	}
	mo.Footer = sz
	sz += META_FOOTER_SZ

	mo.TotalSize = sz

	return mo, nil
}

func (meta *Meta) Offsets() MetaOffsets {
	mo, _ := meta.WritePlusOffsets(ioutil.Discard)
	return mo
}

func (meta *Meta) Write(w io.Writer) (int, error) {
	mo, err := meta.WritePlusOffsets(w)
	if err != nil {
		return 0, err
	}

	return mo.TotalSize, nil
}

func (meta *Meta) Size() int {
	return meta.Offsets().TotalSize
}

func (meta *Meta) Bytes() ([]byte, error) {
	b := &bytes.Buffer{}

	_, err := meta.Write(b)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (meta *Meta) FindTlvIndices(typ uint8) []int {
	indices := []int{}

	for i, tlv := range meta.Tlvs {
		if tlv.Header.Type == typ {
			indices = append(indices, i)
		}
	}

	return indices
}

func (meta *Meta) FindTlvs(typ uint8) []*MetaTlv {
	indices := meta.FindTlvIndices(typ)

	tlvs := []*MetaTlv{}
	for _, index := range indices {
		tlvs = append(tlvs, &meta.Tlvs[index])
	}

	return tlvs
}

func (meta *Meta) FindFirstTlv(typ uint8) *MetaTlv {
	indices := meta.FindTlvIndices(typ)
	if len(indices) == 0 {
		return nil
	}

	return &meta.Tlvs[indices[0]]
}

func (meta *Meta) HashOffset() int {
	mo := meta.Offsets()
	indices := meta.FindTlvIndices(META_TLV_TYPE_HASH)
	if len(indices) == 0 {
		return -1
	}

	return META_TLV_HEADER_SZ + mo.Tlvs[indices[0]]
}

func (meta *Meta) ClearHash() {
	tlv := meta.FindFirstTlv(META_TLV_TYPE_HASH)
	if tlv != nil {
		tlv.Data = make([]byte, META_HASH_SZ)
	}
}

func (meta *Meta) Hash() []byte {
	tlv := meta.FindFirstTlv(META_TLV_TYPE_HASH)
	if tlv == nil {
		return nil
	}
	return tlv.Data
}

func parseMetaTlv(bin []byte) (MetaTlv, int, error) {
	r := bytes.NewReader(bin)

	tlv := MetaTlv{}
	if err := binary.Read(r, binary.LittleEndian, &tlv.Header); err != nil {
		return tlv, 0, util.FmtNewtError(
			"Error reading TLV header: %s", err.Error())
	}

	data := make([]byte, tlv.Header.Size)
	sz, err := r.Read(data)
	if err != nil {
		return tlv, 0, util.FmtNewtError(
			"Error reading %d bytes of TLV data: %s",
			tlv.Header.Size, err.Error())
	}
	if sz != len(data) {
		return tlv, 0, util.FmtNewtError(
			"Error reading %d bytes of TLV data: incomplete read",
			tlv.Header.Size)
	}
	tlv.Data = data

	return tlv, META_TLV_HEADER_SZ + int(tlv.Header.Size), nil
}

func parseMetaFooter(bin []byte) (MetaFooter, int, error) {
	r := bytes.NewReader(bin)

	var ftr MetaFooter
	if err := binary.Read(r, binary.LittleEndian, &ftr); err != nil {
		return ftr, 0, util.FmtNewtError(
			"Error reading meta footer: %s", err.Error())
	}

	if ftr.Magic != META_MAGIC {
		return ftr, 0, util.FmtNewtError(
			"Meta footer contains invalid magic; exp:0x%08x, got:0x%08x",
			META_MAGIC, ftr.Magic)
	}

	return ftr, META_FOOTER_SZ, nil
}

func ParseMeta(bin []byte) (Meta, int, error) {
	if len(bin) < META_FOOTER_SZ {
		return Meta{}, 0, util.FmtNewtError(
			"Binary too small to accommodate meta footer; "+
				"bin-size=%d ftr-size=%d", len(bin), META_FOOTER_SZ)
	}

	ftr, _, err := parseMetaFooter(bin[len(bin)-META_FOOTER_SZ:])
	if err != nil {
		return Meta{}, 0, err
	}

	if int(ftr.Size) > len(bin) {
		return Meta{}, 0, util.FmtNewtError(
			"Binary too small to accommodate meta region; "+
				"bin-size=%d meta-size=%d", len(bin), ftr.Size)
	}

	ftrOff := len(bin) - META_FOOTER_SZ
	off := len(bin) - int(ftr.Size)

	tlvs := []MetaTlv{}
	for off < ftrOff {
		tlv, sz, err := parseMetaTlv(bin[off:])
		if err != nil {
			return Meta{}, 0, err
		}
		tlvs = append(tlvs, tlv)
		off += sz
	}

	return Meta{
		Tlvs:   tlvs,
		Footer: ftr,
	}, off, nil
}
