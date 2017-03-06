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

	"github.com/ugorji/go/codec"
	"mynewt.apache.org/newt/util"
)

const (
    RUN_NMGR_OP_TEST        = 0
    RUN_NMGR_OP_LIST        = 1
)

/*
 * run test [all | testname] [token]
 * max testname and token size is 32 bytes
 *
 * This is written with the idea to provide a more extensible RPC mechanism however
 * the "test" commands constrains the remote calls to those registered to
 * test infrastructure.
 */
type RunTestReq struct {
    Testname        string `codec:"testname"`
    Token           string `codec:"token"`
}

func NewRunTestReq() (*RunTestReq, error) {
    s := &RunTestReq{}

    return s, nil
}

func (sr *RunTestReq) Encode() (*NmgrReq, error) {
    nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_RUN
    nmr.Id = RUN_NMGR_OP_TEST
    req := &RunTestReq{
        Testname: sr.Testname,
        Token: sr.Token,
    }

    data := make([]byte, 0)
	enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))

	enc.Encode(req)
	nmr.Data = data
	nmr.Len = uint16(len(data))

    return nmr, nil
}

type RunTestRsp struct {
    ReturnCode int      `codec:"rc"`
}

func DecodeRunTestResponse(data []byte) (*RunTestRsp, error) {
    var resp RunTestRsp

	dec := codec.NewDecoderBytes(data, new(codec.CborHandle))
	err := dec.Decode(&resp)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid response: %s",
			                          err.Error()))
	}

	return &resp, nil
}

/*
 * run list
 * Returns the list of tests that have been registered on the device.
 */
type RunListReq struct {
}

type RunListRsp struct {
    ReturnCode int      `codec:"rc"`
    List       []string `codec:"run_list"`
}

func NewRunListReq() (*RunListReq, error) {
    s := &RunListReq{}

    return s, nil
}

func (sr *RunListReq) Encode() (*NmgrReq, error) {
    nmr, err := NewNmgrReq()
    if err != nil {
        return nil, err
    }

    nmr.Op = NMGR_OP_READ
    nmr.Flags = 0
    nmr.Group = NMGR_GROUP_ID_RUN
    nmr.Id = RUN_NMGR_OP_LIST

    req := &RunListReq{}

    data := make([]byte, 0)
    enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))
    enc.Encode(req)

    nmr.Data = data
    nmr.Len = uint16(len(data))

    return nmr, nil
}

func DecodeRunListResponse(data []byte) (*RunListRsp, error) {
    var resp RunListRsp

    dec := codec.NewDecoderBytes(data, new(codec.CborHandle))
    err := dec.Decode(&resp)
    if err != nil {
        return nil,
        util.NewNewtError(fmt.Sprintf("Invalid incoming cbor: %s",
                                      err.Error()))
    }

    return &resp, nil
}
