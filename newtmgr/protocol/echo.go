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
	"strconv"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
)

type Echo struct {
	Message string
}

const (
	NMGR_ID_ECHO           = 0
	NMGR_ID_CONS_ECHO_CTRL = 1
)

func NewEcho() (*Echo, error) {
	s := &Echo{}
	return s, nil
}

func (e *Echo) EncodeWriteRequest() (*NmgrReq, error) {
	msg := "{\"d\": \""
	msg += e.Message
	msg += "\"}"

	data := []byte(msg)

	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_DEFAULT
	nmr.Id = NMGR_ID_ECHO
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func (e *Echo) EncodeEchoCtrl() (*NmgrReq, error) {
	type SerialEchoCtl struct {
		Echo int `json:"echo"`
	}

	integer, err := strconv.Atoi(e.Message)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid echo ctrl setting %s",
			err.Error()))
	}
	echoCtl := &SerialEchoCtl{
		Echo: integer,
	}

	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_DEFAULT
	nmr.Id = NMGR_ID_CONS_ECHO_CTRL

	data, _ := json.Marshal(echoCtl)
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func DecodeEchoResponse(data []byte) (*Echo, error) {
	e := &Echo{}
	e.Message = string(data[:])

	return e, nil
}
