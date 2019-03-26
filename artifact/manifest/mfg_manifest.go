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

package manifest

import (
	"encoding/json"
	"io/ioutil"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/util"
)

type MfgManifestTarget struct {
	Name         string `json:"name"`
	Offset       int    `json:"offset"`
	BinPath      string `json:"bin_path,omitempty"`
	ImagePath    string `json:"image_path,omitempty"`
	HexPath      string `json:"hex_path,omitempty"`
	ManifestPath string `json:"manifest_path"`
}

type MfgManifestMetaMmr struct {
	Area      string `json:"area"`
	Device    int    `json:"_device"`
	EndOffset int    `json:"_end_offset"`
}

type MfgManifestMeta struct {
	EndOffset int                  `json:"end_offset"`
	Size      int                  `json:"size"`
	Hash      bool                 `json:"hash_present"`
	FlashMap  bool                 `json:"flash_map_present"`
	Mmrs      []MfgManifestMetaMmr `json:"mmrs,omitempty"`
	// XXX: refhash
}

type MfgManifestSig struct {
	Key string `json:"key"`
	Sig string `json:"sig"`
}

type MfgManifest struct {
	Name       string            `json:"name"`
	BuildTime  string            `json:"build_time"`
	Format     int               `json:"format"`
	MfgHash    string            `json:"mfg_hash"`
	Version    string            `json:"version"`
	Device     int               `json:"device"`
	BinPath    string            `json:"bin_path"`
	HexPath    string            `json:"hex_path"`
	Bsp        string            `json:"bsp"`
	Signatures []MfgManifestSig  `json:"signatures,omitempty"`
	FlashAreas []flash.FlashArea `json:"flash_map"`

	Targets []MfgManifestTarget `json:"targets"`
	Meta    *MfgManifestMeta    `json:"meta,omitempty"`
}

func ReadMfgManifest(path string) (MfgManifest, error) {
	m := MfgManifest{}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return m, util.ChildNewtError(err)
	}

	if err := json.Unmarshal(content, &m); err != nil {
		return m, util.FmtNewtError(
			"Failure decoding mfg manifest with path \"%s\": %s",
			path, err.Error())
	}

	return m, nil
}

func (m *MfgManifest) MarshalJson() ([]byte, error) {
	buffer, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, util.FmtNewtError(
			"Cannot encode mfg manifest: %s", err.Error())
	}

	return buffer, nil
}
