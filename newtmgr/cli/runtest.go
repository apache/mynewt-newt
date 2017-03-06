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
)

func runCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run test procedures on a device",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
	}

	runtestEx := "  newtmgr -c conn run test all 201612161220"

	runTestHelpText := "Run tests on a device. Specify a testname to run a "
	runTestHelpText += "specific test. All tests are\nrun if \"all\" or no "
	runTestHelpText += "testname is specified. If a token-value is "
	runTestHelpText += "specified, the\nvalue is output on the log messages.\n"
	runTestCmd := &cobra.Command{
		Use:     "test [all | testname] [token-value] -c <conn_profile>",
		Short:   "Run tests on a device",
		Long:    runTestHelpText,
		Example: runtestEx,
		Run:     runTestCmd,
	}
	runCmd.AddCommand(runTestCmd)

	runListCmd := &cobra.Command{
		Use:   "list -c <conn_profile>",
		Short: "List registered tests on a device",
		Run:   runListCmd,
	}
	runCmd.AddCommand(runListCmd)

	return runCmd
}

func runTestCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	req, err := protocol.NewRunTestReq()
	if err != nil {
		nmUsage(cmd, err)
	}

	if len(args) > 0 {
		req.Testname = args[0]
		if len(args) > 1 {
			req.Token = args[1]
		} else {
			req.Token = ""
		}
	} else {
		/*
		 * If nothing specified, turn on "all" by default
		 * There is no default token.
		 */
		req.Testname = "all"
		req.Token = ""
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

	decodedResponse, err := protocol.DecodeRunTestResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	if decodedResponse.ReturnCode != 0 {
		fmt.Printf("Return Code = %d\n", decodedResponse.ReturnCode)
	}
}

func runListCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	defer runner.Conn.Close()
	req, err := protocol.NewRunListReq()
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

	decodedResponse, err := protocol.DecodeRunListResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	fmt.Println(decodedResponse.List)
	if decodedResponse.ReturnCode != 0 {
		fmt.Printf("Return Code = %d\n", decodedResponse.ReturnCode)
	}
}
