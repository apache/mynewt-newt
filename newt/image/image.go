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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

type ImageVersion struct {
	Major    uint8
	Minor    uint8
	Rev      uint16
	BuildNum uint32
}

type Image struct {
	builder *builder.Builder

	sourceBin    string
	targetImg    string
	manifestFile string
	version      ImageVersion
	signingKey   *rsa.PrivateKey
	hash         []byte
}

type ImageHdr struct {
	Magic uint32
	TlvSz uint32
	HdrSz uint32
	ImgSz uint32
	Flags uint32
	Vers  ImageVersion
	Pad   uint32
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
)

/*
 * Image trailer TLV types.
 */
const (
	IMAGE_TLV_SHA256  = 1
	IMAGE_TLV_RSA2048 = 2
)

/*
 * Data that's going to go to build manifest file
 */
type ImageManifest struct {
	Date    string              `json:"build_time"`
	Version string              `json:"build_version"`
	Hash    string              `json:"id"`
	Image   string              `json:"image"`
	Pkgs    []*ImageManifestPkg `json:"pkgs"`
	TgtVars []string            `json:"target"`
}

type ImageManifestPkg struct {
	Name string `json:"name"`
}

func NewImage(b *builder.Builder) (*Image, error) {
	image := &Image{
		builder: b,
	}

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

func (image *Image) SetSigningKey(fileName string) error {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Error reading key file: %s", err))
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return util.NewNewtError(fmt.Sprintf("No RSA private key in file"))
	}

	/*
	 * ParsePKCS1PrivateKey returns an RSA private key from its ASN.1 PKCS#1 DER
	 * encoded form.
	 */
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Private key parsing failed: %s",
			err))
	}
	image.signingKey = privateKey

	return nil
}

func (image *Image) Generate() error {
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

	/*
	 * First the header
	 */
	hdr := &ImageHdr{
		Magic: IMAGE_MAGIC,
		TlvSz: 0,
		HdrSz: IMAGE_HEADER_SIZE,
		ImgSz: uint32(binInfo.Size()),
		Flags: 0,
		Vers:  image.version,
		Pad:   0,
	}
	if image.signingKey != nil {
		hdr.TlvSz = 4 + 256
		hdr.Flags = IMAGE_F_PKCS15_RSA2048_SHA256
	} else {
		hdr.TlvSz = 4 + 32
		hdr.Flags = IMAGE_F_SHA256
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

	if image.signingKey == nil {
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
	} else {
		/*
		 * If signing key was set, generate TLV for that.
		 */
		tlv := &ImageTrailerTlv{
			Type: IMAGE_TLV_RSA2048,
			Pad:  0,
			Len:  256, /* 2048 bits */
		}
		signature, err := rsa.SignPKCS1v15(rand.Reader, image.signingKey,
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
	return nil
}

func (image *Image) CreateManifest(t *target.Target) error {
	versionStr := fmt.Sprintf("%d.%d.%d.%d",
		image.version.Major, image.version.Minor,
		image.version.Rev, image.version.BuildNum)
	hashStr := fmt.Sprintf("%x", image.hash)
	timeStr := time.Now().Format(time.RFC3339)

	manifest := &ImageManifest{
		Version: versionStr,
		Hash:    hashStr,
		Image:   filepath.Base(image.targetImg),
		Date:    timeStr,
	}

	for _, builtPkg := range image.builder.Packages {
		imgPkg := &ImageManifestPkg{
			Name: builtPkg.Name(),
		}
		manifest.Pkgs = append(manifest.Pkgs, imgPkg)
	}

	vars := t.Vars
	var keys []string
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		manifest.TgtVars = append(manifest.TgtVars, k+"="+vars[k])
	}
	file, err := os.Create(image.manifestFile)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Cannot create manifest file %s: %s",
			image.manifestFile, err.Error()))
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
