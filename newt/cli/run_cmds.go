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
	"os"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/image"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

func runRunCmd(cmd *cobra.Command, args []string) {
	if err := project.Initialize(); err != nil {
		NewtUsage(cmd, err)
	}
	if len(args) < 1 {
		NewtUsage(cmd, util.NewNewtError("Must specify target"))
	}

	t := ResolveTarget(args[0])
	if t == nil {
		NewtUsage(cmd, util.NewNewtError("Invalid target name: "+args[0]))
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		NewtUsage(nil, err)
	}

	err = b.Build()
	if err != nil {
		NewtUsage(nil, err)
	}

	/*
	 * Run create-image if version number is specified. If no version number,
	 * remove .img which would'be been created. This so that download script
	 * will barf if it needs an image for this type of target, instead of
	 * downloading an older version.
	 */
	var app_img *image.Image
	var loader_img *image.Image
	if len(args) > 1 {
		if b.Loader == nil {
			err, app_img = CreateImage(b.App, args[1], "", 0, nil)
			if err != nil {
				NewtUsage(cmd, err)
			}
		} else {
			err, loader_img = CreateImage(b.Loader, args[1], "", 0, nil)
			if err != nil {
				NewtUsage(cmd, err)
			}
			err, app_img = CreateImage(b.App, args[1], "", 0, loader_img)
			if err != nil {
				NewtUsage(cmd, err)
			}

		}
	} else {
		os.Remove(b.App.AppImgPath())
		os.Remove(b.Loader.AppImgPath())
	}

	build_id := image.CreateBuildId(app_img, loader_img)

	err = image.CreateManifest(b, app_img, loader_img, build_id)
	if err != nil {
		NewtUsage(cmd, err)
	}

	err = b.Load()
	if err != nil {
		NewtUsage(cmd, err)
	}
	err = b.Debug()
	if err != nil {
		NewtUsage(cmd, err)
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
	cmd.AddCommand(runCmd)
}
