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

type FileDownload struct {
	Offset uint32
	Size   uint32
	Name   string
	Data   []byte
}

func NewFileDownload() (*FileDownload, error) {
	f := &FileDownload{}
	f.Offset = 0
	f.Data = make([]byte, 0)

	return f, nil
}

func (f *FileDownload) EncodeWriteRequest() (*NmgrReq, error) {
	type DownloadReq struct {
		Off  uint32 `codec:"off"`
		Name string `codec:"name"`
	}
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_FS
	nmr.Id = FS_NMGR_ID_FILE

	downloadReq := &DownloadReq{
		Off:  f.Offset,
		Name: f.Name,
	}

	data := make([]byte, 0)
	enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))
	enc.Encode(downloadReq)
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func DecodeFileDownloadResponse(data []byte) (*FileDownload, error) {
	type DownloadResp struct {
		Off        uint32 `json:"off"`
		Size       uint32 `json:"len"`
		Data       []byte `json:"data"`
		ReturnCode int    `json:"rc"`
	}
	resp := &DownloadResp{}

	dec := codec.NewDecoderBytes(data, new(codec.CborHandle))
	err := dec.Decode(&resp)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming cbor: %s",
			err.Error()))
	}
	if resp.ReturnCode != 0 {
		return nil, util.NewNewtError(fmt.Sprintf("Target error: %d",
			resp.ReturnCode))
	}
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}
	f := &FileDownload{
		Offset: resp.Off,
		Data:   resp.Data,
		Size:   resp.Size,
	}
	return f, nil
}
