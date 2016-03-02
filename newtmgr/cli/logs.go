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

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/config"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/protocol"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/transport"
	"github.com/spf13/cobra"
)

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
	for i := 0; i < len(decodedResponse.Logs); i++ {
		fmt.Println(decodedResponse.Logs[i])
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
