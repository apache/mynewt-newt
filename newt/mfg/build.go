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
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/artifact/manifest"
	"mynewt.apache.org/newt/artifact/mfg"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/flashmap"
	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

type MfgBuildTarget struct {
	Target  *target.Target
	Area    flash.FlashArea
	Offset  int
	IsBoot  bool
	BinPath string
}

type MfgBuildRaw struct {
	Filename string
	Offset   int
	Area     flash.FlashArea
}

type MfgBuildMetaMmr struct {
	Area flash.FlashArea
}

type MfgBuildMeta struct {
	Area     flash.FlashArea
	Hash     bool
	FlashMap bool
	Mmrs     []MfgBuildMetaMmr
}

// Can be used to construct an Mfg object.
type MfgBuilder struct {
	BasePkg *pkg.LocalPackage
	Bsp     *pkg.BspPackage
	Targets []MfgBuildTarget
	Raws    []MfgBuildRaw
	Meta    *MfgBuildMeta
}

// Searches the provided flash map for the named area.
func lookUpArea(fm flashmap.FlashMap, name string) (flash.FlashArea, error) {
	area, ok := fm.Areas[name]
	if !ok {
		return flash.FlashArea{}, util.FmtNewtError(
			"reference to undefined flash area \"%s\"", name)
	}

	return area, nil
}

// Searches the project for the target corresponding to the specified decoded
// entry (read from `mfg.yml`).
func lookUpTarget(dt DecodedTarget) (*target.Target, error) {
	t := target.GetTargets()[dt.Name]
	if t == nil {
		return nil, util.FmtNewtError(
			"target entry references undefined target \"%s\"", dt.Name)
	}

	return t, nil
}

func normalizeOffset(offset int, length int,
	area flash.FlashArea) (int, error) {

	areaEnd := area.Offset + area.Size
	if offset == OFFSET_END {
		if length > area.Size {
			return 0, util.FmtNewtError(
				"segment is too large to fit in flash area \"%s\"; "+
					"segment=%d, area=%d", area.Name, length, area.Size)
		}
		return areaEnd - length, nil
	}

	if offset+length > area.Size {
		return 0, util.FmtNewtError(
			"segment extends beyond end of flash area \"%s\"; "+
				"offset=%d len=%d area_len=%d",
			area.Name, offset, length, area.Size)
	}

	return area.Offset + offset, nil
}

func calcBsp(dm DecodedMfg,
	basePkg *pkg.LocalPackage) (*pkg.BspPackage, error) {

	var bspLpkg *pkg.LocalPackage
	bspMap := map[*pkg.LocalPackage]struct{}{}
	for _, dt := range dm.Targets {
		t, err := lookUpTarget(dt)
		if err != nil {
			return nil, err
		}

		bspLpkg = t.Bsp()
		bspMap[bspLpkg] = struct{}{}
	}

	if dm.Bsp != "" {
		var err error
		bspLpkg, err = project.GetProject().ResolvePackage(
			basePkg.Repo(), dm.Bsp)
		if err != nil {
			return nil, util.FmtNewtError(
				"failed to resolve BSP package: %s", err.Error())
		}
		bspMap[bspLpkg] = struct{}{}
	}

	if len(bspMap) == 0 {
		return nil, util.FmtNewtError("at least one target required")
	}

	if len(bspMap) > 1 {
		return nil, util.FmtNewtError("multiple BSPs detected")
	}

	bsp, err := pkg.NewBspPackage(bspLpkg)
	if err != nil {
		return nil, util.FmtNewtError(err.Error())
	}

	return bsp, nil
}

func (raw *MfgBuildRaw) ToPart(entryIdx int) (Part, error) {
	data, err := ioutil.ReadFile(raw.Filename)
	if err != nil {
		return Part{}, util.ChildNewtError(err)
	}

	off, err := normalizeOffset(raw.Offset, len(data), raw.Area)
	if err != nil {
		return Part{}, err
	}

	return Part{
		Name:   fmt.Sprintf("raw-%d (%s)", entryIdx, raw.Filename),
		Offset: off,
		Data:   data,
	}, nil
}

func (mt *MfgBuildTarget) ToPart() (Part, error) {
	data, err := ioutil.ReadFile(mt.BinPath)
	if err != nil {
		return Part{}, util.ChildNewtError(err)
	}

	off, err := normalizeOffset(mt.Offset, len(data), mt.Area)
	if err != nil {
		return Part{}, err
	}

	return Part{
		Name:   fmt.Sprintf("%s (%s)", mt.Area.Name, filepath.Base(mt.BinPath)),
		Offset: off,
		Data:   data,
	}, nil
}

