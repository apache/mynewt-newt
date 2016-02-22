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

const (
	LOGS_NMGR_OP_READ  = 0
	LOGS_NMGR_OP_CLEAR = 1
)

type LogsShowReq struct {
}

type LogsShowLog struct {
	Timestamp uint64 `json:"ts"`
	Log       string `json:"log"`
}

type LogsShowRsp struct {
	ReturnCode int           `json:"rc"`
	Logs       []LogsShowLog `json:"logs"`
}

func NewLogsShowReq() (*LogsShowReq, error) {
	s := &LogsShowReq{}

	return s, nil
}

func (sr *LogsShowReq) EncodeWriteRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_LOGS
	nmr.Id = LOGS_NMGR_OP_READ

	req := &LogsShowReq{}

	data, _ := json.Marshal(req)
	nmr.Data = data
	nmr.Len = uint16(len(data))

	return nmr, nil
}

func DecodeLogsShowResponse(data []byte) (*LogsShowRsp, error) {
	var resp LogsShowRsp
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}

	return &resp, nil
}

type LogsClearReq struct {
}

type LogsClearRsp struct {
	ReturnCode int `json:"rc"`
}

func NewLogsClearReq() (*LogsClearReq, error) {
	s := &LogsClearReq{}

	return s, nil
}

func (sr *LogsClearReq) EncodeWriteRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_LOGS
	nmr.Id = LOGS_NMGR_OP_CLEAR

	req := &LogsClearReq{}

	data, _ := json.Marshal(req)
	nmr.Data = data
	nmr.Len = uint16(len(data))

	return nmr, nil
}

func DecodeLogsClearResponse(data []byte) (*LogsClearRsp, error) {
	var resp LogsClearRsp
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}

	return &resp, nil
}
