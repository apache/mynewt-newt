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

// Set this to enable RSA-PSS for RSA signatures, instead of PKCS#1
// v1.5.  Eventually, this should be the default.
var UseRsaPss = false

// Use old image format
var UseV1 = false

type ImageVersion struct {
	Major    uint8
	Minor    uint8
	Rev      uint16
	BuildNum uint32
}

type Image struct {
	SourceBin  string
	SourceImg  string
	TargetImg  string
	Version    ImageVersion
	SigningRSA *rsa.PrivateKey
	SigningEC  *ecdsa.PrivateKey
	KeyId      uint8
	Hash       []byte
	SrcSkip    uint // Number of bytes to skip from the source image.
	HeaderSize uint // If non-zero pad out the header to this size.
	TotalSize  uint // Total size, in bytes, of the generated .img file.
}

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

type ImageHdr struct {
	Magic uint32
	Pad1  uint32
	HdrSz uint16
	Pad2  uint16
	ImgSz uint32
	Flags uint32
	Vers  ImageVersion
	Pad3  uint32
}

type ImageTlvInfo struct {
	Magic     uint16
	TlvTotLen uint16
}

type ImageTrailerTlv struct {
	Type uint8
	Pad  uint8
	Len  uint16
}

const (
	IMAGEv1_MAGIC       = 0x96f3b83c /* Image header magic */
	IMAGE_MAGIC         = 0x96f3b83d /* Image header magic */
	IMAGE_TRAILER_MAGIC = 0x6907     /* Image tlv info magic */
)

const (
	IMAGE_HEADER_SIZE = 32
)

/*
 * Image header flags.
 */
const (
	IMAGEv1_F_PIC                      = 0x00000001
	IMAGEv1_F_SHA256                   = 0x00000002 /* Image contains hash TLV */
	IMAGEv1_F_PKCS15_RSA2048_SHA256    = 0x00000004 /* PKCS15 w/RSA2048 and SHA256 */
	IMAGEv1_F_ECDSA224_SHA256          = 0x00000008 /* ECDSA224 over SHA256 */
	IMAGEv1_F_NON_BOOTABLE             = 0x00000010 /* non bootable image */
	IMAGEv1_F_ECDSA256_SHA256          = 0x00000020 /* ECDSA256 over SHA256 */
	IMAGEv1_F_PKCS1_PSS_RSA2048_SHA256 = 0x00000040 /* RSA-PSS w/RSA2048 and SHA256 */

	IMAGE_F_PIC          = 0x00000001
	IMAGE_F_NON_BOOTABLE = 0x00000002 /* non bootable image */
)

/*
 * Image trailer TLV types.
 */
const (
	IMAGEv1_TLV_SHA256   = 1
	IMAGEv1_TLV_RSA2048  = 2
	IMAGEv1_TLV_ECDSA224 = 3
	IMAGEv1_TLV_ECDSA256 = 4

	IMAGE_TLV_KEYHASH  = 0x01
	IMAGE_TLV_SHA256   = 0x10
	IMAGE_TLV_RSA2048  = 0x20
	IMAGE_TLV_ECDSA224 = 0x21
	IMAGE_TLV_ECDSA256 = 0x22
)

/*
 * Data that's going to go to build manifest file
 */
type ImageManifestSizeArea struct {
	Name string `json:"name"`
	Size uint32 `json:"size"`
}

type ImageManifestSizeSym struct {
	Name  string                   `json:"name"`
	Areas []*ImageManifestSizeArea `json:"areas"`
}

type ImageManifestSizeFile struct {
	Name string                  `json:"name"`
	Syms []*ImageManifestSizeSym `json:"sym"`
}

type ImageManifestSizePkg struct {
	Name  string                   `json:"name"`
	Files []*ImageManifestSizeFile `json:"files"`
}

type ImageManifestSizeCollector struct {
	Pkgs []*ImageManifestSizePkg
}

