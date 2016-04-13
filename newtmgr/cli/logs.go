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

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/newtmgr/protocol"
	"mynewt.apache.org/newt/newtmgr/transport"
)

const (
	DEBUG    uint64 = 1
	INFO     uint64 = 2
	WARN     uint64 = 4
	ERROR    uint64 = 8
	CRITICAL uint64 = 10
	/* Upto 7 custom loglevels */
	PERUSER uint64 = 12
)

const (
	STREAM_LOG  uint64 = 0
	MEMORY_LOG  uint64 = 1
	STORAGE_LOG uint64 = 2
)

func LoglevelToString(ll uint64) string {
	s := ""
	switch ll {
	case DEBUG:
		s = "DEBUG"
	case INFO:
		s = "INFO"
	case WARN:
		s = "WARN"
	case ERROR:
		s = "ERROR"
	case CRITICAL:
		s = "CRITICAL"
	case PERUSER:
		s = "PERUSER"
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
			fmt.Println(fmt.Sprintf("%+v:> %+v usecs :%s: %s",
				decodedResponse.Logs[j].Entries[i].Index,
				decodedResponse.Logs[j].Entries[i].Timestamp,
				LoglevelToString(decodedResponse.Logs[j].Entries[i].Level),
				decodedResponse.Logs[j].Entries[i].Msg))
		}
	}
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

	return logsCmd
}
