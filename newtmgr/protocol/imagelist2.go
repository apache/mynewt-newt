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
	"encoding/json"
	"fmt"

	"mynewt.apache.org/newt/util"
)

type ImageList2 struct {
	Images map[string]string
}

func NewImageList2() (*ImageList2, error) {
	s := &ImageList2{}
	s.Images = map[string]string{}
	return s, nil
}

func (i *ImageList2) EncodeWriteRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_OP_LIST2
	nmr.Len = 0

	return nmr, nil
}

func DecodeImageListResponse2(data []byte) (*ImageList2, error) {
	type ImageInfoJson map[string]string

	type ImageListJson struct {
		Images []ImageInfoJson
	}

	list := &ImageListJson{}

	err := json.Unmarshal(data, &list)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}

	list2, _ := NewImageList2()
	for _, info := range list.Images {
		for hash, ver := range info {
			hash, err := HashDecode(hash)
			if err != nil {
				return nil, err
			}
			list2.Images[hash] = ver
		}
	}
	return list2, nil
}
