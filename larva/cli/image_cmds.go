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

package cli

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"sort"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/artifact/sec"
	"mynewt.apache.org/newt/larva/lvimg"
	"mynewt.apache.org/newt/util"
)

func tlvStr(tlv image.ImageTlv) string {
	return fmt.Sprintf("%s,0x%02x",
		image.ImageTlvTypeName(tlv.Header.Type),
		tlv.Header.Type)
}

func readImage(filename string) (image.Image, error) {
	img, err := image.ReadImage(filename)
	if err != nil {
		return img, err
	}

	log.Debugf("Successfully read image %s", filename)
	return img, nil
}

func writeImage(img image.Image, filename string) error {
	if err := lvimg.VerifyImage(img); err != nil {
		return err
	}

	if err := img.WriteToFile(filename); err != nil {
		return err
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Wrote image %s\n", filename)
	return nil
}

func parseTlvArgs(typeArg string, filenameArg string) (image.ImageTlv, error) {
	tlvType, err := util.AtoiNoOct(typeArg)
	if err != nil || tlvType < 0 {
		return image.ImageTlv{}, util.FmtNewtError(
			"Invalid TLV type integer: %s", typeArg)
	}

	data, err := ioutil.ReadFile(filenameArg)
	if err != nil {
		return image.ImageTlv{}, util.FmtNewtError(
			"Error reading TLV data file: %s", err.Error())
	}

	return image.ImageTlv{
		Header: image.ImageTlvHdr{
			Type: uint8(tlvType),
			Pad:  0,
			Len:  uint16(len(data)),
		},
		Data: data,
	}, nil
}

func runShowCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		LarvaUsage(cmd, nil)
	}

	img, err := readImage(args[0])
	if err != nil {
		LarvaUsage(cmd, err)
	}

	s, err := img.Json()
	if err != nil {
		LarvaUsage(nil, err)
	}
	fmt.Printf("%s\n", s)
}

func runBriefCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		LarvaUsage(cmd, nil)
	}

	img, err := readImage(args[0])
	if err != nil {
		LarvaUsage(cmd, err)
	}

	offsets, err := img.Offsets()
	if err != nil {
		LarvaUsage(nil, err)
	}

	fmt.Printf("%8d| Header\n", offsets.Header)
	fmt.Printf("%8d| Body\n", offsets.Body)
	fmt.Printf("%8d| Trailer\n", offsets.Trailer)
	for i, tlv := range img.Tlvs {
		fmt.Printf("%8d| TLV%d: type=%s(%d)\n",
			offsets.Tlvs[i], i, image.ImageTlvTypeName(tlv.Header.Type),
			tlv.Header.Type)
	}
	fmt.Printf("Total=%d\n", offsets.TotalSize)
}

func runSignCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		LarvaUsage(cmd, nil)
	}

	inFilename := args[0]
	outFilename, err := CalcOutFilename(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	img, err := readImage(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	keys, err := sec.ReadKeys(args[1:])
	if err != nil {
		LarvaUsage(cmd, err)
	}

	hash, err := img.Hash()
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Failed to read hash from specified image: %s", err.Error()))
	}

	tlvs, err := image.BuildSigTlvs(keys, hash)
	if err != nil {
		LarvaUsage(nil, err)
	}

	img.Tlvs = append(img.Tlvs, tlvs...)

	if err := writeImage(img, outFilename); err != nil {
		LarvaUsage(nil, err)
	}
}

