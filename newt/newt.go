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
	"os"
	"runtime"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

var NewtLogLevel log.Level
var newtSilent bool
var newtQuiet bool
var newtVerbose bool
var newtLogFile string
var newtNumJobs int
var newtHelp bool

func newtDfltNumJobs() int {
	maxProcs := runtime.GOMAXPROCS(0)
	numCpu := runtime.NumCPU()

	var numJobs int
	if maxProcs < numCpu {
		numJobs = maxProcs
	} else {
		numJobs = numCpu
	}

	return numJobs
}

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

	logLevelStr := ""
	newtCmd := &cobra.Command{
		Use:     "newt",
		Short:   "Newt is a tool to help you compose and build your own OS",
		Long:    newtHelpText,
		Example: newtHelpEx,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			verbosity := util.VERBOSITY_DEFAULT
			if newtSilent {
				verbosity = util.VERBOSITY_SILENT
			} else if newtQuiet {
				verbosity = util.VERBOSITY_QUIET
			} else if newtVerbose {
				verbosity = util.VERBOSITY_VERBOSE
			}

			var err error
			NewtLogLevel, err = log.ParseLevel(logLevelStr)
			if err != nil {
				cli.NewtUsage(nil, util.NewNewtError(err.Error()))
			}

			err = util.Init(NewtLogLevel, newtLogFile, verbosity)
			if err != nil {
				cli.NewtUsage(nil, err)
			}

			newtutil.NewtNumJobs = newtNumJobs
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	newtCmd.PersistentFlags().BoolVarP(&newtVerbose, "verbose", "v", false,
		"Enable verbose output when executing commands")
	newtCmd.PersistentFlags().BoolVarP(&newtQuiet, "quiet", "q", false,
		"Be quiet; only display error output")
	newtCmd.PersistentFlags().BoolVarP(&newtSilent, "silent", "s", false,
		"Be silent; don't output anything")
	newtCmd.PersistentFlags().StringVarP(&logLevelStr, "loglevel", "l",
		"WARN", "Log level")
	newtCmd.PersistentFlags().StringVarP(&newtLogFile, "outfile", "o",
		"", "Filename to tee output to")
	newtCmd.PersistentFlags().IntVarP(&newtNumJobs, "jobs", "j",
		newtDfltNumJobs(), "Number of concurrent build jobs")
	newtCmd.PersistentFlags().BoolVarP(&newtHelp, "help", "h",
		false, "Help for newt commands")

	versHelpText := cli.FormatHelp(`Display the Newt version number`)
	versHelpEx := "  newt version"
	versCmd := &cobra.Command{
		Use:     "version",
		Short:   "Display the Newt version number",
		Long:    versHelpText,
		Example: versHelpEx,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s\n", newtutil.NewtVersionStr)
		},
	}

	newtCmd.AddCommand(versCmd)

	return newtCmd
}

func main() {
	cmd := newtCmd()

	cli.AddBuildCommands(cmd)
	cli.AddCompleteCommands(cmd)
	cli.AddImageCommands(cmd)
	cli.AddPackageCommands(cmd)
	cli.AddProjectCommands(cmd)
	cli.AddRunCommands(cmd)
	cli.AddTargetCommands(cmd)
	cli.AddValsCommands(cmd)
	cli.AddMfgCommands(cmd)

	/* only pass the first two args to check for complete command */
	if len(os.Args) > 2 {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		bc, _, err := cmd.Find(os.Args[1:2])
		tmpArgs := os.Args
		os.Args = tmpArgs[0:2]

		if err == nil && bc.Name() == "complete" {
			cli.GenerateTabCompleteValues()
			bc.Execute()
			return
		}
		os.Args = tmpArgs
		cmd.SilenceErrors = false
		cmd.SilenceUsage = false
	}

	cmd.Execute()
}
