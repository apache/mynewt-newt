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
	"strconv"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/newtmgr/protocol"
	"mynewt.apache.org/newt/newtmgr/transport"
)

const (
	LEVEL_DEBUG    uint64 = 0
	LEVEL_INFO     uint64 = 1
	LEVEL_WARN     uint64 = 2
	LEVEL_ERROR    uint64 = 3
	LEVEL_CRITICAL uint64 = 4
	/* Upto 7 custom loglevels */
	LEVEL_MAX uint64 = 255
)

const (
	STREAM_LOG  uint64 = 0
	MEMORY_LOG  uint64 = 1
	STORAGE_LOG uint64 = 2
)

const (
	MODULE_DEFAULT     uint64 = 0
	MODULE_OS          uint64 = 1
	MODULE_NEWTMGR     uint64 = 2
	MODULE_NIMBLE_CTLR uint64 = 3
	MODULE_NIMBLE_HOST uint64 = 4
	MODULE_NFFS        uint64 = 5
	MODULE_MAX         uint64 = 255
)

func LogModuleToString(lm uint64) string {
	s := ""
	switch lm {
	case MODULE_DEFAULT:
		s = "DEFAULT"
	case MODULE_OS:
		s = "OS"
	case MODULE_NEWTMGR:
		s = "NEWTMGR"
	case MODULE_NIMBLE_CTLR:
		s = "NIMBLE_CTLR"
	case MODULE_NIMBLE_HOST:
		s = "NIMBLE_HOST"
	case MODULE_NFFS:
		s = "NFFS"
	default:
		s = "CUSTOM"
	}
	return s
}

func LoglevelToString(ll uint64) string {
	s := ""
	switch ll {
	case LEVEL_DEBUG:
		s = "DEBUG"
	case LEVEL_INFO:
		s = "INFO"
	case LEVEL_WARN:
		s = "WARN"
	case LEVEL_ERROR:
		s = "ERROR"
	case LEVEL_CRITICAL:
		s = "CRITICAL"
	default:
		s = "CUSTOM"
	}
	return s
}

func LogTypeToString(lt uint64) string {
	s := ""
	switch lt {
	case STREAM_LOG:
		s = "STREAM"
	case MEMORY_LOG:
		s = "MEMORY"
	case STORAGE_LOG:
		s = "STORAGE"
	default:
		s = "UNDEFINED"
	}
	return s
}

func logsShowCmd(cmd *cobra.Command, args []string) {
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
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	req, err := protocol.NewLogsShowReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	if len(args) > 0 {
		req.Name = args[0]
		if len(args) > 1 {
			req.Timestamp, err = strconv.ParseInt(args[1], 0, 64)
			if len(args) > 2 {
				req.Index, err = strconv.ParseUint(args[2], 0, 64)
			}
		}

		if err != nil {
			nmUsage(cmd, err)
		}
	}

	nmr, err := req.Encode()
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

	decodedResponse, err := protocol.DecodeLogsShowResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	for j := 0; j < len(decodedResponse.Logs); j++ {

		fmt.Println("Name:", decodedResponse.Logs[j].Name)
		fmt.Println("Type:", LogTypeToString(decodedResponse.Logs[j].Type))

		for i := 0; i < len(decodedResponse.Logs[j].Entries); i++ {
			fmt.Println(fmt.Sprintf("%+v:> %+v usecs :%s:%s:%s",
				decodedResponse.Logs[j].Entries[i].Index,
				decodedResponse.Logs[j].Entries[i].Timestamp,
				LogModuleToString(decodedResponse.Logs[j].Entries[i].Module),
				LoglevelToString(decodedResponse.Logs[j].Entries[i].Level),
				decodedResponse.Logs[j].Entries[i].Msg))
		}
	}

	fmt.Printf("Return Code = %d\n", decodedResponse.ReturnCode)
}

func logsModuleListCmd(cmd *cobra.Command, args []string) {
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
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	req, err := protocol.NewLogsModuleListReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := req.Encode()
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

	decodedResponse, err := protocol.DecodeLogsModuleListResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Println(decodedResponse.Map)
	fmt.Printf("Return Code = %d\n", decodedResponse.ReturnCode)

}

func logsListCmd(cmd *cobra.Command, args []string) {
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
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	req, err := protocol.NewLogsListReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := req.Encode()
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

	decodedResponse, err := protocol.DecodeLogsListResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Println(decodedResponse.List)
	fmt.Printf("Return Code = %d\n", decodedResponse.ReturnCode)
}

func logsLevelListCmd(cmd *cobra.Command, args []string) {
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
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	req, err := protocol.NewLogsLevelListReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := req.Encode()
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

	decodedResponse, err := protocol.DecodeLogsLevelListResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Println(decodedResponse.Map)
	fmt.Printf("Return Code = %d\n", decodedResponse.ReturnCode)
}

func logsClearCmd(cmd *cobra.Command, args []string) {
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
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	req, err := protocol.NewLogsClearReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := req.Encode()
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

	decodedResponse, err := protocol.DecodeLogsClearResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Return Code = %d\n", decodedResponse.ReturnCode)
}

func logsCmd() *cobra.Command {
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Handles logs on remote instance",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show logs on target",
		Run:   logsShowCmd,
	}
	logsCmd.AddCommand(showCmd)

	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear logs on target",
		Run:   logsClearCmd,
	}
	logsCmd.AddCommand(clearCmd)

	moduleListCmd := &cobra.Command{
		Use:   "module_list",
		Short: "Module List Command",
		Run:   logsModuleListCmd,
	}
	logsCmd.AddCommand(moduleListCmd)

	levelListCmd := &cobra.Command{
		Use:   "level_list",
		Short: "Level List Command",
		Run:   logsLevelListCmd,
	}

	logsCmd.AddCommand(levelListCmd)

	ListCmd := &cobra.Command{
		Use:   "log_list",
		Short: "Log List Command",
		Run:   logsListCmd,
	}

	logsCmd.AddCommand(ListCmd)
	return logsCmd
}
