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

package porcelain

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, testInfo()); err != nil {
		t.Fatal(err)
	}

	var got interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	// The keys are part of the format; renaming any of them breaks readers.
	keys := map[string][]string{
		"": {"format_version", "newt_version", "packages", "project",
			"repos", "targets"},
		"project": {"name", "path"},
		"repos": {"commit", "dirty", "error", "installed", "local", "name",
			"needs_upgrade", "origin", "path", "requested"},
		"repos.origin": {"branch", "repo", "server", "type", "url", "user"},
		"targets": {"artifacts", "boot_loader", "build_profiles", "deps",
			"flash_area", "name", "path", "repo", "resolved", "syscfg",
			"syscfg_errors",
			"syscfg_warnings", "vars"},
		"targets.artifacts": {"bin_dir", "elf", "img"},
		"targets.syscfg": {"choices", "default", "description", "history",
			"name", "package", "restrictions", "state", "type", "value",
			"value_ref"},
		"targets.syscfg.restrictions": {"expr", "type"},
		"targets.syscfg.history":      {"source", "value"},
		"targets.deps":                {"name", "required_by"},
		"targets.deps.required_by":    {"apis", "name", "syscfg"},
		"packages": {"apis", "author", "bsp", "deps", "description",
			"homepage", "keywords", "name", "path", "repo", "req_apis",
			"syscfg_defs", "type"},
		"packages.bsp":           {"arch", "compiler", "flash_map"},
		"packages.bsp.flash_map": {"device", "id", "name", "offset", "size"},
	}
	for path, want := range keys {
		obj := objectAt(t, got, path)
		have := []string{}
		for k := range obj {
			have = append(have, k)
		}
		sort.Strings(have)
		if !reflect.DeepEqual(have, want) {
			t.Errorf("keys of %q: got %v, want %v", path, have, want)
		}
	}

	var info Info
	if err := json.Unmarshal(buf.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(&info, testInfo()) {
		t.Errorf("JSON does not round-trip: %+v", info)
	}
}

// objectAt returns the first object found at the dotted path of keys,
// trying each element of lists on the way.
func objectAt(t *testing.T, v interface{}, path string) map[string]interface{} {
	t.Helper()

	keys := []string{}
	if path != "" {
		keys = strings.Split(path, ".")
	}
	obj := findObject(v, keys)
	if obj == nil {
		t.Fatalf("no object at %q", path)
	}
	return obj
}

func findObject(v interface{}, keys []string) map[string]interface{} {
	switch v := v.(type) {
	case []interface{}:
		for _, e := range v {
			if obj := findObject(e, keys); obj != nil {
				return obj
			}
		}
	case map[string]interface{}:
		if len(keys) == 0 {
			return v
		}
		return findObject(v[keys[0]], keys[1:])
	}
	return nil
}