type ImageManifest struct {
	Name       string              `json:"name"`
	Date       string              `json:"build_time"`
	Version    string              `json:"build_version"`
	BuildID    string              `json:"id"`
	Image      string              `json:"image"`
	ImageHash  string              `json:"image_hash"`
	Loader     string              `json:"loader"`
	LoaderHash string              `json:"loader_hash"`
	Pkgs       []*ImageManifestPkg `json:"pkgs"`
	LoaderPkgs []*ImageManifestPkg `json:"loader_pkgs,omitempty"`
	TgtVars    []string            `json:"target"`
	Repos      []ImageManifestRepo `json:"repos"`

	PkgSizes       []*ImageManifestSizePkg `json:"pkgsz"`
	LoaderPkgSizes []*ImageManifestSizePkg `json:"loader_pkgsz,omitempty"`
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

func ParseVersion(versStr string) (ImageVersion, error) {
	var err error
	var major uint64
	var minor uint64
	var rev uint64
	var buildNum uint64
	var ver ImageVersion

	components := strings.Split(versStr, ".")
	major, err = strconv.ParseUint(components[0], 10, 8)
	if err != nil {
		return ver, util.FmtNewtError("Invalid version string %s", versStr)
	}
	if len(components) > 1 {
		minor, err = strconv.ParseUint(components[1], 10, 8)
		if err != nil {
			return ver, util.FmtNewtError("Invalid version string %s", versStr)
		}
	}
	if len(components) > 2 {
		rev, err = strconv.ParseUint(components[2], 10, 16)
		if err != nil {
			return ver, util.FmtNewtError("Invalid version string %s", versStr)
		}
	}
	if len(components) > 3 {
		buildNum, err = strconv.ParseUint(components[3], 10, 32)
		if err != nil {
			return ver, util.FmtNewtError("Invalid version string %s", versStr)
		}
	}

	ver.Major = uint8(major)
	ver.Minor = uint8(minor)
	ver.Rev = uint16(rev)
	ver.BuildNum = uint32(buildNum)
	return ver, nil
}

func (ver ImageVersion) String() string {
	return fmt.Sprintf("%d.%d.%d.%d",
		ver.Major, ver.Minor, ver.Rev, ver.BuildNum)
}

func NewImage(srcBinPath string, dstImgPath string) (*Image, error) {
	image := &Image{}

	image.SourceBin = srcBinPath
	image.TargetImg = dstImgPath
	return image, nil
}

func OldImage(imgPath string) (*Image, error) {
	image := &Image{}

	image.SourceImg = imgPath
	return image, nil
}

func (image *Image) SetVersion(versStr string) error {
	ver, err := ParseVersion(versStr)
	if err != nil {
		return err
	}

	log.Debugf("Assigning version number %d.%d.%d.%d\n",
		ver.Major, ver.Minor, ver.Rev, ver.BuildNum)

	image.Version = ver

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

	block, data := pem.Decode(data)
	if block != nil && block.Type == "EC PARAMETERS" {
		/*
		 * Openssl prepends an EC PARAMETERS block before the
		 * key itself.  If we see this first, just skip it,
		 * and go on to the data block.
		 */
		block, _ = pem.Decode(data)
	}
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

func (image *Image) sigHdrTypeV1() (uint32, error) {
	if image.SigningRSA != nil {
		if UseRsaPss {
			return IMAGEv1_F_PKCS1_PSS_RSA2048_SHA256, nil
		} else {
			return IMAGEv1_F_PKCS15_RSA2048_SHA256, nil
		}
	} else if image.SigningEC != nil {
		switch image.SigningEC.Curve.Params().Name {
		case "P-224":
			return IMAGEv1_F_ECDSA224_SHA256, nil
		case "P-256":
			return IMAGEv1_F_ECDSA256_SHA256, nil
		default:
			return 0, util.NewNewtError("Unsupported ECC curve")
		}
	} else {
		return 0, nil
	}
}

func (image *Image) sigKeyHash() ([]uint8, error) {
	if image.SigningRSA != nil {
		pubkey, _ := asn1.Marshal(image.SigningRSA.PublicKey)
		sum := sha256.Sum256(pubkey)
		return sum[:4], nil
	} else if image.SigningEC != nil {
		switch image.SigningEC.Curve.Params().Name {
		case "P-224":
			fallthrough
		case "P-256":
			pubkey, _ := x509.MarshalPKIXPublicKey(&image.SigningEC.PublicKey)
			sum := sha256.Sum256(pubkey)
			return sum[:4], nil
		default:
			return []uint8{}, util.NewNewtError("Unsupported ECC curve")
		}
	} else {
		return []uint8{}, util.NewNewtError("No public key to hash")
	}
}

func (image *Image) sigLen() uint16 {
	if image.SigningRSA != nil {
		return 256
	} else if image.SigningEC != nil {
		switch image.SigningEC.Curve.Params().Name {
		case "P-224":
			return 68
		case "P-256":
			return 72
		default:
			return 0
		}
	} else {
		return 0
	}
}

func (image *Image) sigTlvTypeV1() uint8 {
	if image.SigningRSA != nil {
		return IMAGEv1_TLV_RSA2048
	} else if image.SigningEC != nil {
		switch image.SigningEC.Curve.Params().Name {
		case "P-224":
			return IMAGEv1_TLV_ECDSA224
		case "P-256":
			return IMAGEv1_TLV_ECDSA256
		default:
			return 0
		}
	} else {
		return 0
	}
}

func (image *Image) sigTlvType() uint8 {
	if image.SigningRSA != nil {
		return IMAGE_TLV_RSA2048
	} else if image.SigningEC != nil {
		switch image.SigningEC.Curve.Params().Name {
		case "P-224":
			return IMAGE_TLV_ECDSA224
		case "P-256":
			return IMAGE_TLV_ECDSA256
		default:
			return 0
		}
	} else {
		return 0
	}
}

func (image *Image) ReSign() error {
	srcImg, err := os.Open(image.SourceImg)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Can't open image file %s: %s",
			image.SourceImg, err.Error()))
	}

	srcInfo, err := srcImg.Stat()
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Can't stat image file %s: %s",
			image.SourceImg, err.Error()))
	}

	var hdr1 ImageHdrV1
	var hdr2 ImageHdr
	var hdrSz uint16
	var imgSz uint32

	err = binary.Read(srcImg, binary.LittleEndian, &hdr1)
	if err == nil {
		srcImg.Seek(0, 0)
		err = binary.Read(srcImg, binary.LittleEndian, &hdr2)
	}
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Failing to access image %s: %s",
			image.SourceImg, err.Error()))
	}
	if hdr1.Magic == IMAGEv1_MAGIC {
		if uint32(srcInfo.Size()) !=
			uint32(hdr1.HdrSz)+hdr1.ImgSz+uint32(hdr1.TlvSz) {

			return util.NewNewtError(fmt.Sprintf("File %s is not an image\n",
				image.SourceImg))
		}
		imgSz = hdr1.ImgSz
		hdrSz = hdr1.HdrSz
		image.Version = hdr1.Vers

		log.Debugf("Resigning %s (ver %d.%d.%d.%d)", image.SourceImg,
			hdr1.Vers.Major, hdr1.Vers.Minor, hdr1.Vers.Rev,
			hdr1.Vers.BuildNum)
	} else if hdr2.Magic == IMAGE_MAGIC {
		if uint32(srcInfo.Size()) < uint32(hdr2.HdrSz)+hdr2.ImgSz {
			return util.NewNewtError(fmt.Sprintf("File %s is not an image\n",
				image.SourceImg))
		}
		imgSz = hdr2.ImgSz
		hdrSz = hdr2.HdrSz
		image.Version = hdr2.Vers

		log.Debugf("Resigning %s (ver %d.%d.%d.%d)", image.SourceImg,
			hdr2.Vers.Major, hdr2.Vers.Minor, hdr2.Vers.Rev,
			hdr2.Vers.BuildNum)
	} else {
		return util.NewNewtError(fmt.Sprintf("File %s is not an image\n",
			image.SourceImg))
	}
	srcImg.Seek(int64(hdrSz), 0)

	tmpBin, err := ioutil.TempFile("", "")
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Creating temp file failed: %s",
			err.Error()))
	}
	tmpBinName := tmpBin.Name()
	defer os.Remove(tmpBinName)

	log.Debugf("Extracting data from %s:%d-%d to %s\n",
		image.SourceImg, int64(hdrSz), int64(hdrSz)+int64(imgSz), tmpBinName)
	_, err = io.CopyN(tmpBin, srcImg, int64(imgSz))
	srcImg.Close()
	tmpBin.Close()
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Cannot copy to tmpfile %s: %s",
			tmpBin.Name(), err.Error()))
	}

	image.SourceBin = tmpBinName
	image.TargetImg = image.SourceImg
	image.HeaderSize = uint(hdrSz)

	return image.Generate(nil)
}

