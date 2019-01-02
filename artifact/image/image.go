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

package image

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"mynewt.apache.org/newt/util"
)

const (
	IMAGE_MAGIC         = 0x96f3b83d /* Image header magic */
	IMAGE_TRAILER_MAGIC = 0x6907     /* Image tlv info magic */
)

const (
	IMAGE_HEADER_SIZE  = 32
	IMAGE_TRAILER_SIZE = 4
	IMAGE_TLV_SIZE     = 4 /* Plus `value` field. */
)

/*
 * Image header flags.
 */
const (
	IMAGE_F_PIC          = 0x00000001
	IMAGE_F_NON_BOOTABLE = 0x00000002 /* non bootable image */
	IMAGE_F_ENCRYPTED    = 0x00000004 /* encrypted image */
)

/*
 * Image trailer TLV types.
 */
const (
	IMAGE_TLV_KEYHASH  = 0x01
	IMAGE_TLV_SHA256   = 0x10
	IMAGE_TLV_RSA2048  = 0x20
	IMAGE_TLV_ECDSA224 = 0x21
	IMAGE_TLV_ECDSA256 = 0x22
	IMAGE_TLV_ENC_RSA  = 0x30
	IMAGE_TLV_ENC_KEK  = 0x31
)

var imageTlvTypeNameMap = map[uint8]string{
	IMAGE_TLV_KEYHASH:  "KEYHASH",
	IMAGE_TLV_SHA256:   "SHA256",
	IMAGE_TLV_RSA2048:  "RSA2048",
	IMAGE_TLV_ECDSA224: "ECDSA224",
	IMAGE_TLV_ECDSA256: "ECDSA256",
	IMAGE_TLV_ENC_RSA:  "ENC_RSA",
	IMAGE_TLV_ENC_KEK:  "ENC_KEK",
}

type ImageVersion struct {
	Major    uint8
	Minor    uint8
	Rev      uint16
	BuildNum uint32
}

type ImageHdr struct {
	Magic uint32
	Pad1  uint32
	HdrSz uint16
	Pad2  uint16
	ImgSz uint32
	Flags uint32
	Vers  ImageVersion
	Pad3  uint32
}

type ImageTlvHdr struct {
	Type uint8
	Pad  uint8
	Len  uint16
}

type ImageTlv struct {
	Header ImageTlvHdr
	Data   []byte
}

type ImageTrailer struct {
	Magic     uint16
	TlvTotLen uint16
}

type Image struct {
	Header ImageHdr
	Pad    []byte
	Body   []byte
	Tlvs   []ImageTlv
}

type ImageOffsets struct {
	Header    int
	Body      int
	Trailer   int
	Tlvs      []int
	TotalSize int
}

func ImageTlvTypeName(tlvType uint8) string {
	name, ok := imageTlvTypeNameMap[tlvType]
	if !ok {
		return "???"
	}

	return name
}

func ImageTlvTypeIsSig(tlvType uint8) bool {
	return tlvType == IMAGE_TLV_RSA2048 ||
		tlvType == IMAGE_TLV_ECDSA224 ||
		tlvType == IMAGE_TLV_ECDSA256
}

func ParseVersion(versStr string) (ImageVersion, error) {
	var err error
	var major uint64
	var minor uint64
	var rev uint64
	var buildNum uint64
	var ver ImageVersion

	components := strings.Split(versStr, ".")
	major, err = strconv.ParseUint(components[0], 10, 8)
	if err != nil {
		return ver, util.FmtNewtError("Invalid version string %s", versStr)
	}
	if len(components) > 1 {
		minor, err = strconv.ParseUint(components[1], 10, 8)
		if err != nil {
			return ver, util.FmtNewtError("Invalid version string %s", versStr)
		}
	}
	if len(components) > 2 {
		rev, err = strconv.ParseUint(components[2], 10, 16)
		if err != nil {
			return ver, util.FmtNewtError("Invalid version string %s", versStr)
		}
	}
	if len(components) > 3 {
		buildNum, err = strconv.ParseUint(components[3], 10, 32)
		if err != nil {
			return ver, util.FmtNewtError("Invalid version string %s", versStr)
		}
	}

	ver.Major = uint8(major)
	ver.Minor = uint8(minor)
	ver.Rev = uint16(rev)
	ver.BuildNum = uint32(buildNum)
	return ver, nil
}

func (ver ImageVersion) String() string {
	return fmt.Sprintf("%d.%d.%d.%d",
		ver.Major, ver.Minor, ver.Rev, ver.BuildNum)
}

func (h *ImageHdr) Map(offset int) map[string]interface{} {
	return map[string]interface{}{
		"magic":   h.Magic,
		"hdr_sz":  h.HdrSz,
		"img_sz":  h.ImgSz,
		"flags":   h.Flags,
		"vers":    h.Vers.String(),
		"_offset": offset,
	}
}

func rawBodyMap(offset int) map[string]interface{} {
	return map[string]interface{}{
		"_offset": offset,
	}
}

