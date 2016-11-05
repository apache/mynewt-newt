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

package flash

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

const FLASH_AREA_NAME_BOOTLOADER = "FLASH_AREA_BOOTLOADER"
const FLASH_AREA_NAME_IMAGE_0 = "FLASH_AREA_IMAGE_0"
const FLASH_AREA_NAME_IMAGE_1 = "FLASH_AREA_IMAGE_1"
const FLASH_AREA_NAME_IMAGE_SCRATCH = "FLASH_AREA_IMAGE_SCRATCH"

var SYSTEM_AREA_NAME_ID_MAP = map[string]int{
	FLASH_AREA_NAME_BOOTLOADER:    0,
	FLASH_AREA_NAME_IMAGE_0:       1,
	FLASH_AREA_NAME_IMAGE_1:       2,
	FLASH_AREA_NAME_IMAGE_SCRATCH: 3,
}

const AREA_USER_ID_MIN = 16

const HEADER_PATH = "sysflash/sysflash.h"
const C_VAR_NAME = "sysflash_map_dflt"
const C_VAR_COMMENT = `/**
 * This flash map definition is used for two purposes:
 * 1. To locate the meta area, which contains the true flash map definition.
 * 2. As a fallback in case the meta area cannot be read from flash.
 */
`

type FlashArea struct {
	Name   string
	Id     int
	Device int
	Offset int
	Size   int
}

type FlashMap struct {
	Areas       map[string]FlashArea
	Overlaps    [][]FlashArea
	IdConflicts [][]FlashArea
}

func newFlashMap() FlashMap {
	return FlashMap{
		Areas:    map[string]FlashArea{},
		Overlaps: [][]FlashArea{},
	}
}

func flashAreaErr(areaName string, format string, args ...interface{}) error {
	return util.NewNewtError(
		"failure while parsing flash area \"" + areaName + "\": " +
			fmt.Sprintf(format, args...))
}

func parseSize(val string) (int, error) {
	lower := strings.ToLower(val)

	multiplier := 1
	if strings.HasSuffix(lower, "kb") {
		multiplier = 1024
		lower = strings.TrimSuffix(lower, "kb")
	}

	num, err := util.AtoiNoOct(lower)
	if err != nil {
		return 0, err
	}

	return num * multiplier, nil
}

func parseFlashArea(
	name string, ymlFields map[string]interface{}) (FlashArea, error) {

	area := FlashArea{
		Name: name,
	}

	idPresent := false
	devicePresent := false
	offsetPresent := false
	sizePresent := false

	var isSystem bool
	area.Id, isSystem = SYSTEM_AREA_NAME_ID_MAP[name]

	var err error

	fields := cast.ToStringMapString(ymlFields)
	for k, v := range fields {
		switch k {
		case "user_id":
			if isSystem {
				return area, flashAreaErr(name,
					"system areas cannot specify a user ID")
			}
			userId, err := util.AtoiNoOct(v)
			if err != nil {
				return area, flashAreaErr(name, "invalid user id: %s", v)
			}
			area.Id = userId + AREA_USER_ID_MIN
			idPresent = true

		case "device":
			area.Device, err = util.AtoiNoOct(v)
			if err != nil {
				return area, flashAreaErr(name, "invalid device: %s", v)
			}
			devicePresent = true

		case "offset":
			area.Offset, err = util.AtoiNoOct(v)
			if err != nil {
				return area, flashAreaErr(name, "invalid offset: %s", v)
			}
			offsetPresent = true

		case "size":
			area.Size, err = parseSize(v)
			if err != nil {
				return area, flashAreaErr(name, err.Error())
			}
			sizePresent = true

		default:
			util.StatusMessage(util.VERBOSITY_QUIET,
				"Warning: flash area \"%s\" contains unrecognized field: %s",
				name, k)
		}
	}

	if !isSystem && !idPresent {
		return area, flashAreaErr(name, "required field \"user_id\" missing")
	}
	if !devicePresent {
		return area, flashAreaErr(name, "required field \"device\" missing")
	}
	if !offsetPresent {
		return area, flashAreaErr(name, "required field \"offset\" missing")
	}
	if !sizePresent {
		return area, flashAreaErr(name, "required field \"size\" missing")
	}

	return area, nil
}

func (flashMap FlashMap) unSortedAreas() []FlashArea {
	areas := make([]FlashArea, 0, len(flashMap.Areas))
	for _, area := range flashMap.Areas {
		areas = append(areas, area)
	}

	return areas
}

func (flashMap FlashMap) SortedAreas() []FlashArea {
	idMap := make(map[int]FlashArea, len(flashMap.Areas))
	ids := make([]int, 0, len(flashMap.Areas))
	for _, area := range flashMap.Areas {
		idMap[area.Id] = area
		ids = append(ids, area.Id)
	}
	sort.Ints(ids)

	areas := make([]FlashArea, len(ids))
	for i, id := range ids {
		areas[i] = idMap[id]
	}

	return areas
}

func (flashMap FlashMap) DeviceIds() []int {
	deviceMap := map[int]struct{}{}

	for _, area := range flashMap.Areas {
		deviceMap[area.Device] = struct{}{}
	}

	devices := make([]int, 0, len(deviceMap))
	for device, _ := range deviceMap {
		devices = append(devices, device)
	}
	sort.Ints(devices)

	return devices
}

func areasDistinct(a FlashArea, b FlashArea) bool {
	var lo FlashArea
	var hi FlashArea

	if a.Offset < b.Offset {
		lo = a
		hi = b
	} else {
		lo = b
		hi = a
	}

	return lo.Device != hi.Device || lo.Offset+lo.Size <= hi.Offset
}

