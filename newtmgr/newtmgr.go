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

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/cli"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/protocol"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/transport"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
	"github.com/hashicorp/logutils"
	"github.com/spf13/cobra"
)

var ConnProfileName string
var LogLevel string = "WARN"

func setupLog() {
	filter := &logutils.LevelFilter{
		Levels: []logutils.LogLevel{"DEBUG", "VERBOSE", "INFO",
			"WARN", "ERROR"},
		MinLevel: logutils.LogLevel(LogLevel),
		Writer:   os.Stderr,
	}

	log.SetOutput(filter)
}

func nmUsage(cmd *cobra.Command, err error) {
	if err != nil {
		sErr := err.(*util.NewtError)
		fmt.Printf("ERROR: %s\n", err.Error())
		fmt.Fprintf(os.Stderr, "[DEBUG] %s", sErr.StackTrace)
	}

	if cmd != nil {
		cmd.Help()
	}

	os.Exit(1)
}

func connProfileAddCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := args[0]
	cp, err := cli.NewConnProfile(name)
	if err != nil {
		nmUsage(cmd, err)
	}

	for _, vdef := range args[1:] {
		s := strings.Split(vdef, "=")
		switch s[0] {
		case "name":
			cp.MyName = s[1]
		case "type":
			cp.MyType = s[1]
		case "connstring":
			cp.MyConnString = s[1]
		default:
			nmUsage(cmd, util.NewNewtError("Unknown variable "+s[0]))
		}
	}

	if err := cpm.AddConnProfile(cp); err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Connection profile %s successfully added\n", name)
}

func connProfileShowCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	cpList, err := cpm.GetConnProfileList()
	if err != nil {
		nmUsage(cmd, err)
	}

	found := false
	for _, cp := range cpList {
		// Print out the connection profile, if name is "" or name
		// matches cp.Name
		if name != "" && cp.Name() != name {
			continue
		}

		if !found {
			found = true
			fmt.Printf("Connection profiles: \n")
		}
		fmt.Printf("  %s: type=%s, connstring='%s'\n", cp.MyName, cp.MyType,
			cp.MyConnString)
	}

	if !found {
		if name == "" {
			fmt.Printf("No connection profiles found!\n")
		} else {
			fmt.Printf("No connection profiles found matching %s\n", name)
		}
	}
}

func connProfileDelCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := args[0]

	if err := cpm.DeleteConnProfile(name); err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Connection profile %s successfully deleted.\n", name)
}

func connProfileCmd() *cobra.Command {
	cpCmd := &cobra.Command{
		Use:   "conn",
		Short: "Manage newtmgr connection profiles",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a newtmgr connection profile",
		Run:   connProfileAddCmd,
	}
	cpCmd.AddCommand(addCmd)

	deleCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a newtmgr connection profile",
		Run:   connProfileDelCmd,
	}
	cpCmd.AddCommand(deleCmd)

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show newtmgr connection profiles",
		Run:   connProfileShowCmd,
	}
	cpCmd.AddCommand(showCmd)

	return cpCmd
}

func echoRunCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	echo, err := protocol.NewEcho()
	if err != nil {
		nmUsage(cmd, err)
	}

	echo.Message = args[0]

	nmr, err := echo.EncodeWriteRequest()
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

	ersp, err := protocol.DecodeEchoResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	fmt.Println(ersp.Message)
}

func echoCmd() *cobra.Command {
	echoCmd := &cobra.Command{
		Use:   "echo",
		Short: "Send data to remote endpoint using newtmgr, and receive data back",
		Run:   echoRunCmd,
	}

	return echoCmd
}

func taskStatsRunCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	srr, err := protocol.NewTaskStatsReadReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := srr.EncodeWriteRequest()
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

	tsrsp, err := protocol.DecodeTaskStatsReadResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Return Code = %d\n", tsrsp.ReturnCode)
	if tsrsp.ReturnCode == 0 {
		for k, info := range tsrsp.Tasks {
			fmt.Printf("  %s ", k)
			fmt.Printf("(prio=%d tid=%d runtime=%d cswcnt=%d stksize=%d "+
				"stkusage=%d last_checkin=%d next_checkin=%d)",
				int(info["prio"].(float64)),
				int(info["tid"].(float64)),
				int(info["runtime"].(float64)),
				int(info["cswcnt"].(float64)),
				int(info["stksiz"].(float64)),
				int(info["stkuse"].(float64)),
				int(info["last_checkin"].(float64)),
				int(info["next_checkin"].(float64)))
			fmt.Printf("\n")
		}
	}
}

