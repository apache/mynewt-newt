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
	"strings"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/util"
)

func (mi *MfgImage) Validate() error {
	binPath := builder.MfgBinPath(mi.basePkg.Name())

	blob, err := ioutil.ReadFile(binPath)
	if err != nil {
		return util.ChildNewtError(err)
	}

	buf := bytes.NewReader(blob)
	var hdr mfgImageHeader
	if err := binary.Read(buf, binary.BigEndian, &hdr); err != nil {
		return util.ChildNewtError(err)
	}

	if hdr.Version != MFG_IMAGE_VERSION {
		return util.FmtNewtError(
			"Manufacturing image \"%s\" specifies unrecognized version: %d",
			binPath, hdr.Version)
	}

	if int(hdr.HashOffset)+META_HASH_SZ > len(blob) {
		return util.FmtNewtError(
			"Manufacturing image \"%s\" specifies hash offset beyond end of "+
				"file: %d",
			binPath, hdr.HashOffset)
	}

	writtenHash := blob[hdr.HashOffset : hdr.HashOffset+META_HASH_SZ]
	calcHash := calcMetaHash(blob, int(hdr.HashOffset))
	if bytes.Compare(writtenHash, calcHash) != 0 {
		return util.FmtNewtError(
			"Manufacturing image \"%s\" contains incorrect hash; "+
				"expected=%x actual=%x",
			binPath, calcHash, writtenHash)
	}

	return nil
}

func extractSection(mfgImageBlob []byte, sectionOff int) (int, []byte, error) {
	var hdr mfgImageSectionHeader

	buf := bytes.NewReader(mfgImageBlob)
	if _, err := buf.Seek(int64(MFG_IMAGE_HEADER_SIZE), 0); err != nil {
		return 0, nil, util.ChildNewtError(err)
	}

	if err := binary.Read(buf, binary.BigEndian, &hdr); err != nil {
		return 0, nil, util.ChildNewtError(err)
	}

	fmt.Printf("off=%d SECTIONHDR=%+v\n", sectionOff, hdr)

	dataOff := sectionOff + MFG_IMAGE_SECTION_HEADER_SIZE
	dataEnd := dataOff + int(hdr.Size)
	if dataEnd > len(mfgImageBlob) {
		return 0, nil, util.FmtNewtError(
			"invalid mfg image; section %d (off=%d len=%d) extends beyond "+
				"dataEnd of image (len=%d)",
			hdr.DeviceId, sectionOff, hdr.Size, len(mfgImageBlob))
	}

	// 0xff-fill the pre-data portion of the section.
	sectionData := make([]byte, hdr.Offset+hdr.Size)
	for i := 0; i < int(hdr.Offset); i++ {
		sectionData[i] = 0xff
	}

	// Copy data into section.
	src := mfgImageBlob[dataOff:dataEnd]
	dst := sectionData[hdr.Offset : hdr.Offset+hdr.Size]
	copy(dst, src)

	return int(hdr.DeviceId), sectionData, nil
}

func (mi *MfgImage) ExtractSections() ([][]byte, error) {
	if err := mi.Validate(); err != nil {
		return nil, err
	}

	binPath := builder.MfgBinPath(mi.basePkg.Name())

	data, err := ioutil.ReadFile(binPath)
	if err != nil {
		return nil, util.ChildNewtError(err)
	}

	sections := [][]byte{}
	off := MFG_IMAGE_HEADER_SIZE
	for off < len(data) {
		sectionIdx, sectionData, err := extractSection(data, off)
		if err != nil {
			return nil, err
		}

		for len(sections) <= sectionIdx {
			sections = append(sections, nil)
		}

		sections[sectionIdx] = sectionData

		off += MFG_IMAGE_SECTION_HEADER_SIZE + len(sectionData)
	}

	return sections, nil
}

// @return						mfg-image-path, error
func (mi *MfgImage) Upload() (string, error) {
	// For now, we always upload section 0 only.
	section0Path := builder.MfgSectionPath(mi.basePkg.Name(), 0)
	baseName := strings.TrimSuffix(section0Path, ".bin")

	envSettings := map[string]string{"MFG_IMAGE": "1"}
	if err := builder.Load(baseName, mi.bsp, envSettings); err != nil {
		return "", err
	}

	return section0Path, nil
}
