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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"mynewt.apache.org/newt/util"
)

type CoreDownload struct {
	File   *os.File
	Runner *CmdRunner
	Size   int
}

type coreLoadReq struct {
	Off uint32 `json:"off"`
}

type coreLoadResp struct {
	ErrCode uint32 `json:"rc"`
	Off     uint32 `json:"off"`
	Data    string
}

func NewCoreDownload() (*CoreDownload, error) {
	f := &CoreDownload{}

	return f, nil
}

func (cl *CoreDownload) Download(off, size uint32) error {
	if cl.File == nil {
		return util.NewNewtError("Missing target file")
	}
	if cl.Runner == nil {
		return util.NewNewtError("Missing target")
	}

	imageDone := 0
	var bytesWritten uint32 = 0

	nmr, err := NewNmgrReq()
	if err != nil {
		return err
	}
	req := &coreLoadReq{}

	for imageDone != 1 {
		req.Off = off
		data, _ := json.Marshal(req)

		nmr.Op = NMGR_OP_READ
		nmr.Flags = 0
		nmr.Group = NMGR_GROUP_ID_IMAGE
		nmr.Id = IMGMGR_NMGR_OP_CORELOAD
		nmr.Len = uint16(len(data))
		nmr.Data = data

		if err := cl.Runner.WriteReq(nmr); err != nil {
			return err
		}

		nmRsp, err := cl.Runner.ReadResp()
		if err != nil {
			return err
		}

		clRsp := coreLoadResp{}
		if err = json.Unmarshal(nmRsp.Data, &clRsp); err != nil {
			return util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
				err.Error()))
		}
		if clRsp.ErrCode == NMGR_ERR_ENOENT {
			return util.NewNewtError("No corefile present")
		}
		if clRsp.ErrCode != 0 {
			return util.NewNewtError(fmt.Sprintf("Download failed: %d",
				clRsp.ErrCode))
		}

		if off != clRsp.Off {
			return util.NewNewtError(
				fmt.Sprintf("Invalid data offset %d, expected %d",
					clRsp.Off, off))
		}

		data, err = base64.StdEncoding.DecodeString(clRsp.Data)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
				err.Error()))
		}
		if len(data) > 0 {
			if size > 0 && uint32(len(data))+bytesWritten >= size {
				data = data[:size-bytesWritten]
				imageDone = 1
			}
			n, err := cl.File.Write(data)
			if err == nil && n < len(data) {
				err = io.ErrShortWrite
			}
			if err != nil {
				return util.NewNewtError(
					fmt.Sprintf("Cannot write to file: %s",
						err.Error()))
			}
			off += uint32(len(data))
			bytesWritten += uint32(len(data))
		} else {
			imageDone = 1
		}
	}
	return nil
}
