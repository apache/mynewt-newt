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

type Crash struct {
	crashType string
	Err       int `json:"rc"`
}

func NewCrash(crashType string) (*Crash, error) {
	c := &Crash{
		crashType: crashType,
	}
	return c, nil
}

func (c *Crash) EncodeWriteRequest() (*NmgrReq, error) {
	msg := "{\"t\":\""
	msg += c.crashType
	msg += "\"}"

	data := []byte(msg)

	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_CRASH
	nmr.Id = 0
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func DecodeCrashResponse(data []byte) (*Crash, error) {
	c := &Crash{}

	if err := json.Unmarshal(data, &c); err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid response: %s",
			err.Error()))
	}
	return c, nil
}
