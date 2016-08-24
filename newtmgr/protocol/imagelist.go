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
	"encoding/hex"
	"encoding/json"
	"fmt"

	"mynewt.apache.org/newt/util"
)

type ImageList struct {
	Images []string
}

const (
	IMGMGR_NMGR_OP_LIST     = 0
	IMGMGR_NMGR_OP_UPLOAD   = 1
	IMGMGR_NMGR_OP_BOOT     = 2
	IMGMGR_NMGR_OP_FILE     = 3
	IMGMGR_NMGR_OP_LIST2    = 4
	IMGMGR_NMGR_OP_BOOT2    = 5
	IMGMGR_NMGR_OP_CORELIST = 6
	IMGMGR_NMGR_OP_CORELOAD = 7
)

func HashDecode(src string) (string, error) {
	imgHex, err := base64.StdEncoding.DecodeString(src)
	if err != nil {
		return "", util.NewNewtError(fmt.Sprintf("Hash decode error: %s",
			err.Error()))
	}
	return hex.EncodeToString(imgHex), nil
}

func HashEncode(src string) (string, error) {
	imgHex, err := hex.DecodeString(src)
	if err != nil {
		return "", util.NewNewtError(fmt.Sprintf("Hash encode error: %s",
			err.Error()))
	}
	return base64.StdEncoding.EncodeToString(imgHex), nil
}

func NewImageList() (*ImageList, error) {
	s := &ImageList{}
	s.Images = []string{}
	return s, nil
}

func (i *ImageList) EncodeWriteRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_OP_LIST
	nmr.Len = 0

	return nmr, nil
}

func DecodeImageListResponse(data []byte) (*ImageList, error) {
	list := &ImageList{}

	err := json.Unmarshal(data, &list)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}
	return list, nil
}
