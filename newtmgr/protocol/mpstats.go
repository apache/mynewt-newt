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

type MempoolStatsReadReq struct {
}

type MempoolStatsReadRsp struct {
	ReturnCode int                               `codec:"rc"`
	MPools     map[string]map[string]interface{} `codec:"mpools"`
}

func NewMempoolStatsReadReq() (*MempoolStatsReadReq, error) {
	s := &MempoolStatsReadReq{}

	return s, nil
}

func (tsr *MempoolStatsReadReq) EncodeWriteRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_READ
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_DEFAULT
	nmr.Id = NMGR_ID_MPSTATS

	nmr.Len = 0

	return nmr, nil
}

func DecodeMempoolStatsReadResponse(data []byte) (*MempoolStatsReadRsp, error) {
	var tsr MempoolStatsReadRsp
	dec := codec.NewDecoderBytes(data, new(codec.CborHandle))
	err := dec.Decode(&tsr)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming cbor: %s",
			err.Error()))
	}

	return &tsr, nil
}
