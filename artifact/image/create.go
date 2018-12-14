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

package image

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"io"
	"io/ioutil"
	"math/big"

	"mynewt.apache.org/newt/util"
)

type ImageCreator struct {
	Body         []byte
	Version      ImageVersion
	SigKeys      []ImageSigKey
	PlainSecret  []byte
	CipherSecret []byte
	HeaderSize   int
	InitialHash  []byte
	Bootable     bool
}

type ImageCreateOpts struct {
	SrcBinFilename    string
	SrcEncKeyFilename string
	Version           ImageVersion
	SigKeys           []ImageSigKey
	LoaderHash        []byte
}

type ECDSASig struct {
	R *big.Int
	S *big.Int
}

func NewImageCreator() ImageCreator {
	return ImageCreator{
		HeaderSize: IMAGE_HEADER_SIZE,
		Bootable:   true,
	}
}

func generateEncTlv(cipherSecret []byte) (ImageTlv, error) {
	var encType uint8

	if len(cipherSecret) == 256 {
		encType = IMAGE_TLV_ENC_RSA
	} else if len(cipherSecret) == 24 {
		encType = IMAGE_TLV_ENC_KEK
	} else {
		return ImageTlv{}, util.FmtNewtError("Invalid enc TLV size ")
	}

	return ImageTlv{
		Header: ImageTlvHdr{
			Type: encType,
			Pad:  0,
			Len:  uint16(len(cipherSecret)),
		},
		Data: cipherSecret,
	}, nil
}

func generateSigRsa(key ImageSigKey, hash []byte) ([]byte, error) {
	opts := rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	}
	signature, err := rsa.SignPSS(
		rand.Reader, key.Rsa, crypto.SHA256, hash, &opts)
	if err != nil {
		return nil, util.FmtNewtError("Failed to compute signature: %s", err)
	}

	return signature, nil
}

func generateSigEc(key ImageSigKey, hash []byte) ([]byte, error) {
	r, s, err := ecdsa.Sign(rand.Reader, key.Ec, hash)
	if err != nil {
		return nil, util.FmtNewtError("Failed to compute signature: %s", err)
	}

	ECDSA := ECDSASig{
		R: r,
		S: s,
	}

	signature, err := asn1.Marshal(ECDSA)
	if err != nil {
		return nil, util.FmtNewtError("Failed to construct signature: %s", err)
	}

	sigLen := key.sigLen()
	if len(signature) > int(sigLen) {
		return nil, util.FmtNewtError("Something is really wrong\n")
	}

	pad := make([]byte, int(sigLen)-len(signature))
	signature = append(signature, pad...)

	return signature, nil
}

func generateSig(key ImageSigKey, hash []byte) ([]byte, error) {
	key.assertValid()

	if key.Rsa != nil {
		return generateSigRsa(key, hash)
	} else {
		return generateSigEc(key, hash)
	}
}

func BuildKeyHashTlv(keyBytes []byte) ImageTlv {
	data := RawKeyHash(keyBytes)
	return ImageTlv{
		Header: ImageTlvHdr{
			Type: IMAGE_TLV_KEYHASH,
			Pad:  0,
			Len:  uint16(len(data)),
		},
		Data: data,
	}
}

func BuildSigTlvs(keys []ImageSigKey, hash []byte) ([]ImageTlv, error) {
	var tlvs []ImageTlv

	for _, key := range keys {
		key.assertValid()

		// Key hash TLV.
		pubKey, err := key.PubBytes()
		if err != nil {
			return nil, err
		}
		tlv := BuildKeyHashTlv(pubKey)
		tlvs = append(tlvs, tlv)

		// Signature TLV.
		sig, err := generateSig(key, hash)
		if err != nil {
			return nil, err
		}
		tlv = ImageTlv{
			Header: ImageTlvHdr{
				Type: key.sigTlvType(),
				Len:  uint16(len(sig)),
			},
			Data: sig,
		}
		tlvs = append(tlvs, tlv)
	}

	return tlvs, nil
}

func GenerateImage(opts ImageCreateOpts) (Image, error) {
	ic := NewImageCreator()

	srcBin, err := ioutil.ReadFile(opts.SrcBinFilename)
	if err != nil {
		return Image{}, util.FmtNewtError(
			"Can't read app binary: %s", err.Error())
	}

	ic.Body = srcBin
	ic.Version = opts.Version
	ic.SigKeys = opts.SigKeys

	if opts.LoaderHash != nil {
		ic.InitialHash = opts.LoaderHash
		ic.Bootable = false
	} else {
		ic.Bootable = true
	}

	if opts.SrcEncKeyFilename != "" {
		plainSecret := make([]byte, 16)
		if _, err := rand.Read(plainSecret); err != nil {
			return Image{}, util.FmtNewtError(
				"Random generation error: %s\n", err)
		}

		cipherSecret, err := ReadEncKey(opts.SrcEncKeyFilename, plainSecret)
		if err != nil {
			return Image{}, err
		}

		ic.PlainSecret = plainSecret
		ic.CipherSecret = cipherSecret
	}

	ri, err := ic.Create()
	if err != nil {
		return Image{}, err
	}

	return ri, nil
}

