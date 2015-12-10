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
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/cli"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
	"github.com/jacobsa/go-serial/serial"
	"io"
)

type ConnSerial struct {
	cp            *cli.ConnProfile
	currentPacket *Packet

	scanner       *bufio.Scanner
	serialChannel io.ReadWriteCloser
}

func (cs *ConnSerial) Open(cp *cli.ConnProfile) error {
	var err error

	opts := serial.OpenOptions{
		PortName:        cp.ConnString,
		BaudRate:        9600,
		DataBits:        8,
		StopBits:        1,
		MinimumReadSize: 1,
	}

	cs.serialChannel, err = serial.Open(opts)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	defer cs.serialChannel.Close()

	// Most of the reading will be done line by line, use the
	// bufio.Scanner to do this
	cs.scanner = bufio.NewScanner(cs.serialChannel)

	return nil
}

func (cs *ConnSerial) ReadPacket() (*Packet, error) {
	scanner := cs.scanner
	for scanner.Scan() {
		line := scanner.Text()

		if len(line) < 2 || ((line[0] != 4 || line[1] != 20) &&
			(line[0] != 6 || line[1] != 9)) {
			continue
		}

		base64Data := line[2:]

		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			return nil, util.NewNewtError("Couldn't decode base64 string: " +
				line)
		}

		if line[0] == 4 && line[1] == 20 {
			if len(data) < 2 {
				continue
			}

			pktLen := binary.LittleEndian.Uint16(data[0:2])
			cs.currentPacket, err = NewPacket(pktLen)
			if err != nil {
				return nil, err
			}
			data = data[2:]
		}

		full := cs.currentPacket.AddBytes(data)
		if full {
			pkt := cs.currentPacket
			cs.currentPacket = nil
			return pkt, nil
		}
	}

	return nil, nil
}

func (cs *ConnSerial) WritePacket(pkt *Packet) error {
	data := pkt.GetBytes()
	dLen := uint16(len(data))

	base64Data := []byte{}
	pktData := []byte{}

	binary.LittleEndian.PutUint16(pktData, dLen)
	pktData = append(pktData, data...)

	base64.StdEncoding.Encode(base64Data, pktData)

	written := 0
	totlen := len(base64Data)
	for written < totlen {
		if written == 0 {
			cs.serialChannel.Write([]byte{4, 20})
		} else {
			cs.serialChannel.Write([]byte{6, 9})
		}

		writeLen := util.Min(122, totlen)
		writeBytes := base64Data[:writeLen]
		cs.serialChannel.Write(writeBytes)
		cs.serialChannel.Write([]byte{'\n'})

		written += writeLen
	}

	return nil
}
