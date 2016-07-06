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
	"mynewt.apache.org/newt/newtmgr/protocol"
	"mynewt.apache.org/newt/util"
)

func splitStatusCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	split, err := protocol.NewSplit()
	if err != nil {
		nmUsage(cmd, err)
	}
	var nmr *protocol.NmgrReq
	if len(args) == 0 {
		nmr, err = split.EncoderReadRequest()
	} else if len(args) == 1 {
		b, err := protocol.ParseSplitMode(args[0])

		if err != nil {
			nmUsage(cmd, util.NewNewtError("Invalid Boolean Argument"))
		}
		split.Split = b
		nmr, err = split.EncoderWriteRequest()
	} else {
		nmUsage(cmd, nil)
		return
	}

	if err := runner.WriteReq(nmr); err != nil {
		nmUsage(cmd, err)
	}

	rsp, err := runner.ReadResp()
	if err != nil {
		nmUsage(cmd, err)
	}

	srsp, err := protocol.DecodeSplitReadResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	if len(args) == 0 {
		fmt.Printf("Split value is %s\n", srsp.Split)
		fmt.Printf("Split status is %s\n", srsp.Status)

	}
	if srsp.ReturnCode != 0 {
		fmt.Printf("Error executing split command: rc=%d\n", srsp.ReturnCode)
	}
}

func splitCmd() *cobra.Command {
	splitImgCmd := &cobra.Command{
		Use:   "split",
		Short: "Manage split images on remote instance",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
	}

	splitEx := "  newtmgr -c olimex image split 1\n"
	splitEx += "  newtmgr -c olimex image split 0\n"
	splitEx += "  newtmgr -c olimex image split\n"

	splitStatusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Erase core on target",
		Example: splitEx,
		Run:     splitStatusCmd,
	}
	splitImgCmd.AddCommand(splitStatusCmd)

	return splitImgCmd
}
