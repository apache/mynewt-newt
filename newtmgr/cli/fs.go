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

	"mynewt.apache.org/newt/newtmgr/protocol"
	"mynewt.apache.org/newt/util"

	"github.com/spf13/cobra"
)

func fsUploadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		nmUsage(cmd, util.NewNewtError(
			"Need to specify source file and destination file to upload"))
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
	defer runner.Conn.Close()

	err = echoCtrl(runner, "0")
	if err != nil {
		nmUsage(cmd, err)
	}
	defer echoCtrl(runner, "1")
	var currOff uint32 = 0
	var cnt int = 0

	fileSz := uint32(len(file))

	for currOff < fileSz {
		fsUpload, err := protocol.NewFileUpload()
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

		fsUpload.Offset = currOff
		fsUpload.Size = fileSz
		fsUpload.Name = filename
		fsUpload.Data = file[currOff : currOff+blockSz]

		nmr, err := fsUpload.EncodeWriteRequest()
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

func fsDownloadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		nmUsage(cmd, util.NewNewtError(
			"Need to specify source file and destination file to download"))
	}

	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	var currOff uint32 = 0
	var cnt int = 0
	var fileSz uint32 = 1

	filename := args[0]
	if len(filename) > 64 {
		nmUsage(cmd, util.NewNewtError("Source filename too long"))
	}

	file, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		nmUsage(cmd, util.NewNewtError(fmt.Sprintf(
			"Cannot open file %s - %s", args[1], err.Error())))
	}

	for currOff < fileSz {
		fsDownload, err := protocol.NewFileDownload()
		if err != nil {
			nmUsage(cmd, err)
		}

		fsDownload.Offset = currOff
		fsDownload.Name = filename

		nmr, err := fsDownload.EncodeWriteRequest()
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

func fsCmd() *cobra.Command {
	fsCmd := &cobra.Command{
		Use:   "fs",
		Short: "Access files on a device",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
	}

	uploadEx := "  newtmgr -c olimex fs upload sample.lua /sample.lua\n"

	uploadCmd := &cobra.Command{
		Use:     "upload <src-filename> <dst-filename> -c <conn_profile>",
		Short:   "Upload file to a device",
		Example: uploadEx,
		Run:     fsUploadCmd,
	}
	fsCmd.AddCommand(uploadCmd)

	downloadEx := "  newtmgr -c olimex image download /cfg/mfg mfg.txt\n"

	downloadCmd := &cobra.Command{
		Use:     "download <src-filename> <dst-filename> -c <conn_profile>",
		Short:   "Download file from a device",
		Example: downloadEx,
		Run:     fsDownloadCmd,
	}
	fsCmd.AddCommand(downloadCmd)

	return fsCmd
}
