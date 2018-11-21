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
	"fmt"
	"sort"
)

const FLASH_AREA_NAME_BOOTLOADER = "FLASH_AREA_BOOTLOADER"
const FLASH_AREA_NAME_IMAGE_0 = "FLASH_AREA_IMAGE_0"
const FLASH_AREA_NAME_IMAGE_1 = "FLASH_AREA_IMAGE_1"
const FLASH_AREA_NAME_IMAGE_SCRATCH = "FLASH_AREA_IMAGE_SCRATCH"
const AREA_USER_ID_MIN = 16

var SYSTEM_AREA_NAME_ID_MAP = map[string]int{
	FLASH_AREA_NAME_BOOTLOADER:    0,
	FLASH_AREA_NAME_IMAGE_0:       1,
	FLASH_AREA_NAME_IMAGE_1:       2,
	FLASH_AREA_NAME_IMAGE_SCRATCH: 3,
}

type FlashArea struct {
	Name   string
	Id     int
	Device int
	Offset int
	Size   int
}

type areaOffSorter struct {
	areas []FlashArea
}

func (s areaOffSorter) Len() int {
	return len(s.areas)
}
func (s areaOffSorter) Swap(i, j int) {
	s.areas[i], s.areas[j] = s.areas[j], s.areas[i]
}
func (s areaOffSorter) Less(i, j int) bool {
	ai := s.areas[i]
	aj := s.areas[j]

	if ai.Device < aj.Device {
		return true
	}
	if ai.Device > aj.Device {
		return false
	}
	return ai.Offset < aj.Offset
}

func SortFlashAreasByDevOff(areas []FlashArea) []FlashArea {
	sorter := areaOffSorter{
		areas: make([]FlashArea, len(areas)),
	}

	for i, a := range areas {
		sorter.areas[i] = a
	}

	sort.Sort(sorter)
	return sorter.areas
}

func SortFlashAreasById(areas []FlashArea) []FlashArea {
	idMap := make(map[int]FlashArea, len(areas))
	ids := make([]int, 0, len(areas))
	for _, area := range areas {
		idMap[area.Id] = area
		ids = append(ids, area.Id)
	}
	sort.Ints(ids)

	sorted := make([]FlashArea, len(ids))
	for i, id := range ids {
		sorted[i] = idMap[id]
	}

	return sorted
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

// @return overlapping-areas, id-conflicts.
func DetectErrors(areas []FlashArea) ([][]FlashArea, [][]FlashArea) {
	var overlaps [][]FlashArea
	var conflicts [][]FlashArea

	for i := 0; i < len(areas)-1; i++ {
		iarea := areas[i]
		for j := i + 1; j < len(areas); j++ {
			jarea := areas[j]

			if !areasDistinct(iarea, jarea) {
				overlaps = append(overlaps, []FlashArea{iarea, jarea})
			}

			if iarea.Id == jarea.Id {
				conflicts = append(conflicts, []FlashArea{iarea, jarea})
			}
		}
	}

	return overlaps, conflicts
}

func ErrorText(overlaps [][]FlashArea, conflicts [][]FlashArea) string {
	str := ""

	if len(conflicts) > 0 {
		str += "Conflicting flash area IDs detected:\n"

		for _, pair := range conflicts {
			str += fmt.Sprintf("    (%d) %s =/= %s\n",
				pair[0].Id-AREA_USER_ID_MIN, pair[0].Name, pair[1].Name)
		}
	}

	if len(overlaps) > 0 {
		str += "Overlapping flash areas detected:\n"

		for _, pair := range overlaps {
			str += fmt.Sprintf("    %s =/= %s\n", pair[0].Name, pair[1].Name)
		}
	}

	return str
}
