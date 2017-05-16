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
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/newtmgr/core"
	"mynewt.apache.org/newt/newtmgr/protocol"
	"mynewt.apache.org/newt/newtmgr/transport"
	"mynewt.apache.org/newt/util"

	"github.com/spf13/cobra"
)

var (
	coreElfify   bool
	coreOffset   uint32
	coreNumBytes uint32
)

func imageFlagsStr(image protocol.ImageStateEntry) string {
	strs := []string{}

	if image.Active {
		strs = append(strs, "active")
	}
	if image.Confirmed {
		strs = append(strs, "confirmed")
	}
	if image.Pending {
		strs = append(strs, "pending")
	}
	if image.Permanent {
		strs = append(strs, "permanent")
	}

	return strings.Join(strs, " ")
}

func imageStatePrintRsp(rsp *protocol.ImageStateRsp) error {
	if rsp.ReturnCode != 0 {
		return util.FmtNewtError("rc=%d\n", rsp.ReturnCode)
	}
	fmt.Println("Images:")
	for _, img := range rsp.Images {
		fmt.Printf(" slot=%d\n", img.Slot)
		fmt.Printf("    version: %s\n", img.Version)
		fmt.Printf("    bootable: %v\n", img.Bootable)
		fmt.Printf("    flags: %s\n", imageFlagsStr(img))
		if len(img.Hash) == 0 {
			fmt.Printf("    hash: Unavailable\n")
		} else {
			fmt.Printf("    hash: %x\n", img.Hash)
		}
	}

	fmt.Printf("Split status: %s\n", rsp.SplitStatus.String())
	return nil
}

func imageStateListCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(nil, err)
	}
	defer runner.Conn.Close()

	var nmr *protocol.NmgrReq

	req, err := protocol.NewImageStateReadReq()
	if err != nil {
		nmUsage(nil, err)
	}

	nmr, err = req.Encode()
	if err != nil {
		nmUsage(nil, err)
	}

	if err := runner.WriteReq(nmr); err != nil {
		nmUsage(nil, err)
	}

	rawRsp, err := runner.ReadResp()
	if err != nil {
		nmUsage(nil, err)
	}

	rsp, err := protocol.DecodeImageStateResponse(rawRsp.Data)
	if err != nil {
		nmUsage(nil, err)
	}
	if err := imageStatePrintRsp(rsp); err != nil {
		nmUsage(nil, err)
	}
}

func imageStateTestCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		nmUsage(cmd, nil)
	}

	hexBytes, _ := hex.DecodeString(args[0])

	req, err := protocol.NewImageStateWriteReq()
	if err != nil {
		nmUsage(nil, err)
	}

	req.Hash = hexBytes
	req.Confirm = false

	nmr, err := req.Encode()
	if err != nil {
		nmUsage(nil, err)
	}

	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(nil, err)
	}
	defer runner.Conn.Close()

	if err := runner.WriteReq(nmr); err != nil {
		nmUsage(nil, err)
	}

	rawRsp, err := runner.ReadResp()
	if err != nil {
		nmUsage(nil, err)
	}

	rsp, err := protocol.DecodeImageStateResponse(rawRsp.Data)
	if err != nil {
		nmUsage(nil, err)
	}
	if err := imageStatePrintRsp(rsp); err != nil {
		nmUsage(nil, err)
	}
}

func imageStateConfirmCmd(cmd *cobra.Command, args []string) {
	req, err := protocol.NewImageStateWriteReq()
	if err != nil {
		nmUsage(nil, err)
	}

	if len(args) >= 1 {
		hexBytes, _ := hex.DecodeString(args[0])
		req.Hash = hexBytes
	}
	req.Confirm = true

	nmr, err := req.Encode()
	if err != nil {
		nmUsage(cmd, err)
	}

	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(nil, err)
	}
	defer runner.Conn.Close()

	if err := runner.WriteReq(nmr); err != nil {
		nmUsage(nil, err)
	}

	rawRsp, err := runner.ReadResp()
	if err != nil {
		nmUsage(nil, err)
	}

	rsp, err := protocol.DecodeImageStateResponse(rawRsp.Data)
	if err != nil {
		nmUsage(nil, err)
	}
	if err := imageStatePrintRsp(rsp); err != nil {
		nmUsage(nil, err)
	}
}

