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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
)

type ImageUpload struct {
	Offset  uint32	`json:"off"`
	Data []byte
}

func NewImageUpload() (*ImageUpload, error) {
	s := &ImageUpload{}
	s.Offset = 0

	return s, nil
}

func (i *ImageUpload) EncodeWriteRequest() (*NmgrReq, error) {
	type UploadReq struct {
		Off uint32 `json:"off"`
		Data string `json:"data"`
	}
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_OP_UPLOAD

	uploadReq := &UploadReq{
		Off : i.Offset,
		Data : base64.StdEncoding.EncodeToString(i.Data),
	}
	data, _ := json.Marshal(uploadReq)
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func DecodeImageUploadResponse(data []byte) (*ImageUpload, error) {
	i := &ImageUpload{}

	err := json.Unmarshal(data, &i)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}
	return i, nil
}
