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

package cli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"io"
	"os"
//	"path/filepath"
	"strconv"
	"strings"
)

type ImageVersion struct {
	Major    uint8
	Minor    uint8
	Rev      uint16
	BuildNum uint32
}

type Image struct {
	target    *Target

	SourceBin string
	TargetImg string

	version   ImageVersion
}

type ImageHdr struct {
        Magic uint32
        Pad1  uint32
        HdrSz uint32
        ImgSz uint32
        Flags uint32
        Vers  ImageVersion
        Pad2  uint32
}

type ImageTrailerTlv struct {
	Type uint8
	Pad  uint8
	Len  uint16
}

const (
        IMAGE_MAGIC = 0x96f3b83c /* Image header magic */
)

const (
	IMAGE_HEADER_SIZE = 32
)

/*
 * Image header flags.
 */
const (
        IMAGE_F_PIC        = 0x00000001
        IMAGE_F_HAS_SHA256 = 0x00000002 /* Image contains hash TLV */
)

/*
 * Image trailer TLV types.
 */
const (
        IMAGE_TLV_SHA256 = 1
)

func NewImage(t *Target) (*Image, error) {
	image := &Image{
		target: t,
	}
	return image, nil
}

func (image *Image) SetVersion(versStr string) error {
	var err error
	var major uint64
	var minor uint64
	var rev uint64
	var buildNum uint64

	components := strings.Split(versStr, ".")
	major, err = strconv.ParseUint(components[0], 10, 8)
	if err != nil {
		return NewNewtError(fmt.Sprintf("Invalid version string %s", versStr))
	}
	if len(components) > 1 {
		minor, err = strconv.ParseUint(components[1], 10, 8)
		if err != nil {
			return NewNewtError(fmt.Sprintf("Invalid version string %s",
				versStr))
		}
	}
	if len(components) > 2 {
		rev, err = strconv.ParseUint(components[2], 10, 16)
		if err != nil {
			return NewNewtError(fmt.Sprintf("Invalid version string %s",
				versStr))
		}
	}
	if len(components) > 3 {
		buildNum, err = strconv.ParseUint(components[3], 10, 32)
		if err != nil {
			return NewNewtError(fmt.Sprintf("Invalid version string %s",
				versStr))
		}
	}
	image.version.Major = uint8(major)
	image.version.Minor = uint8(minor)
	image.version.Rev = uint16(rev)
	image.version.BuildNum = uint32(buildNum)
	log.Printf("[VERBOSE] Version number %d.%d.%d.%d\n",
		image.version.Major, image.version.Minor,
		image.version.Rev, image.version.BuildNum)

	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.LittleEndian, image.version)
	if err != nil {
		fmt.Printf("Bombing out\n")
		return nil
	}

	return nil
}

func (image *Image) Generate() error {
	binFile, err := os.Open(image.SourceBin)
	if err != nil {
		return NewNewtError(fmt.Sprintf("Can't open target binary: %s",
			err.Error()))
	}
	binInfo, err := binFile.Stat()
	if err != nil {
		return NewNewtError(fmt.Sprintf("Can't stat target binary: %s",
			err.Error()))
	}

	imgFile, err := os.OpenFile(image.TargetImg,
		os.O_CREATE | os.O_TRUNC | os.O_WRONLY, 0777)
	if err != nil {
		return NewNewtError(fmt.Sprintf("Can't open target image: %s",
			err.Error()))
	}

	/*
	 * First the header
	 */
	hdr := &ImageHdr {
		Magic: IMAGE_MAGIC,
		Pad1:  0,
		HdrSz: IMAGE_HEADER_SIZE,
		ImgSz: uint32(binInfo.Size()),
		Flags: 0,
		Vers:  image.version,
		Pad2:  0,
	}

	err = binary.Write(imgFile, binary.LittleEndian, hdr)
	if err != nil {
		return NewNewtError(fmt.Sprintf("Failed to serialize image hdr: %s",
			err.Error()))
	}

	/*
	 * Followed by data.
	 */
	dataBuf := make([]byte, 1024)
	for {
		cnt, err := binFile.Read(dataBuf)
		if err != nil && err != io.EOF {
			return NewNewtError(fmt.Sprintf("Failed to read from %s: %s",
				image.SourceBin, err.Error()))
		}
		if cnt == 0 {
			break
		}
		_, err = imgFile.Write(dataBuf[0:cnt])
		if err != nil {
			return NewNewtError(fmt.Sprintf("Failed to write to %s: %s",
				image.TargetImg, err.Error()))
		}
	}
	binFile.Close()
	imgFile.Close()

	return nil
}
