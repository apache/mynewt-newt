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

func echoRunCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	echo, err := protocol.NewEcho()
	if err != nil {
		nmUsage(cmd, err)
	}

	if len(args) != 1 {
		nmUsage(cmd, nil)
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
	fmt.Println(ersp.Response)
}

func echoCmd() *cobra.Command {
	echoCmd := &cobra.Command{
		Use:   "echo <text> -c <conn_profile>",
		Short: "Send data to a device and display the echoed back data",
		Run:   echoRunCmd,
	}

	return echoCmd
}
