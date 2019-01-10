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
	"mynewt.apache.org/newt/artifact/sec"
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

// XXX: Only RSA supported for now.
func ExtractSecret(img *image.Image) ([]byte, error) {
	tlvs := img.RemoveTlvsWithType(image.IMAGE_TLV_ENC_RSA)
	if len(tlvs) != 1 {
		return nil, util.FmtNewtError(
			"Image contains invalid count of ENC_RSA TLVs: %d; must contain 1",
			len(tlvs))
	}

	return tlvs[0].Data, nil
}

// XXX: Only RSA supported for now.
func DecryptImage(img image.Image, privKeBytes []byte) (image.Image, error) {
	cipherSecret, err := ExtractSecret(&img)
	if err != nil {
		return img, err
	}

	privKe, err := sec.ParsePrivKeDer(privKeBytes)
	if err != nil {
		return img, err
	}

	plainSecret, err := sec.DecryptSecretRsa(privKe, cipherSecret)
	if err != nil {
		return img, err
	}

	body, err := sec.EncryptAES(img.Body, plainSecret)
	if err != nil {
		return img, err
	}

	img.Body = body
	return img, nil
}

func EncryptImage(img image.Image, pubKeBytes []byte) (image.Image, error) {
	tlvp, err := img.FindUniqueTlv(image.IMAGE_TLV_ENC_RSA)
	if err != nil {
		return img, err
	}
	if tlvp != nil {
		return img, util.FmtNewtError("Image already contains an ENC_RSA TLV")
	}

	plainSecret, err := image.GeneratePlainSecret()
	if err != nil {
		return img, err
	}

	cipherSecret, err := image.GenerateCipherSecret(pubKeBytes, plainSecret)
	if err != nil {
		return img, err
	}

	body, err := sec.EncryptAES(img.Body, plainSecret)
	if err != nil {
		return img, err
	}
	img.Body = body

	tlv, err := image.GenerateEncTlv(cipherSecret)
	if err != nil {
		return img, err
	}
	img.Tlvs = append(img.Tlvs, tlv)

	return img, nil
}
