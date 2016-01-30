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
	"fmt"
	"log"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
)

type NmgrReq struct {
	Op    uint8
	Flags uint8
	Len   uint16
	Group uint16
	Id    uint16
	Data  []byte
}

const (
	NMGR_OP_READ      = 0
	NMGR_OP_READ_RSP  = 1
	NMGR_OP_WRITE     = 2
	NMGR_OP_WRITE_RSP = 3
)

const (
	NMGR_GROUP_ID_DEFAULT = 0
	NMGR_GROUP_ID_IMAGE   = 1
	NMGR_GROUP_ID_CONFIG  = 3
)

func NewNmgrReq() (*NmgrReq, error) {
	nmr := &NmgrReq{}
	nmr.Data = []byte{}

	return nmr, nil
}

func DeserializeNmgrReq(data []byte) (*NmgrReq, error) {
	if len(data) < 8 {
		return nil, util.NewNewtError(fmt.Sprintf(
			"Newtmgr request buffer too small %d bytes", len(data)))
	}

	nmr := &NmgrReq{}

	nmr.Op = uint8(data[0])
	nmr.Flags = uint8(data[1])
	nmr.Len = binary.BigEndian.Uint16(data[2:4])
	nmr.Group = binary.BigEndian.Uint16(data[4:6])
	nmr.Id = binary.BigEndian.Uint16(data[6:8])

	data = data[8:]
	if int(nmr.Len) != len(data) {
		return nil, util.NewNewtError(
			fmt.Sprintf("Newtmgr request length doesn't match data length."+
				"  Newtmgr length = %d, Data length = %d\n", nmr.Len,
				len(data)))
	}
	nmr.Data = data

	return nmr, nil
}

func (nmr *NmgrReq) SerializeRequest(data []byte) ([]byte, error) {
	log.Printf("[DEBUG] Serializing request %s into buffer %s", nmr, data)

	u16b := make([]byte, 2)

	data = append(data, byte(nmr.Op))
	data = append(data, byte(nmr.Flags))

	binary.BigEndian.PutUint16(u16b, nmr.Len)
	data = append(data, u16b...)

	binary.BigEndian.PutUint16(u16b, nmr.Group)
	data = append(data, u16b...)

	binary.BigEndian.PutUint16(u16b, nmr.Id)
	data = append(data, u16b...)

	data = append(data, nmr.Data...)

	return data, nil
}
