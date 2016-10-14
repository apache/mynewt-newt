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
	"os"
	"path/filepath"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/flash"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

func insertPartIntoBlob(blob []byte, part mfgPart) {
	partEnd := part.offset + len(part.data)

	if len(blob) < partEnd {
		panic("internal error; mfg blob too small")
	}

	copy(blob[part.offset:partEnd], part.data)
}

func (mi *MfgImage) partFromImage(
	imgPath string, flashAreaName string) (mfgPart, error) {

	part := mfgPart{}

	area, ok := mi.bsp.FlashMap.Areas[flashAreaName]
	if !ok {
		return part, util.FmtNewtError(
			"Image at \"%s\" requires undefined flash area \"%s\"",
			imgPath, flashAreaName)
	}

	part.name = fmt.Sprintf("%s (%s)", flashAreaName, filepath.Base(imgPath))
	part.offset = area.Offset

	var err error

	part.data, err = ioutil.ReadFile(imgPath)
	if err != nil {
		return part, util.ChildNewtError(err)
	}

	overflow := len(part.data) - area.Size
	if overflow > 0 {
		return part, util.FmtNewtError(
			"Image \"%s\" is too large to fit in flash area \"%s\"; "+
				"image-size=%d flash-area-size=%d overflow=%d",
			imgPath, flashAreaName, len(part.data), area.Size, overflow)
	}

	return part, nil
}

func (mi *MfgImage) section0Size() int {
	greatest := 0

	bootArea := mi.bsp.FlashMap.Areas[flash.FLASH_AREA_NAME_BOOTLOADER]
	image0Area := mi.bsp.FlashMap.Areas[flash.FLASH_AREA_NAME_IMAGE_0]
	image1Area := mi.bsp.FlashMap.Areas[flash.FLASH_AREA_NAME_IMAGE_1]

	if mi.boot != nil {
		greatest = util.IntMax(greatest, bootArea.Offset+bootArea.Size)
	}
	if len(mi.images) >= 1 {
		greatest = util.IntMax(greatest, image0Area.Offset+image0Area.Size)
	}
	if len(mi.images) >= 2 {
		greatest = util.IntMax(greatest, image1Area.Offset+image1Area.Size)
	}

	for _, section := range mi.rawSections {
		greatest = util.IntMax(greatest, section.offset+len(section.data))
	}

	return greatest
}

// @return						section-0-blob, hash-offset, error
func (mi *MfgImage) section0Data(parts []mfgPart) ([]byte, int, error) {
	blobSize := mi.section0Size()
	blob := make([]byte, blobSize)

	// Initialize section 0's data as unwritten flash (0xff).
	for i, _ := range blob {
		blob[i] = 0xff
	}

	for _, part := range parts {
		insertPartIntoBlob(blob, part)
	}

	hashOffset, err := insertMeta(blob, mi.bsp.FlashMap)
	if err != nil {
		return nil, 0, err
	}

	return blob, hashOffset, nil

}

func createImageHeader(hashOffset int) ([]byte, error) {
	buf := &bytes.Buffer{}

	hdr := mfgImageHeader{
		Version:    uint8(MFG_IMAGE_VERSION),
		HashOffset: uint32(hashOffset),
	}
	if err := binary.Write(buf, binary.BigEndian, hdr); err != nil {
		return nil, util.ChildNewtError(err)
	}

	return buf.Bytes(), nil
}

func createSectionHeader(deviceId int, offset int, size int) ([]byte, error) {
	buf := &bytes.Buffer{}

	sectionHdr := mfgImageSectionHeader{
		DeviceId: uint8(deviceId),
		Offset:   uint32(offset),
		Size:     uint32(size),
	}
	if err := binary.Write(buf, binary.BigEndian, sectionHdr); err != nil {
		return nil, util.ChildNewtError(err)
	}

	return buf.Bytes(), nil
}