func runAddTlvsCmd(cmd *cobra.Command, args []string) {
	if len(args) < 3 {
		LarvaUsage(cmd, nil)
	}

	inFilename := args[0]
	outFilename, err := CalcOutFilename(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	img, err := readImage(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	tlvArgs := args[1:]
	if len(tlvArgs)%2 != 0 {
		LarvaUsage(cmd, util.FmtNewtError(
			"Invalid argument count; each TLV requires two arguments"))
	}

	tlvs := []image.ImageTlv{}
	for i := 0; i < len(tlvArgs); i += 2 {
		tlv, err := parseTlvArgs(tlvArgs[i], tlvArgs[i+1])
		if err != nil {
			LarvaUsage(cmd, err)
		}

		tlvs = append(tlvs, tlv)
	}

	img.Tlvs = append(img.Tlvs, tlvs...)

	if err := writeImage(img, outFilename); err != nil {
		LarvaUsage(nil, err)
	}
}

func runRmtlvsCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		LarvaUsage(cmd, nil)
	}

	inFilename := args[0]
	outFilename, err := CalcOutFilename(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	img, err := readImage(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	tlvIndices := []int{}
	idxMap := map[int]struct{}{}
	for _, arg := range args[1:] {
		idx, err := util.AtoiNoOct(arg)
		if err != nil {
			LarvaUsage(cmd, util.FmtNewtError("Invalid TLV index: %s", arg))
		}

		if idx < 0 || idx >= len(img.Tlvs) {
			LarvaUsage(nil, util.FmtNewtError(
				"TLV index %s out of range; "+
					"must be in range [0, %d] for this image",
				arg, len(img.Tlvs)-1))
		}

		if _, ok := idxMap[idx]; ok {
			LarvaUsage(nil, util.FmtNewtError(
				"TLV index %d specified more than once", idx))
		}
		idxMap[idx] = struct{}{}

		tlvIndices = append(tlvIndices, idx)
	}

	// Remove TLVs in reverse order to preserve index mapping.
	sort.Sort(sort.Reverse(sort.IntSlice(tlvIndices)))
	for _, idx := range tlvIndices {
		tlv := img.Tlvs[idx]
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Removing TLV%d: %s\n", idx, tlvStr(tlv))

		img.Tlvs = append(img.Tlvs[0:idx], img.Tlvs[idx+1:]...)
	}

	if err := writeImage(img, outFilename); err != nil {
		LarvaUsage(nil, err)
	}
}

func runRmsigsCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		LarvaUsage(cmd, nil)
	}

	inFilename := args[0]
	outFilename, err := CalcOutFilename(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	img, err := readImage(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	cnt := img.RemoveTlvsIf(func(tlv image.ImageTlv) bool {
		return tlv.Header.Type == image.IMAGE_TLV_KEYHASH ||
			tlv.Header.Type == image.IMAGE_TLV_RSA2048 ||
			tlv.Header.Type == image.IMAGE_TLV_ECDSA224 ||
			tlv.Header.Type == image.IMAGE_TLV_ECDSA256
	})

	log.Debugf("Removed %d existing signatures", cnt)

	if err := writeImage(img, outFilename); err != nil {
		LarvaUsage(nil, err)
	}
}

func runHashableCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		LarvaUsage(cmd, nil)
	}

	if OptOutFilename == "" {
		LarvaUsage(cmd, util.FmtNewtError("--outfile (-o) option required"))
	}

	inFilename := args[0]
	outFilename := OptOutFilename

	img, err := readImage(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	f, err := os.Create(outFilename)
	if err != nil {
		LarvaUsage(nil, util.ChildNewtError(err))
	}
	defer f.Close()

	if err := binary.Write(f, binary.LittleEndian, &img.Header); err != nil {
		LarvaUsage(nil, util.FmtNewtError(
			"Error writing image header: %s", err.Error()))
	}
	_, err = f.Write(img.Body)
	if err != nil {
		LarvaUsage(nil, util.FmtNewtError(
			"Error writing image body: %s", err.Error()))
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Wrote hashable content to %s\n", outFilename)
}

func runAddsigCmd(cmd *cobra.Command, args []string) {
	if len(args) < 4 {
		LarvaUsage(cmd, nil)
	}

	imgFilename := args[0]
	keyFilename := args[1]
	sigFilename := args[2]

	sigType, err := util.AtoiNoOct(args[3])
	if err != nil || sigType < 0 || sigType > 255 ||
		!image.ImageTlvTypeIsSig(uint8(sigType)) {

		LarvaUsage(cmd, util.FmtNewtError(
			"Invalid signature type: %s", args[3]))
	}

	outFilename, err := CalcOutFilename(imgFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	img, err := readImage(imgFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	keyData, err := ioutil.ReadFile(keyFilename)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Error reading key file: %s", err.Error()))
	}

	sigData, err := ioutil.ReadFile(sigFilename)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Error reading signature file: %s", err.Error()))
	}

	// ECDSA256 signatures need to be padded out to >=72 bytes.
	if sigType == image.IMAGE_TLV_ECDSA256 {
		sigData, err = lvimg.PadEcdsa256Sig(sigData)
		if err != nil {
			LarvaUsage(nil, err)
		}
	}

	// Build and append key hash TLV.
	keyHashTlv := image.BuildKeyHashTlv(keyData)
	util.StatusMessage(util.VERBOSITY_DEFAULT, "Adding TLV%d (%s)\n",
		len(img.Tlvs), tlvStr(keyHashTlv))
	img.Tlvs = append(img.Tlvs, keyHashTlv)

	// Build and append signature TLV.
	sigTlv := image.ImageTlv{
		Header: image.ImageTlvHdr{
			Type: uint8(sigType),
			Len:  uint16(len(sigData)),
		},
		Data: sigData,
	}
	util.StatusMessage(util.VERBOSITY_DEFAULT, "Adding TLV%d (%s)\n",
		len(img.Tlvs), tlvStr(sigTlv))
	img.Tlvs = append(img.Tlvs, sigTlv)

	if err := writeImage(img, outFilename); err != nil {
		LarvaUsage(nil, err)
	}
}

func runDecryptCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		LarvaUsage(cmd, nil)
	}

	imgFilename := args[0]
	keyFilename := args[1]

	outFilename, err := CalcOutFilename(imgFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	img, err := readImage(imgFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	keyBytes, err := ioutil.ReadFile(keyFilename)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Error reading key file: %s", err.Error()))
	}

	img, err = lvimg.DecryptImage(img, keyBytes)
	if err != nil {
		LarvaUsage(nil, err)
	}

	if err := writeImage(img, outFilename); err != nil {
		LarvaUsage(nil, err)
	}
}

func runEncryptCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		LarvaUsage(cmd, nil)
	}

	imgFilename := args[0]
	keyFilename := args[1]

	outFilename, err := CalcOutFilename(imgFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	img, err := readImage(imgFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	keyBytes, err := ioutil.ReadFile(keyFilename)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Error reading key file: %s", err.Error()))
	}

	img, err = lvimg.EncryptImage(img, keyBytes)
	if err != nil {
		LarvaUsage(nil, err)
	}

	if err := writeImage(img, outFilename); err != nil {
		LarvaUsage(nil, err)
	}
}

