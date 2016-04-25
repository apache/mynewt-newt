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
	"strings"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/util"
)

func connProfileAddCmd(cmd *cobra.Command, args []string) {
	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := args[0]
	cp, err := config.NewConnProfile(name)
	if err != nil {
		nmUsage(cmd, err)
	}

	for _, vdef := range args[1:] {
		s := strings.Split(vdef, "=")
		switch s[0] {
		case "name":
			cp.MyName = s[1]
		case "type":
			cp.MyType = s[1]
		case "connstring":
			cp.MyConnString = s[1]
		default:
			nmUsage(cmd, util.NewNewtError("Unknown variable "+s[0]))
		}
	}

	if err := cpm.AddConnProfile(cp); err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Connection profile %s successfully added\n", name)
}

func connProfileShowCmd(cmd *cobra.Command, args []string) {
	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	cpList, err := cpm.GetConnProfileList()
	if err != nil {
		nmUsage(cmd, err)
	}

	found := false
	for _, cp := range cpList {
		// Print out the connection profile, if name is "" or name
		// matches cp.Name
		if name != "" && cp.Name() != name {
			continue
		}

		if !found {
			found = true
			fmt.Printf("Connection profiles: \n")
		}
		fmt.Printf("  %s: type=%s, connstring='%s'\n", cp.MyName, cp.MyType,
			cp.MyConnString)
	}

	if !found {
		if name == "" {
			fmt.Printf("No connection profiles found!\n")
		} else {
			fmt.Printf("No connection profiles found matching %s\n", name)
		}
	}
}

func connProfileDelCmd(cmd *cobra.Command, args []string) {
	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := args[0]

	if err := cpm.DeleteConnProfile(name); err != nil {
		nmUsage(cmd, err)
	}

	fmt.Printf("Connection profile %s successfully deleted.\n", name)
}

func connProfileCmd() *cobra.Command {
	cpCmd := &cobra.Command{
		Use:   "conn",
		Short: "Manage newtmgr connection profiles",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a newtmgr connection profile",
		Run:   connProfileAddCmd,
	}
	cpCmd.AddCommand(addCmd)

	deleCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a newtmgr connection profile",
		Run:   connProfileDelCmd,
	}
	cpCmd.AddCommand(deleCmd)

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show newtmgr connection profiles",
		Run:   connProfileShowCmd,
	}
	cpCmd.AddCommand(showCmd)

	return cpCmd
}
