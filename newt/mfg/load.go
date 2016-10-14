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
	"io/ioutil"
	"sort"
	"strings"

	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/flash"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

type partSorter struct {
	parts []mfgPart
}

func (s partSorter) Len() int {
	return len(s.parts)
}
func (s partSorter) Swap(i, j int) {
	s.parts[i], s.parts[j] = s.parts[j], s.parts[i]
}
func (s partSorter) Less(i, j int) bool {
	return s.parts[i].offset < s.parts[j].offset
}

func sortParts(parts []mfgPart) []mfgPart {
	sorter := partSorter{
		parts: parts,
	}

	sort.Sort(sorter)
	return sorter.parts
}

func (mi *MfgImage) loadError(
	msg string, args ...interface{}) *util.NewtError {

	return util.FmtNewtError("Error in %s mfg: %s", mi.basePkg.Name(),
		fmt.Sprintf(msg, args...))

}

func (mi *MfgImage) loadTarget(targetName string) (
	*target.Target, error) {

	tgt := target.GetTargets()[targetName]
	if tgt == nil {
		return nil, mi.loadError("cannot resolve referenced target \"%s\"",
			targetName)
	}

	return tgt, nil
}

func (mi *MfgImage) loadRawSection(
	entryIdx int, rawEntry map[string]string) (MfgRawSection, error) {

	section := MfgRawSection{}

	offsetStr := rawEntry["offset"]
	if offsetStr == "" {
		return section, mi.loadError(
			"raw rawEntry %d missing required \"offset\" field", entryIdx)
	}

	var err error
	section.offset, err = util.AtoiNoOct(offsetStr)
	if err != nil {
		return section, mi.loadError(
			"raw rawEntry %d contains invalid offset: %s", entryIdx, offsetStr)
	}

	section.filename = rawEntry["file"]
	if section.filename == "" {
		return section, mi.loadError(
			"raw rawEntry %d missing required \"file\" field", entryIdx)
	}

	if !strings.HasPrefix(section.filename, "/") {
		section.filename = mi.basePkg.BasePath() + "/" + section.filename
	}

	section.data, err = ioutil.ReadFile(section.filename)
	if err != nil {
		return section, mi.loadError(
			"error loading file for raw rawEntry %d; filename=%s: %s",
			entryIdx, section.filename, err.Error())
	}

	return section, nil
}

func (mi *MfgImage) areaNameToPart(areaName string) (mfgPart, bool) {
	part := mfgPart{}

	area, ok := mi.bsp.FlashMap.Areas[areaName]
	if !ok {
		return part, false
	}

	part.offset = area.Offset
	part.data = make([]byte, area.Size)
	part.name = area.Name

	return part, true
}

func (mi *MfgImage) detectOverlaps() error {
	type overlap struct {
		part0 mfgPart
		part1 mfgPart
	}

	parts := []mfgPart{}

	// If an image slot is used, the entire flash area is unwritable.  This
	// restriction comes from the boot loader's need to write status at the end
	// of an area.
	if mi.boot != nil {
		part, ok := mi.areaNameToPart(flash.FLASH_AREA_NAME_BOOTLOADER)
		if ok {
			parts = append(parts, part)
		}
	}
	if len(mi.images) >= 1 {
		part, ok := mi.areaNameToPart(flash.FLASH_AREA_NAME_IMAGE_0)
		if ok {
			parts = append(parts, part)
		}
	}
	if len(mi.images) >= 2 {
		part, ok := mi.areaNameToPart(flash.FLASH_AREA_NAME_IMAGE_1)
		if ok {
			parts = append(parts, part)
		}
	}

	parts = append(parts, mi.rawSectionParts()...)
	sortParts(parts)

	overlaps := []overlap{}

	for i, part0 := range parts[:len(parts)-1] {
		part0End := part0.offset + len(part0.data)
		for _, part1 := range parts[i+1:] {
			// Parts are sorted by offset, so only one comparison is necessary
			// to detect overlap.
			if part1.offset < part0End {
				overlaps = append(overlaps, overlap{
					part0: part0,
					part1: part1,
				})
			}
		}
	}

	if len(overlaps) > 0 {
		str := "flash overlaps detected:"
		for _, overlap := range overlaps {

			part0End := overlap.part0.offset + len(overlap.part0.data)
			part1End := overlap.part1.offset + len(overlap.part1.data)
			str += fmt.Sprintf("\n    * [%s] (%d - %d) <=> [%s] (%d - %d)",
				overlap.part0.name, overlap.part0.offset, part0End,
				overlap.part1.name, overlap.part1.offset, part1End)
		}

		return util.NewNewtError(str)
	}

	return nil
}

func Load(basePkg *pkg.LocalPackage) (*MfgImage, error) {
	v, err := util.ReadConfig(basePkg.BasePath(),
		strings.TrimSuffix(MFG_YAML_FILENAME, ".yml"))
	if err != nil {
		return nil, err
	}

	mi := &MfgImage{
		basePkg: basePkg,
	}

	bootName := v.GetString("mfg.bootloader")
	if bootName == "" {
		return nil, mi.loadError("mfg.bootloader field required")
	}
	mi.boot, err = mi.loadTarget(bootName)
	if err != nil {
		return nil, err
	}

	imgNames := v.GetStringSlice("mfg.images")
	if imgNames != nil {
		for _, imgName := range imgNames {
			imgTarget, err := mi.loadTarget(imgName)
			if err != nil {
				return nil, err
			}

			mi.images = append(mi.images, imgTarget)
		}
	}

	if len(mi.images) > 2 {
		return nil, mi.loadError("too many images (%d); maximum is 2",
			len(mi.images))
	}

	itf := v.Get("mfg.raw")
	slice := cast.ToSlice(itf)
	if slice != nil {
		for i, entryItf := range slice {
			entry := cast.ToStringMapString(entryItf)
			section, err := mi.loadRawSection(i, entry)
			if err != nil {
				return nil, err
			}

			mi.rawSections = append(mi.rawSections, section)
		}
	}

	proj := project.GetProject()

	bspLpkg, err := proj.ResolvePackage(mi.basePkg.Repo(),
		mi.boot.BspName)
	if err != nil {
		return nil, mi.loadError(
			"could not resolve boot loader BSP package: %s",
			mi.boot.BspName)
	}
	mi.bsp, err = pkg.NewBspPackage(bspLpkg)
	if err != nil {
		return nil, mi.loadError(err.Error())
	}

	for _, imgTarget := range mi.images {
		if len(mi.images) > 1 && imgTarget.LoaderName != "" {
			return nil, mi.loadError("only one image allowed in "+
				"split image mode (%s is a split build)", imgTarget.Name())
		}

		if imgTarget.BspName != mi.bsp.Name() {
			return nil, mi.loadError(
				"image target \"%s\" specified conflicting BSP; "+
					"boot loader uses %s, image uses %s",
				imgTarget.Name(), mi.bsp.Name(), imgTarget.BspName)
		}
	}

	if err := mi.detectOverlaps(); err != nil {
		return nil, err
	}

	return mi, nil
}