func AddImageCommands(cmd *cobra.Command) {
	imageCmd := &cobra.Command{
		Use:   "image",
		Short: "Shows and manipulates Mynewt image (.img) files",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(imageCmd)

	showCmd := &cobra.Command{
		Use:   "show <img-file>",
		Short: "Displays JSON describing a Mynewt image file",
		Run:   runShowCmd,
	}
	imageCmd.AddCommand(showCmd)

	briefCmd := &cobra.Command{
		Use:   "brief <img-file>",
		Short: "Displays brief text description of a Mynewt image file",
		Run:   runBriefCmd,
	}
	imageCmd.AddCommand(briefCmd)

	signCmd := &cobra.Command{
		Use:   "sign <img-file> <priv-key-pem> [priv-key-pem...]",
		Short: "Appends signatures to a Mynewt image file",
		Run:   runSignCmd,
	}

	signCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o", "",
		"File to write to")
	signCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	imageCmd.AddCommand(signCmd)

	addtlvsCmd := &cobra.Command{
		Use: "addtlvs <img-file> <tlv-type> <data-filename> " +
			"[tlv-type] [data-filename] [...]",
		Short: "Adds the specified TLVs to a Mynewt image file",
		Run:   runAddTlvsCmd,
	}

	addtlvsCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o", "",
		"File to write to")
	addtlvsCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	imageCmd.AddCommand(addtlvsCmd)

	rmtlvsCmd := &cobra.Command{
		Use:   "rmtlvs <img-file> <tlv-index> [tlv-index] [...]",
		Short: "Removes the specified TLVs from a Mynewt image file",
		Run:   runRmtlvsCmd,
	}

	rmtlvsCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o", "",
		"File to write to")
	rmtlvsCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	imageCmd.AddCommand(rmtlvsCmd)

	rmsigsCmd := &cobra.Command{
		Use:   "rmsigs",
		Short: "Removes all signatures from a Mynewt image file",
		Run:   runRmsigsCmd,
	}

	rmsigsCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o", "",
		"File to write to")
	rmsigsCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	imageCmd.AddCommand(rmsigsCmd)

	hashableCmd := &cobra.Command{
		Use:   "hashable <img-file>",
		Short: "Removes all signatures from a Mynewt image file",
		Run:   runHashableCmd,
	}

	hashableCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o",
		"", "File to write to")

	imageCmd.AddCommand(hashableCmd)

	addsigCmd := &cobra.Command{
		Use:   "addsig <image> <pub-key-der> <sig-der> <sig-tlv-type>",
		Short: "Adds a signature to a Mynewt image file",
		Run:   runAddsigCmd,
	}

	addsigCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o",
		"", "File to write to")
	addsigCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	imageCmd.AddCommand(addsigCmd)

	decryptCmd := &cobra.Command{
		Use:   "decrypt <image> <priv-key-der>",
		Short: "Decrypts an encrypted Mynewt image file",
		Run:   runDecryptCmd,
	}

	decryptCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o",
		"", "File to write to")
	decryptCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	imageCmd.AddCommand(decryptCmd)

	encryptCmd := &cobra.Command{
		Use:   "encrypt <image> <priv-key-der>",
		Short: "Encrypts a Mynewt image file",
		Run:   runEncryptCmd,
	}

	encryptCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o",
		"", "File to write to")
	encryptCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	imageCmd.AddCommand(encryptCmd)
}
