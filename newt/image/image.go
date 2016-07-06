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
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/util"
)

type ImageVersion struct {
	Major    uint8
	Minor    uint8
	Rev      uint16
	BuildNum uint32
}

type Image struct {
	sourceBin    string
	targetImg    string
	manifestFile string
	version      ImageVersion
	signingRSA   *rsa.PrivateKey
	signingEC    *ecdsa.PrivateKey
	keyId        uint8
	hash         []byte
}

type ImageHdr struct {
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

type ImageTrailerTlv struct {
	Type uint8
	Pad  uint8
	Len  uint16
}

const (
	IMAGE_MAGIC = 0x96f3b83c /* Image header magic */
)

const (
	IMAGE_HEADER_SIZE = 32
)

/*
 * Image header flags.
 */
const (
	IMAGE_F_PIC                   = 0x00000001
	IMAGE_F_SHA256                = 0x00000002 /* Image contains hash TLV */
	IMAGE_F_PKCS15_RSA2048_SHA256 = 0x00000004 /* PKCS15 w/RSA2048 and SHA256 */
	IMAGE_F_ECDSA224_SHA256       = 0x00000008 /* ECDSA224 over SHA256 */
	IMAGE_F_NON_BOOTABLE          = 0x00000010 /* non bootable image */
)

/*
 * Image trailer TLV types.
 */
const (
	IMAGE_TLV_SHA256   = 1
	IMAGE_TLV_RSA2048  = 2
	IMAGE_TLV_ECDSA224 = 3
)

/*
 * Data that's going to go to build manifest file
 */
type ImageManifest struct {
	Date       string              `json:"build_time"`
	Version    string              `json:"build_version"`
	BuildID    string              `json:"id"`
	Image      string              `json:"image"`
	ImageHash  string              `json:"image_hash"`
	Loader     string              `json:"loader"`
	LoaderHash string              `json:"loader_hash"`
	Pkgs       []*ImageManifestPkg `json:"pkgs"`
	LoaderPkgs []*ImageManifestPkg `json:"loader_pkgs"`
	TgtVars    []string            `json:"target"`
}

type ImageManifestPkg struct {
	Name string `json:"name"`
}

type ECDSASig struct {
	R *big.Int
	S *big.Int
}

func NewImage(b *builder.Builder) (*Image, error) {
	image := &Image{}

	image.sourceBin = b.AppElfPath() + ".bin"
	image.targetImg = b.AppImgPath()
	image.manifestFile = b.AppPath() + "manifest.json"
	return image, nil
}

func (image *Image) TargetImg() string {
	return image.targetImg
}

func (image *Image) ManifestFile() string {
	return image.manifestFile
}

func (image *Image) SetVersion(versStr string) error {
	var err error
	var major uint64
	var minor uint64
	var rev uint64
	var buildNum uint64

	components := strings.Split(versStr, ".")
	major, err = strconv.ParseUint(components[0], 10, 8)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Invalid version string %s",
			versStr))
	}
	if len(components) > 1 {
		minor, err = strconv.ParseUint(components[1], 10, 8)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Invalid version string %s",
				versStr))
		}
	}
	if len(components) > 2 {
		rev, err = strconv.ParseUint(components[2], 10, 16)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Invalid version string %s",
				versStr))
		}
	}
	if len(components) > 3 {
		buildNum, err = strconv.ParseUint(components[3], 10, 32)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Invalid version string %s",
				versStr))
		}
	}
	image.version.Major = uint8(major)
	image.version.Minor = uint8(minor)
	image.version.Rev = uint16(rev)
	image.version.BuildNum = uint32(buildNum)
	log.Debugf("Assigning version number %d.%d.%d.%d\n",
		image.version.Major, image.version.Minor,
		image.version.Rev, image.version.BuildNum)

	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.LittleEndian, image.version)
	if err != nil {
		fmt.Printf("Bombing out\n")
		return nil
	}

	return nil
}

