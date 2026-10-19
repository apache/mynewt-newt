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

package builder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const sizeTestMapNoDefault = `Archive member included to satisfy reference

Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000000008000 0x0000000000040000 xr

RAM              0x0000000020000000 0x0000000000010000 xrw

Linker script and memory map

 .text          0x0000000000008000       0x10 obj/foo.o
`

const sizeTestMapShortLine = `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000000008000
*default*        0x0000000000000000 0xffffffffffffffff

Linker script and memory map
`

const sizeTestMapNoMemory = `Memory Configuration

Name             Origin             Length             Attributes

Linker script and memory map

 .text          0x0000000000008000       0x10 obj/foo.o
`

func sizeTestWriteMap(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.elf.map")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	return path
}

// sizeTestRun runs f and fails the test if it panics or does not finish
// within a few seconds.
func sizeTestRun(t *testing.T, f func()) {
	t.Helper()

	done := make(chan interface{})
	go func() {
		defer func() {
			done <- recover()
		}()
		f()
	}()

	select {
	case p := <-done:
		if p != nil {
			t.Fatalf("panic: %v", p)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}
}

func TestParseMapFileSizesMalformed(t *testing.T) {
	for name, content := range map[string]string{
		"no_default": sizeTestMapNoDefault,
		"no_memory":  sizeTestMapNoMemory,
	} {
		t.Run(name, func(t *testing.T) {
			path := sizeTestWriteMap(t, content)
			sizeTestRun(t, func() {
				if _, err := ParseMapFileSizes(path); err != nil {
					t.Errorf("unexpected error: %s", err)
				}
			})
		})
	}

	t.Run("short_line", func(t *testing.T) {
		path := sizeTestWriteMap(t, sizeTestMapShortLine)
		sizeTestRun(t, func() {
			if _, err := ParseMapFileSizes(path); err == nil {
				t.Errorf("expected error")
			}
		})
	})
}

func TestParseMapFileSizesNoDefault(t *testing.T) {
	path := sizeTestWriteMap(t, sizeTestMapNoDefault)
	sizeTestRun(t, func() {
		pkgs, err := ParseMapFileSizes(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(globalMemSections) != 2 ||
			globalMemSections["FLASH"] == nil ||
			globalMemSections["RAM"] == nil {

			t.Errorf("unexpected memory sections: %v", globalMemSections)
		}
		if pkgs["obj/foo.o"] == nil {
			t.Errorf("symbols not parsed: %v", pkgs)
		}
	})
}

func TestParseMapFileRegionsMalformed(t *testing.T) {
	for name, content := range map[string]string{
		"no_default": sizeTestMapNoDefault,
		"short_line": sizeTestMapShortLine,
		"no_memory":  sizeTestMapNoMemory,
	} {
		t.Run(name, func(t *testing.T) {
			path := sizeTestWriteMap(t, content)
			sizeTestRun(t, func() {
				if _, err := parseMapFileRegions(path, "FLASH"); err != nil {
					t.Errorf("unexpected error: %s", err)
				}
			})
		})
	}

	path := sizeTestWriteMap(t, sizeTestMapNoDefault)
	sizeTestRun(t, func() {
		r, err := parseMapFileRegions(path, "RAM")
		if err != nil {
			t.Fatal(err)
		}
		if r.Name != "RAM" || r.Offset != 0x20000000 ||
			r.EndOff != 0x20010000 {

			t.Errorf("unexpected region: %+v", r)
		}
	})
}
