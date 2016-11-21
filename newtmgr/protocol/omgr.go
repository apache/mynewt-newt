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

	log "github.com/Sirupsen/logrus"
	"github.com/dustin/go-coap"
	"github.com/ugorji/go/codec"

	"mynewt.apache.org/newt/util"
)

type OicRsp struct {
	Read  interface{} `codec:"r"`
	Write interface{} `codec:"w"`
}

/*
 * Not able to install custom decoder for indefite length objects with the codec.
 * So we need to decode the whole response, and then re-encode the newtmgr response
 * part.
 */
func DeserializeOmgrReq(data []byte) (*NmgrReq, error) {
	req := coap.Message{}
	err := req.UnmarshalBinary(data)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf(
			"Oicmgr request invalid %s", err.Error()))
	}
	if req.Code == coap.GET || req.Code == coap.PUT {
		return nil, nil
	}
	if req.Code != coap.Created && req.Code != coap.Deleted &&
		req.Code != coap.Valid && req.Code != coap.Changed &&
		req.Code != coap.Content {
		return nil, util.NewNewtError(fmt.Sprintf(
			"OIC error rsp: %s", req.Code.String()))
	}

	var rsp OicRsp
	err = codec.NewDecoderBytes(req.Payload, new(codec.CborHandle)).Decode(&rsp)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Invalid incoming cbor: %s",
			err.Error()))
	}
	log.Debugf("Deserialized response %+v", rsp)

	nmr := &NmgrReq{}

	var ndata []byte = make([]byte, 0)

	if rsp.Read != nil {
		err = codec.NewEncoderBytes(&ndata,
			new(codec.CborHandle)).Encode(rsp.Read)
		nmr.Op = NMGR_OP_READ_RSP
	} else {
		err = codec.NewEncoderBytes(&ndata,
			new(codec.CborHandle)).Encode(rsp.Write)
		nmr.Op = NMGR_OP_WRITE_RSP
	}
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Internal error: %s",
			err.Error()))
	}

	nmr.Len = uint16(len(ndata))
	nmr.Flags = 0
	nmr.Group = 0
	nmr.Seq = 0
	nmr.Id = 0

	nmr.Data = ndata

	log.Debugf("Deserialized response %+v", nmr)

	return nmr, nil
}

func (nmr *NmgrReq) SerializeOmgrRequest(data []byte) ([]byte, error) {
	req := coap.Message{
		Type:      coap.Confirmable,
		MessageID: uint16(nmr.Seq),
	}
	if nmr.Op == NMGR_OP_READ {
		req.Code = coap.GET
	} else {
		req.Code = coap.PUT
	}
	req.SetPathString("/omgr")
	req.AddOption(coap.URIQuery, fmt.Sprintf("gr=%d", nmr.Group))
	req.AddOption(coap.URIQuery, fmt.Sprintf("id=%d", nmr.Id))

	req.Payload = nmr.Data

	log.Debugf("Serializing request %+v into buffer %+v", nmr, data)

	data, err := req.MarshalBinary()
	if err != nil {
		return nil, util.NewNewtError(
			fmt.Sprintf("Failed to encode: %s\n", err.Error()))
	}
	return data, nil
}
