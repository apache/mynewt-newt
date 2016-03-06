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

package main

import (
	"fmt"
	"strings"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"

	"github.com/spf13/cobra"
)

var NewtVersion string = "0.8.0"
var NewtLogLevel string = ""
var NewtVerbosity bool

func newtCmd() *cobra.Command {
	newtHelpText := cli.FormatHelp(`Newt allows you to create your own embedded 
		application based on the Mynewt operating system.  Newt provides both 
		build and package management in a single tool, which allows you to 
		compose an embedded application, and set of projects, and then build
		the necessary artifacts from those projects.  For more information 
		on the Mynewt operating system, please visit 
		https://mynewt.apache.org/.`)
	newtHelpText += "\n\n" + cli.FormatHelp(`Please use the newt help command, 
		and specify the name of the command you want help for, for help on 
		how to use a specific command`)
	newtHelpEx := "  newt\n"
	newtHelpEx += "  newt help [<command-name>]\n"
	newtHelpEx += "    For help on <command-name>.  If not specified, " +
		"print this message."

	newtCmd := &cobra.Command{
		Use:     "newt",
		Short:   "Newt is a tool to help you compose and build your own OS",
		Long:    newtHelpText,
		Example: newtHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	newtCmd.PersistentFlags().StringVarP(&NewtLogLevel, "loglevel", "l",
		"WARN", "Log level, defaults to WARN.")

	NewtLogLevel = strings.ToUpper(NewtLogLevel)

	versHelpText := cli.FormatHelp(`Display the Newt version number.`)
	versHelpEx := "  newt version"
	versCmd := &cobra.Command{
		Use:     "version",
		Short:   "Display the Newt version number.",
		Long:    versHelpText,
		Example: versHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Newt version: %s\n", NewtVersion)
		},
	}

	newtCmd.AddCommand(versCmd)

	return newtCmd
}

func main() {
	cmd := newtCmd()
	project.AddCommands(cmd)
	pkg.AddCommands(cmd)

	cmd.Execute()
}