func echoOnNmUsage(
	runner *protocol.CmdRunner, cmderr error, cmd *cobra.Command) {

	echoCtrl(runner, "1")
	nmUsage(cmd, cmderr)
}

func imageUploadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		nmUsage(cmd, util.NewNewtError("Need to specify image to upload"))
	}

	imageFile, err := ioutil.ReadFile(args[0])
	if err != nil {
		nmUsage(cmd, util.NewNewtError(err.Error()))
	}

	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConnWithTimeout(profile, time.Second*16)
	if err != nil {
		nmUsage(nil, err)
	}
	defer conn.Close()

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	if profile.Type() == "serial" {
		err = echoCtrl(runner, "0")
		if err != nil {
			nmUsage(cmd, err)
		}
		defer echoCtrl(runner, "1")
	}

	var currOff uint32 = 0
	var mtu uint32 = 0
	imageSz := uint32(len(imageFile))
	rexmits := 0

	if profile.Type() == "ble" {
		mtu = uint32((transport.BleMTU - 64) * 3 / 4)
	} else {
		/* since this possibly gets base 64 encoded, we want
		 * to ensure that the payload leaving this layer is 91
		 * bytes or less (91 bytes plus 2 byte crc will encode
		 * to 124 with 4 bytes of header
		 * left over */

		/* 00000000  02 00 00 4f 00 01 00 01  a2 64 64 61 74 61 58 40  |...O.....ddataX@|
		 * 00000010  00 f0 5a f8 0e 4b 1c 70  0e 4b 5a 88 12 05 10 0f  |..Z..K.p.KZ.....|
		 * 00000020  59 88 0d 4a 0a 40 5a 80  59 88 0c 4a 0a 40 5a 80  |Y..J.@Z.Y..J.@Z.|
		 * 00000030  19 1c 80 22 d2 01 4b 88  13 42 fc d1 05 49 02 02  |..."..K..B...I..|
		 * 00000040  48 88 05 4b 03 40 13 43  4b 80 00 f0 5d f8 10 bd  |H..K.@.CK...]...|
		 * 00000050  63 6f 66 66 1a 00 01 5d  b8                       |coff..x|
		 */

		/* from this dump we can see the following
		* 1) newtmgr hdr 8 bytes
		* 2) cbor wrapper up to data (and length) 8 bytes
		* 3) cbor data 64 bytes
		* 4) offset tag 4 bytes
		* 5) offsert value 3 (safely say 5 bytes since it could be bigger
		*      than uint16_t
		* That makes 25 bytes plus the data needs to fit in 91 bytes
		 */

		/* however, something is not calcualated properly as we
		 * can only do 66 bytes here.  Use 64 for power of 2 */
		mtu = 64
	}

	for currOff < imageSz {
		imageUpload, err := protocol.NewImageUpload()
		if err != nil {
			echoOnNmUsage(runner, err, cmd)
		}

		blockSz := imageSz - currOff
		if blockSz > mtu {
			blockSz = mtu
		}
		if currOff == 0 {
			/* we need extra space to encode the image size */
			if blockSz > (mtu - 8) {
				/*
				 * to encode the image size, we write clen=val in CBOR.
				 * From below (for up to 2G images, you can see that it
				 * will take up to 9 bytes.  (starts at 63.. ends at e8)
				 * 00000040  7d c4 00 00 7d c4 00 00  63 6c 65 6e 1a 00 01 5d  |}...}...clen...]|
				 * 00000050  e8 63 6f 66 66 00                                 |.coff.|
				 * However, since the offset is zero, we will use less
				 * bytes (we budgeted for 5 bytes but will only use 1
				 */

				/* to make these powers of 2, just go with 8 bytes */
				blockSz = mtu - 8
			}
		}

		imageUpload.Offset = currOff
		imageUpload.Size = imageSz
		imageUpload.Data = imageFile[currOff : currOff+blockSz]

		nmr, err := imageUpload.EncodeWriteRequest()
		if err != nil {
			echoOnNmUsage(runner, err, cmd)
		}

		var rsp *protocol.NmgrReq
		var i int
		for i = 0; i < 5; i++ {
			if err := runner.WriteReq(nmr); err != nil {
				echoOnNmUsage(runner, err, cmd)
			}

			rsp, err = runner.ReadResp()
			if err == nil {
				break
			}

			/*
			 * Failed. Reopening tty.
			 */
			conn, err = transport.NewConnWithTimeout(profile, time.Second)
			if err != nil {
				echoOnNmUsage(runner, err, cmd)
			}

			runner, err = protocol.NewCmdRunner(conn)
			if err != nil {
				echoOnNmUsage(runner, err, cmd)
			}
		}
		rexmits += i
		if i == 5 {
			err = util.NewNewtError("Maximum number of TX retries reached")
		}
		if err != nil {
			echoOnNmUsage(runner, err, cmd)
		}

		ersp, err := protocol.DecodeImageUploadResponse(rsp.Data)
		if err != nil {
			echoOnNmUsage(runner, err, nil)
		}
		currOff = ersp.Offset

		fmt.Println(currOff)
	}

	if rexmits != 0 {
		fmt.Printf(" %d retransmits\n", rexmits)
	}
	fmt.Println("Done")
}

func coreConvertCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		nmUsage(cmd, nil)
		return
	}

	coreConvert, err := core.ConvertFilenames(args[0], args[1])
	if err != nil {
		nmUsage(cmd, err)
		return
	}

	fmt.Printf("Corefile created for\n   %x\n", coreConvert.ImageHash)
}

func coreDownloadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		nmUsage(cmd, util.NewNewtError("Need to specify filename for core"))
		return
	}

	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	tmpName := args[0] + ".tmp"
	file, err := os.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		nmUsage(cmd, util.NewNewtError(fmt.Sprintf(
			"Cannot open file %s - %s", tmpName, err.Error())))
	}

	coreDownload, err := protocol.NewCoreDownload()
	if err != nil {
		nmUsage(cmd, err)
	}
	coreDownload.Runner = runner
	coreDownload.File = file

	err = coreDownload.Download(coreOffset, coreNumBytes)
	file.Close()
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Coredump download completed")

	if !coreElfify {
		os.Rename(tmpName, args[0])
		return
	}

	/*
	 * Download finished. Now convert to ELF corefile format.
	 */
	coreConvert, err := core.ConvertFilenames(tmpName, args[0])
	if err != nil {
		nmUsage(cmd, err)
		return
	}

	if err != nil {
		fmt.Println(err)
		return
	}

	os.Remove(tmpName)
	fmt.Printf("Corefile created for\n   %x\n", coreConvert.ImageHash)
}

func coreListCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	coreList, err := protocol.NewCoreList()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := coreList.EncodeWriteRequest()
	if err != nil {
		nmUsage(cmd, err)
	}

	if err := runner.WriteReq(nmr); err != nil {
		nmUsage(cmd, err)
	}

	rsp, err := runner.ReadResp()
	if err != nil {
		nmUsage(cmd, err)
	}

	clRsp, err := protocol.DecodeCoreListResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	if clRsp.ErrCode == protocol.NMGR_ERR_OK {
		fmt.Printf("Corefile present\n")
	} else if clRsp.ErrCode == protocol.NMGR_ERR_ENOENT {
		fmt.Printf("No corefiles\n")
	} else {
		fmt.Printf("List failed: %d\n", clRsp.ErrCode)
	}
}

func coreEraseCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	coreErase, err := protocol.NewCoreErase()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := coreErase.EncodeWriteRequest()
	if err != nil {
		nmUsage(cmd, err)
	}

	if err := runner.WriteReq(nmr); err != nil {
		nmUsage(cmd, err)
	}

	rsp, err := runner.ReadResp()
	if err != nil {
		nmUsage(cmd, err)
	}

	ceRsp, err := protocol.DecodeCoreEraseResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	if ceRsp.ErrCode != 0 {
		fmt.Printf("Erase failed: %d\n", ceRsp.ErrCode)
	} else {
		fmt.Printf("Done\n")
	}
}

func imageEraseCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	imageErase, err := protocol.NewImageErase()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := imageErase.EncodeWriteRequest()
	if err != nil {
		nmUsage(cmd, err)
	}

	if err := runner.WriteReq(nmr); err != nil {
		nmUsage(cmd, err)
	}

	rsp, err := runner.ReadResp()
	if err != nil {
		nmUsage(cmd, err)
	}

	ieRsp, err := protocol.DecodeImageEraseResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	if ieRsp.ErrCode != 0 {
		fmt.Printf("Erase failed: %d\n", ieRsp.ErrCode)
	} else {
		fmt.Printf("Done\n")
	}
}


func imageCmd() *cobra.Command {
	imageCmd := &cobra.Command{
		Use:   "image",
		Short: "Manage images on a device",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Show images on a device",
		Run:   imageStateListCmd,
	}
	imageCmd.AddCommand(listCmd)

	testCmd := &cobra.Command{
		Use:   "test <hex-image-hash>",
		Short: "Test an image on next reboot",
		Run:   imageStateTestCmd,
	}
	imageCmd.AddCommand(testCmd)

	confirmCmd := &cobra.Command{
		Use:   "confirm [hex-image-hash] -c <conn_profile>",
		Short: "Permanently run image",
		Long: "If a hash is specified, permanently switch to the " +
			"corresponding image.  If no hash is specified, the current " +
			"image setup is made permanent.",
		Run: imageStateConfirmCmd,
	}
	imageCmd.AddCommand(confirmCmd)

	uploadEx := "  newtmgr -c olimex image upload bin/slinky_zero/apps/slinky.img\n"

	uploadCmd := &cobra.Command{
		Use:     "upload <image-file> -c <conn_profile>",
		Short:   "Upload image to a device",
		Example: uploadEx,
		Run:     imageUploadCmd,
	}
	imageCmd.AddCommand(uploadCmd)

	coreListEx := "  newtmgr -c olimex image corelist\n"

	coreListCmd := &cobra.Command{
		Use:     "corelist -c <conn_profile>",
		Short:   "List core(s) on a device",
		Example: coreListEx,
		Run:     coreListCmd,
	}
	imageCmd.AddCommand(coreListCmd)

	coreEx := "  newtmgr -c olimex image coredownload -e core\n"
	coreEx += "  newtmgr -c olimex image coredownload --offset 10 -n 10 core\n"

	coreDownloadCmd := &cobra.Command{
		Use:     "coredownload <core-filename> -c <conn_profile>",
		Short:   "Download core from a device",
		Example: coreEx,
		Run:     coreDownloadCmd,
	}
	coreDownloadCmd.Flags().BoolVarP(&coreElfify, "elfify", "e", false, "Create an ELF file")
	coreDownloadCmd.Flags().Uint32Var(&coreOffset, "offset", 0, "Start offset")
	coreDownloadCmd.Flags().Uint32VarP(&coreNumBytes, "bytes", "n", 0, "Number of bytes of the core to download")
	imageCmd.AddCommand(coreDownloadCmd)

	coreConvertCmd := &cobra.Command{
		Use:   "coreconvert <core-filename> <elf-filename>",
		Short: "Convert core to ELF",
		Run:   coreConvertCmd,
	}
	imageCmd.AddCommand(coreConvertCmd)

	coreEraseEx := "  newtmgr -c olimex image coreerase\n"

	coreEraseCmd := &cobra.Command{
		Use:     "coreerase -c <conn_profile>",
		Short:   "Erase core on a device",
		Example: coreEraseEx,
		Run:     coreEraseCmd,
	}
	imageCmd.AddCommand(coreEraseCmd)

	imageEraseEx := "  newtmgr -c olimex image erase\n"

	imageEraseCmd := &cobra.Command{
		Use:     "erase -c <conn_profile>",
		Short:   "Erase image on a device",
		Example: imageEraseEx,
		Run:     imageEraseCmd,
	}
	imageCmd.AddCommand(imageEraseCmd)

	return imageCmd
}
