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

package mfg

import (
	"fmt"
	"sort"
	"strings"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/util"
)

type MfgMap map[string][]byte

func errInvalidArea(areaName string, format string,
	args ...interface{}) error {

	suffix := fmt.Sprintf(format, args...)
	return util.FmtNewtError("Invalid flash area \"%s\": %s", areaName, suffix)
}

func verifyArea(area flash.FlashArea, minOffset int) error {
	if area.Offset < minOffset {
		return errInvalidArea(area.Name, "invalid offset %d; expected >= %d",
			area.Offset, minOffset)
	}

	if area.Size < 0 {
		return errInvalidArea(area.Name, "invalid size %d", area.Size)
	}

	return nil
}

// `areas` must be sorted by device ID, then by offset.
func VerifyAreas(areas []flash.FlashArea, deviceNum int) error {
	off := 0
	for _, area := range areas {
		if area.Device == deviceNum {
			if err := verifyArea(area, off); err != nil {
				return err
			}
			off += area.Size
		}
	}

	return nil
}

func Split(mfgBin []byte, deviceNum int,
	areas []flash.FlashArea) (MfgMap, error) {

	mm := MfgMap{}

	for _, area := range areas {
		if _, ok := mm[area.Name]; ok {
			return nil, util.FmtNewtError(
				"two or more flash areas with same name: \"%s\"", area.Name)
		}

		if area.Device == deviceNum && area.Offset < len(mfgBin) {
			end := area.Offset + area.Size
			if end > len(mfgBin) {
				return nil, util.FmtNewtError(
					"area \"%s\" (offset=%d size=%d) "+
						"extends beyond end of manufacturing image",
					area.Name, area.Offset, area.Size)
			}

			mm[area.Name] = mfgBin[area.Offset:end]
		}
	}

	return mm, nil
}

// `areas` must be sorted by device ID, then by offset.
func Join(mm MfgMap, eraseVal byte, areas []flash.FlashArea) ([]byte, error) {
	// Ensure all areas in the mfg map belong to the same flash device.
	device := -1
	for _, area := range areas {
		if _, ok := mm[area.Name]; ok {
			if device == -1 {
				device = area.Device
			} else if device != area.Device {
				return nil, util.FmtNewtError(
					"multiple flash devices: %d != %d", device, area.Device)
			}
		}
	}

	// Keep track of which areas we haven't seen yet.
	unseen := map[string]struct{}{}
	for name, _ := range mm {
		unseen[name] = struct{}{}
	}

	joined := []byte{}

	off := 0
	for _, area := range areas {
		bin := mm[area.Name]
		if bin == nil {
			break
		}
		delete(unseen, area.Name)

		padSize := area.Offset - off
		for i := 0; i < padSize; i++ {
			joined = append(joined, 0xff)
		}

		joined = append(joined, bin...)
	}

	// Ensure we processed every area in the mfg map.
	if len(unseen) > 0 {
		names := []string{}
		for name, _ := range unseen {
			names = append(names, name)
		}
		sort.Strings(names)

		return nil, util.FmtNewtError(
			"unprocessed flash areas: %s", strings.Join(names, ", "))
	}

	return joined, nil
}
