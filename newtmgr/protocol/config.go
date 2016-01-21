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

type Config struct {
	Name  string `json:"name"`
	Value string `json:"val"`
}

func NewConfig() (*Config, error) {
	c := &Config{}
	return c, nil
}

func (c *Config) EncodeRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_CONFIG
	nmr.Id = 0
	nmr.Len = 0

	if c.Value == "" {
		type ConfigReadReq struct {
			Name string `json:"name"`
		}

		readReq := &ConfigReadReq{
			Name: c.Name,
		}
		data, _ := json.Marshal(readReq)
		nmr.Data = data
		nmr.Op = NMGR_OP_READ
	} else {
		data, _ := json.Marshal(c)
		nmr.Data = data
	}
	nmr.Len = uint16(len(nmr.Data))
	return nmr, nil
}

func DecodeConfigResponse(data []byte) (*Config, error) {
	c := &Config{}

	if len(data) == 0 {
		return c, nil
	}

	err := json.Unmarshal(data, &c)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}
	return c, nil
}
