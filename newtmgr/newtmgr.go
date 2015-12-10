/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package main

import (
	"fmt"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/cli"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/protocol"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/newtmgr/transport"
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var ConnProfileName string

func nmUsage(cmd *cobra.Command, err error) {
	if err != nil {
		sErr := err.(*util.NewtError)
		fmt.Fprintf(os.Stderr, "[DEBUG] %s", sErr.StackTrace)
	}

	if cmd != nil {
		cmd.Help()
	}

	os.Exit(1)
}

func connProfileAddCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	name := args[0]
	cp, err := cli.NewConnProfile(name)
	if err != nil {
		nmUsage(cmd, err)
	}

	for _, vdef := range args[1:] {
		s := strings.Split(vdef, "=")
		switch s[0] {
		case "name":
			cp.Name = s[1]
		case "type":
			cp.Type = s[1]
		case "connstring":
			cp.ConnString = s[1]
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
	cpm, err := cli.NewCpMgr()
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
		if name != "" && cp.Name != name {
			continue
		}

		if !found {
			found = true
			fmt.Printf("Connection profiles: \n")
		}
		fmt.Printf("  %s: type=%s, connstring='%s'\n", cp.Name, cp.Type,
			cp.ConnString)
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
	cpm, err := cli.NewCpMgr()
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

func statShowCmd(cmd *cobra.Command, args []string) {
	cpm, err := cli.NewCpMgr()
	if err != nil {
		nmUsage(cmd, err)
	}

	cp, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		nmUsage(cmd, err)
	}

	conn, err := transport.NewConn(cp)
	if err != nil {
		nmUsage(cmd, err)
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		nmUsage(cmd, err)
	}

	stats, err := runner.Read(protocol.Stats)
	if err != nil {
		nmUsage(cmd, err)
	}

	stats.Display()
}

func statCmd() *cobra.Command {
	statCmd := &cobra.Command{
		Use:   "stat",
		Short: "Display statistics from a remote newtmgr device",
		Run:   statShowCmd,
	}

	return statCmd
}

func parseCmds() *cobra.Command {
	nmCmd := &cobra.Command{
		Use:   "newtmgr",
		Short: "Newtmgr helps you manage remote instances of the Mynewt OS.",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	nmCmd.PersistentFlags().StringVar(&ConnProfileName, "conn", "c", "",
		"connection profile to use.")

	nmCmd.AddCommand(connProfileCmd())
	nmCmd.AddCommand(statCmd())

	return nmCmd
}

func main() {
	cmd := parseCmds()
	cmd.Execute()
}
