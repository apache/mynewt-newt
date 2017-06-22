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
	"strconv"
	"strings"

	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

const MFG_YAML_FILENAME string = "mfg.yml"

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

func (mi *MfgImage) loadRawEntry(
	entryIdx int, rawEntry map[string]string) (MfgRawEntry, error) {

	raw := MfgRawEntry{}

	var err error

	deviceStr := rawEntry["device"]
	if deviceStr == "" {
		return raw, mi.loadError(
			"raw entry %d missing required \"device\" field", entryIdx)
	}

	raw.device, err = util.AtoiNoOct(deviceStr)
	if err != nil {
		return raw, mi.loadError(
			"raw entry %d contains invalid offset: %s", entryIdx, deviceStr)
	}

	offsetStr := rawEntry["offset"]
	if offsetStr == "" {
		return raw, mi.loadError(
			"raw entry %d missing required \"offset\" field", entryIdx)
	}

	raw.offset, err = util.AtoiNoOct(offsetStr)
	if err != nil {
		return raw, mi.loadError(
			"raw entry %d contains invalid offset: %s", entryIdx, offsetStr)
	}

	raw.filename = rawEntry["file"]
	if raw.filename == "" {
		return raw, mi.loadError(
			"raw entry %d missing required \"file\" field", entryIdx)
	}

	if !strings.HasPrefix(raw.filename, "/") {
		raw.filename = mi.basePkg.BasePath() + "/" + raw.filename
	}

	raw.data, err = ioutil.ReadFile(raw.filename)
	if err != nil {
		return raw, mi.loadError(
			"error loading file for raw entry %d; filename=%s: %s",
			entryIdx, raw.filename, err.Error())
	}

	return raw, nil
}

func (mi *MfgImage) detectInvalidDevices() error {
	sectionIds := mi.sectionIds()
	deviceIds := mi.bsp.FlashMap.DeviceIds()

	deviceMap := map[int]struct{}{}
	for _, device := range deviceIds {
		deviceMap[device] = struct{}{}
	}

	invalidIds := []int{}
	for _, sectionId := range sectionIds {
		if _, ok := deviceMap[sectionId]; !ok {
			invalidIds = append(invalidIds, sectionId)
		}
	}

	if len(invalidIds) == 0 {
		return nil
	}

	listStr := ""
	for i, id := range invalidIds {
		if i != 0 {
			listStr += ", "
		}
		listStr += strconv.Itoa(id)
	}

	return util.FmtNewtError(
		"image specifies flash devices that are not present in the BSP's "+
			"flash map: %s", listStr)
}

func (mi *MfgImage) detectOverlaps() error {
	type overlap struct {
		part0 mfgPart
		part1 mfgPart
	}

	overlaps := []overlap{}

	dpMap, err := mi.devicePartMap()
	if err != nil {
		return err
	}

	// Iterate flash devices in order.
	devices := make([]int, 0, len(dpMap))
	for device, _ := range dpMap {
		devices = append(devices, device)
	}
	sort.Ints(devices)

	for _, device := range devices {
		parts := dpMap[device]
		for i, part0 := range parts[:len(parts)-1] {
			part0End := part0.offset + len(part0.data)
			for _, part1 := range parts[i+1:] {
				// Parts are sorted by offset, so only one comparison is
				// necessary to detect overlap.
				if part1.offset < part0End {
					overlaps = append(overlaps, overlap{
						part0: part0,
						part1: part1,
					})
				}
			}
		}
	}

	if len(overlaps) > 0 {
		str := "flash overlaps detected:"
		for _, overlap := range overlaps {

			part0End := overlap.part0.offset + len(overlap.part0.data)
			part1End := overlap.part1.offset + len(overlap.part1.data)
			str += fmt.Sprintf("\n    * s%d [%s] (%d - %d) <=> [%s] (%d - %d)",
				overlap.part0.device,
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
			yamlEntry := cast.ToStringMapString(entryItf)
			entry, err := mi.loadRawEntry(i, yamlEntry)
			if err != nil {
				return nil, err
			}

			mi.rawEntries = append(mi.rawEntries, entry)
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

	compilerPkg, err := proj.ResolvePackage(mi.bsp.Repo(), mi.bsp.CompilerName)
	if err != nil {
		return nil, mi.loadError(err.Error())
	}
	mi.compiler, err = toolchain.NewCompiler(compilerPkg.BasePath(), "",
							target.DEFAULT_BUILD_PROFILE)
	if err != nil {
		return nil, mi.loadError(err.Error())
	}

	for _, imgTarget := range mi.images {
		if len(mi.images) > 1 && imgTarget.LoaderName != "" {
			return nil, mi.loadError("only one image allowed in "+
				"split image mode (%s is a split build)", imgTarget.Name())
		}

		if imgTarget.Bsp() != mi.bsp.LocalPackage {
			return nil, mi.loadError(
				"image target \"%s\" specified conflicting BSP; "+
					"boot loader uses %s, image uses %s",
				imgTarget.Name(), mi.bsp.Name(), imgTarget.BspName)
		}
	}

	if err := mi.detectInvalidDevices(); err != nil {
		return nil, err
	}

	return mi, nil
}
