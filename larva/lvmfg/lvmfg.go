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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"
	"time"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/artifact/manifest"
	"mynewt.apache.org/newt/artifact/mfg"
	"mynewt.apache.org/newt/util"
)

type RTManifestMeta struct {
	Name    string `json:"name"`
	Time    string `json:"time"`
	Version string `json:"version"`
}

type RTManifestFile struct {
	Sha256 string `json:"sha256"`
}

type RTManifest struct {
	Meta    RTManifestMeta            `json:"meta"`
	Version string                    `json:"version"`
	ID      string                    `json:"id"`
	Files   map[string]RTManifestFile `json:"files"`
}

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

func ReplaceKey(sec0 []byte, okey []byte, nkey []byte) error {
	if len(okey) != len(nkey) {
		return util.FmtNewtError(
			"key lengths differ (%d != %d)", len(okey), len(nkey))
	}

	if len(okey) > len(sec0) {
		return util.FmtNewtError(
			"key longer than flash section (%d > %d)", len(okey), len(sec0))
	}

	idx := bytes.Index(sec0, okey)
	if idx == -1 {
		return util.FmtNewtError("old key not present in flash section")
	}

	lastIdx := bytes.LastIndex(sec0, okey)
	if idx != lastIdx {
		return util.FmtNewtError(
			"multiple instances of old key in flash section")
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Replacing boot key at offset %d\n", idx)

	copy(sec0[idx:idx+len(okey)], nkey)

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

func RehashRTManifest(rmBytes []byte,
	mm manifest.MfgManifest) ([]byte, error) {

	rm := RTManifest{}
	if err := json.Unmarshal(rmBytes, &rm); err != nil {
		return nil, util.ChildNewtError(err)
	}

	rm.Meta.Time = time.Now().Format(time.RFC3339)
	rm.ID = mm.MfgHash

	newFiles := map[string]RTManifestFile{}
	for name, _ := range rm.Files {
		fb, err := ioutil.ReadFile(name)
		if err != nil {
			return nil, util.ChildNewtError(err)
		}
		hb := sha256.Sum256(fb)
		newFiles[name] = RTManifestFile{
			Sha256: hex.EncodeToString(hb[:]),
		}
	}
	rm.Files = newFiles

	j, err := json.MarshalIndent(rm, "", "  ")
	if err != nil {
		return nil, util.ChildNewtError(err)
	}

	return j, nil
}
