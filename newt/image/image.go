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
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/util"
)

type ImageVersion struct {
	Major    uint8
	Minor    uint8
	Rev      uint16
	BuildNum uint32
}

type Image struct {
	SourceBin  string
	TargetImg  string
	Version    ImageVersion
	SigningRSA *rsa.PrivateKey
	SigningEC  *ecdsa.PrivateKey
	KeyId      uint8
	Hash       []byte
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
	Repos      []ImageManifestRepo `json:"repos"`
}

type ImageManifestPkg struct {
	Name string `json:"name"`
	Repo string `json:"repo"`
}

type ImageManifestRepo struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty,omitempty"`
	URL    string `json:"url,omitempty"`
}

type RepoManager struct {
	repos map[string]ImageManifestRepo
}

type ECDSASig struct {
	R *big.Int
	S *big.Int
}

func NewImage(srcBinPath string, dstImgPath string) (*Image, error) {
	image := &Image{}

	image.SourceBin = srcBinPath
	image.TargetImg = dstImgPath
	return image, nil
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
	image.Version.Major = uint8(major)
	image.Version.Minor = uint8(minor)
	image.Version.Rev = uint16(rev)
	image.Version.BuildNum = uint32(buildNum)
	log.Debugf("Assigning version number %d.%d.%d.%d\n",
		image.Version.Major, image.Version.Minor,
		image.Version.Rev, image.Version.BuildNum)

	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.LittleEndian, image.Version)
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
		image.SigningRSA = privateKey
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
		image.SigningEC = privateKey
	}
	if image.SigningEC == nil && image.SigningRSA == nil {
		return util.NewNewtError("Unknown private key format, EC/RSA private " +
			"key in PEM format only.")
	}
	image.KeyId = keyId

	return nil
}

func (image *Image) Generate(loader *Image) error {
	binFile, err := os.Open(image.SourceBin)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Can't open app binary: %s",
			err.Error()))
	}
	defer binFile.Close()

	binInfo, err := binFile.Stat()
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Can't stat app binary %s: %s",
			image.SourceBin, err.Error()))
	}

	imgFile, err := os.OpenFile(image.TargetImg,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Can't open target image %s: %s",
			image.TargetImg, err.Error()))
	}
	defer imgFile.Close()

	/*
	 * Compute hash while updating the file.
	 */
	hash := sha256.New()

	if loader != nil {
		err = binary.Write(hash, binary.LittleEndian, loader.Hash)
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
		Vers:  image.Version,
		Pad3:  0,
	}

	if image.SigningRSA != nil {
		hdr.TlvSz = 4 + 256
		hdr.Flags = IMAGE_F_PKCS15_RSA2048_SHA256
		hdr.KeyId = image.KeyId
	} else if image.SigningEC != nil {
		hdr.TlvSz = 4 + 68
		hdr.Flags = IMAGE_F_ECDSA224_SHA256
		hdr.KeyId = image.KeyId
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
				image.SourceBin, err.Error()))
		}
		if cnt == 0 {
			break
		}
		_, err = imgFile.Write(dataBuf[0:cnt])
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to write to %s: %s",
				image.TargetImg, err.Error()))
		}
		_, err = hash.Write(dataBuf[0:cnt])
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to hash data: %s",
				err.Error()))
		}
	}

	image.Hash = hash.Sum(nil)

	/*
	 * Trailer with hash of the data
	 */
	tlv := &ImageTrailerTlv{
		Type: IMAGE_TLV_SHA256,
		Pad:  0,
		Len:  uint16(len(image.Hash)),
	}
	err = binary.Write(imgFile, binary.LittleEndian, tlv)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Failed to serialize image "+
			"trailer: %s", err.Error()))
	}
	_, err = imgFile.Write(image.Hash)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Failed to append hash: %s",
			err.Error()))
	}

	if image.SigningRSA != nil {
		/*
		 * If signing key was set, generate TLV for that.
		 */
		tlv := &ImageTrailerTlv{
			Type: IMAGE_TLV_RSA2048,
			Pad:  0,
			Len:  256, /* 2048 bits */
		}
		signature, err := rsa.SignPKCS1v15(rand.Reader, image.SigningRSA,
			crypto.SHA256, image.Hash)
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
	if image.SigningEC != nil {
		r, s, err := ecdsa.Sign(rand.Reader, image.SigningEC, image.Hash)
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
		"Computed Hash for image %s as %s \n",
		image.TargetImg, hex.EncodeToString(image.Hash))
	return nil
}

func CreateBuildId(app *Image, loader *Image) []byte {
	return app.Hash
}

func NewRepoManager() *RepoManager {
	return &RepoManager{
		repos: make(map[string]ImageManifestRepo),
	}
}

func (r *RepoManager) GetImageManifestPkg(
	lpkg *pkg.LocalPackage) *ImageManifestPkg {

	ip := &ImageManifestPkg{
		Name: lpkg.Name(),
	}

	var path string
	if lpkg.Repo().IsLocal() {
		ip.Repo = lpkg.Repo().Name()
		path = lpkg.BasePath()
	} else {
		ip.Repo = lpkg.Repo().Name()
		path = lpkg.BasePath()
	}

	if _, present := r.repos[ip.Repo]; present {
		return ip
	}

	repo := ImageManifestRepo{
		Name: ip.Repo,
	}

	res, err := util.ShellCommand(fmt.Sprintf("cd %s && git rev-parse HEAD",
		path))
	if err != nil {
		log.Debugf("Unable to determine commit hash for %s: %v", path, err)
		repo.Commit = "UNKNOWN"
	} else {
		repo.Commit = strings.TrimSpace(string(res))
		res, err := util.ShellCommand(fmt.Sprintf(
			"cd %s && git status --porcelain", path))
		if err != nil {
			log.Debugf("Unable to determine dirty state for %s: %v", path, err)
		} else {
			if len(res) > 0 {
				repo.Dirty = true
			}
		}
		res, err = util.ShellCommand(fmt.Sprintf(
			"cd %s && git config --get remote.origin.url", path))
		if err != nil {
			log.Debugf("Unable to determine URL for %s: %v", path, err)
		} else {
			repo.URL = strings.TrimSpace(string(res))
		}
	}
	r.repos[ip.Repo] = repo

	return ip
}

func (r *RepoManager) AllRepos() []ImageManifestRepo {
	keys := make([]string, 0, len(r.repos))
	for k := range r.repos {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	repos := make([]ImageManifestRepo, 0, len(keys))
	for _, key := range keys {
		repos = append(repos, r.repos[key])
	}

	return repos
}
