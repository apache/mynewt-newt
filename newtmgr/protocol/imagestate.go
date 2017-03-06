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
	"fmt"

	"github.com/ugorji/go/codec"
	"mynewt.apache.org/newt/util"
)

type ImageStateEntry struct {
	Slot      int    `codec:"slot"`
	Version   string `codec:"version"`
	Hash      []byte `codec:"hash"`
	Bootable  bool   `codec:"bootable"`
	Pending   bool   `codec:"pending"`
	Confirmed bool   `codec:"confirmed"`
	Active    bool   `codec:"active"`
	Permanent bool   `codec:"permanent"`
}

type ImageStateRsp struct {
	ReturnCode  int               `codec:"rc"`
	Images      []ImageStateEntry `codec:"images"`
	SplitStatus SplitStatus       `codec:"splitStatus"`
}

type ImageStateReadReq struct {
}

type ImageStateWriteReq struct {
	Hash    []byte `codec:"hash"`
	Confirm bool   `codec:"confirm"`
}

func NewImageStateReadReq() (*ImageStateReadReq, error) {
	s := &ImageStateReadReq{}
	return s, nil
}

func NewImageStateWriteReq() (*ImageStateWriteReq, error) {
	s := &ImageStateWriteReq{}
	return s, nil
}

func NewImageStateRsp() (*ImageStateRsp, error) {
	s := &ImageStateRsp{}
	return s, nil
}

func (i *ImageStateReadReq) Encode() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_ID_STATE
	nmr.Len = 0

	return nmr, nil
}

func (i *ImageStateWriteReq) Encode() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	clone, err := NewImageStateWriteReq()
	if err != nil {
		return nil, err
	}

	clone.Confirm = i.Confirm

	if len(i.Hash) != 0 {
		clone.Hash = i.Hash
		if err != nil {
			return nil, err
		}
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_ID_STATE

	data := make([]byte, 0)
	enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))
	enc.Encode(clone)
	nmr.Data = data
	nmr.Len = uint16(len(data))

	return nmr, nil
}

func DecodeImageStateResponse(data []byte) (*ImageStateRsp, error) {
	rsp := &ImageStateRsp{}

	dec := codec.NewDecoderBytes(data, new(codec.CborHandle))
	err := dec.Decode(&rsp)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming cbor: %s",
			err.Error()))
	}
	return rsp, nil
}