func newMfgBuildTarget(dt DecodedTarget,
	fm flashmap.FlashMap) (MfgBuildTarget, error) {

	t, err := lookUpTarget(dt)
	if err != nil {
		return MfgBuildTarget{}, err
	}

	area, err := lookUpArea(fm, dt.Area)
	if err != nil {
		return MfgBuildTarget{}, err
	}

	mpath := builder.ManifestPath(dt.Name, builder.BUILD_NAME_APP,
		t.App().Name())
	man, err := manifest.ReadManifest(mpath)
	if err != nil {
		return MfgBuildTarget{}, util.FmtNewtError("%s", err.Error())
	}

	isBoot := parse.ValueIsTrue(man.Syscfg["BOOT_LOADER"])

	return MfgBuildTarget{
		Target:  t,
		Area:    area,
		Offset:  dt.Offset,
		IsBoot:  isBoot,
		BinPath: targetSrcBinPath(t, isBoot),
	}, nil
}

func newMfgBuildRaw(dr DecodedRaw,
	fm flashmap.FlashMap, basePath string) (MfgBuildRaw, error) {

	filename := dr.Filename
	if !strings.HasPrefix(filename, "/") {
		filename = basePath + "/" + filename
	}

	area, err := lookUpArea(fm, dr.Area)
	if err != nil {
		return MfgBuildRaw{}, err
	}

	return MfgBuildRaw{
		Filename: filename,
		Offset:   dr.Offset,
		Area:     area,
	}, nil
}

func newMfgBuildMeta(dm DecodedMeta,
	fm flashmap.FlashMap) (MfgBuildMeta, error) {

	area, ok := fm.Areas[dm.Area]
	if !ok {
		return MfgBuildMeta{}, util.FmtNewtError(
			"meta region specifies unrecognized flash area: \"%s\"", dm.Area)
	}

	var mmrs []MfgBuildMetaMmr
	for _, dmmr := range dm.Mmrs {
		area, err := lookUpArea(fm, dmmr.Area)
		if err != nil {
			return MfgBuildMeta{}, err
		}
		mmr := MfgBuildMetaMmr{
			Area: area,
		}
		mmrs = append(mmrs, mmr)
	}

	return MfgBuildMeta{
		Area:     area,
		Hash:     dm.Hash,
		FlashMap: dm.FlashMap,
		Mmrs:     mmrs,
	}, nil
}

func (mb *MfgBuilder) parts() ([]Part, error) {
	parts := []Part{}

	// Create parts from the raw entries.
	for i, raw := range mb.Raws {
		part, err := raw.ToPart(i)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}

	// Create parts from the target entries.
	for _, t := range mb.Targets {
		part, err := t.ToPart()
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}

	// Sort by offset.
	return SortParts(parts), nil
}

func (mb *MfgBuilder) detectOverlaps() error {
	type overlap struct {
		p1 Part
		p2 Part
	}

	overlaps := []overlap{}

	parts, err := mb.parts()
	if err != nil {
		return err
	}

	for i, p1 := range parts[:len(parts)-1] {
		p1end := p1.Offset + len(p1.Data)
		for _, p2 := range parts[i+1:] {
			// Parts are sorted by offset, so only one comparison is
			// necessary to detect overlap.
			if p2.Offset < p1end {
				overlaps = append(overlaps, overlap{
					p1: p1,
					p2: p2,
				})
			}
		}
	}

	if len(overlaps) > 0 {
		str := "flash overlaps detected:"
		for _, overlap := range overlaps {

			p1end := overlap.p1.Offset + len(overlap.p1.Data)
			p2end := overlap.p2.Offset + len(overlap.p2.Data)
			str += fmt.Sprintf("\n    * [%s] (%d - %d) <=> [%s] (%d - %d)",
				overlap.p1.Name, overlap.p1.Offset, p1end,
				overlap.p2.Name, overlap.p2.Offset, p2end)
		}

		return util.NewNewtError(str)
	}

	return nil
}

// Determines which flash device the manufacturing image is intended for.  It
// is an error if the mfg definition specifies 0 or >1 devices.
func (mb *MfgBuilder) calcDevice() (int, error) {
	deviceMap := map[int]struct{}{}
	for _, t := range mb.Targets {
		deviceMap[t.Area.Device] = struct{}{}
	}
	for _, r := range mb.Raws {
		deviceMap[r.Area.Device] = struct{}{}
	}

	devices := make([]int, 0, len(deviceMap))
	for d, _ := range deviceMap {
		devices = append(devices, d)
	}
	sort.Ints(devices)

	if len(devices) == 0 {
		return 0, util.FmtNewtError(
			"manufacturing image definition does not indicate flash device")
	}

	if len(devices) > 1 {
		return 0, util.FmtNewtError(
			"multiple flash devices in use by single manufacturing image: %v",
			devices)
	}

	return devices[0], nil
}

