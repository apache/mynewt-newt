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

package transport

import (
	"bytes"
	"time"

	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/util"
)

type Conn interface {
	Open(cp config.NewtmgrConnProfile, timeout time.Duration) error
	ReadPacket() (*Packet, error)
	WritePacket(pkt *Packet) error
	Close() error
	SetOICEncoded(bool)
	GetOICEncoded() bool
}

type Packet struct {
	expectedLen uint16
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

func (pkt *Packet) TrimEnd(count int) {

	if pkt.buffer.Len() < count {
		count = pkt.buffer.Len()
	}
	pkt.buffer.Truncate(pkt.buffer.Len() - count)
}

func NewConn(cp config.NewtmgrConnProfile) (Conn, error) {
	return newConn(cp, 0)
}

func NewConnWithTimeout(cp config.NewtmgrConnProfile, readTimeout time.Duration) (Conn, error) {
	return newConn(cp, readTimeout)
}

func newConn(cp config.NewtmgrConnProfile, readTimeout time.Duration) (Conn, error) {
	// Based on ConnProfile, instantiate the right type of conn object, that
	// implements the conn interface.
	var c Conn
	switch cp.Type() {
	case "serial":
		c = &ConnSerial{}
		c.SetOICEncoded(false)
	case "oic_serial":
		c = &ConnSerial{}
		c.SetOICEncoded(true)
	case "ble":
		c = &ConnBLE{}
		c.SetOICEncoded(false)
	case "oic_ble":
		c = &ConnBLE{}
		c.SetOICEncoded(true)
	case "udp":
		c = &ConnUDP{}
		c.SetOICEncoded(false)
	case "oic_udp":
		c = &ConnUDP{}
		c.SetOICEncoded(true)
	default:
		return nil, util.NewNewtError("Invalid conn profile " + cp.Type() +
			" not implemented")
	}

	if err := c.Open(cp, readTimeout); err != nil {
		return nil, err
	}
	return c, nil
}

func CloseConn(c Conn) error {
	c.Close()
	return nil
}
