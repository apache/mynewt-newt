/**
 * Licensed to the Apache Software Foundation (ASF) under one
	iog.Debugf("Writing %+v to data channel", bytes)
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

	"github.com/paypal/gatt"

	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/util"
)

var rxBLEPkt = make(chan []byte)
var CharDisc = make(chan bool)

var newtmgrServiceId = gatt.MustParseUUID("59462f12-9543-9999-12c8-58b459a27120")
var newtmgrServiceCharId = gatt.MustParseUUID("5c3a659e-897e-45e1-b016-007107c96d00")
var deviceName string

type ConnBLE struct {
	connProfile   config.NewtmgrConnProfile
	currentPacket *Packet

	bleDevice gatt.Device
}

var deviceChar *gatt.Characteristic
var devicePerph gatt.Peripheral

var bleTxData []byte

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
	if a.LocalName == deviceName {
		log.Debugf("Peripheral Discovered: %s", p.Name())
		p.Device().StopScanning()
		p.Device().Connect(p)
	}
}

func newtmgrNotifyCB(c *gatt.Characteristic, incomingDatabuf []byte, err error) {
	log.Debugf("BLE Newtmgr rx data:%+v", incomingDatabuf)
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

	for _, service := range services {

		if service.UUID().Equal(newtmgrServiceId) {
			log.Debugf("Newtmgr Service Found %s", service.Name())

			cs, _ := p.DiscoverCharacteristics(nil, service)

			for _, c := range cs {
				if c.UUID().Equal(newtmgrServiceCharId) {
					log.Debugf("Newtmgr Characteristic Found")
					p.SetNotifyValue(c, newtmgrNotifyCB)
					deviceChar = c
					devicePerph = p
					p.SetMTU(240)
					<-CharDisc
				}
			}
		}
	}
}

func onPeriphDisconnected(p gatt.Peripheral, err error) {
	log.Debugf("Disconnected", err)
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

func (cb *ConnBLE) Close () error {
	log.Debugf("Closing Connection %+v", cb)
        cb.bleDevice.CancelConnection(devicePerph)
	return nil
}