func (flashMap *FlashMap) detectOverlaps() {
	flashMap.Overlaps = [][]FlashArea{}

	// Convert the map to a slice.
	areas := flashMap.unSortedAreas()

	for i := 0; i < len(areas)-1; i++ {
		iarea := areas[i]
		for j := i + 1; j < len(areas); j++ {
			jarea := areas[j]

			if !areasDistinct(iarea, jarea) {
				flashMap.Overlaps = append(
					flashMap.Overlaps, []FlashArea{iarea, jarea})
			}

			if iarea.Id == jarea.Id {
				flashMap.IdConflicts = append(
					flashMap.IdConflicts, []FlashArea{iarea, jarea})
			}
		}
	}
}

func (flashMap FlashMap) ErrorText() string {
	str := ""

	if len(flashMap.IdConflicts) > 0 {
		str += "Conflicting flash area IDs detected:\n"

		for _, pair := range flashMap.IdConflicts {
			str += fmt.Sprintf("    (%d) %s =/= %s\n",
				pair[0].Id-AREA_USER_ID_MIN, pair[0].Name, pair[1].Name)
		}
	}

	if len(flashMap.Overlaps) > 0 {
		str += "Overlapping flash areas detected:\n"

		for _, pair := range flashMap.Overlaps {
			str += fmt.Sprintf("    %s =/= %s\n", pair[0].Name, pair[1].Name)
		}
	}

	return str
}

func Read(ymlFlashMap map[string]interface{}) (FlashMap, error) {
	flashMap := newFlashMap()

	ymlAreas := ymlFlashMap["areas"]
	if ymlAreas == nil {
		return flashMap, util.NewNewtError(
			"\"areas\" mapping missing from flash map definition")
	}

	areaMap := cast.ToStringMap(ymlAreas)
	for k, v := range areaMap {
		if _, ok := flashMap.Areas[k]; ok {
			return flashMap, flashAreaErr(k, "name conflict")
		}

		ymlArea := cast.ToStringMap(v)
		area, err := parseFlashArea(k, ymlArea)
		if err != nil {
			return flashMap, flashAreaErr(k, err.Error())
		}

		flashMap.Areas[k] = area
	}

	flashMap.detectOverlaps()

	return flashMap, nil
}

func (flashMap FlashMap) varDecl() string {
	return fmt.Sprintf("const struct flash_area %s[%d]", C_VAR_NAME,
		len(flashMap.Areas))
}

func (area FlashArea) writeHeader(w io.Writer) {
	fmt.Fprintf(w, "#define %-40s %d\n", area.Name, area.Id)
}

func (flashMap FlashMap) writeHeader(w io.Writer) {
	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	fmt.Fprintf(w, "#ifndef H_MYNEWT_SYSFLASH_\n")
	fmt.Fprintf(w, "#define H_MYNEWT_SYSFLASH_\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "#include \"flash_map/flash_map.h\"\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%s", C_VAR_COMMENT)
	fmt.Fprintf(w, "extern %s;\n", flashMap.varDecl())
	fmt.Fprintf(w, "\n")

	for _, area := range flashMap.SortedAreas() {
		area.writeHeader(w)
	}

	fmt.Fprintf(w, "\n#endif\n")
}

func sizeComment(size int) string {
	if size%1024 != 0 {
		return ""
	}

	return fmt.Sprintf(" /* %d kB */", size/1024)
}

func (area FlashArea) writeSrc(w io.Writer) {
	fmt.Fprintf(w, "    /* %s */\n", area.Name)
	fmt.Fprintf(w, "    {\n")
	fmt.Fprintf(w, "        .fa_id = %d,\n", area.Id)
	fmt.Fprintf(w, "        .fa_device_id = %d,\n", area.Device)
	fmt.Fprintf(w, "        .fa_off = 0x%08x,\n", area.Offset)
	fmt.Fprintf(w, "        .fa_size = %d,%s\n", area.Size,
		sizeComment(area.Size))
	fmt.Fprintf(w, "    },\n")
}

func (flashMap FlashMap) writeSrc(w io.Writer) {
	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	fmt.Fprintf(w, "#include \"%s\"\n", HEADER_PATH)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%s", C_VAR_COMMENT)
	fmt.Fprintf(w, "%s = {", flashMap.varDecl())

	for _, area := range flashMap.SortedAreas() {
		fmt.Fprintf(w, "\n")
		area.writeSrc(w)
	}

	fmt.Fprintf(w, "};\n")
}

func (flashMap FlashMap) ensureWrittenGen(path string, contents []byte) error {
	writeReqd, err := util.FileContentsChanged(path, contents)
	if err != nil {
		return err
	}
	if !writeReqd {
		log.Debugf("flash map unchanged; not writing file (%s).", path)
		return nil
	}

	log.Debugf("flash map changed; writing file (%s).", path)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return util.NewNewtError(err.Error())
	}

	if err := ioutil.WriteFile(path, contents, 0644); err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}

func (flashMap FlashMap) EnsureWritten(
	srcDir string, includeDir string, targetName string) error {

	buf := bytes.Buffer{}
	flashMap.writeSrc(&buf)
	if err := flashMap.ensureWrittenGen(
		fmt.Sprintf("%s/%s-sysflash.c", srcDir, targetName),
		buf.Bytes()); err != nil {

		return err
	}

	buf = bytes.Buffer{}
	flashMap.writeHeader(&buf)
	if err := flashMap.ensureWrittenGen(
		includeDir+"/"+HEADER_PATH, buf.Bytes()); err != nil {
		return err
	}

	return nil
}
