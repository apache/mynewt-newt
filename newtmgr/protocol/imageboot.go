/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package protocol

import (
	"encoding/binary"
	"strconv"
	"strings"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
)

type ImageBoot struct {
	BootTarget  string
	TestImage   string
	MainImage   string
	ActiveImage string
}

func NewImageBoot() (*ImageBoot, error) {
	s := &ImageBoot{}
	s.BootTarget = ""
	s.TestImage = ""
	s.MainImage = ""
	s.ActiveImage = ""
	return s, nil
}

func (i *ImageBoot) EncodeWriteRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_OP_BOOT
	nmr.Len = 0

	if i.BootTarget != "" {
		verArray := strings.Split(i.BootTarget, ".")
		major, err := strconv.ParseUint(verArray[0], 10, 8)
		if err != nil {
			return nil, util.NewNewtError("Invalid version string")
		}
		minor, err := strconv.ParseUint(verArray[1], 10, 8)
		if err != nil {
			return nil, util.NewNewtError("Invalid version string")
		}

		var revision uint64 = 0
		if len(verArray) > 2 {
			revision, err = strconv.ParseUint(verArray[2], 10, 16)
			if err != nil {
				return nil, util.NewNewtError("Invalid version string")
			}
		}

		var buildNum uint64 = 0
		if len(verArray) > 3 {
			buildNum, err = strconv.ParseUint(verArray[3], 10, 32)
			if err != nil {
				return nil, util.NewNewtError("Invalid version string")
			}
		}

		u8b := make([]byte, 1)
		u16b := make([]byte, 2)
		u32b := make([]byte, 4)
		data := make([]byte, 0)

		u8b[0] = byte(major)
		data = append(data, u8b...)

		u8b[0] = byte(minor)
		data = append(data, u8b...)

		binary.BigEndian.PutUint16(u16b, uint16(revision))
		data = append(data, u16b...)

		binary.BigEndian.PutUint32(u32b, uint32(buildNum))
		data = append(data, u32b...)

		nmr.Data = data
		nmr.Len = 8
		nmr.Op = NMGR_OP_WRITE
	}
	return nmr, nil
}

func DecodeImageBootResponse(data []byte) (*ImageBoot, error) {
	i := &ImageBoot{}

	var idx int = 0
	for len(data) >= 8 {
		major := uint8(data[0])
		minor := uint8(data[1])
		revision := binary.BigEndian.Uint16(data[2:4])
		buildNum := binary.BigEndian.Uint32(data[4:8])
		data = data[8:]

		versStr := ImageVersStr(major, minor, revision, buildNum)
		switch idx {
		case 0:
			i.TestImage = versStr
		case 1:
			i.MainImage = versStr
		case 2:
			i.ActiveImage = versStr
		default:
			/* XXXX? */
		}
		idx++
	}

	return i, nil
}