func (t *ImageTrailer) Map(offset int) map[string]interface{} {
	return map[string]interface{}{
		"magic":       t.Magic,
		"tlv_tot_len": t.TlvTotLen,
		"_offset":     offset,
	}
}

func (t *ImageTlv) Map(offset int) map[string]interface{} {
	return map[string]interface{}{
		"type":     t.Header.Type,
		"len":      t.Header.Len,
		"data":     hex.EncodeToString(t.Data),
		"_typestr": ImageTlvTypeName(t.Header.Type),
		"_offset":  offset,
	}
}

func (img *Image) Map() (map[string]interface{}, error) {
	offs, err := img.Offsets()
	if err != nil {
		return nil, err
	}

	m := map[string]interface{}{}
	m["header"] = img.Header.Map(offs.Header)
	m["body"] = rawBodyMap(offs.Body)
	trailer := img.Trailer()
	m["trailer"] = trailer.Map(offs.Trailer)

	tlvMaps := []map[string]interface{}{}
	for i, tlv := range img.Tlvs {
		tlvMaps = append(tlvMaps, tlv.Map(offs.Tlvs[i]))
	}
	m["tlvs"] = tlvMaps

	return m, nil
}

func (img *Image) Json() (string, error) {
	m, err := img.Map()
	if err != nil {
		return "", err
	}

	b, err := json.MarshalIndent(m, "", "    ")
	if err != nil {
		return "", util.ChildNewtError(err)
	}

	return string(b), nil
}

func (tlv *ImageTlv) Write(w io.Writer) (int, error) {
	totalSize := 0

	err := binary.Write(w, binary.LittleEndian, &tlv.Header)
	if err != nil {
		return totalSize, util.ChildNewtError(err)
	}
	totalSize += IMAGE_TLV_SIZE

	size, err := w.Write(tlv.Data)
	if err != nil {
		return totalSize, util.ChildNewtError(err)
	}
	totalSize += size

	return totalSize, nil
}

func (i *Image) FindTlvs(tlvType uint8) []ImageTlv {
	var tlvs []ImageTlv

	for _, tlv := range i.Tlvs {
		if tlv.Header.Type == tlvType {
			tlvs = append(tlvs, tlv)
		}
	}

	return tlvs
}

func (i *Image) FindUniqueTlv(tlvType uint8) (*ImageTlv, error) {
	tlvs := i.FindTlvs(tlvType)
	if len(tlvs) == 0 {
		return nil, nil
	}
	if len(tlvs) > 1 {
		return nil, util.FmtNewtError("Image contains %d TLVs with type %d",
			len(tlvs), tlvType)
	}

	return &tlvs[0], nil
}

func (i *Image) RemoveTlvsIf(pred func(tlv ImageTlv) bool) []ImageTlv {
	rmed := []ImageTlv{}

	for idx := 0; idx < len(i.Tlvs); {
		tlv := i.Tlvs[idx]
		if pred(tlv) {
			rmed = append(rmed, tlv)
			i.Tlvs = append(i.Tlvs[:idx], i.Tlvs[idx+1:]...)
		} else {
			idx++
		}
	}

	return rmed
}

func (i *Image) RemoveTlvsWithType(tlvType uint8) []ImageTlv {
	return i.RemoveTlvsIf(func(tlv ImageTlv) bool {
		return tlv.Header.Type == tlvType
	})
}

func (img *Image) Trailer() ImageTrailer {
	trailer := ImageTrailer{
		Magic:     IMAGE_TRAILER_MAGIC,
		TlvTotLen: IMAGE_TRAILER_SIZE,
	}
	for _, tlv := range img.Tlvs {
		trailer.TlvTotLen += IMAGE_TLV_SIZE + tlv.Header.Len
	}

	return trailer
}

func (i *Image) Hash() ([]byte, error) {
	tlv, err := i.FindUniqueTlv(IMAGE_TLV_SHA256)
	if err != nil {
		return nil, err
	}

	if tlv == nil {
		return nil, util.FmtNewtError("Image does not contain hash TLV")
	}

	return tlv.Data, nil
}

func (i *Image) WritePlusOffsets(w io.Writer) (ImageOffsets, error) {
	offs := ImageOffsets{}
	offset := 0

	offs.Header = offset

	err := binary.Write(w, binary.LittleEndian, &i.Header)
	if err != nil {
		return offs, util.ChildNewtError(err)
	}
	offset += IMAGE_HEADER_SIZE

	err = binary.Write(w, binary.LittleEndian, i.Pad)
	if err != nil {
		return offs, util.ChildNewtError(err)
	}
	offset += len(i.Pad)

	offs.Body = offset
	size, err := w.Write(i.Body)
	if err != nil {
		return offs, util.ChildNewtError(err)
	}
	offset += size

	trailer := i.Trailer()
	offs.Trailer = offset
	err = binary.Write(w, binary.LittleEndian, &trailer)
	if err != nil {
		return offs, util.ChildNewtError(err)
	}
	offset += IMAGE_TRAILER_SIZE

	for _, tlv := range i.Tlvs {
		offs.Tlvs = append(offs.Tlvs, offset)
		size, err := tlv.Write(w)
		if err != nil {
			return offs, util.ChildNewtError(err)
		}
		offset += size
	}

	offs.TotalSize = offset

	return offs, nil
}