func calcHash(initialHash []byte, hdr ImageHdr,
	plainBody []byte) ([]byte, error) {

	hash := sha256.New()

	add := func(itf interface{}) error {
		b := &bytes.Buffer{}
		if err := binary.Write(b, binary.LittleEndian, itf); err != nil {
			return err
		}
		if err := binary.Write(hash, binary.LittleEndian, itf); err != nil {
			return util.FmtNewtError("Failed to hash data: %s", err.Error())
		}

		return nil
	}

	if initialHash != nil {
		if err := add(initialHash); err != nil {
			return nil, err
		}
	}

	if err := add(hdr); err != nil {
		return nil, err
	}

	extra := hdr.HdrSz - IMAGE_HEADER_SIZE
	if extra > 0 {
		b := make([]byte, extra)
		if err := add(b); err != nil {
			return nil, err
		}
	}

	if err := add(plainBody); err != nil {
		return nil, err
	}

	return hash.Sum(nil), nil
}

func (ic *ImageCreator) Create() (Image, error) {
	ri := Image{}

	// First the header
	hdr := ImageHdr{
		Magic: IMAGE_MAGIC,
		Pad1:  0,
		HdrSz: IMAGE_HEADER_SIZE,
		Pad2:  0,
		ImgSz: uint32(len(ic.Body)),
		Flags: 0,
		Vers:  ic.Version,
		Pad3:  0,
	}

	if !ic.Bootable {
		hdr.Flags |= IMAGE_F_NON_BOOTABLE
	}

	if ic.CipherSecret != nil {
		hdr.Flags |= IMAGE_F_ENCRYPTED
	}

	if ic.HeaderSize != 0 {
		// Pad the header out to the given size.  There will
		// just be zeros between the header and the start of
		// the image when it is padded.
		extra := ic.HeaderSize - IMAGE_HEADER_SIZE
		if extra < 0 {
			return ri, util.FmtNewtError("Image header must be at "+
				"least %d bytes", IMAGE_HEADER_SIZE)
		}

		hdr.HdrSz = uint16(ic.HeaderSize)
		for i := 0; i < extra; i++ {
			ri.Body = append(ri.Body, 0)
		}
	}

	ri.Header = hdr

	hashBytes, err := calcHash(ic.InitialHash, hdr, ic.Body)
	if err != nil {
		return ri, err
	}

	var stream cipher.Stream
	if ic.CipherSecret != nil {
		block, err := aes.NewCipher(ic.PlainSecret)
		if err != nil {
			return ri, util.NewNewtError("Failed to create block cipher")
		}
		nonce := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		stream = cipher.NewCTR(block, nonce)
	}

	/*
	 * Followed by data.
	 */
	dataBuf := make([]byte, 16)
	encBuf := make([]byte, 16)
	r := bytes.NewReader(ic.Body)
	w := bytes.Buffer{}
	for {
		cnt, err := r.Read(dataBuf)
		if err != nil && err != io.EOF {
			return ri, util.FmtNewtError(
				"Failed to read from image body: %s", err.Error())
		}
		if cnt == 0 {
			break
		}

		if ic.CipherSecret == nil {
			_, err = w.Write(dataBuf[0:cnt])
		} else {
			stream.XORKeyStream(encBuf, dataBuf[0:cnt])
			_, err = w.Write(encBuf[0:cnt])
		}
		if err != nil {
			return ri, util.FmtNewtError(
				"Failed to write to image body: %s", err.Error())
		}
	}
	ri.Body = append(ri.Body, w.Bytes()...)

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Computed Hash for image as %s\n", hex.EncodeToString(hashBytes))

	// Hash TLV.
	tlv := ImageTlv{
		Header: ImageTlvHdr{
			Type: IMAGE_TLV_SHA256,
			Pad:  0,
			Len:  uint16(len(hashBytes)),
		},
		Data: hashBytes,
	}
	ri.Tlvs = append(ri.Tlvs, tlv)

	tlvs, err := BuildSigTlvs(ic.SigKeys, hashBytes)
	if err != nil {
		return ri, err
	}
	ri.Tlvs = append(ri.Tlvs, tlvs...)

	if ic.CipherSecret != nil {
		tlv, err := generateEncTlv(ic.CipherSecret)
		if err != nil {
			return ri, err
		}
		ri.Tlvs = append(ri.Tlvs, tlv)
	}

	return ri, nil
}
