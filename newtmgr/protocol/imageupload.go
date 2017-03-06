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

type ImageUpload struct {
	Offset     uint32 `codec:"off"`
	Size       uint32
	Data       []byte
	ReturnCode int `codec:"rc"`
}

func NewImageUpload() (*ImageUpload, error) {
	s := &ImageUpload{}
	s.Offset = 0

	return s, nil
}

func (i *ImageUpload) EncodeWriteRequest() (*NmgrReq, error) {
	type UploadReq struct {
		Off  uint32 `codec:"off"`
		Data []byte `codec:"data"`
	}
	type UploadFirstReq struct {
		Off  uint32 `codec:"off"`
		Size uint32 `codec:"len"`
		Data []byte `codec:"data"`
	}
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_ID_UPLOAD

	data := []byte{}

	if i.Offset == 0 {
		uploadReq := &UploadFirstReq{
			Off:  i.Offset,
			Size: i.Size,
			Data: i.Data,
		}
		enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))
		enc.Encode(uploadReq)
	} else {
		uploadReq := &UploadReq{
			Off:  i.Offset,
			Data: i.Data,
		}
		enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))
		enc.Encode(uploadReq)
	}
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func DecodeImageUploadResponse(data []byte) (*ImageUpload, error) {
	i := &ImageUpload{}

	dec := codec.NewDecoderBytes(data, new(codec.CborHandle))
	err := dec.Decode(&i)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming cbor: %s",
			err.Error()))
	}
	if i.ReturnCode != 0 {
		return nil, util.NewNewtError(fmt.Sprintf("Target error: %d",
			i.ReturnCode))
	}
	return i, nil
}
