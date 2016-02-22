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
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"log"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/cli"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
	"github.com/tarm/serial"
)

type ConnSerial struct {
	cp            cli.NewtmgrConnProfile
	currentPacket *Packet

	scanner       *bufio.Scanner
	serialChannel *serial.Port
}

func (cs *ConnSerial) Open(cp cli.NewtmgrConnProfile) error {
	var err error

	c := &serial.Config{
		Name: cp.ConnString(),
		Baud: 115200,
	}

	cs.serialChannel, err = serial.OpenPort(c)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	//defer cs.serialChannel.Close()

	// Most of the reading will be done line by line, use the
	// bufio.Scanner to do this
	cs.scanner = bufio.NewScanner(cs.serialChannel)

	return nil
}

func (cs *ConnSerial) ReadPacket() (*Packet, error) {
	scanner := cs.scanner
	for scanner.Scan() {
		line := []byte(scanner.Text())

		for {
			if len(line) > 1 && line[0] == '\r' {
				line = line[1:]
			} else {
				break
			}
		}
		if len(line) < 2 || ((line[0] != 4 || line[1] != 20) &&
			(line[0] != 6 || line[1] != 9)) {
			continue
		}

		base64Data := string(line[2:])

		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			return nil, util.NewNewtError(
				fmt.Sprintf("Couldn't decode base64 string: %b",
					base64Data))
		}

		if line[0] == 6 && line[1] == 9 {
			if len(data) < 2 {
				continue
			}

			pktLen := binary.BigEndian.Uint16(data[0:2])
			cs.currentPacket, err = NewPacket(pktLen)
			if err != nil {
				return nil, err
			}
			data = data[2:]
		}

		if cs.currentPacket == nil {
			continue
		}

		full := cs.currentPacket.AddBytes(data)
		if full {
			pkt := cs.currentPacket
			cs.currentPacket = nil
			return pkt, nil
		}
	}

	return nil, util.NewNewtError("Scanning incoming data failed")
}

func (cs *ConnSerial) writeData(bytes []byte) {
	log.Printf("[DEBUG] Writing %b to data channel", bytes)
	cs.serialChannel.Write(bytes)
}

func (cs *ConnSerial) WritePacket(pkt *Packet) error {
	data := pkt.GetBytes()
	dLen := uint16(len(data))

	pktData := make([]byte, 2)

	binary.BigEndian.PutUint16(pktData, dLen)
	pktData = append(pktData, data...)

	base64Data := make([]byte, base64.StdEncoding.EncodedLen(len(pktData)))

	base64.StdEncoding.Encode(base64Data, pktData)

	written := 0
	totlen := len(base64Data)

	for written < totlen {
		if written == 0 {
			cs.writeData([]byte{6, 9})
		} else {
			cs.writeData([]byte{4, 20})
		}

		writeLen := util.Min(122, totlen)
		writeBytes := base64Data[:writeLen]
		cs.writeData(writeBytes)
		cs.writeData([]byte{'\n'})

		written += writeLen
	}

	return nil
}