func (image *Image) generateV1(loader *Image) error {
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
	hdr := &ImageHdrV1{
		Magic: IMAGEv1_MAGIC,
		TlvSz: 0,
		KeyId: 0,
		Pad1:  0,
		HdrSz: IMAGE_HEADER_SIZE,
		Pad2:  0,
		ImgSz: uint32(binInfo.Size()) - uint32(image.SrcSkip),
		Flags: 0,
		Vers:  image.Version,
		Pad3:  0,
	}

	hdr.Flags, err = image.sigHdrTypeV1()
	if err != nil {
		return err
	}
	if hdr.Flags != 0 {
		/*
		 * Signature present
		 */
		hdr.TlvSz = 4 + image.sigLen()
		hdr.KeyId = image.KeyId
	}

	hdr.TlvSz += 4 + 32
	hdr.Flags |= IMAGEv1_F_SHA256

	if loader != nil {
		hdr.Flags |= IMAGEv1_F_NON_BOOTABLE
	}

	if image.HeaderSize != 0 {
		/*
		 * Pad the header out to the given size.  There will
		 * just be zeros between the header and the start of
		 * the image when it is padded.
		 */
		if image.HeaderSize < IMAGE_HEADER_SIZE {
			return util.NewNewtError(fmt.Sprintf("Image header must be at least %d bytes", IMAGE_HEADER_SIZE))
		}

		hdr.HdrSz = uint16(image.HeaderSize)
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

	if image.HeaderSize > IMAGE_HEADER_SIZE {
		/*
		 * Pad the image (and hash) with zero bytes to fill
		 * out the buffer.
		 */
		buf := make([]byte, image.HeaderSize-IMAGE_HEADER_SIZE)

		_, err = imgFile.Write(buf)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to write padding: %s",
				err.Error()))
		}

		_, err = hash.Write(buf)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to hash padding: %s",
				err.Error()))
		}
	}

	/*
	 * Skip requested initial part of image.
	 */
	if image.SrcSkip > 0 {
		buf := make([]byte, image.SrcSkip)
		_, err = binFile.Read(buf)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to read from %s: %s",
				image.SourceBin, err.Error()))
		}

		nonZero := false
		for _, b := range buf {
			if b != 0 {
				nonZero = true
				break
			}
		}
		if nonZero {
			log.Warnf("Skip requested of image %s, but image not preceeded "+
				"by %d bytes of all zeros",
				image.SourceBin, image.SrcSkip)
		}
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
		Type: IMAGEv1_TLV_SHA256,
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
			Type: IMAGEv1_TLV_RSA2048,
			Pad:  0,
			Len:  256, /* 2048 bits */
		}
		var signature []byte
		if UseRsaPss {
			opts := rsa.PSSOptions{
				SaltLength: rsa.PSSSaltLengthEqualsHash,
			}
			signature, err = rsa.SignPSS(rand.Reader, image.SigningRSA,
				crypto.SHA256, image.Hash, &opts)
		} else {
			signature, err = rsa.SignPKCS1v15(rand.Reader, image.SigningRSA,
				crypto.SHA256, image.Hash)
		}
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

		sigLen := image.sigLen()

		var ECDSA ECDSASig
		ECDSA.R = r
		ECDSA.S = s
		signature, err := asn1.Marshal(ECDSA)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf(
				"Failed to construct signature: %s", err))
		}
		if len(signature) > int(sigLen) {
			return util.NewNewtError(fmt.Sprintf(
				"Something is really wrong\n"))
		}
		tlv := &ImageTrailerTlv{
			Type: image.sigTlvTypeV1(),
			Pad:  0,
			Len:  sigLen,
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
		pad := make([]byte, int(sigLen)-len(signature))
		_, err = imgFile.Write(pad)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to serialize image "+
				"trailer: %s", err.Error()))
		}
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Computed Hash for image %s as %s \n",
		image.TargetImg, hex.EncodeToString(image.Hash))

	// XXX: Replace "1" with io.SeekCurrent when go 1.7 becomes mainstream.
	sz, err := imgFile.Seek(0, 1)
	if err != nil {
		return util.FmtNewtError("Failed to calculate file size of generated "+
			"image %s: %s", image.TargetImg, err.Error())
	}
	image.TotalSize = uint(sz)

	return nil
}

