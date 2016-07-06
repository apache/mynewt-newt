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
	"strings"

	"mynewt.apache.org/newt/util"
)

const (
	SPLIT_NMGR_OP_SPLIT = 0
)

type SplitMode int

const (
	NONE SplitMode = iota
	TEST
	RUN
)

var splitMode = [...]string{NONE: "none", TEST: "test", RUN: "run"}

/* is the enum valid */
func (sm SplitMode) Valid() bool {
	for val, _ := range splitMode {
		if int(sm) == val {
			return true
		}
	}
	return false
}

/* returns the enum as a string */
func (sm SplitMode) String() string {
	if sm > RUN || sm < 0 {
		return "Invalid!"
	}
	return splitMode[sm]
}

type SplitStatus int

const (
	NOT_APPLICABLE SplitStatus = iota
	NOT_MATCHING
	MATCHING
)

/* parses the enum from a string */
func ParseSplitMode(str string) (SplitMode, error) {
	for val, key := range splitMode {
		if strings.EqualFold(key, str) {
			return SplitMode(val), nil
		}
	}
	return NONE, util.NewNewtError("Invalid value for Split Mode %v" + str)
}

var splitStatus = [...]string{NOT_APPLICABLE: "N/A", NOT_MATCHING: "Non-matching", MATCHING: "matching"}

/* is the enum valid */
func (sm SplitStatus) Valid() bool {
	for val, _ := range splitStatus {
		if int(sm) == val {
			return true
		}
	}
	return false
}

/* returns the enum as a string */
func (sm SplitStatus) String() string {
	if sm > MATCHING || sm < 0 {
		return "Unknown!"
	}
	return splitStatus[sm]
}

/* parses the enum from a string */
func ParseSplitStatus(str string) (SplitStatus, error) {
	for val, key := range splitStatus {
		if strings.EqualFold(key, str) {
			return SplitStatus(val), nil
		}
	}
	return NOT_APPLICABLE, util.NewNewtError("Invalid value for Split Status %v" + str)
}

type Split struct {
	Split      SplitMode   `json:"splitMode"`
	Status     SplitStatus `json:"splitStatus"`
	ReturnCode int         `json:"rc"`
}

func NewSplit() (*Split, error) {
	s := &Split{}
	return s, nil
}

func (s *Split) EncoderReadRequest() (*NmgrReq, error) {
	msg := "{}"

	data := []byte(msg)

	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_SPLIT
	nmr.Id = SPLIT_NMGR_OP_SPLIT
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func (s *Split) EncoderWriteRequest() (*NmgrReq, error) {

	data, err := json.Marshal(s)

	if err != nil {
		return nil, err
	}

	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_SPLIT
	nmr.Id = SPLIT_NMGR_OP_SPLIT
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func DecodeSplitReadResponse(data []byte) (*Split, error) {
	i := &Split{}

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
	return i, nil
}
