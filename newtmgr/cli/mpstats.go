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

func mempoolStatsRunCmd(cmd *cobra.Command, args []string) {
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
