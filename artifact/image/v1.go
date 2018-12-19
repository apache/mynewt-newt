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

// This file implements parsing and generation of version-1 images.  Much of
// this code duplicates the v2 code.  The expectation is that this file will be
// removed when version 1 is oficially retired (soon).

package image

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"io/ioutil"

	"mynewt.apache.org/newt/util"
)

const IMAGEv1_MAGIC = 0x96f3b83c /* Image header magic */

const (
	IMAGEv1_F_PIC                      = 0x00000001
	IMAGEv1_F_SHA256                   = 0x00000002 /* Image contains hash TLV */
	IMAGEv1_F_PKCS15_RSA2048_SHA256    = 0x00000004 /* PKCS15 w/RSA2048 and SHA256 */
	IMAGEv1_F_ECDSA224_SHA256          = 0x00000008 /* ECDSA224 over SHA256 */
	IMAGEv1_F_NON_BOOTABLE             = 0x00000010 /* non bootable image */
	IMAGEv1_F_ECDSA256_SHA256          = 0x00000020 /* ECDSA256 over SHA256 */
	IMAGEv1_F_PKCS1_PSS_RSA2048_SHA256 = 0x00000040 /* RSA-PSS w/RSA2048 and SHA256 */
)

const (
	IMAGEv1_TLV_SHA256   = 1
	IMAGEv1_TLV_RSA2048  = 2
	IMAGEv1_TLV_ECDSA224 = 3
	IMAGEv1_TLV_ECDSA256 = 4
)

// Set this to enable RSA-PSS for RSA signatures, instead of PKCS#1
// v1.5.  Eventually, this should be the default.
var UseRsaPss = false

type ImageHdrV1 struct {
	Magic uint32
	TlvSz uint16
	KeyId uint8
	Pad1  uint8
	HdrSz uint16
	Pad2  uint16
	ImgSz uint32
	Flags uint32
	Vers  ImageVersion
	Pad3  uint32
}

type ImageV1 struct {
	Header ImageHdrV1
	Body   []byte
	Tlvs   []ImageTlv
}

func (img *ImageV1) FindTlvs(tlvType uint8) []ImageTlv {
	var tlvs []ImageTlv

	for _, tlv := range img.Tlvs {
		if tlv.Header.Type == tlvType {
			tlvs = append(tlvs, tlv)
		}
	}

	return tlvs
}

func (img *ImageV1) Hash() ([]byte, error) {
	tlvs := img.FindTlvs(IMAGEv1_TLV_SHA256)
	if len(tlvs) == 0 {
		return nil, util.FmtNewtError("Image does not contain hash TLV")
	}
	if len(tlvs) > 1 {
		return nil, util.FmtNewtError("Image contains %d hash TLVs", len(tlvs))
	}

	return tlvs[0].Data, nil
}

func (img *ImageV1) WritePlusOffsets(w io.Writer) (ImageOffsets, error) {
	offs := ImageOffsets{}
	offset := 0

	offs.Header = offset

	err := binary.Write(w, binary.LittleEndian, &img.Header)
	if err != nil {
		return offs, util.ChildNewtError(err)
	}
	offset += IMAGE_HEADER_SIZE

	offs.Body = offset
	size, err := w.Write(img.Body)
	if err != nil {
		return offs, util.ChildNewtError(err)
	}
	offset += size

	for _, tlv := range img.Tlvs {
		offs.Tlvs = append(offs.Tlvs, offset)
		size, err := tlv.Write(w)
		if err != nil {
			return offs, util.ChildNewtError(err)
		}
		offset += size
	}

	offs.TotalSize = offset

	return offs, nil
}

func (img *ImageV1) Offsets() (ImageOffsets, error) {
	return img.WritePlusOffsets(ioutil.Discard)
}

func (img *ImageV1) TotalSize() (int, error) {
	offs, err := img.Offsets()
	if err != nil {
		return 0, err
	}
	return offs.TotalSize, nil
}

