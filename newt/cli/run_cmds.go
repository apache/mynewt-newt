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

	"mynewt.apache.org/newt/newt/image"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

func runRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	if useV1 && useV2 {
		NewtUsage(cmd, util.NewNewtError("Either -1, or -2, but not both"))
	}
	if useV2 {
		image.UseV1 = false
	} else {
		image.UseV1 = true
	}

	TryGetProject()

	b, err := TargetBuilderForTargetOrUnittest(args[0])
	if err != nil {
		NewtUsage(cmd, err)
	}

	testPkg := b.GetTestPkg()
	if testPkg != nil {
		b.InjectSetting("TESTUTIL_SYSTEM_ASSERT", "1")
		if err := b.SelfTestCreateExe(); err != nil {
			NewtUsage(nil, err)
		}
		if err := b.SelfTestDebug(); err != nil {
			NewtUsage(nil, err)
		}
	} else {
		var version string = ""
		if len(args) > 1 {
			version = args[1]
		} else {
			// If user did not provide version number and the target is not a
			// bootloader and doesn't run in the simulator, then ask the user
			// to enter a version or use 0 for default value.

			// Resolve to get the config values.
			res, err := b.Resolve()
			if err != nil {
				NewtUsage(nil, err)
			}
			features := res.Cfg.Features()

			if !features["BOOT_LOADER"] && !features["BSP_SIMULATED"] {
				version = "0"
				fmt.Println("Enter image version(default 0):")
				fmt.Scanf("%s\n", &version)
			}
		}
		if err := b.Build(); err != nil {
			NewtUsage(nil, err)
		}

		if len(version) > 0 {
			_, _, err = b.CreateImages(version, "", 0)
			if err != nil {
				NewtUsage(cmd, err)
			}

		}

		if err := b.Load(extraJtagCmd); err != nil {
			NewtUsage(nil, err)
		}

		if err := b.Debug(extraJtagCmd, true, noGDB_flag); err != nil {
			NewtUsage(nil, err)
		}
	}
}

func AddRunCommands(cmd *cobra.Command) {
	runHelpText := "Same as running\n" +
		" - build <target>\n" +
		" - create-image <target> <version>\n" +
		" - load <target>\n" +
		" - debug <target>\n\n" +
		"Note if version number is omitted, create-image step is skipped\n"
	runHelpEx := "  newt run <target-name> [<version>]\n"

	runCmd := &cobra.Command{
		Use:     "run",
		Short:   "build/create-image/download/debug <target>",
		Long:    runHelpText,
		Example: runHelpEx,
		Run:     runRunCmd,
	}

	runCmd.PersistentFlags().StringVarP(&extraJtagCmd, "extrajtagcmd", "", "",
		"Extra commands to send to JTAG software")
	runCmd.PersistentFlags().BoolVarP(&noGDB_flag, "noGDB", "n", false,
		"Do not start GDB from command line")
	runCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
		"Ignore flash overflow errors during image creation")
	runCmd.PersistentFlags().BoolVarP(&useV1,
		"1", "1", false, "Use old image header format")
	runCmd.PersistentFlags().BoolVarP(&useV2,
		"2", "2", false, "Use new image header format")

	cmd.AddCommand(runCmd)
	AddTabCompleteFn(runCmd, func() []string {
		return append(targetList(), unittestList()...)
	})
}
