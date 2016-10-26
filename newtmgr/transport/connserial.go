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
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/joaojeronimo/go-crc16"
	"github.com/tarm/serial"

	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/newtmgr/nmutil"
	"mynewt.apache.org/newt/util"
)

type ConnSerial struct {
	connProfile   config.NewtmgrConnProfile
	currentPacket *Packet

	scanner       *bufio.Scanner
	serialChannel *serial.Port
	isOIC         bool
}

func (cs *ConnSerial) SetOICEncoded(b bool) {
	cs.isOIC = b
}

func (cs *ConnSerial) GetOICEncoded() bool {
	return cs.isOIC
}

func newSerialConfig(
	connString string, readTimeout time.Duration) (*serial.Config, error) {

	fields := strings.Split(connString, ":")
	if len(fields) == 0 {
		return nil, util.FmtNewtError("invalid connstring: %s", connString)
	}

	name := ""
	baud := 115200

	for _, field := range fields {
		parts := strings.Split(field, "=")
		if len(parts) == 2 {
			if parts[0] == "baud" {
				var err error
				baud, err = strconv.Atoi(parts[1])
				if err != nil {
					return nil, util.ChildNewtError(err)
				}

			}

			if parts[0] == "dev" {
				name = parts[1]
			}
		}
	}

	// Handle old-style conn string (single token indicating dev file).
	if name == "" {
		name = fields[0]
	}

	c := &serial.Config{
		Name:        name,
		Baud:        baud,
		ReadTimeout: readTimeout,
	}

	return c, nil
}

func (cs *ConnSerial) Open(cp config.NewtmgrConnProfile, readTimeout time.Duration) error {

	c, err := newSerialConfig(cp.ConnString(), readTimeout)
	if err != nil {
		return err
	}

	cs.serialChannel, err = serial.OpenPort(c)
	if err != nil {
		return util.ChildNewtError(err)
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

		nmutil.TraceIncoming(line)

		for {
			if len(line) > 1 && line[0] == '\r' {
				line = line[1:]
			} else {
				break
			}
		}
		log.Debugf("Reading %+v from data channel", line)
		if len(line) < 2 || ((line[0] != 4 || line[1] != 20) &&
			(line[0] != 6 || line[1] != 9)) {
			continue
		}

		base64Data := string(line[2:])

		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			nmutil.TraceMessage("base64 decode error\n")
			return nil, util.NewNewtError(
				fmt.Sprintf("Couldn't decode base64 string: %s\n"+
					"Packet hex dump:\n%s",
					base64Data, hex.Dump(line)))
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
			if crc16.Crc16(cs.currentPacket.GetBytes()) != 0 {
				nmutil.TraceMessage("CRC error\n")
				return nil, util.NewNewtError("CRC error")
			}

			/*
			 * Trim away the 2 bytes of CRC
			 */
			cs.currentPacket.TrimEnd(2)
			pkt := cs.currentPacket
			cs.currentPacket = nil
			return pkt, nil
		}
	}

	err := scanner.Err()
	if err == nil {
		// Scanner hit EOF, so we'll need to create a new one.  This only
		// happens on timeouts.
		nmutil.TraceMessage("Timeout reading from serial connection\n")
		err = util.NewNewtError("Timeout reading from serial connection")
		cs.scanner = bufio.NewScanner(cs.serialChannel)
	}
	return nil, err
}

func (cs *ConnSerial) writeData(bytes []byte) {
	nmutil.TraceOutgoing(bytes)
	log.Debugf("Writing %+v to data channel", bytes)
	cs.serialChannel.Write(bytes)
}

func (cs *ConnSerial) WritePacket(pkt *Packet) error {
	data := pkt.GetBytes()

	pktData := make([]byte, 2)

	crc := crc16.Crc16(data)
	binary.BigEndian.PutUint16(pktData, crc)
	data = append(data, pktData...)

	dLen := uint16(len(data))
	binary.BigEndian.PutUint16(pktData, dLen)
	pktData = append(pktData, data...)

	base64Data := make([]byte, base64.StdEncoding.EncodedLen(len(pktData)))

	base64.StdEncoding.Encode(base64Data, pktData)

	written := 0
	totlen := len(base64Data)

	for written < totlen {
		/* write the packet stat designators. They are
		* different whether we are starting a new packet or continuing one */
		if written == 0 {
			cs.writeData([]byte{6, 9})
		} else {
			/* slower platforms take some time to process each segment
			 * and have very small receive buffers.  Give them a bit of
			 * time here */
			time.Sleep(20 * time.Millisecond)
			cs.writeData([]byte{4, 20})
		}

		/* ensure that the total frame fits into 128 bytes.
		 * base 64 is 3 ascii to 4 base 64 byte encoding.  so
		 * the number below should be a multiple of 4.  Also,
		 * we need to save room for the header (2 byte) and
		 * carriage return (and possibly LF 2 bytes), */

		/* all totaled, 124 bytes should work */
		writeLen := util.Min(124, totlen-written)

		writeBytes := base64Data[written : written+writeLen]
		cs.writeData(writeBytes)
		cs.writeData([]byte{'\n'})

		written += writeLen
	}

	return nil
}

func (cs *ConnSerial) Close() error {
	return nil
}
