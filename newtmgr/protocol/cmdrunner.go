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
	"encoding/hex"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newtmgr/transport"
)

type CmdRunner struct {
	Conn transport.Conn
}

func (cr *CmdRunner) ReadResp() (*NmgrReq, error) {
	var nmr NmgrReq
	var nmrfrag *NmgrReq
	var nonmgrhdr bool

	nonmgrhdr = false

	for {
		pkt, err := cr.Conn.ReadPacket()
		if err != nil {
			return nil, err
		}

		bytes := pkt.GetBytes()
		log.Debugf("Rx packet dump:\n%s", hex.Dump(bytes))

		if nonmgrhdr == false {
			if cr.Conn.GetOICEncoded() == true {
				nmrfrag, err = DeserializeOmgrReq(bytes)
			} else {
				nmrfrag, err = DeserializeNmgrReq(bytes)
			}
			if err != nil {
				return nil, err
			}
			if nmrfrag == nil {
				continue
			}
		}
		if nmrfrag.Op == NMGR_OP_READ_RSP || nmrfrag.Op == NMGR_OP_WRITE_RSP {
			if nonmgrhdr == false {
				nmr.Data = append(nmr.Data, nmrfrag.Data...)
				nmr.Len += uint16(len(nmrfrag.Data))
			} else {
				nmr.Data = append(nmr.Data, bytes...)
				nmr.Len += uint16(len(bytes))
			}
			if nmr.Len >= nmrfrag.Len {
				return &nmr, nil
			} else {
				nonmgrhdr = true
			}
		}
	}
}

func (cr *CmdRunner) WriteReq(nmr *NmgrReq) error {
	data := []byte{}
	var err error

	log.Debugf("Writing newtmgr request %+v", nmr)

	if cr.Conn.GetOICEncoded() == true {
		data, err = nmr.SerializeOmgrRequest(data)
	} else {
		data, err = nmr.SerializeRequest(data)
	}
	if err != nil {
		return err
	}

	log.Debugf("Tx packet dump:\n%s", hex.Dump(data))

	pkt, err := transport.NewPacket(uint16(len(data)))
	if err != nil {
		return err
	}

	pkt.AddBytes(data)

	if err := cr.Conn.WritePacket(pkt); err != nil {
		return err
	}

	return nil
}

func NewCmdRunner(conn transport.Conn) (*CmdRunner, error) {
	cmd := &CmdRunner{
		Conn: conn,
	}

	return cmd, nil
}