func taskStatsCmd() *cobra.Command {
	taskStatsCmd := &cobra.Command{
		Use:   "taskstats",
		Short: "Read statistics from a remote endpoint",
		Run:   taskStatsRunCmd,
	}

	return taskStatsCmd
}

func mempoolStatsRunCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	srr, err := protocol.NewMempoolStatsReadReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := srr.EncodeWriteRequest()
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

	msrsp, err := protocol.DecodeMempoolStatsReadResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Return Code = %d\n", msrsp.ReturnCode)
	if msrsp.ReturnCode == 0 {
		for k, info := range msrsp.MPools {
			fmt.Printf("  %s ", k)
			fmt.Printf("(blksize=%d nblocks=%d nfree=%d)",
				int(info["blksiz"].(float64)),
				int(info["nblks"].(float64)),
				int(info["nfree"].(float64)))
			fmt.Printf("\n")
		}
	}
}

func mempoolStatsCmd() *cobra.Command {
	mempoolStatsCmd := &cobra.Command{
		Use:   "mpstats",
		Short: "Read statistics from a remote endpoint",
		Run:   mempoolStatsRunCmd,
	}

	return mempoolStatsCmd
}

func statsRunCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	srr, err := protocol.NewStatsReadReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	srr.Name = args[0]

	nmr, err := srr.EncodeWriteRequest()
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

	srrsp, err := protocol.DecodeStatsReadResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Return Code = %d\n", srrsp.ReturnCode)
	if srrsp.ReturnCode == 0 {
		fmt.Printf("Stats Name: %s\n", srrsp.Name)
		for k, v := range srrsp.Fields {
			fmt.Printf("  %s: %d\n", k, int(v.(float64)))
		}
	}
}

func statsCmd() *cobra.Command {
	statsCmd := &cobra.Command{
		Use:   "stat",
		Short: "Read statistics from a remote endpoint",
		Run:   statsRunCmd,
	}

	return statsCmd
}

func imageListCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
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

func echoCtrl(runner *protocol.CmdRunner, echoOn string) error {
	echoCtrl, err := protocol.NewEcho()
	if err != nil {
		return err
	}
	echoCtrl.Message = echoOn

	nmr, err := echoCtrl.EncodeEchoCtrl()
	if err != nil {
		return err
	}

	if err := runner.WriteReq(nmr); err != nil {
		return err
	}

	_, err = runner.ReadResp()
	if err != nil {
		return err
	}
	return nil
}

func imageUploadCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		nmUsage(cmd, util.NewNewtError("Need to specify image to upload"))
	}

	imageFile, err := ioutil.ReadFile(args[0])
	if err != nil {
		nmUsage(cmd, util.NewNewtError(err.Error()))
	}

	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
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
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
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
		fmt.Println("    Test image :", iRsp.Test)
		fmt.Println("    Main image :", iRsp.Main)
		fmt.Println("    Active img :", iRsp.Active)
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

	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
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

	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
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
		Use:   "list",
		Short: "Show target images",
		Run:   imageListCmd,
	}
	imageCmd.AddCommand(listCmd)

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

func configRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		nmUsage(cmd, util.NewNewtError("Need variable name"))
	}
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	config, err := protocol.NewConfig()
	if err != nil {
		nmUsage(cmd, err)
	}

	config.Name = args[0]
	if len(args) > 1 {
		config.Value = args[1]
	}
	nmr, err := config.EncodeRequest()
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

	configRsp, err := protocol.DecodeConfigResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	if configRsp.Value != "" {
		fmt.Printf("Value: %s\n", configRsp.Value)
	}
}

func configCmd() *cobra.Command {
	statsCmd := &cobra.Command{
		Use:   "config",
		Short: "Read or write config value on target",
		Run:   configRunCmd,
	}

	return statsCmd
}

func parseCmds() *cobra.Command {
	nmCmd := &cobra.Command{
		Use:   "newtmgr",
		Short: "Newtmgr helps you manage remote instances of the Mynewt OS.",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	nmCmd.PersistentFlags().StringVarP(&ConnProfileName, "conn", "c", "",
		"connection profile to use.")

	nmCmd.PersistentFlags().StringVarP(&LogLevel, "loglevel", "l", "",
		"log level to use (default WARN.)")

	nmCmd.AddCommand(connProfileCmd())
	nmCmd.AddCommand(echoCmd())
	nmCmd.AddCommand(imageCmd())
	nmCmd.AddCommand(statsCmd())
	nmCmd.AddCommand(taskStatsCmd())
	nmCmd.AddCommand(mempoolStatsCmd())
	nmCmd.AddCommand(configCmd())

	return nmCmd
}

func main() {
	cmd := parseCmds()
	setupLog()
	cmd.Execute()
}