func (img *ImageV1) Write(w io.Writer) (int, error) {
	offs, err := img.WritePlusOffsets(w)
	if err != nil {
		return 0, err
	}

	return offs.TotalSize, nil
}

func (key *ImageSigKey) sigHdrTypeV1() (uint32, error) {
	key.assertValid()

	if key.Rsa != nil {
		if UseRsaPss {
			return IMAGEv1_F_PKCS1_PSS_RSA2048_SHA256, nil
		} else {
			return IMAGEv1_F_PKCS15_RSA2048_SHA256, nil
		}
	} else {
		switch key.Ec.Curve.Params().Name {
		case "P-224":
			return IMAGEv1_F_ECDSA224_SHA256, nil
		case "P-256":
			return IMAGEv1_F_ECDSA256_SHA256, nil
		default:
			return 0, util.FmtNewtError("Unsupported ECC curve")
		}
	}
}

func (key *ImageSigKey) sigTlvTypeV1() uint8 {
	key.assertValid()

	if key.Rsa != nil {
		return IMAGEv1_TLV_RSA2048
	} else {
		switch key.Ec.Curve.Params().Name {
		case "P-224":
			return IMAGEv1_TLV_ECDSA224
		case "P-256":
			return IMAGEv1_TLV_ECDSA256
		default:
			return 0
		}
	}
}

func generateV1SigRsa(key *rsa.PrivateKey, hash []byte) ([]byte, error) {
	var signature []byte
	var err error

	if UseRsaPss {
		opts := rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthEqualsHash,
		}
		signature, err = rsa.SignPSS(
			rand.Reader, key, crypto.SHA256, hash, &opts)
	} else {
		signature, err = rsa.SignPKCS1v15(
			rand.Reader, key, crypto.SHA256, hash)
	}
	if err != nil {
		return nil, util.FmtNewtError("Failed to compute signature: %s", err)
	}

	return signature, nil
}

func generateV1SigTlvRsa(key ImageSigKey, hash []byte) (ImageTlv, error) {
	sig, err := generateV1SigRsa(key.Rsa, hash)
	if err != nil {
		return ImageTlv{}, err
	}

	return ImageTlv{
		Header: ImageTlvHdr{
			Type: key.sigTlvTypeV1(),
			Pad:  0,
			Len:  256, /* 2048 bits */
		},
		Data: sig,
	}, nil
}

func generateV1SigTlvEc(key ImageSigKey, hash []byte) (ImageTlv, error) {
	sig, err := GenerateSigEc(key, hash)
	if err != nil {
		return ImageTlv{}, err
	}

	sigLen := key.sigLen()
	if len(sig) > int(sigLen) {
		return ImageTlv{}, util.FmtNewtError("Something is really wrong\n")
	}

	b := &bytes.Buffer{}

	if _, err := b.Write(sig); err != nil {
		return ImageTlv{},
			util.FmtNewtError("Failed to append sig: %s", err.Error())
	}

	pad := make([]byte, int(sigLen)-len(sig))
	if _, err := b.Write(pad); err != nil {
		return ImageTlv{}, util.FmtNewtError(
			"Failed to serialize image trailer: %s", err.Error())
	}

	return ImageTlv{
		Header: ImageTlvHdr{
			Type: key.sigTlvTypeV1(),
			Pad:  0,
			Len:  sigLen + uint16(len(pad)),
		},
		Data: b.Bytes(),
	}, nil
}

func generateV1SigTlv(key ImageSigKey, hash []byte) (ImageTlv, error) {
	key.assertValid()

	if key.Rsa != nil {
		return generateV1SigTlvRsa(key, hash)
	} else {
		return generateV1SigTlvEc(key, hash)
	}
}

