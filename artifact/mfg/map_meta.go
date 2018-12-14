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
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"

	"mynewt.apache.org/newt/util"
)

func (t *MetaTlv) bodyMap() (map[string]interface{}, error) {
	r := bytes.NewReader(t.Data)

	readBody := func(dst interface{}) error {
		if err := binary.Read(r, binary.LittleEndian, dst); err != nil {
			return util.FmtNewtError(
				"Error parsing TLV data: %s", err.Error())
		}
		return nil
	}

	switch t.Header.Type {
	case META_TLV_TYPE_HASH:
		var body MetaTlvBodyHash
		if err := readBody(&body); err != nil {
			return nil, err
		}
		return body.Map(), nil

	case META_TLV_TYPE_FLASH_AREA:
		var body MetaTlvBodyFlashArea
		if err := readBody(&body); err != nil {
			return nil, err
		}
		return body.Map(), nil

	case META_TLV_TYPE_MMR_REF:
		var body MetaTlvBodyMmrRef
		if err := readBody(&body); err != nil {
			return nil, err
		}
		return body.Map(), nil

	default:
		return nil, util.FmtNewtError(
			"Unknown meta TLV type: %d", t.Header.Type)
	}
}

func (b *MetaTlvBodyFlashArea) Map() map[string]interface{} {
	return map[string]interface{}{
		"area":   b.Area,
		"device": b.Device,
		"offset": b.Offset,
		"size":   b.Size,
	}
}

func (b *MetaTlvBodyHash) Map() map[string]interface{} {
	return map[string]interface{}{
		"hash": hex.EncodeToString(b.Hash[:]),
	}
}

func (b *MetaTlvBodyMmrRef) Map() map[string]interface{} {
	return map[string]interface{}{
		"area": b.Area,
	}
}

func (t *MetaTlv) Map(offset int) map[string]interface{} {
	hmap := map[string]interface{}{
		"_type_name": MetaTlvTypeName(t.Header.Type),
		"type":       t.Header.Type,
		"size":       t.Header.Size,
	}

	var body interface{}

	bmap, err := t.bodyMap()
	if err != nil {
		body = hex.EncodeToString(t.Data)
	} else {
		body = bmap
	}

	return map[string]interface{}{
		"_offset": offset,
		"header":  hmap,
		"data":    body,
	}
}

func (f *MetaFooter) Map(offset int) map[string]interface{} {
	return map[string]interface{}{
		"_offset": offset,
		"size":    f.Size,
		"magic":   f.Magic,
		"version": f.Version,
	}
}

func (m *Meta) Map(endOffset int) map[string]interface{} {
	offsets := m.Offsets()
	startOffset := endOffset - int(m.Footer.Size)

	tlvs := []map[string]interface{}{}
	for i, t := range m.Tlvs {
		tlv := t.Map(startOffset + offsets.Tlvs[i])
		tlvs = append(tlvs, tlv)
	}

	ftr := m.Footer.Map(startOffset + offsets.Footer)

	return map[string]interface{}{
		"_offset":     startOffset,
		"_end_offset": endOffset,
		"_size":       m.Footer.Size,
		"tlvs":        tlvs,
		"footer":      ftr,
	}
}

func (m *Meta) Json(offset int) (string, error) {
	mmap := m.Map(offset)

	bin, err := json.MarshalIndent(mmap, "", "    ")
	if err != nil {
		return "", util.ChildNewtError(err)
	}

	return string(bin), nil
}
