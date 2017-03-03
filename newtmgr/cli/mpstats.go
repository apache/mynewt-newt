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

func mempoolStatsRunCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

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
		fmt.Printf("%32s %5s %4s %4s %4s\n", "name", "blksz",
			"cnt", "free", "min")
		for k, info := range msrsp.MPools {
			fmt.Printf("%32s %5d %4d %4d %4d\n", k,
				int(info["blksiz"].(uint64)),
				int(info["nblks"].(uint64)),
				int(info["nfree"].(uint64)),
				int(info["min"].(uint64)))
		}
	}
}

func mempoolStatsCmd() *cobra.Command {
	mempoolStatsCmd := &cobra.Command{
		Use:   "mpstats -c <conn_profile>",
		Short: "Read memory pool statistics from a device",
		Run:   mempoolStatsRunCmd,
	}

	return mempoolStatsCmd
}
