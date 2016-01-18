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
	"encoding/json"

	"fmt"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
)

type ImageBoot struct {
	BootTarget string
	Test       string
	Main       string
	Active     string
}

func NewImageBoot() (*ImageBoot, error) {
	s := &ImageBoot{}
	s.BootTarget = ""
	s.Test = ""
	s.Main = ""
	s.Active = ""
	return s, nil
}

func (i *ImageBoot) EncodeWriteRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_IMAGE
	nmr.Id = IMGMGR_NMGR_OP_BOOT
	nmr.Len = 0

	if i.BootTarget != "" {
		type BootReq struct {
			Test string `json:"test"`
		}

		bReq := &BootReq{
			Test: i.BootTarget,
		}
		data, _ := json.Marshal(bReq)
		nmr.Data = data
		nmr.Len = uint16(len(data))
		nmr.Op = NMGR_OP_WRITE
	}
	return nmr, nil
}

func DecodeImageBootResponse(data []byte) (*ImageBoot, error) {
	i := &ImageBoot{}

	if len(data) == 0 {
		return i, nil
	}
	err := json.Unmarshal(data, &i)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}
	return i, nil
}
