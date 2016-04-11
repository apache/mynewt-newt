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

type DateTime struct {
	DateTime string `json:"datetime"`
	Return   uint64 `json:"rc"`
}

func NewDateTime() (*DateTime, error) {
	dt := &DateTime{}
	return dt, nil
}

func (i *DateTime) EncodeRequest() (*NmgrReq, error) {
	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Len = 0
	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_DEFAULT
	nmr.Id = NMGR_ID_DATETIME_STR

	if i.DateTime != "" {
		data, _ := json.Marshal(i)
		nmr.Data = data
		nmr.Len = uint16(len(data))
	} else {
		type DateTimeReq struct {
			DtStr string `json:"datetime"`
		}

		dtReq := &DateTimeReq{
			DtStr: i.DateTime,
		}

		nmr.Op = NMGR_OP_READ
		data, _ := json.Marshal(dtReq)
		nmr.Data = data
		nmr.Len = uint16(len(data))
	}
	return nmr, nil
}

func DecodeDateTimeResponse(data []byte) (*DateTime, error) {
	i := &DateTime{}

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