func (i *Image) Offsets() (ImageOffsets, error) {
	return i.WritePlusOffsets(ioutil.Discard)
}

func (i *Image) TotalSize() (int, error) {
	offs, err := i.Offsets()
	if err != nil {
		return 0, err
	}
	return offs.TotalSize, nil
}

func (i *Image) Write(w io.Writer) (int, error) {
	offs, err := i.WritePlusOffsets(w)
	if err != nil {
		return 0, err
	}

	return offs.TotalSize, nil
}

func (i *Image) WriteToFile(filename string) error {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return util.ChildNewtError(err)
	}

	if _, err := i.Write(f); err != nil {
		return util.ChildNewtError(err)
	}

	return nil
}

func parseRawHeader(imgData []byte, offset int) (ImageHdr, int, error) {
	var hdr ImageHdr

	r := bytes.NewReader(imgData)
	r.Seek(int64(offset), io.SeekStart)

	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return hdr, 0, util.FmtNewtError(
			"Error reading image header: %s", err.Error())
	}

	if hdr.Magic != IMAGE_MAGIC {
		return hdr, 0, util.FmtNewtError(
			"Image magic incorrect; expected 0x%08x, got 0x%08x",
			uint32(IMAGE_MAGIC), hdr.Magic)
	}

	remLen := len(imgData) - offset
	if remLen < int(hdr.HdrSz) {
		return hdr, 0, util.FmtNewtError(
			"Image header incomplete; expected %d bytes, got %d bytes",
			hdr.HdrSz, remLen)
	}

	return hdr, int(hdr.HdrSz), nil
}

func parseRawBody(imgData []byte, hdr ImageHdr,
	offset int) ([]byte, int, error) {

	imgSz := int(hdr.ImgSz)
	remLen := len(imgData) - offset

	if remLen < imgSz {
		return nil, 0, util.FmtNewtError(
			"Image body incomplete; expected %d bytes, got %d bytes",
			imgSz, remLen)
	}

	return imgData[offset : offset+imgSz], imgSz, nil
}

func parseRawTrailer(imgData []byte, offset int) (ImageTrailer, int, error) {
	var trailer ImageTrailer

	r := bytes.NewReader(imgData)
	r.Seek(int64(offset), io.SeekStart)

	if err := binary.Read(r, binary.LittleEndian, &trailer); err != nil {
		return trailer, 0, util.FmtNewtError(
			"Image contains invalid trailer at offset %d: %s",
			offset, err.Error())
	}

	return trailer, IMAGE_TRAILER_SIZE, nil
}

func parseRawTlv(imgData []byte, offset int) (ImageTlv, int, error) {
	tlv := ImageTlv{}

	r := bytes.NewReader(imgData)
	r.Seek(int64(offset), io.SeekStart)

	if err := binary.Read(r, binary.LittleEndian, &tlv.Header); err != nil {
		return tlv, 0, util.FmtNewtError(
			"Image contains invalid TLV at offset %d: %s", offset, err.Error())
	}

	tlv.Data = make([]byte, tlv.Header.Len)
	if _, err := r.Read(tlv.Data); err != nil {
		return tlv, 0, util.FmtNewtError(
			"Image contains invalid TLV at offset %d: %s", offset, err.Error())
	}

	return tlv, IMAGE_TLV_SIZE + int(tlv.Header.Len), nil
}

func ParseImage(imgData []byte) (Image, error) {
	img := Image{}
	offset := 0

	hdr, size, err := parseRawHeader(imgData, offset)
	if err != nil {
		return img, err
	}
	offset += size

	body, size, err := parseRawBody(imgData, hdr, offset)
	if err != nil {
		return img, err
	}
	offset += size

	trailer, size, err := parseRawTrailer(imgData, offset)
	if err != nil {
		return img, err
	}
	offset += size

	var tlvs []ImageTlv
	tlvLen := IMAGE_TRAILER_SIZE
	for offset < len(imgData) {
		tlv, size, err := parseRawTlv(imgData, offset)
		if err != nil {
			return img, err
		}

		tlvs = append(tlvs, tlv)
		offset += size

		tlvLen += IMAGE_TLV_SIZE + int(tlv.Header.Len)
	}

	if int(trailer.TlvTotLen) != tlvLen {
		return img, util.FmtNewtError(
			"invalid image: trailer indicates TLV-length=%d; actual=%d",
			trailer.TlvTotLen, tlvLen)
	}

	img.Header = hdr
	img.Body = body
	img.Tlvs = tlvs

	return img, nil
}

func ReadImage(filename string) (Image, error) {
	ri := Image{}

	imgData, err := ioutil.ReadFile(filename)
	if err != nil {
		return ri, util.ChildNewtError(err)
	}

	return ParseImage(imgData)
}