func (image *Image) generateV2(loader *Image) error {
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
		Pad1:  0,
		HdrSz: IMAGE_HEADER_SIZE,
		Pad2:  0,
		ImgSz: uint32(binInfo.Size()) - uint32(image.SrcSkip),
		Flags: 0,
		Vers:  image.Version,
		Pad3:  0,
	}

	if loader != nil {
		hdr.Flags |= IMAGE_F_NON_BOOTABLE
	}

	if image.HeaderSize != 0 {
		/*
		 * Pad the header out to the given size.  There will
		 * just be zeros between the header and the start of
		 * the image when it is padded.
		 */
		if image.HeaderSize < IMAGE_HEADER_SIZE {
			return util.NewNewtError(fmt.Sprintf("Image header must be at "+
				"least %d bytes", IMAGE_HEADER_SIZE))
		}

		hdr.HdrSz = uint16(image.HeaderSize)
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

	if image.HeaderSize > IMAGE_HEADER_SIZE {
		/*
		 * Pad the image (and hash) with zero bytes to fill
		 * out the buffer.
		 */
		buf := make([]byte, image.HeaderSize-IMAGE_HEADER_SIZE)

		_, err = imgFile.Write(buf)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to write padding: %s",
				err.Error()))
		}

		_, err = hash.Write(buf)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to hash padding: %s",
				err.Error()))
		}
	}

	/*
	 * Skip requested initial part of image.
	 */
	if image.SrcSkip > 0 {
		buf := make([]byte, image.SrcSkip)
		_, err = binFile.Read(buf)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to read from %s: %s",
				image.SourceBin, err.Error()))
		}

		nonZero := false
		for _, b := range buf {
			if b != 0 {
				nonZero = true
				break
			}
		}
		if nonZero {
			log.Warnf("Skip requested of image %s, but image not preceeded by %d bytes of all zeros",
				image.SourceBin, image.SrcSkip)
		}
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
	 * Write TLV info.
	 */
	tlvInfo := &ImageTlvInfo{
		Magic:     IMAGE_TRAILER_MAGIC,
		TlvTotLen: 0,
	}
	tlvInfoOff, err := imgFile.Seek(0, 1)
	if err != nil {
		return util.FmtNewtError("Failed to calculate file size of generated "+
			"image %s: %s", image.TargetImg, err.Error())
	}
	err = binary.Write(imgFile, binary.LittleEndian, tlvInfo)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Failed to serialize image hdr: %s",
			err.Error()))
	}

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

	if image.SigningRSA != nil || image.SigningEC != nil {
		keyHash, err := image.sigKeyHash()
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to compute hash " +
				"of the public key"))
		}

		tlv = &ImageTrailerTlv{
			Type: IMAGE_TLV_KEYHASH,
			Pad:  0,
			Len:  uint16(len(keyHash)),
		}
		err = binary.Write(imgFile, binary.LittleEndian, tlv)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to serial image "+
				"trailer: %s", err.Error()))
		}
		_, err = imgFile.Write(keyHash)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to append "+
				"key hash: %s", err.Error()))
		}
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
		var signature []byte
		opts := rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthEqualsHash,
		}
		signature, err = rsa.SignPSS(rand.Reader, image.SigningRSA,
			crypto.SHA256, image.Hash, &opts)
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

		sigLen := image.sigLen()

		var ECDSA ECDSASig
		ECDSA.R = r
		ECDSA.S = s
		signature, err := asn1.Marshal(ECDSA)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf(
				"Failed to construct signature: %s", err))
		}
		if len(signature) > int(sigLen) {
			return util.NewNewtError(fmt.Sprintf(
				"Something is really wrong\n"))
		}
		tlv := &ImageTrailerTlv{
			Type: image.sigTlvType(),
			Pad:  0,
			Len:  sigLen,
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
		pad := make([]byte, int(sigLen)-len(signature))
		_, err = imgFile.Write(pad)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Failed to serialize image "+
				"trailer: %s", err.Error()))
		}
	}

	util.StatusMessage(util.VERBOSITY_VERBOSE,
		"Computed Hash for image %s as %s \n",
		image.TargetImg, hex.EncodeToString(image.Hash))

	// XXX: Replace "1" with io.SeekCurrent when go 1.7 becomes mainstream.
	sz, err := imgFile.Seek(0, 1)
	if err != nil {
		return util.FmtNewtError("Failed to calculate file size of generated "+
			"image %s: %s", image.TargetImg, err.Error())
	}
	image.TotalSize = uint(sz)

	tlvInfo.TlvTotLen = uint16(sz - tlvInfoOff)

	/*
	 * Go back and write tlv info total length
	 */
	_, err = imgFile.Seek(tlvInfoOff, 0)
	if err != nil {
		return util.FmtNewtError("Failed to move to tlvInfo offset %d "+
			"image: %s", int(tlvInfoOff), err.Error())
	}
	err = binary.Write(imgFile, binary.LittleEndian, tlvInfo)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Failed to serialize image hdr: %s",
			err.Error()))
	}

	return nil
}

