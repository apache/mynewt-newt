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

package protocol

import (
	"encoding/binary"
	"fmt"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/util"
)

type NmgrReq struct {
	Op    uint8 /* 3 bits of opcode */
	Flags uint8
	Len   uint16
	Group uint16
	Seq   uint8
	Id    uint8
	Data  []byte
}

const (
	NMGR_OP_READ      = 0
	NMGR_OP_READ_RSP  = 1
	NMGR_OP_WRITE     = 2
	NMGR_OP_WRITE_RSP = 3
)

const (
	NMGR_ERR_OK       = 0
	NMGR_ERR_EUNKNOWN = 1
	NMGR_ERR_ENOMEM   = 2
	NMGR_ERR_EINVAL   = 3
	NMGR_ERR_ETIMEOUT = 4
	NMGR_ERR_ENOENT   = 5
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
	nmr.Seq = uint8(data[6])
	nmr.Id = uint8(data[7])

	data = data[8:]
	nmr.Data = data

	log.Debugf("Deserialized response %+v", nmr)

	return nmr, nil
}

func (nmr *NmgrReq) SerializeRequest(data []byte) ([]byte, error) {

	u16b := make([]byte, 2)

	data = append(data, byte(nmr.Op))
	data = append(data, byte(nmr.Flags))

	binary.BigEndian.PutUint16(u16b, nmr.Len)
	data = append(data, u16b...)

	binary.BigEndian.PutUint16(u16b, nmr.Group)
	data = append(data, u16b...)

	data = append(data, byte(nmr.Seq))
	data = append(data, byte(nmr.Id))

	data = append(data, nmr.Data...)

	log.Debugf("Serializing request %+v into buffer %+v", nmr, data)
	return data, nil
}
