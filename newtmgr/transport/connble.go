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
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/paypal/gatt"

	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/util"
)

var done = make(chan struct{})
var newtmgrServiceId = gatt.MustParseUUID("59462f12-9543-9999-12c8-58b459a27120")
var newtmgrServiceCharId = gatt.MustParseUUID("5c3a659e-897e-45e1-b016-007107c96d00")
var deviceName string

type ConnBLE struct {
	connProfile   config.NewtmgrConnProfile
	currentPacket *Packet

	bleDevice gatt.Device
}

var pktData []byte

func onStateChanged(d gatt.Device, s gatt.State) {
	fmt.Println("State:", s)
	switch s {
	case gatt.StatePoweredOn:
		fmt.Println("scanning...")
		d.Scan([]gatt.UUID{}, false)
		return
	default:
		d.StopScanning()
	}
}

func onPeriphDiscovered(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
	if a.LocalName == deviceName {
		fmt.Printf("Peripheral Discovered: %s \n", p.Name())
		p.Device().StopScanning()
		p.Device().Connect(p)
	}
}

func newtmgrNotifyCB(c *gatt.Characteristic, incomingDatabuf []byte, err error) {
	fmt.Printf("Newtmgr response rxd:%+v", incomingDatabuf)
}

func onPeriphConnected(p gatt.Peripheral, err error) {
	fmt.Printf("Peripheral connected\n")

	services, err := p.DiscoverServices(nil)
	if err != nil {
		fmt.Printf("Failed to discover services, err: %s\n", err)
		return
	}

	for _, service := range services {

		if service.UUID().Equal(newtmgrServiceId) {
			fmt.Printf("Newtmgr Service Found %s\n", service.Name())

			cs, _ := p.DiscoverCharacteristics(nil, service)

			for _, c := range cs {
				if c.UUID().Equal(newtmgrServiceCharId) {
					fmt.Printf("Newtmgr Characteristic Found %+v", c)
					p.SetNotifyValue(c, newtmgrNotifyCB)
					log.Debugf("Writing %+v to ble", pktData)
					p.WriteCharacteristic(c, pktData, true)
				}
			}
		}
	}
}

func onPeriphDisconnected(p gatt.Peripheral, err error) {
	fmt.Println("Disconnected")
}

func (cb *ConnBLE) Open(cp config.NewtmgrConnProfile, readTimeout time.Duration) error {
	var err error

	var DefaultClientOptions = []gatt.Option{
		gatt.LnxMaxConnections(1),
		gatt.LnxDeviceID(-1, false),
	}

	deviceName = cp.ConnString()
	cb.bleDevice, err = gatt.NewDevice(DefaultClientOptions...)
	if err != nil {
		return util.NewNewtError(err.Error())
	}
	//defer cs.serialChannel.Close()

	cb.bleDevice.Handle(
		gatt.PeripheralDiscovered(onPeriphDiscovered),
		gatt.PeripheralConnected(onPeriphConnected),
		gatt.PeripheralDisconnected(onPeriphDisconnected),
	)
	return nil
}

func (cb *ConnBLE) ReadPacket() (*Packet, error) {
	var err error
	/*
			pktLen := binary.BigEndian.Uint16(data[0:2])
			cb.currentPacket, err = NewPacket(pktLen)
			if err != nil {
					return nil, err
		    }
				data = data[2:]

			if cs.currentPacket == nil {
				continue
			}

			full := cs.currentPacket.AddBytes(data)
			if full {
				if crc16.Crc16(cs.currentPacket.GetBytes()) != 0 {
					return nil, util.NewNewtError("CRC error")
				}

				/*
				 * Trim away the 2 bytes of CRC
	*/
	/*			cs.currentPacket.TrimEnd(2)
			pkt := cs.currentPacket
			cs.currentPacket = nil
			return pkt, nil
		}
	}
	*/
	return nil, err
}

func (cb *ConnBLE) WritePacket(pkt *Packet) error {
	pktData = pkt.GetBytes()
	cb.bleDevice.Init(onStateChanged)
	<-done
	return nil
}