func (image *Image) SetSigningKey(fileName string, keyId uint8) error {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Error reading key file: %s", err))
	}

	block, _ := pem.Decode(data)
	if block != nil && block.Type == "RSA PRIVATE KEY" {
		/*
		 * ParsePKCS1PrivateKey returns an RSA private key from its ASN.1
		 * PKCS#1 DER encoded form.
		 */
		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Private key parsing "+
				"failed: %s", err))
		}
		image.signingRSA = privateKey
	}
	if block != nil && block.Type == "EC PRIVATE KEY" {
		/*
		 * ParseECPrivateKey returns a EC private key
		 */
		privateKey, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Private key parsing "+
				"failed: %s", err))
		}
		image.signingEC = privateKey
	}
	if image.signingEC == nil && image.signingRSA == nil {
		return util.NewNewtError("Unknown private key format, EC/RSA private " +
			"key in PEM format only.")
	}
	image.keyId = keyId

	return nil
}

func (image *Image) Generate(loader *Image) error {
	binFile, err := os.Open(image.sourceBin)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Can't open app binary: %s",
			err.Error()))
	}
	defer binFile.Close()

	binInfo, err := binFile.Stat()
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Can't stat app binary %s: %s",
			image.sourceBin, err.Error()))
	}

	imgFile, err := os.OpenFile(image.targetImg,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Can't open target image %s: %s",
			image.targetImg, err.Error()))
	}
	defer imgFile.Close()

	/*
	 * Compute hash while updating the file.
	 */
	hash := sha256.New()

	if loader != nil {
		err = binary.Write(hash, binary.LittleEndian, loader.hash)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to seed hash: %s",
				err.Error()))
		}
	}

	/*
	 * First the header
	 */
	hdr := &ImageHdr{
		Magic: IMAGE_MAGIC,
		TlvSz: 0,
		KeyId: 0,
		Pad1:  0,
		HdrSz: IMAGE_HEADER_SIZE,
		Pad2:  0,
		ImgSz: uint32(binInfo.Size()),
		Flags: 0,
		Vers:  image.version,
		Pad3:  0,
	}

	if image.signingRSA != nil {
		hdr.TlvSz = 4 + 256
		hdr.Flags = IMAGE_F_PKCS15_RSA2048_SHA256
		hdr.KeyId = image.keyId
	} else if image.signingEC != nil {
		hdr.TlvSz = 4 + 68
		hdr.Flags = IMAGE_F_ECDSA224_SHA256
	} else {
		hdr.TlvSz = 4 + 32
		hdr.Flags = IMAGE_F_SHA256
	}

	hdr.TlvSz += 4 + 32
	hdr.Flags |= IMAGE_F_SHA256

	if loader != nil {
		hdr.Flags |= IMAGE_F_NON_BOOTABLE
	}

	err = binary.Write(imgFile, binary.LittleEndian, hdr)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Failed to serialize image hdr: %s",
			err.Error()))
	}
	err = binary.Write(hash, binary.LittleEndian, hdr)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Failed to hash data: %s",
			err.Error()))
	}

	/*
	 * Followed by data.
	 */
	dataBuf := make([]byte, 1024)
	for {
		cnt, err := binFile.Read(dataBuf)
		if err != nil && err != io.EOF {
			return util.NewNewtError(fmt.Sprintf("Failed to read from %s: %s",
				image.sourceBin, err.Error()))
		}
		if cnt == 0 {
			break
		}
		_, err = imgFile.Write(dataBuf[0:cnt])
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to write to %s: %s",
				image.targetImg, err.Error()))
		}
		_, err = hash.Write(dataBuf[0:cnt])
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to hash data: %s",
				err.Error()))
		}
	}

	image.hash = hash.Sum(nil)

	/*
	 * Trailer with hash of the data
	 */
	tlv := &ImageTrailerTlv{
		Type: IMAGE_TLV_SHA256,
		Pad:  0,
		Len:  uint16(len(image.hash)),
	}
	err = binary.Write(imgFile, binary.LittleEndian, tlv)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Failed to serialize image "+
			"trailer: %s", err.Error()))
	}
	_, err = imgFile.Write(image.hash)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Failed to append hash: %s",
			err.Error()))
	}

	if image.signingRSA != nil {
		/*
		 * If signing key was set, generate TLV for that.
		 */
		tlv := &ImageTrailerTlv{
			Type: IMAGE_TLV_RSA2048,
			Pad:  0,
			Len:  256, /* 2048 bits */
		}
		signature, err := rsa.SignPKCS1v15(rand.Reader, image.signingRSA,
			crypto.SHA256, image.hash)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf(
				"Failed to compute signature: %s", err))
		}

		err = binary.Write(imgFile, binary.LittleEndian, tlv)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to serialize image "+
				"trailer: %s", err.Error()))
		}
		_, err = imgFile.Write(signature)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to append sig: %s",
				err.Error()))
		}
	}
	if image.signingEC != nil {
		r, s, err := ecdsa.Sign(rand.Reader, image.signingEC, image.hash)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf(
				"Failed to compute signature: %s", err))
		}

		var ECDSA ECDSASig
		ECDSA.R = r
		ECDSA.S = s
		signature, err := asn1.Marshal(ECDSA)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf(
				"Failed to construct signature: %s", err))
		}
		if len(signature) > 68 {
			return util.NewNewtError(fmt.Sprintf(
				"Something is really wrong\n"))
		}
		tlv := &ImageTrailerTlv{
			Type: IMAGE_TLV_ECDSA224,
			Pad:  0,
			Len:  68,
		}
		err = binary.Write(imgFile, binary.LittleEndian, tlv)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to serialize image "+
				"trailer: %s", err.Error()))
		}
		_, err = imgFile.Write(signature)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to append sig: %s",
				err.Error()))
		}
		pad := make([]byte, 68-len(signature))
		_, err = imgFile.Write(pad)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to serialize image "+
				"trailer: %s", err.Error()))
		}
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Computed Hash for image %s as %s \n", image.TargetImg(), hex.EncodeToString(image.hash[:]))
	return nil
}

