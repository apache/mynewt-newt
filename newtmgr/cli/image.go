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
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
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

func imageListCmd(cmd *cobra.Command, args []string) {
	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConnWithTimeout(profile, time.Second*3)
	if err != nil {
		nmUsage(nil, err)
	}
	defer conn.Close()

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	imageList, err := protocol.NewImageList()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := imageList.EncodeWriteRequest()
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

	iRsp, err := protocol.DecodeImageListResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	fmt.Println("Images:")
	for i := 0; i < len(iRsp.Images); i++ {
		fmt.Println("   ", i, ": "+iRsp.Images[i])
	}

}

func imageListCmd2(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	imageList, err := protocol.NewImageList2()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := imageList.EncodeWriteRequest()
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

	iRsp, err := protocol.DecodeImageListResponse2(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	fmt.Println("Images:")
	for _, img := range iRsp.Images {

		fmt.Printf(" slot=%d\n", img.Slot)
		fmt.Printf("    version=%s\n", img.Version)
		fmt.Printf("    bootable=%v\n", img.Bootable)
		if img.Hash == "" {
			fmt.Printf("    hash=Unavailable\n")
		} else {
			dec, err := base64.StdEncoding.DecodeString(img.Hash)
			if err != nil {
				fmt.Printf("    hash=Unable to Decode")
			} else {
				fmt.Printf("    hash=%s\n", hex.EncodeToString(dec[:]))
			}
		}
	}
}

func echoOnNmUsage(runner *protocol.CmdRunner, cmderr error, cmd *cobra.Command) {
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
		mtu = uint32((transport.BleMTU - 33) * 3 / 4)
	} else {
		mtu = 36
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
			blockSz = 33
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
			echoOnNmUsage(runner, err, cmd)
		}
		currOff = ersp.Offset

		fmt.Println(currOff)
	}

	if rexmits != 0 {
		fmt.Printf(" %d retransmits\n", rexmits)
	}
	fmt.Println("Done")
}

func imageBootCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	imageBoot, err := protocol.NewImageBoot()
	if err != nil {
		nmUsage(cmd, err)
	}

	if len(args) >= 1 {
		imageBoot.BootTarget = args[0]
	}
	nmr, err := imageBoot.EncodeWriteRequest()
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

	iRsp, err := protocol.DecodeImageBootResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	if len(args) == 0 {
		fmt.Println("    Test image:", iRsp.Test)
		fmt.Println("    Main image:", iRsp.Main)
		fmt.Println("    Active img:", iRsp.Active)
	}
}

func imageBoot2Cmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	imageBoot, err := protocol.NewImageBoot2()
	if err != nil {
		nmUsage(cmd, err)
	}

	if len(args) >= 1 {
		imageBoot.BootTarget = args[0]
	}
	nmr, err := imageBoot.EncodeWriteRequest()
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

	iRsp, err := protocol.DecodeImageBoot2Response(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	if len(args) == 0 {
		fmt.Println("   Test image:", iRsp.Test)
		fmt.Println("   Main image:", iRsp.Main)
		fmt.Println("   Active img:", iRsp.Active)
	}
}

func fileUploadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		nmUsage(cmd, util.NewNewtError(
			"Need to specify file and target filename to upload"))
	}

	file, err := ioutil.ReadFile(args[0])
	if err != nil {
		nmUsage(cmd, util.NewNewtError(err.Error()))
	}

	filename := args[1]
	if len(filename) > 64 {
		nmUsage(cmd, util.NewNewtError("Target filename too long"))
	}

	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	err = echoCtrl(runner, "0")
	if err != nil {
		nmUsage(cmd, err)
	}
	defer echoCtrl(runner, "1")
	var currOff uint32 = 0
	var cnt int = 0

	fileSz := uint32(len(file))

	for currOff < fileSz {
		fileUpload, err := protocol.NewFileUpload()
		if err != nil {
			echoOnNmUsage(runner, err, cmd)
		}

		blockSz := fileSz - currOff
		if currOff == 0 {
			blockSz = 3
		} else {
			if blockSz > 36 {
				blockSz = 36
			}
		}

		fileUpload.Offset = currOff
		fileUpload.Size = fileSz
		fileUpload.Name = filename
		fileUpload.Data = file[currOff : currOff+blockSz]

		nmr, err := fileUpload.EncodeWriteRequest()
		if err != nil {
			echoOnNmUsage(runner, err, cmd)
		}

		if err := runner.WriteReq(nmr); err != nil {
			echoOnNmUsage(runner, err, cmd)
		}

		rsp, err := runner.ReadResp()
		if err != nil {
			echoOnNmUsage(runner, err, cmd)
		}

		ersp, err := protocol.DecodeFileUploadResponse(rsp.Data)
		if err != nil {
			echoOnNmUsage(runner, err, cmd)
		}
		currOff = ersp.Offset
		cnt++
		fmt.Println(cnt, currOff)
	}
	fmt.Println("Done")
}

func fileDownloadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		nmUsage(cmd, util.NewNewtError(
			"Need to specify file and target filename to download"))
	}

	filename := args[0]
	if len(filename) > 64 {
		nmUsage(cmd, util.NewNewtError("Target filename too long"))
	}

	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	var currOff uint32 = 0
	var cnt int = 0
	var fileSz uint32 = 1

	file, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		nmUsage(cmd, util.NewNewtError(fmt.Sprintf(
			"Cannot open file %s - %s", args[1], err.Error())))
	}
	for currOff < fileSz {
		fileDownload, err := protocol.NewFileDownload()
		if err != nil {
			nmUsage(cmd, err)
		}

		fileDownload.Offset = currOff
		fileDownload.Name = filename

		nmr, err := fileDownload.EncodeWriteRequest()
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

		ersp, err := protocol.DecodeFileDownloadResponse(rsp.Data)
		if err != nil {
			nmUsage(cmd, err)
		}
		if currOff == ersp.Offset {
			n, err := file.Write(ersp.Data)
			if err == nil && n < len(ersp.Data) {
				err = io.ErrShortWrite
				nmUsage(cmd, util.NewNewtError(fmt.Sprintf(
					"Cannot write file %s - %s", args[1],
					err.Error())))
			}
		}
		if currOff == 0 {
			fileSz = ersp.Size
		}
		cnt++
		currOff += uint32(len(ersp.Data))
		fmt.Println(cnt, currOff)

	}
	file.Close()
	fmt.Println("Done")
}

func coreDownloadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		nmUsage(cmd, errors.New("Need to specify target filename to download"))
		return
	}

	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

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
	coreConvert := core.NewCoreConvert()

	file, err = os.OpenFile(tmpName, os.O_RDONLY, 0)
	if err != nil {
		nmUsage(cmd, util.NewNewtError(fmt.Sprintf(
			"Cannot open file %s - %s", tmpName, err.Error())))
	}

	coreConvert.Source = file

	file, err = os.OpenFile(args[0], os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		nmUsage(cmd, util.NewNewtError(fmt.Sprintf(
			"Cannot open file %s - %s", args[0], err.Error())))
	}
	coreConvert.Target = file

	err = coreConvert.Convert()

	coreConvert.Source.Close()
	coreConvert.Target.Close()
	os.Remove(tmpName)

	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Printf("Corefile created for\n   %x\n", coreConvert.ImageHash)
	}
}

func coreListCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

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

func imageCmd() *cobra.Command {
	imageCmd := &cobra.Command{
		Use:   "image",
		Short: "Manage images on remote instance",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list2",
		Short: "Show target images",
		Run:   imageListCmd2,
	}
	imageCmd.AddCommand(listCmd)

	listOldCmd := &cobra.Command{
		Use:   "list",
		Short: "Show target images",
		Run:   imageListCmd,
	}
	imageCmd.AddCommand(listOldCmd)

	uploadEx := "  newtmgr -c olimex image upload <image_file\n"
	uploadEx += "  newtmgr -c olimex image upload bin/slinky_zero/apps/slinky.img\n"

	uploadCmd := &cobra.Command{
		Use:     "upload",
		Short:   "Upload image to target",
		Example: uploadEx,
		Run:     imageUploadCmd,
	}
	imageCmd.AddCommand(uploadCmd)

	bootEx := "  newtmgr -c olimex image boot [<version>]\n"
	bootEx += "  newtmgr -c olimex image boot\n"
	bootEx += "  newtmgr -c olimex image boot 1.2.3\n"

	bootCmd := &cobra.Command{
		Use:     "boot",
		Short:   "Which image to boot",
		Example: bootEx,
		Run:     imageBootCmd,
	}
	imageCmd.AddCommand(bootCmd)

	boot2Cmd := &cobra.Command{
		Use:   "boot2",
		Short: "Which image to boot",
		Run:   imageBoot2Cmd,
	}
	imageCmd.AddCommand(boot2Cmd)

	fileUploadEx := "  newtmgr -c olimex image fileupload <filename> <tgt_file>\n"
	fileUploadEx += "  newtmgr -c olimex image fileupload sample.lua /sample.lua\n"

	fileUploadCmd := &cobra.Command{
		Use:     "fileupload",
		Short:   "Upload file to target",
		Example: fileUploadEx,
		Run:     fileUploadCmd,
	}
	imageCmd.AddCommand(fileUploadCmd)

	fileDownloadEx := "  newtmgr -c olimex image filedownload <tgt_file> <filename>\n"
	fileDownloadEx += "  newtmgr -c olimex image filedownload /cfg/mfg mfg.txt\n"

	fileDownloadCmd := &cobra.Command{
		Use:     "filedownload",
		Short:   "Download file from target",
		Example: fileDownloadEx,
		Run:     fileDownloadCmd,
	}
	imageCmd.AddCommand(fileDownloadCmd)

	coreListEx := "  newtmgr -c olimex image corelist\n"

	coreListCmd := &cobra.Command{
		Use:     "corelist",
		Short:   "List core(s) on target",
		Example: coreListEx,
		Run:     coreListCmd,
	}
	imageCmd.AddCommand(coreListCmd)

	coreEx := "  newtmgr -c olimex image coredownload -e <filename>\n"
	coreEx += "  newtmgr -c olimex image coredownload -e core\n"
	coreEx += "  newtmgr -c olimex image coredownload --offset 10 -n 10 core\n"

	coreDownloadCmd := &cobra.Command{
		Use:     "coredownload",
		Short:   "Download core from target",
		Example: coreEx,
		Run:     coreDownloadCmd,
	}
	coreDownloadCmd.Flags().BoolVarP(&coreElfify, "elfify", "e", false, "Creat an elf file")
	coreDownloadCmd.Flags().Uint32Var(&coreOffset, "offset", 0, "Start offset")
	coreDownloadCmd.Flags().Uint32VarP(&coreNumBytes, "bytes", "n", 0, "Number of bytes of the core to download")
	imageCmd.AddCommand(coreDownloadCmd)

	coreEraseEx := "  newtmgr -c olimex image coreerase\n"

	coreEraseCmd := &cobra.Command{
		Use:     "coreerase",
		Short:   "Erase core on target",
		Example: coreEraseEx,
		Run:     coreEraseCmd,
	}
	imageCmd.AddCommand(coreEraseCmd)

	return imageCmd
}
