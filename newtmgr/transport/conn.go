/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package transport

import (
	"bytes"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/cli"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
)

type Conn interface {
	Open(cp *ConnProfile) error
	ReadPacket() (*Packet, error)
	WritePacket(pkt *Packet) error
}

type Packet struct {
	expectedLen uint16
	readLen     uint16
	buffer      *bytes.Buffer
}

func NewPacket(expectedLen uint16) (*Packet, error) {
	pkt := &Packet{
		expectedLen: expectedLen,
		buffer:      bytes.NewBuffer([]byte{}),
	}

	return pkt, nil
}

func (pkt *Packet) AddBytes(bytes []byte) bool {
	pkt.buffer.Write(bytes)
	if pkt.buffer.Len() >= int(pkt.expectedLen) {
		return true
	} else {
		return false
	}
}

func (pkt *Packet) GetBytes() []byte {
	return pkt.buffer.Bytes()
}

func NewConn(cp *cli.ConnProfile) (Conn, error) {
	// Based on ConnProfile, instantiate the right type of conn object, that
	// implements the conn interface.
	var c Conn
	switch cp.Type {
	case "serial":
		c = &ConnSerial{}
	default:
		return nil, util.NewNewtError("Invalid conn profile " + cp.Type +
			" not implemented")
	}

	return c, nil
}
