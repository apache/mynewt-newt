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

package lvimg

import (
	"encoding/hex"
	"fmt"
	"strings"

	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/util"
)

func GetDupSigs(img image.Image) []string {
	m := map[string]struct{}{}
	var dups []string

	for _, tlv := range img.Tlvs {
		if tlv.Header.Type == image.IMAGE_TLV_KEYHASH {
			h := hex.EncodeToString(tlv.Data)
			if _, ok := m[h]; ok {
				dups = append(dups, h)
			} else {
				m[h] = struct{}{}
			}
		}
	}

	return dups
}

func DetectInvalidSigTlvs(img image.Image) error {
	var errStrs []string
	addErr := func(format string, args ...interface{}) {
		s := fmt.Sprintf(format, args...)
		errStrs = append(errStrs, s)
	}

	prevIsHash := false
	for i, tlv := range img.Tlvs {
		curIsHash := tlv.Header.Type == image.IMAGE_TLV_KEYHASH
		curIsSig := image.ImageTlvTypeIsSig(tlv.Header.Type)
		isLast := i == len(img.Tlvs)-1

		if prevIsHash && !curIsSig {
			prevTlv := img.Tlvs[i-1]
			addErr("TLV%d (%s) not immediately followed by signature TLV",
				i-1, image.ImageTlvTypeName(prevTlv.Header.Type))
		} else if curIsHash && isLast {
			addErr("TLV%d (%s) not immediately followed by signature TLV",
				i, image.ImageTlvTypeName(tlv.Header.Type))
		} else if !prevIsHash && curIsSig {
			addErr("TLV%d (%s) not immediately preceded by key hash TLV",
				i, image.ImageTlvTypeName(tlv.Header.Type))
		}

		prevIsHash = curIsHash
	}

	if len(errStrs) > 0 {
		return util.FmtNewtError("%s", strings.Join(errStrs, "\n"))
	}

	return nil
}

func VerifyImage(img image.Image) error {
	if len(img.Tlvs) == 0 || img.Tlvs[0].Header.Type != image.IMAGE_TLV_SHA256 {
		return util.FmtNewtError("First TLV must be SHA256")
	}

	if err := DetectInvalidSigTlvs(img); err != nil {
		return err
	}

	if dups := GetDupSigs(img); len(dups) > 0 {
		s := "Duplicate signatures detected:"
		for _, d := range dups {
			s += fmt.Sprintf("\n    %s", d)
		}

		return util.FmtNewtError("%s", s)
	}

	return nil
}

func PadEcdsa256Sig(sig []byte) ([]byte, error) {
	if len(sig) < 70 {
		return nil, util.FmtNewtError(
			"Invalid ECDSA256 signature; length (%d) less than 70", len(sig))
	}

	if len(sig) < 72 {
		sig = append(sig, []byte{0x00, 0x00}...)
	}

	return sig, nil
}
