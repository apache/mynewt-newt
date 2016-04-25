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
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/newtmgr/protocol"
	"mynewt.apache.org/newt/newtmgr/transport"
	"mynewt.apache.org/newt/util"
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

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(nil, err)
	}

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
	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(nil, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
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
	for hash, ver := range iRsp.Images {
		fmt.Printf(" %8s %s\n", ver, hash)
	}
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

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(nil, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}
	err = echoCtrl(runner, "0")
	if err != nil {
		nmUsage(cmd, err)
	}
	var currOff uint32 = 0
	imageSz := uint32(len(imageFile))

	for currOff < imageSz {
		imageUpload, err := protocol.NewImageUpload()
		if err != nil {
			nmUsage(cmd, err)
		}

		blockSz := imageSz - currOff
		if blockSz > 36 {
			blockSz = 36
		}

		imageUpload.Offset = currOff
		imageUpload.Size = imageSz
		imageUpload.Data = imageFile[currOff : currOff+blockSz]

		nmr, err := imageUpload.EncodeWriteRequest()
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

		ersp, err := protocol.DecodeImageUploadResponse(rsp.Data)
		if err != nil {
			nmUsage(cmd, err)
		}
		currOff = ersp.Offset
		fmt.Println(currOff)
	}
	err = echoCtrl(runner, "1")
	if err != nil {
		nmUsage(cmd, err)
	}
	fmt.Println("Done")
}

func imageBootCmd(cmd *cobra.Command, args []string) {
	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(nil, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
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
	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(nil, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
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

	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(nil, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}
	err = echoCtrl(runner, "0")
	if err != nil {
		nmUsage(cmd, err)
	}
	var currOff uint32 = 0
	var cnt int = 0

	fileSz := uint32(len(file))

	for currOff < fileSz {
		fileUpload, err := protocol.NewFileUpload()
		if err != nil {
			nmUsage(cmd, err)
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
			nmUsage(cmd, err)
		}

		if err := runner.WriteReq(nmr); err != nil {
			nmUsage(cmd, err)
		}

		rsp, err := runner.ReadResp()
		if err != nil {
			nmUsage(cmd, err)
		}

		ersp, err := protocol.DecodeFileUploadResponse(rsp.Data)
		if err != nil {
			nmUsage(cmd, err)
		}
		currOff = ersp.Offset
		cnt++
		fmt.Println(cnt, currOff)
	}
	err = echoCtrl(runner, "1")
	if err != nil {
		nmUsage(cmd, err)
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

	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(nil, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
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

func imageCmd() *cobra.Command {
	imageCmd := &cobra.Command{
		Use:   "image",
		Short: "Manage images on remote instance",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
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

	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload image to target",
		Run:   imageUploadCmd,
	}
	imageCmd.AddCommand(uploadCmd)

	bootCmd := &cobra.Command{
		Use:   "boot",
		Short: "Which image to boot",
		Run:   imageBootCmd,
	}
	imageCmd.AddCommand(bootCmd)

	boot2Cmd := &cobra.Command{
		Use:   "boot2",
		Short: "Which image to boot",
		Run:   imageBoot2Cmd,
	}
	imageCmd.AddCommand(boot2Cmd)

	fileUploadCmd := &cobra.Command{
		Use:   "fileupload",
		Short: "Upload file to target",
		Run:   fileUploadCmd,
	}
	imageCmd.AddCommand(fileUploadCmd)

	fileDownloadCmd := &cobra.Command{
		Use:   "filedownload",
		Short: "Download file from target",
		Run:   fileDownloadCmd,
	}
	imageCmd.AddCommand(fileDownloadCmd)

	return imageCmd
}
