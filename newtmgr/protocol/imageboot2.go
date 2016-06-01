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

type ImageBoot2 struct {
	BootTarget string
	Test       string
	Main       string
	Active     string
	ReturnCode int `json:"rc"`
}

func NewImageBoot2() (*ImageBoot2, error) {
	s := &ImageBoot2{}
	s.BootTarget = ""
	s.Test = ""
	s.Main = ""
	s.Active = ""
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

	if i.BootTarget != "" {
		type BootReq struct {
			Test string `json:"test"`
		}

		hash, err := HashEncode(i.BootTarget)
		if err != nil {
			return nil, err
		}
		bReq := &BootReq{
			Test: hash,
		}
		data, _ := json.Marshal(bReq)
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
	err := json.Unmarshal(data, &i)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}
	if i.ReturnCode != 0 {
		return nil, util.NewNewtError(fmt.Sprintf("Target error: %d",
			i.ReturnCode))
	}
	if i.Test != "" {
		i.Test, err = HashDecode(i.Test)
		if err != nil {
			return nil, err
		}
	}
	if i.Main != "" {
		i.Main, err = HashDecode(i.Main)
		if err != nil {
			return nil, err
		}
	}
	if i.Active != "" {
		i.Active, err = HashDecode(i.Active)
		if err != nil {
			return nil, err
		}
	}
	return i, nil
}
