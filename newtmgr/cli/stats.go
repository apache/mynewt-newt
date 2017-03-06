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

	"mynewt.apache.org/newt/newtmgr/protocol"

	"github.com/spf13/cobra"
)

func statsListRunCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	slr, err := protocol.NewStatsListReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	nmr, err := slr.Encode()
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

	slrsp, err := protocol.DecodeStatsListResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Println(slrsp.List)
	fmt.Printf("Return Code = %d\n", slrsp.ReturnCode)
}

func statsRunCmd(cmd *cobra.Command, args []string) {

	if len(args) == 0 {
		statsListRunCmd(cmd, args)
		return
	}
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	srr, err := protocol.NewStatsReadReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	srr.Name = args[0]

	nmr, err := srr.Encode()
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
			fmt.Printf("  %s: %d\n", k, v)
		}
	}
}

func statsCmd() *cobra.Command {
	statsHelpText := "Read statistics for the specified stats_name from a device"
	statsCmd := &cobra.Command{
		Use:   "stat [stats_name] -c <conn_profile>",
		Short: "Read statistics from a device",
		Long:  statsHelpText,
		Run:   statsRunCmd,
	}

	ListCmd := &cobra.Command{
		Use:   "list -c <conn_profile>",
		Short: "Read the list of Stats names from a device",
		Run:   statsListRunCmd,
	}

	statsCmd.AddCommand(ListCmd)

	return statsCmd
}
