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
	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/man"
)

func manBuildRunCmd(cmd *cobra.Command, args []string) {
	proj := TryGetProject()
	err := man.BuildManPages(proj)
	if err != nil {
		NewtUsage(nil, err)
	}
}

func manRunCmd(cmd *cobra.Command, args []string) {
	proj := TryGetProject()
	err := man.RunMan(proj, args)
	if err != nil {
		NewtUsage(nil, err)
	}
}

func aproposRunCmd(cmd *cobra.Command, args []string) {
	proj := TryGetProject()
	err := man.RunApropos(proj, args)
	if err != nil {
		NewtUsage(nil, err)
	}
}

func AddManCommands(cmd *cobra.Command) {
	manBuildCmd := &cobra.Command{
		Use:   "man-build",
		Short: "Build man pages",
		Run:   manBuildRunCmd,
	}

	cmd.AddCommand(manBuildCmd)
	AddTabCompleteFn(manBuildCmd, func() []string {
		return append(targetList(), "all")
	})

	manCmd := &cobra.Command{
		Use:   "man <feature-name>",
		Short: "Browse the man-page for given argument",
		Run: func(cmd *cobra.Command, args []string) {
			manRunCmd(cmd, args)
		},
	}

	cmd.AddCommand(manCmd)
	AddTabCompleteFn(manCmd, func() []string {
		return append(append(targetList(), unittestList()...), "all")
	})

	aproposCmd := &cobra.Command{
		Use:   "apropos <search-expression>",
		Short: "Search manual page names and descriptions",
		Run: func(cmd *cobra.Command, args []string) {
			aproposRunCmd(cmd, args)
		},
	}

	cmd.AddCommand(aproposCmd)
	AddTabCompleteFn(aproposCmd, func() []string {
		return append(append(targetList(), unittestList()...), "all")
	})
}
