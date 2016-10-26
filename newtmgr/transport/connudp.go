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
	"fmt"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/util"
)

type ConnUDP struct {
	connProfile config.NewtmgrConnProfile
	conn        *net.UDPConn
	dst         *net.UDPAddr
	isOIC       bool
}

func (cs *ConnUDP) Open(cp config.NewtmgrConnProfile, readTimeout time.Duration) error {
	addr, err := net.ResolveUDPAddr("udp", cp.ConnString())
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Name not resolving: %s",
			err.Error()))
	}
	cs.dst = addr

	// bind local endpoint to wait for response afterwards
	s, err := net.ListenUDP("udp", nil)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("UDP conn failed: %s\n",
			err.Error()))
	}
	cs.conn = s
	return nil
}

func (cs *ConnUDP) Close() error {
	cs.conn.Close()
	cs.conn = nil
	return nil
}

func (cs *ConnUDP) SetOICEncoded(b bool) {
	cs.isOIC = b
}

func (cs *ConnUDP) GetOICEncoded() bool {
	return cs.isOIC
}

func (cs *ConnUDP) WritePacket(pkt *Packet) error {
	if cs.conn == nil {
		return util.NewNewtError("Connection not open")
	}

	_, err := cs.conn.WriteTo(pkt.GetBytes(), cs.dst)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("failed to write: %s",
			err.Error()))
	}
	return nil
}

func (cs *ConnUDP) ReadPacket() (*Packet, error) {
	// cs.conn.SetDeadline(time.Now().Add(time.Second * 4))

	data := make([]byte, 2048)
	nr, srcAddr, err := cs.conn.ReadFromUDP(data)
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("failed to read: %s",
			err.Error()))
	}
	data = data[0:nr]
	log.Debugf("Received message from %v %d\n", srcAddr, nr)
	pkt, err := NewPacket(uint16(nr))
	if err != nil {
		return nil, err
	}
	pkt.AddBytes(data)
	return pkt, nil
}
