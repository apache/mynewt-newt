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
	log "github.com/Sirupsen/logrus"
	"time"

	"github.com/runtimeco/gatt"

	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/util"
)

/* This is used by different command handlers */
var BleMTU uint16 = 180

var rxBLEPkt = make(chan []byte)
var CharDisc = make(chan bool)

var newtmgrServiceId = gatt.MustParseUUID("8D53DC1D-1DB7-4CD3-868B-8A527460AA84")
var newtmgrServiceCharId = gatt.MustParseUUID("DA2E7828-FBCE-4E01-AE9E-261174997C48")

var newtmgrCoapServiceId = gatt.MustParseUUID("e3f9f9c4-8a83-4055-b647-728b769745d6")
var newtmgrCoapServiceCharId = gatt.MustParseUUID("e467fee6-d6bb-4956-94df-0090350631f5")

var deviceName string
var deviceAddress []byte
var deviceAddressType uint8

type ConnBLE struct {
	connProfile   config.NewtmgrConnProfile
	currentPacket *Packet
	bleDevice     gatt.Device
	isOIC         bool
}

var deviceChar *gatt.Characteristic
var devicePerph gatt.Peripheral

var bleTxData []byte

func reverseBytes(arr []byte) []byte {
	if len(arr) == 0 {
		return arr
	}
	return append(reverseBytes(arr[1:]), arr[0])
}

func onStateChanged(d gatt.Device, s gatt.State) {
	log.Debugf("State:%+v", s)
	switch s {
	case gatt.StatePoweredOn:
		log.Debugf("scanning...")
		d.Scan([]gatt.UUID{}, false)
		return
	default:
		d.StopScanning()
	}
}

func onPeriphDiscovered(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
	var matched bool = false

	if len(deviceName) > 0 {
		matched = a.LocalName == deviceName
		if matched == false {
			return
		}
	}

	if len(deviceAddress) > 0 {
		var deviceAddrArr [6]byte
		copy(deviceAddrArr[:], deviceAddress[0:6])
		matched = a.Address == deviceAddrArr && a.AddressType == deviceAddressType
	}

	if matched == true {
		log.Debugf("Peripheral Discovered: %s, Address:%+v Address Type:%+v",
			p.Name(), a.Address, a.AddressType)
		p.Device().StopScanning()
		p.Device().Connect(p)
	}
}

func newtmgrNotifyCB(c *gatt.Characteristic, incomingDatabuf []byte, err error) {
	err = nil
	rxBLEPkt <- incomingDatabuf
	return
}

func onPeriphConnected(p gatt.Peripheral, err error) {
	log.Debugf("Peripheral Connected")

	services, err := p.DiscoverServices(nil)
	if err != nil {
		log.Debugf("Failed to discover services, err: %s", err)
		return
	}

	var isCoap bool = false

	for _, service := range services {

		if service.UUID().Equal(newtmgrServiceId) ||
			service.UUID().Equal(newtmgrCoapServiceId) {
			log.Debugf("Newtmgr Service Found %s", service.Name())

			if service.UUID().Equal(newtmgrCoapServiceId) {
				isCoap = true
			}

			cs, _ := p.DiscoverCharacteristics(nil, service)

			for _, c := range cs {
				if (isCoap == false &&
					c.UUID().Equal(newtmgrServiceCharId)) ||
					(isCoap == true &&
						c.UUID().Equal(newtmgrCoapServiceCharId)) {
					log.Debugf("Newtmgr Characteristic Found")
					p.SetNotifyValue(c, newtmgrNotifyCB)
					deviceChar = c
					devicePerph = p
					p.SetMTU(BleMTU)
					<-CharDisc
				}
			}
		}
	}
}

func onPeriphDisconnected(p gatt.Peripheral, err error) {
	log.Debugf("Disconnected", err)
}

func (cs *ConnBLE) SetOICEncoded(b bool) {
	cs.isOIC = b
}

func (cs *ConnBLE) GetOICEncoded() bool {
	return cs.isOIC
}

func (cb *ConnBLE) Open(cp config.NewtmgrConnProfile, readTimeout time.Duration) error {
	var err error

	var DefaultClientOptions = BleOptions

	deviceName = cp.ConnString()
	deviceAddress = reverseBytes(cp.DeviceAddress())
	log.Debugf("BLE Connection devaddr:%+v", deviceAddress)
	deviceAddressType = cp.DeviceAddressType()
	cb.bleDevice, err = gatt.NewDevice(DefaultClientOptions...)
	if err != nil {
		return util.NewNewtError(err.Error())
	}

	cb.bleDevice.Handle(
		gatt.PeripheralDiscovered(onPeriphDiscovered),
		gatt.PeripheralConnected(onPeriphConnected),
		gatt.PeripheralDisconnected(onPeriphDisconnected),
	)
	cb.bleDevice.Init(onStateChanged)
	CharDisc <- true

	return nil
}

func (cb *ConnBLE) ReadPacket() (*Packet, error) {
	var err error

	bleRxData := <-rxBLEPkt

	cb.currentPacket, err = NewPacket(uint16(len(bleRxData)))
	if err != nil {
		return nil, err
	}

	cb.currentPacket.AddBytes(bleRxData)
	log.Debugf("Read BLE Packet:buf::%+v len::%+v", cb.currentPacket.buffer,
		cb.currentPacket.expectedLen)
	bleRxData = bleRxData[:0]
	pkt := cb.currentPacket
	cb.currentPacket = nil
	return pkt, err
}

func (cb *ConnBLE) writeData() error {
	devicePerph.WriteCharacteristic(deviceChar, bleTxData, true)
	return nil
}

func (cb *ConnBLE) WritePacket(pkt *Packet) error {
	log.Debugf("Write BLE Packet:buf::%+v len::%+v", pkt.buffer,
		pkt.expectedLen)
	bleTxData = pkt.GetBytes()
	cb.writeData()
	return nil
}

func (cb *ConnBLE) Close() error {
	log.Debugf("Closing Connection %+v", cb)
	return cb.bleDevice.Stop()
}
