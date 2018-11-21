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

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/larva/cli"
	"mynewt.apache.org/newt/util"
)

var LarvaLogLevel log.Level
var larvaVersion = "0.0.1"

func main() {
	larvaHelpText := ""
	larvaHelpEx := ""

	logLevelStr := ""
	larvaCmd := &cobra.Command{
		Use:     "larva",
		Short:   "larva is a tool to help you compose and build your own OS",
		Long:    larvaHelpText,
		Example: larvaHelpEx,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logLevel, err := log.ParseLevel(logLevelStr)
			if err != nil {
				cli.LarvaUsage(nil, util.ChildNewtError(err))
			}
			LarvaLogLevel = logLevel

			if err := util.Init(LarvaLogLevel, "",
				util.VERBOSITY_DEFAULT); err != nil {

				cli.LarvaUsage(nil, err)
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	larvaCmd.PersistentFlags().StringVarP(&logLevelStr, "loglevel", "l",
		"WARN", "Log level")

	versHelpText := `Display the larva version number`
	versHelpEx := "  larva version"
	versCmd := &cobra.Command{
		Use:     "version",
		Short:   "Display the larva version number",
		Long:    versHelpText,
		Example: versHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s\n", larvaVersion)
		},
	}
	larvaCmd.AddCommand(versCmd)

	cli.AddImageCommands(larvaCmd)
	cli.AddMfgCommands(larvaCmd)

	larvaCmd.Execute()
}
