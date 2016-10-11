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
	Slot        int    `codec:"slot"`
	Version     string `codec:"version"`
	Hash        string `codec:"hash"`
	Bootable    bool   `codec:"bootable"`
	Pending bool   `codec:"test-pending"`
	Confirmed   bool   `codec:"confirmed"`
	Active      bool   `codec:"active"`
}

type ImageState struct {
	Images []ImageStateEntry `codec:"images"`
}

func NewImageState() (*ImageState, error) {
	s := &ImageState{}
	return s, nil
}

func (i *ImageState) EncodeWriteRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_OP_STATE
	nmr.Len = 0

	return nmr, nil
}

func DecodeImageStateResponse(data []byte) (*ImageState, error) {
	state := &ImageState{}

	dec := codec.NewDecoderBytes(data, new(codec.JsonHandle))
	err := dec.Decode(&state)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}
	return state, nil
}
