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

package lvmfg

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/artifact/mfg"
	"mynewt.apache.org/newt/util"
)

type NameBlobMap map[string][]byte

func (to NameBlobMap) Union(from NameBlobMap) {
	for k, v := range from {
		to[k] = v
	}
}

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
func VerifyAreas(areas []flash.FlashArea) error {
	prevDevice := -1
	off := 0
	for _, area := range areas {
		if area.Device != prevDevice {
			off = 0
		}

		if err := verifyArea(area, off); err != nil {
			return err
		}
		off += area.Size
	}

	return nil
}

func Split(mfgBin []byte, deviceNum int,
	areas []flash.FlashArea, eraseVal byte) (NameBlobMap, error) {

	mm := NameBlobMap{}

	for _, area := range areas {
		if _, ok := mm[area.Name]; ok {
			return nil, util.FmtNewtError(
				"two or more flash areas with same name: \"%s\"", area.Name)
		}

		if area.Device == deviceNum {
			var areaBin []byte
			if area.Offset < len(mfgBin) {
				end := area.Offset + area.Size
				overflow := end - len(mfgBin)
				if overflow > 0 {
					end -= overflow
				}
				areaBin = mfgBin[area.Offset:end]
			}

			mm[area.Name] = StripPadding(areaBin, eraseVal)
		}
	}

	return mm, nil
}

// `areas` must be sorted by device ID, then by offset.
func Join(mm NameBlobMap, eraseVal byte,
	areas []flash.FlashArea) ([]byte, error) {

	// Keep track of which areas we haven't seen yet.
	unseen := map[string]struct{}{}
	for name, _ := range mm {
		unseen[name] = struct{}{}
	}

	joined := []byte{}
	for _, area := range areas {
		bin := mm[area.Name]

		// Only include this area if it belongs to the mfg image we are
		// joining.
		if bin != nil {
			delete(unseen, area.Name)

			// Pad remainder of previous area in this section.
			padSize := area.Offset - len(joined)
			if padSize > 0 {
				joined = mfg.AddPadding(joined, eraseVal, padSize)
			}

			// Append data to joined binary.
			binstr := ""
			if len(bin) >= 4 {
				binstr = fmt.Sprintf("%x", bin[:4])
			}
			util.StatusMessage(util.VERBOSITY_DEFAULT,
				"inserting %s (%s) at offset %d (0x%x)\n",
				area.Name, binstr, len(joined), len(joined))
			joined = append(joined, bin...)
		}
	}

	// Ensure we processed every area in the map.
	if len(unseen) > 0 {
		names := []string{}
		for name, _ := range unseen {
			names = append(names, name)
		}
		sort.Strings(names)

		return nil, util.FmtNewtError(
			"unprocessed flash areas: %s", strings.Join(names, ", "))
	}

	// Strip padding from the end of the joined binary.
	joined = StripPadding(joined, eraseVal)

	return joined, nil
}

func replaceKey(mfgBin []byte, okey []byte, nkey []byte) (int, error) {
	if len(okey) > len(mfgBin) {
		return 0, util.FmtNewtError(
			"key longer than flash section (%d > %d)", len(okey), len(mfgBin))
	}

	idx := bytes.Index(mfgBin, okey)
	if idx == -1 {
		return 0, util.FmtNewtError("old key not present in flash section")
	}

	lastIdx := bytes.LastIndex(mfgBin, okey)
	if idx != lastIdx {
		return 0, util.FmtNewtError(
			"multiple instances of old key in flash section")
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Replacing key at offset %d\n", idx)

	copy(mfgBin[idx:idx+len(okey)], nkey)

	return idx, nil
}

func ReplaceIsk(mfgBin []byte, okey []byte, nkey []byte) error {
	if len(nkey) != len(okey) {
		return util.FmtNewtError(
			"key lengths differ (%d != %d)", len(nkey), len(okey))
	}

	if _, err := replaceKey(mfgBin, okey, nkey); err != nil {
		return err
	}

	return nil
}

func ReplaceKek(mfgBin []byte, okey []byte, nkey []byte) error {
	if len(nkey) > len(okey) {
		return util.FmtNewtError(
			"new key longer than old key (%d > %d)", len(nkey), len(okey))
	}

	keyIdx, err := replaceKey(mfgBin, okey, nkey)
	if err != nil {
		return err
	}

	// The key length is an unsigned int immediately prior to the key.
	var kl uint32
	klIdx := keyIdx - 4
	buf := bytes.NewBuffer(mfgBin[klIdx : klIdx+4])
	if err := binary.Read(buf, binary.LittleEndian, &kl); err != nil {
		return util.ChildNewtError(err)
	}

	if int(kl) != len(okey) {
		return util.FmtNewtError(
			"embedded key length (off=%d) has unexpected value; "+
				"want=%d have=%d",
			klIdx, len(okey), kl)
	}

	buf = &bytes.Buffer{}
	kl = uint32(len(nkey))
	if err := binary.Write(buf, binary.LittleEndian, kl); err != nil {
		return util.ChildNewtError(err)
	}

	copy(mfgBin[klIdx:klIdx+4], buf.Bytes())

	return nil
}

func StripPadding(b []byte, eraseVal byte) []byte {
	var pad int
	for pad = 0; pad < len(b); pad++ {
		off := len(b) - pad - 1
		if b[off] != eraseVal {
			break
		}
	}

	return b[:len(b)-pad]
}
