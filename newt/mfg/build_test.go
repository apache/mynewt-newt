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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apache/mynewt-artifact/flash"
	"mynewt.apache.org/newt/newt/flashmap"
	"mynewt.apache.org/newt/newt/pkg"
)

var testAreaBoot = flash.FlashArea{
	Name:   "FLASH_AREA_BOOTLOADER",
	Id:     0,
	Device: 0,
	Offset: 0x0,
	Size:   0x4000,
}

var testAreaImage = flash.FlashArea{
	Name:   "FLASH_AREA_IMAGE_0",
	Id:     1,
	Device: 0,
	Offset: 0x4000,
	Size:   0x8000,
}

// testRaw creates a raw entry with a file of the given size placed in the
// given area.
func testRaw(t *testing.T, area flash.FlashArea, offset int,
	size int) MfgBuildRaw {

	t.Helper()

	path := filepath.Join(t.TempDir(), "raw.bin")
	if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
		t.Fatal(err)
	}

	return MfgBuildRaw{
		Filename: path,
		Offset:   offset,
		Size:     size,
		Area:     area,
	}
}

func testBuilder(meta *MfgBuildMeta, raws ...MfgBuildRaw) MfgBuilder {
	fm := flashmap.FlashMap{
		Areas: map[string]flash.FlashArea{},
	}
	for _, r := range raws {
		fm.Areas[r.Area.Name] = r.Area
	}
	if meta != nil {
		fm.Areas[meta.Area.Name] = meta.Area
	}

	return MfgBuilder{
		Bsp:  &pkg.BspPackage{FlashMap: fm},
		Raws: raws,
		Meta: meta,
	}
}

// The meta region is located at the end of its area and must not overlap
// any part of the image.
func TestDetectOverlapsMeta(t *testing.T) {
	meta := &MfgBuildMeta{
		Area:     testAreaBoot,
		Hash:     true,
		FlashMap: true,
	}

	// Raw part followed by the meta region in the same area.
	mb := testBuilder(meta, testRaw(t, testAreaBoot, 0, 0x100),
		testRaw(t, testAreaImage, 0, testAreaImage.Size))
	if err := mb.detectOverlaps(); err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	// Raw part filling the whole area of the meta region.
	mb = testBuilder(meta, testRaw(t, testAreaBoot, 0, testAreaBoot.Size))
	err := mb.detectOverlaps()
	if err == nil {
		t.Fatal("overlap of meta region not detected")
	}
	if !strings.Contains(err.Error(), "meta (FLASH_AREA_BOOTLOADER)") {
		t.Errorf("unexpected error: %s", err)
	}

	// Meta region in an area which is fully used by a raw part.
	meta = &MfgBuildMeta{
		Area: testAreaImage,
		Hash: true,
	}
	mb = testBuilder(meta, testRaw(t, testAreaBoot, 0, 0x100),
		testRaw(t, testAreaImage, 0, testAreaImage.Size))
	if err := mb.detectOverlaps(); err == nil {
		t.Fatal("overlap of meta region not detected")
	}
}
