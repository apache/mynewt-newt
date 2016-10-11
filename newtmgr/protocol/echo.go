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
	"fmt"
	"strconv"

	"github.com/ugorji/go/codec"
	"mynewt.apache.org/newt/util"
)

type Echo struct {
	Message  string `codec:"d"`
	Response string `codec:"r,omitempty"`
}

func NewEcho() (*Echo, error) {
	s := &Echo{}
	return s, nil
}

func (e *Echo) EncodeWriteRequest() (*NmgrReq, error) {
	data := make([]byte, 0)
	enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))
	if err := enc.Encode(e); err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Failed to encode message %s",
			err.Error()))
	}

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
		Echo int `codec:"echo"`
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

	data := make([]byte, 0)
	enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))
	if err := enc.Encode(echoCtl); err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Failed to encode message %s",
			err.Error()))
	}
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func DecodeEchoResponse(data []byte) (*Echo, error) {
	e := &Echo{}

	cborCodec := new(codec.CborHandle)
	dec := codec.NewDecoderBytes(data, cborCodec)

	if err := dec.Decode(e); err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid response\n"))
	}
	return e, nil
}