func (mi *MfgImage) createBlob(parts []mfgPart) ([]byte, error) {
	section0Data, hashSubOff, err := mi.section0Data(parts)
	if err != nil {
		return nil, err
	}

	hashOff := MFG_IMAGE_HEADER_SIZE + MFG_IMAGE_SECTION_HEADER_SIZE +
		hashSubOff

	imageHdr, err := createImageHeader(hashOff)
	if err != nil {
		return nil, err
	}

	section0Hdr, err := createSectionHeader(0, 0, len(section0Data))
	if err != nil {
		return nil, err
	}

	blob := append(imageHdr, section0Hdr...)
	blob = append(blob, section0Data...)

	fillMetaHash(blob, hashOff)

	return blob, nil
}

// @return                      bin-path, error
func (mi *MfgImage) buildBoot() (string, error) {
	t, err := builder.NewTargetBuilder(mi.boot)
	if err != nil {
		return "", err
	}

	if err := t.Build(); err != nil {
		return "", err
	}

	binPath := t.AppBuilder.AppBinPath()
	project.ResetProject()

	return binPath, nil
}

// @return                      [[loader-path], app-path], error
func (mi *MfgImage) buildTarget(target *target.Target) ([]string, error) {
	t, err := builder.NewTargetBuilder(target)
	if err != nil {
		return nil, err
	}

	// XXX: Currently using made up version and key values.
	appImg, loaderImg, err := t.CreateImages("99.0.99.0", "", 0)
	if err != nil {
		return nil, err
	}

	paths := []string{}

	if loaderImg != nil {
		paths = append(paths, loaderImg.TargetImg)
	}

	paths = append(paths, appImg.TargetImg)

	project.ResetProject()

	return paths, nil
}

func areaNameFromImgIdx(imgIdx int) (string, error) {
	switch imgIdx {
	case 0:
		return flash.FLASH_AREA_NAME_IMAGE_0, nil
	case 1:
		return flash.FLASH_AREA_NAME_IMAGE_1, nil
	default:
		return "", util.FmtNewtError("invalid image index: %d", imgIdx)
	}
}

func (mi *MfgImage) rawSectionParts() []mfgPart {
	parts := make([]mfgPart, len(mi.rawSections))
	for i, section := range mi.rawSections {
		parts[i].name = fmt.Sprintf("section-%d (%s)", i, section.filename)
		parts[i].offset = section.offset
		parts[i].data = section.data
	}

	return parts
}

func (mi *MfgImage) build() ([]byte, error) {
	bootPath, err := mi.buildBoot()
	if err != nil {
		return nil, err
	}

	paths := []string{bootPath}

	bootPart, err := mi.partFromImage(
		paths[0],
		flash.FLASH_AREA_NAME_BOOTLOADER)
	if err != nil {
		return nil, err
	}

	imgParts := []mfgPart{}
	for _, img := range mi.images {
		paths, err := mi.buildTarget(img)
		if err != nil {
			return nil, err
		}

		for _, path := range paths {
			areaName, err := areaNameFromImgIdx(len(imgParts))
			if err != nil {
				return nil, err
			}

			part, err := mi.partFromImage(path, areaName)
			if err != nil {
				return nil, err
			}
			imgParts = append(imgParts, part)
		}
	}

	sectionParts := mi.rawSectionParts()

	parts := []mfgPart{bootPart}
	parts = append(parts, imgParts...)
	parts = append(parts, sectionParts...)

	sortParts(parts)

	blob, err := mi.createBlob(parts)
	if err != nil {
		return nil, err
	}

	return blob, nil
}

// @return                      path-of-image, [paths-of-sections], error
func (mi *MfgImage) CreateMfgImage() (string, []string, error) {
	blob, err := mi.build()
	if err != nil {
		return "", nil, err
	}

	dstPath := builder.MfgBinPath(mi.basePkg.Name())

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return "", nil, util.ChildNewtError(err)
	}

	if err := ioutil.WriteFile(dstPath, blob, 0644); err != nil {
		return "", nil, util.ChildNewtError(err)
	}

	sections, err := mi.ExtractSections()
	if err != nil {
		return "", nil, err
	}

	sectionPaths := make([]string, len(sections))
	for i, section := range sections {
		sectionPath := builder.MfgSectionPath(mi.basePkg.Name(), i)
		if err := ioutil.WriteFile(sectionPath, section, 0644); err != nil {
			return "", nil, util.ChildNewtError(err)
		}
		sectionPaths[i] = sectionPath
	}

	return dstPath, sectionPaths, nil
}
