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
)

type ImageUpload struct {
	Offset uint32
	Data   []byte
}

func NewImageUpload() (*ImageUpload, error) {
	s := &ImageUpload{}
	s.Offset = 0
	s.Data = []byte{}

	return s, nil
}

func (i *ImageUpload) SerializeImageUploadReq(data []byte) ([]byte, error) {
	u32b := make([]byte, 4)

	binary.BigEndian.PutUint32(u32b, i.Offset)
	data = append(data, u32b...)

	data = append(data, i.Data...)

	return data, nil
}

func (i *ImageUpload) EncodeWriteRequest() (*NmgrReq, error) {
	data := []byte(i.Data)

	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = 1
	nmr.Len = uint16(len(data) + 4)
	data, err = i.SerializeImageUploadReq([]byte{})
	if err != nil {
		return nil, err
	}
	nmr.Data = data

	return nmr, nil
}

func DecodeImageUploadResponse(data []byte) (*ImageUpload, error) {
	i := &ImageUpload{}
	i.Offset = binary.BigEndian.Uint32(data[0:4])
	i.Data = data[4:]

	return i, nil
}