func (image *Image) Generate(loader *Image) error {
	if UseV1 {
		return image.generateV1(loader)
	} else {
		return image.generateV2(loader)
	}
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

	// Make sure we restore the current working dir to whatever it was when
	// this function was called
	cwd, err := os.Getwd()
	if err != nil {
		log.Debugf("Unable to determine current working directory: %v", err)
		return ip
	}
	defer os.Chdir(cwd)

	if err := os.Chdir(path); err != nil {
		return ip
	}

	var res []byte

	res, err = util.ShellCommand([]string{
		"git",
		"rev-parse",
		"HEAD",
	}, nil)
	if err != nil {
		log.Debugf("Unable to determine commit hash for %s: %v", path, err)
		repo.Commit = "UNKNOWN"
	} else {
		repo.Commit = strings.TrimSpace(string(res))
		res, err = util.ShellCommand([]string{
			"git",
			"status",
			"--porcelain",
		}, nil)
		if err != nil {
			log.Debugf("Unable to determine dirty state for %s: %v", path, err)
		} else {
			if len(res) > 0 {
				repo.Dirty = true
			}
		}
		res, err = util.ShellCommand([]string{
			"git",
			"config",
			"--get",
			"remote.origin.url",
		}, nil)
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

func NewImageManifestSizeCollector() *ImageManifestSizeCollector {
	return &ImageManifestSizeCollector{}
}

func (c *ImageManifestSizeCollector) AddPkg(pkg string) *ImageManifestSizePkg {
	p := &ImageManifestSizePkg{
		Name: pkg,
	}
	c.Pkgs = append(c.Pkgs, p)

	return p
}

func (c *ImageManifestSizePkg) AddSymbol(file string, sym string, area string,
	symSz uint32) {
	f := c.addFile(file)
	s := f.addSym(sym)
	s.addArea(area, symSz)
}

func (p *ImageManifestSizePkg) addFile(file string) *ImageManifestSizeFile {
	for _, f := range p.Files {
		if f.Name == file {
			return f
		}
	}
	f := &ImageManifestSizeFile{
		Name: file,
	}
	p.Files = append(p.Files, f)

	return f
}

func (f *ImageManifestSizeFile) addSym(sym string) *ImageManifestSizeSym {
	s := &ImageManifestSizeSym{
		Name: sym,
	}
	f.Syms = append(f.Syms, s)

	return s
}

func (s *ImageManifestSizeSym) addArea(area string, areaSz uint32) {
	a := &ImageManifestSizeArea{
		Name: area,
		Size: areaSz,
	}
	s.Areas = append(s.Areas, a)
}