func calcHashV1(initialHash []byte, hdr ImageHdrV1,
	plainBody []byte) ([]byte, error) {

	hash := sha256.New()

	add := func(itf interface{}) error {
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

func (ic *ImageCreator) CreateV1() (ImageV1, error) {
	ri := ImageV1{}

	if len(ic.SigKeys) > 1 {
		return ri, util.FmtNewtError(
			"V1 image format only allows one key, %d keys specified",
			len(ic.SigKeys))
	}

	// First the header
	hdr := ImageHdrV1{
		Magic: IMAGEv1_MAGIC,
		TlvSz: 0, // Filled in later.
		KeyId: 0,
		Pad1:  0,
		HdrSz: IMAGE_HEADER_SIZE,
		Pad2:  0,
		ImgSz: uint32(len(ic.Body)),
		Flags: IMAGEv1_F_SHA256,
		Vers:  ic.Version,
		Pad3:  0,
	}

	if !ic.Bootable {
		hdr.Flags |= IMAGEv1_F_NON_BOOTABLE
	}

	if ic.HeaderSize != 0 {
		/*
		 * Pad the header out to the given size.  There will
		 * just be zeros between the header and the start of
		 * the image when it is padded.
		 */
		if ic.HeaderSize < IMAGE_HEADER_SIZE {
			return ri, util.FmtNewtError("Image header must be at "+
				"least %d bytes", IMAGE_HEADER_SIZE)
		}

		hdr.HdrSz = uint16(ic.HeaderSize)
	}

	if len(ic.SigKeys) > 0 {
		keyFlag, err := ic.SigKeys[0].sigHdrTypeV1()
		if err != nil {
			return ri, err
		}
		hdr.Flags |= keyFlag
		hdr.TlvSz = 4 + ic.SigKeys[0].sigLen()
	}
	hdr.TlvSz += 4 + 32

	if hdr.HdrSz > IMAGE_HEADER_SIZE {
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

	hashBytes, err := calcHashV1(ic.InitialHash, hdr, ic.Body)
	if err != nil {
		return ri, err
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Computed Hash for image as %s\n", hex.EncodeToString(hashBytes))

	/*
	 * Followed by data.
	 */
	dataBuf := make([]byte, 1024)
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

		if _, err = w.Write(dataBuf[0:cnt]); err != nil {
			return ri, util.FmtNewtError(
				"Failed to write to image body: %s", err.Error())
		}
	}
	ri.Body = w.Bytes()

	// Hash TLV.
	tlv := ImageTlv{
		Header: ImageTlvHdr{
			Type: IMAGEv1_TLV_SHA256,
			Pad:  0,
			Len:  uint16(len(hashBytes)),
		},
		Data: hashBytes,
	}
	ri.Tlvs = append(ri.Tlvs, tlv)

	if len(ic.SigKeys) > 0 {
		tlv, err := generateV1SigTlv(ic.SigKeys[0], hashBytes)
		if err != nil {
			return ri, err
		}
		ri.Tlvs = append(ri.Tlvs, tlv)
	}

	offs, err := ri.Offsets()
	if err != nil {
		return ri, err
	}
	hdr.TlvSz = uint16(offs.TotalSize - offs.Tlvs[0])

	ri.Header = hdr

	return ri, nil
}

func GenerateV1Image(opts ImageCreateOpts) (ImageV1, error) {
	ic := NewImageCreator()

	srcBin, err := ioutil.ReadFile(opts.SrcBinFilename)
	if err != nil {
		return ImageV1{}, util.FmtNewtError(
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
		plainSecret, err := GeneratePlainSecret()
		if err != nil {
			return ImageV1{}, err
		}

		pubKeBytes, err := ioutil.ReadFile(opts.SrcEncKeyFilename)
		if err != nil {
			return ImageV1{}, util.FmtNewtError(
				"Error reading pubkey file: %s", err.Error())
		}
		cipherSecret, err := GenerateCipherSecret(pubKeBytes, plainSecret)
		if err != nil {
			return ImageV1{}, err
		}

		ic.PlainSecret = plainSecret
		ic.CipherSecret = cipherSecret
	}

	ri, err := ic.CreateV1()
	if err != nil {
		return ImageV1{}, err
	}

	return ri, nil
}
