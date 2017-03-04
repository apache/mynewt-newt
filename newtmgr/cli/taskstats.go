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

func taskStatsRunCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()
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
		fmt.Printf("  %8s %3s %3s %8s %8s %8s %8s %8s %8s\n",
			"task", "pri", "tid", "runtime", "csw", "stksz",
			"stkuse", "last_checkin", "next_checkin")
		for k, info := range tsrsp.Tasks {
			fmt.Printf("  %8s %3d %3d %8d %8d %8d %8d %8d %8d\n",
				k,
				int(info["prio"].(uint64)),
				int(info["tid"].(uint64)),
				int(info["runtime"].(uint64)),
				int(info["cswcnt"].(uint64)),
				int(info["stksiz"].(uint64)),
				int(info["stkuse"].(uint64)),
				int(info["last_checkin"].(uint64)),
				int(info["next_checkin"].(uint64)))
		}
	}
}

func taskStatsCmd() *cobra.Command {
	taskStatsCmd := &cobra.Command{
		Use:   "taskstats -c <conn_profile>",
		Short: "Read task statistics from a device",
		Run:   taskStatsRunCmd,
	}

	return taskStatsCmd
}
