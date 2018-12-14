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

// This file contains functionality for loading mfg definitions from `mfg.yml`
// files.

package mfg

import (
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

// Indicates that an element is located at the end of a flash area.
const OFFSET_END = -1

type DecodedTarget struct {
	Name   string
	Area   string
	Offset int
}

type DecodedRaw struct {
	Filename string
	Area     string
	Offset   int
}

type DecodedMmrRef struct {
	Area string
}

type DecodedMeta struct {
	Area     string
	Hash     bool
	FlashMap bool
	Mmrs     []DecodedMmrRef
}

type DecodedMfg struct {
	Targets []DecodedTarget
	Raws    []DecodedRaw
	Meta    *DecodedMeta

	// Only required if no targets present.
	Bsp string
}

func decodeOffsetStr(offsetStr string) (int, error) {
	if offsetStr == "end" {
		return OFFSET_END, nil
	}

	offsetInt, err := cast.ToIntE(offsetStr)
	if err != nil {
		return 0, util.FmtNewtError("invalid offset value: \"%s\"", offsetStr)
	}

	return offsetInt, nil
}

func decodeBool(kv map[string]interface{}, key string) (*bool, error) {
	var bp *bool

	val := kv[key]
	if val != nil {
		b, err := cast.ToBoolE(val)
		if err != nil {
			return nil, util.FmtNewtError(
				"invalid `%s` value \"%v\"; "+
					"value must be either \"true\" or \"false\"", key, val)
		}

		bp = &b
	}

	return bp, nil
}

func decodeBoolDflt(kv map[string]interface{}, key string,
	dflt bool) (bool, error) {

	bp, err := decodeBool(kv, key)
	if err != nil {
		return false, err
	}

	if bp == nil {
		return dflt, nil
	} else {
		return *bp, nil
	}
}

func decodeTarget(yamlTarget interface{}) (DecodedTarget, error) {
	dt := DecodedTarget{}

	kv, err := cast.ToStringMapE(yamlTarget)
	if err != nil {
		return dt, util.FmtNewtError(
			"mfg contains invalid `mfg.targets` map: %s", err.Error())
	}

	nameVal := kv["name"]
	if nameVal == nil {
		return dt, util.FmtNewtError(
			"mfg target entry missing required field \"name\"")
	}
	dt.Name = cast.ToString(nameVal)

	areaVal := kv["area"]
	if areaVal == nil {
		return dt, util.FmtNewtError(
			"target entry \"%s\" missing required field \"area\"", dt.Name)
	}
	dt.Area = cast.ToString(areaVal)

	offsetVal := kv["offset"]
	if offsetVal == nil {
		return dt, util.FmtNewtError(
			"target entry \"%s\" missing required field \"offset\"", dt.Name)
	}
	offsetStr := cast.ToString(offsetVal)
	offsetInt, err := decodeOffsetStr(offsetStr)
	if err != nil {
		return dt, util.FmtNewtError(
			"in target entry \"%s\": %s", dt.Name, err.Error())
	}
	dt.Offset = offsetInt

	return dt, nil
}

func decodeRaw(yamlRaw interface{}, entryIdx int) (DecodedRaw, error) {
	dr := DecodedRaw{}

	kv, err := cast.ToStringMapE(yamlRaw)
	if err != nil {
		return dr, util.FmtNewtError(
			"mfg contains invalid `mfg.raw` map: %s", err.Error())
	}

	areaVal := kv["area"]
	if areaVal == nil {
		return dr, util.FmtNewtError(
			"raw entry missing required field \"area\"")
	}
	dr.Area = cast.ToString(areaVal)

	offsetVal := kv["offset"]
	if offsetVal == nil {
		return dr, util.FmtNewtError(
			"mfg raw entry missing required field \"offset\"")
	}
	offsetStr := cast.ToString(offsetVal)
	offsetInt, err := decodeOffsetStr(offsetStr)
	if err != nil {
		return dr, util.FmtNewtError(
			"in raw entry %d: %s", entryIdx, err.Error())
	}
	dr.Offset = offsetInt

	filenameVal := kv["name"]
	if filenameVal == nil {
		return dr, util.FmtNewtError(
			"mfg raw entry missing required field \"filename\"")
	}
	dr.Filename = cast.ToString(filenameVal)

	return dr, nil
}

func decodeMmr(yamlMmr interface{}) (DecodedMmrRef, error) {
	dm := DecodedMmrRef{}

	kv, err := cast.ToStringMapE(yamlMmr)
	if err != nil {
		return dm, util.FmtNewtError(
			"mfg meta contains invalid `mmrs` sequence: %s", err.Error())
	}

	areaVal := kv["area"]
	if areaVal == nil {
		return dm, util.FmtNewtError(
			"mmr entry missing required field \"area\"")
	}
	dm.Area = cast.ToString(areaVal)

	return dm, nil
}

func decodeMmrs(yamlMmrs interface{}) ([]DecodedMmrRef, error) {
	yamlSlice, err := cast.ToSliceE(yamlMmrs)
	if err != nil {
		return nil, util.FmtNewtError(
			"mfg meta contains invalid `mmrs` sequence: %s", err.Error())
	}

	mmrs := []DecodedMmrRef{}
	for _, yamlMmr := range yamlSlice {
		mmr, err := decodeMmr(yamlMmr)
		if err != nil {
			return nil, err
		}
		mmrs = append(mmrs, mmr)
	}

	return mmrs, nil
}

func decodeMeta(
	kv map[string]interface{}) (DecodedMeta, error) {

	dm := DecodedMeta{}

	areaVal := kv["area"]
	if areaVal == nil {
		return dm, util.FmtNewtError(
			"meta map missing required field \"area\"")
	}
	dm.Area = cast.ToString(areaVal)

	hash, err := decodeBoolDflt(kv, "hash", false)
	if err != nil {
		return dm, err
	}
	dm.Hash = hash

	fm, err := decodeBoolDflt(kv, "flash_map", false)
	if err != nil {
		return dm, err
	}
	dm.FlashMap = fm

	yamlMmrs := kv["mmrs"]
	if yamlMmrs != nil {
		mmrs, err := decodeMmrs(yamlMmrs)
		if err != nil {
			return dm, err
		}
		dm.Mmrs = mmrs
	}

	return dm, nil
}

func decodeMfg(yc ycfg.YCfg) (DecodedMfg, error) {
	dm := DecodedMfg{}

	yamlTargets := yc.GetValSlice("mfg.targets", nil)
	if yamlTargets != nil {
		for _, yamlTarget := range yamlTargets {
			t, err := decodeTarget(yamlTarget)
			if err != nil {
				return dm, err
			}

			dm.Targets = append(dm.Targets, t)
		}
	}

	dm.Bsp = yc.GetValString("mfg.bsp", nil)

	if len(dm.Targets) == 0 && dm.Bsp == "" {
		return dm, util.FmtNewtError(
			"\"mfg.bsp\" field required for mfg images without any targets")
	}

	itf := yc.GetValSlice("mfg.raw", nil)
	slice := cast.ToSlice(itf)
	if slice != nil {
		for i, yamlRaw := range slice {
			raw, err := decodeRaw(yamlRaw, i)
			if err != nil {
				return dm, err
			}

			dm.Raws = append(dm.Raws, raw)
		}
	}

	yamlMeta := yc.GetValStringMap("mfg.meta", nil)
	if yamlMeta != nil {
		meta, err := decodeMeta(yamlMeta)
		if err != nil {
			return dm, err
		}
		dm.Meta = &meta
	}

	return dm, nil
}