func CreateBuildId(app *Image, loader *Image) []byte {
	return app.hash
}

func CreateManifest(t *builder.TargetBuilder, app *Image, loader *Image, build_id []byte) error {
	versionStr := fmt.Sprintf("%d.%d.%d.%d",
		app.version.Major, app.version.Minor,
		app.version.Rev, app.version.BuildNum)
	hashStr := fmt.Sprintf("%x", app.hash)
	timeStr := time.Now().Format(time.RFC3339)

	manifest := &ImageManifest{
		Version:   versionStr,
		ImageHash: hashStr,
		Image:     filepath.Base(app.targetImg),
		Date:      timeStr,
	}

	for _, builtPkg := range t.App.Packages {
		imgPkg := &ImageManifestPkg{
			Name: builtPkg.Name(),
		}
		manifest.Pkgs = append(manifest.Pkgs, imgPkg)
	}

	if loader != nil {
		manifest.Loader = filepath.Base(loader.targetImg)
		manifest.LoaderHash = fmt.Sprintf("%x", loader.hash)

		for _, builtPkg := range t.Loader.Packages {
			imgPkg := &ImageManifestPkg{
				Name: builtPkg.Name(),
			}
			manifest.LoaderPkgs = append(manifest.LoaderPkgs, imgPkg)
		}
	}

	manifest.BuildID = fmt.Sprintf("%x", build_id)

	vars := t.GetTarget().Vars
	var keys []string
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		manifest.TgtVars = append(manifest.TgtVars, k+"="+vars[k])
	}
	file, err := os.Create(app.manifestFile)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Cannot create manifest file %s: %s",
			app.manifestFile, err.Error()))
	}
	defer file.Close()

	buffer, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Cannot encode manifest: %s",
			err.Error()))
	}
	_, err = file.Write(buffer)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Cannot write manifest file: %s",
			err.Error()))
	}

	return nil
}
