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

package flashmap

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

const HEADER_PATH = "sysflash/sysflash.h"
const C_VAR_NAME = "sysflash_map_dflt"
const C_VAR_COMMENT = `/**
 * This flash map definition is used for two purposes:
 * 1. To locate the meta area, which contains the true flash map definition.
 * 2. As a fallback in case the meta area cannot be read from flash.
 */
`

type FlashMap struct {
	Areas       map[string]flash.FlashArea
	Overlaps    [][]flash.FlashArea
	IdConflicts [][]flash.FlashArea
}

func newFlashMap() FlashMap {
	return FlashMap{
		Areas:    map[string]flash.FlashArea{},
		Overlaps: [][]flash.FlashArea{},
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
	name string, ymlFields map[string]interface{}) (flash.FlashArea, error) {

	area := flash.FlashArea{
		Name: name,
	}

	idPresent := false
	devicePresent := false
	offsetPresent := false
	sizePresent := false

	var isSystem bool
	area.Id, isSystem = flash.SYSTEM_AREA_NAME_ID_MAP[name]

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
			area.Id = userId + flash.AREA_USER_ID_MIN
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

func (flashMap FlashMap) unSortedAreas() []flash.FlashArea {
	areas := make([]flash.FlashArea, 0, len(flashMap.Areas))
	for _, area := range flashMap.Areas {
		areas = append(areas, area)
	}

	return areas
}

func (flashMap FlashMap) SortedAreas() []flash.FlashArea {
	areas := flashMap.unSortedAreas()
	return flash.SortFlashAreasById(areas)
}

func areasDistinct(a flash.FlashArea, b flash.FlashArea) bool {
	var lo flash.FlashArea
	var hi flash.FlashArea

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
	flashMap.Overlaps, flashMap.IdConflicts =
		flash.DetectErrors(flashMap.unSortedAreas())
}

func (flashMap FlashMap) ErrorText() string {
	return flash.ErrorText(flashMap.Overlaps, flashMap.IdConflicts)
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

func flashMapVarDecl(fm FlashMap) string {
	return fmt.Sprintf("const struct flash_area %s[%d]", C_VAR_NAME,
		len(fm.Areas))
}

func writeFlashAreaHeader(w io.Writer, area flash.FlashArea) {
	fmt.Fprintf(w, "#define %-40s %d\n", area.Name, area.Id)
}

func writeFlashMapHeader(w io.Writer, fm FlashMap) {
	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	fmt.Fprintf(w, "#ifndef H_MYNEWT_SYSFLASH_\n")
	fmt.Fprintf(w, "#define H_MYNEWT_SYSFLASH_\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "#include \"flash_map/flash_map.h\"\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%s", C_VAR_COMMENT)
	fmt.Fprintf(w, "extern %s;\n", flashMapVarDecl(fm))
	fmt.Fprintf(w, "\n")

	for _, area := range fm.SortedAreas() {
		writeFlashAreaHeader(w, area)
	}

	fmt.Fprintf(w, "\n#endif\n")
}

func sizeComment(size int) string {
	if size%1024 != 0 {
		return ""
	}

	return fmt.Sprintf(" /* %d kB */", size/1024)
}

func writeFlashAreaSrc(w io.Writer, area flash.FlashArea) {
	fmt.Fprintf(w, "    /* %s */\n", area.Name)
	fmt.Fprintf(w, "    {\n")
	fmt.Fprintf(w, "        .fa_id = %d,\n", area.Id)
	fmt.Fprintf(w, "        .fa_device_id = %d,\n", area.Device)
	fmt.Fprintf(w, "        .fa_off = 0x%08x,\n", area.Offset)
	fmt.Fprintf(w, "        .fa_size = %d,%s\n", area.Size,
		sizeComment(area.Size))
	fmt.Fprintf(w, "    },\n")
}

func writeFlashMapSrc(w io.Writer, fm FlashMap) {
	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	fmt.Fprintf(w, "#include \"%s\"\n", HEADER_PATH)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "%s", C_VAR_COMMENT)
	fmt.Fprintf(w, "%s = {", flashMapVarDecl(fm))

	for _, area := range fm.SortedAreas() {
		fmt.Fprintf(w, "\n")
		writeFlashAreaSrc(w, area)
	}

	fmt.Fprintf(w, "};\n")
}

func ensureFlashMapWrittenGen(path string, contents []byte) error {
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

func EnsureFlashMapWritten(
	fm FlashMap,
	srcDir string,
	includeDir string,
	targetName string) error {

	buf := bytes.Buffer{}
	writeFlashMapSrc(&buf, fm)
	if err := ensureFlashMapWrittenGen(
		fmt.Sprintf("%s/%s-sysflash.c", srcDir, targetName),
		buf.Bytes()); err != nil {

		return err
	}

	buf = bytes.Buffer{}
	writeFlashMapHeader(&buf, fm)
	if err := ensureFlashMapWrittenGen(
		includeDir+"/"+HEADER_PATH, buf.Bytes()); err != nil {
		return err
	}

	return nil
}
