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
	"mynewt.apache.org/newt/util"

	"github.com/spf13/cobra"
)

func configRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		nmUsage(cmd, util.NewNewtError("Need variable name"))
	}

	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}
	defer runner.Conn.Close()

	config, err := protocol.NewConfig()
	if err != nil {
		nmUsage(cmd, err)
	}

	config.Name = args[0]
	if len(args) > 1 {
		config.Value = args[1]
	}
	nmr, err := config.EncodeRequest()
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

	configRsp, err := protocol.DecodeConfigResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}
	if configRsp.Value != "" {
		fmt.Printf("Value: %s\n", configRsp.Value)
	}
}

func configCmd() *cobra.Command {
	configCmdLongHelp := "Read or write a config value for <var-name> variable on " +
		"a device.\nSpecify a var-value to write a value to a device.\n"
	statsCmd := &cobra.Command{
		Use:   "config <var-name> [var-value] -c <conn_profile>",
		Short: "Read or write a config value on a device",
		Long:  configCmdLongHelp,
		Run:   configRunCmd,
	}

	return statsCmd
}
