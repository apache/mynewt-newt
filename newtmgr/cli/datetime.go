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

func dateTimeCmd(cmd *cobra.Command, args []string) {
	runner, err := getTargetCmdRunner()
	if err != nil {
		nmUsage(cmd, err)
	}

	dateTime, err := protocol.NewDateTime()
	if err != nil {
		nmUsage(cmd, err)
	}

	if len(args) > 0 {
		dateTime.DateTime = args[0]
	}
	nmr, err := dateTime.EncodeRequest()
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

	iRsp, err := protocol.DecodeDateTimeResponse(rsp.Data)
	if err != nil {
		nmUsage(cmd, err)
	}

	err_str := "Command: datetime\n" +
		"For writing datetime <argument>\n" +
		"Need to specify a datetime in RFC 3339 format\n" +
		"2016-03-02T22:44:00                  UTC time (implicit)\n" +
		"2016-03-02T22:44:00Z                 UTC time (explicit)\n" +
		"2016-03-02T22:44:00-08:00            PST timezone\n" +
		"2016-03-02T22:44:00.1                fractional seconds\n" +
		"2016-03-02T22:44:00.101+05:30        fractional seconds with timezone\n"

	if len(args) > 1 {
		nmUsage(cmd, util.NewNewtError(err_str))
	} else if len(args) == 1 {
		if iRsp.Return != 0 {
			nmUsage(cmd, util.NewNewtError(fmt.Sprintf("Return:%d\n%s",
				iRsp.Return, err_str)))
		} else {
			fmt.Println("Return:", iRsp.Return)
		}
	} else if len(args) == 0 {
		fmt.Println("Datetime(RFC 3339 format):", iRsp.DateTime)
	}
}

func dTimeCmd() *cobra.Command {
	dateTCmd := &cobra.Command{
		Use:   "datetime",
		Short: "Manage datetime on the device",
		Run:   dateTimeCmd,
	}

	return dateTCmd
}
