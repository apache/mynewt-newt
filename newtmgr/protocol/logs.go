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

	"mynewt.apache.org/newt/util"
)

const (
	LOGS_NMGR_OP_READ        = 0
	LOGS_NMGR_OP_CLEAR       = 1
	LOGS_NMGR_OP_APPEND      = 2
	LOGS_NMGR_OP_MODULE_LIST = 3
	LOGS_NMGR_OP_LEVEL_LIST  = 4
)

type LogsShowReq struct {
	Name      string `json:"log_name"`
	Timestamp int64  `json:"ts"`
	Index     uint64 `json:"index"`
}

type LogsModuleListReq struct {
}

type LogsLevelListReq struct {
}

type LogEntry struct {
	Timestamp int64  `json:"ts"`
	Msg       string `json:"msg"`
	Level     uint64 `json:"level"`
	Index     uint64 `json:"index"`
	Module    uint64 `json:"module"`
}

type LogsShowLog struct {
	Name    string     `json:"name"`
	Type    uint64     `json:"type"`
	Entries []LogEntry `json:"entries"`
}

type LogsShowRsp struct {
	ReturnCode int           `json:"rc"`
	Logs       []LogsShowLog `json:"logs"`
}

type LogsModuleListRsp struct {
	ReturnCode int            `json:"rc"`
	Map        map[string]int `json:"module_map"`
}

type LogsLevelListRsp struct {
	ReturnCode int            `json:"rc"`
	Map        map[string]int `json:"level_map"`
}

func NewLogsShowReq() (*LogsShowReq, error) {
	s := &LogsShowReq{}

	return s, nil
}

func NewLogsModuleListReq() (*LogsModuleListReq, error) {
	s := &LogsModuleListReq{}

	return s, nil
}

func NewLogsLevelListReq() (*LogsLevelListReq, error) {
	s := &LogsLevelListReq{}

	return s, nil
}

func (sr *LogsModuleListReq) Encode() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_LOGS
	nmr.Id = LOGS_NMGR_OP_MODULE_LIST

	req := &LogsModuleListReq{}

	data, _ := json.Marshal(req)
	nmr.Data = data
	nmr.Len = uint16(len(data))

	return nmr, nil
}

func DecodeLogsLevelListResponse(data []byte) (*LogsLevelListRsp, error) {
	var resp LogsLevelListRsp
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}

	return &resp, nil
}

func (sr *LogsLevelListReq) Encode() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_LOGS
	nmr.Id = LOGS_NMGR_OP_LEVEL_LIST

	req := &LogsLevelListReq{}

	data, _ := json.Marshal(req)
	nmr.Data = data
	nmr.Len = uint16(len(data))

	return nmr, nil
}

func DecodeLogsModuleListResponse(data []byte) (*LogsModuleListRsp, error) {
	var resp LogsModuleListRsp
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}

	return &resp, nil
}

func (sr *LogsShowReq) Encode() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_LOGS
	nmr.Id = LOGS_NMGR_OP_READ
	req := &LogsShowReq{
		Name:      sr.Name,
		Timestamp: sr.Timestamp,
		Index:     sr.Index,
	}

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

func (sr *LogsClearReq) Encode() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
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

type LogsAppendReq struct {
	Msg   string `json:"msg"`
	Level uint   `json:"level"`
}

type LogsAppendRsp struct {
	ReturnCode int `json:"rc"`
}

func NewLogsAppendReq() (*LogsAppendReq, error) {
	s := &LogsAppendReq{}

	return s, nil
}

func (sr *LogsAppendReq) Encode() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_LOGS
	nmr.Id = LOGS_NMGR_OP_APPEND

	req := &LogsAppendReq{}

	data, _ := json.Marshal(req)
	nmr.Data = data
	nmr.Len = uint16(len(data))

	return nmr, nil
}

func DecodeLogsAppendResponse(data []byte) (*LogsAppendRsp, error) {
	var resp LogsAppendRsp
	err := json.Unmarshal(data, &resp)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming json: %s",
			err.Error()))
	}

	return &resp, nil
}