func newMfgBuilder(basePkg *pkg.LocalPackage, dm DecodedMfg,
	ver image.ImageVersion) (MfgBuilder, error) {

	mb := MfgBuilder{
		BasePkg: basePkg,
	}

	bsp, err := calcBsp(dm, basePkg)
	if err != nil {
		return mb, err
	}
	mb.Bsp = bsp

	for _, dt := range dm.Targets {
		mbt, err := newMfgBuildTarget(dt, bsp.FlashMap)
		if err != nil {
			return mb, err
		}
		mb.Targets = append(mb.Targets, mbt)
	}

	for _, dr := range dm.Raws {
		mbr, err := newMfgBuildRaw(dr, bsp.FlashMap, basePkg.BasePath())
		if err != nil {
			return mb, err
		}
		mb.Raws = append(mb.Raws, mbr)
	}

	if dm.Meta != nil {
		meta, err := newMfgBuildMeta(*dm.Meta, mb.Bsp.FlashMap)
		if err != nil {
			return mb, err
		}
		mb.Meta = &meta
	}

	if _, err := mb.calcDevice(); err != nil {
		return mb, err
	}

	if err := mb.detectOverlaps(); err != nil {
		return mb, err
	}

	return mb, nil
}

// Creates a zeroed-out hash MMR TLV.  The hash's original value must be zero
// for the actual hash to be calculated later.  After the actual value is
// calculated, it replaces the zeros in the TLV.
func newZeroHashTlv() mfg.MetaTlv {
	return mfg.MetaTlv{
		Header: mfg.MetaTlvHeader{
			Type: mfg.META_TLV_TYPE_HASH,
			Size: mfg.META_TLV_HASH_SZ,
		},
		Data: make([]byte, mfg.META_HASH_SZ),
	}
}

// Creates a flash area MMR TLV.
func newFlashAreaTlv(area flash.FlashArea) (mfg.MetaTlv, error) {
	tlv := mfg.MetaTlv{
		Header: mfg.MetaTlvHeader{
			Type: mfg.META_TLV_TYPE_FLASH_AREA,
			Size: mfg.META_TLV_FLASH_AREA_SZ,
		},
	}

	body := mfg.MetaTlvBodyFlashArea{
		Area:   uint8(area.Id),
		Device: uint8(area.Device),
		Offset: uint32(area.Offset),
		Size:   uint32(area.Size),
	}

	b := &bytes.Buffer{}
	if err := binary.Write(b, binary.LittleEndian, body); err != nil {
		return tlv, util.ChildNewtError(err)
	}

	tlv.Data = b.Bytes()

	return tlv, nil
}

// Creates an MMR ref TLV.
func newMmrRefTlv(area flash.FlashArea) (mfg.MetaTlv, error) {
	tlv := mfg.MetaTlv{
		Header: mfg.MetaTlvHeader{
			Type: mfg.META_TLV_TYPE_MMR_REF,
			Size: mfg.META_TLV_MMR_REF_SZ,
		},
	}

	body := mfg.MetaTlvBodyMmrRef{
		Area: uint8(area.Id),
	}

	b := &bytes.Buffer{}
	if err := binary.Write(b, binary.LittleEndian, body); err != nil {
		return tlv, util.ChildNewtError(err)
	}

	tlv.Data = b.Bytes()

	return tlv, nil
}

// Builds a manufacturing meta region.
func (mb *MfgBuilder) buildMeta() (mfg.Meta, error) {
	meta := mfg.Meta{
		Footer: mfg.MetaFooter{
			Size:    0, // Filled in later.
			Version: mfg.META_VERSION,
			Pad8:    0xff,
			Magic:   mfg.META_MAGIC,
		},
	}

	// Hash TLV.
	if mb.Meta.Hash {
		meta.Tlvs = append(meta.Tlvs, newZeroHashTlv())
	}

	// Flash map TLVs.
	if mb.Meta.FlashMap {
		for _, area := range mb.Bsp.FlashMap.SortedAreas() {
			tlv, err := newFlashAreaTlv(area)
			if err != nil {
				return meta, err
			}

			meta.Tlvs = append(meta.Tlvs, tlv)
		}
	}

	// MMR ref TLVs.
	for _, mmr := range mb.Meta.Mmrs {
		tlv, err := newMmrRefTlv(mmr.Area)
		if err != nil {
			return meta, err
		}

		meta.Tlvs = append(meta.Tlvs, tlv)
	}

	// Fill in region size in footer now that we know the value.
	meta.Footer.Size = uint16(meta.Size())

	return meta, nil
}

// Builds a manufacturing image.
func (mb *MfgBuilder) Build() (mfg.Mfg, error) {
	parts, err := mb.parts()
	if err != nil {
		return mfg.Mfg{}, err
	}

	bin, err := PartsBytes(parts)
	if err != nil {
		return mfg.Mfg{}, err
	}

	var metaOff int
	var metap *mfg.Meta
	if mb.Meta != nil {
		meta, err := mb.buildMeta()
		if err != nil {
			return mfg.Mfg{}, err
		}
		metap = &meta
		metaOff = mb.Meta.Area.Offset + mb.Meta.Area.Size - meta.Size()
	}

	return mfg.Mfg{
		Bin:     bin,
		Meta:    metap,
		MetaOff: metaOff,
	}, nil
}
