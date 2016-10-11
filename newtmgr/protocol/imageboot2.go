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

type ImageBoot2 struct {
	BootTarget []byte
	Test       []byte `codec:"test"`
	Main       []byte `codec:"main"`
	Active     []byte `codec:"active"`
	ReturnCode int    `codec:"rc"`
}

func NewImageBoot2() (*ImageBoot2, error) {
	s := &ImageBoot2{}
	s.BootTarget = make([]byte, 0)
	s.Test = make([]byte, 0)
	s.Main = make([]byte, 0)
	s.Active = make([]byte, 0)
	return s, nil
}

func (i *ImageBoot2) EncodeWriteRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_OP_BOOT2
	nmr.Len = 0

	if len(i.BootTarget) != 0 {
		type BootReq struct {
			Test []byte `codec:"test"`
		}

		bReq := &BootReq{
			Test: i.BootTarget,
		}
		data := make([]byte, 0)
		enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))
		enc.Encode(bReq)
		nmr.Data = data
		nmr.Len = uint16(len(data))
		nmr.Op = NMGR_OP_WRITE
	}
	return nmr, nil
}

func DecodeImageBoot2Response(data []byte) (*ImageBoot2, error) {
	i := &ImageBoot2{}

	if len(data) == 0 {
		return i, nil
	}
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
