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

type Config struct {
	Name  string `codec:"name"`
	Value string `codec:"val"`
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

	data := make([]byte, 0)
	enc := codec.NewEncoderBytes(&data, new(codec.CborHandle))

	if c.Value == "" {
		type ConfigReadReq struct {
			Name string `codec:"name"`
		}

		readReq := &ConfigReadReq{
			Name: c.Name,
		}

		enc.Encode(readReq)

		nmr.Data = data
		nmr.Op = NMGR_OP_READ
	} else {
		enc.Encode(c)
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

	dec := codec.NewDecoderBytes(data, new(codec.CborHandle))
	err := dec.Decode(&c)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming cbor: %s",
			err.Error()))
	}
	return c, nil
}
