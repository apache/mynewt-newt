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

package builder

import (
	"fmt"

	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/newt/cli"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

const TARGET_TEST_NAME = "targets/unittest"

func buildRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, nil)
	}

	targetName := args[0]
	t := target.GetTargets()[targetName]
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("invalid target name"))
	}

	b, err := NewBuilder(t)
	if err != nil {
		fmt.Println(err)
	}
	err = b.Build()
	if err != nil {
		fmt.Println(err)
	}
}

func cleanRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, nil)
	}

	targetName := args[0]
	t := target.GetTargets()[targetName]
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("invalid target name"))
	}

	b, err := NewBuilder(t)
	if err != nil {
		fmt.Println(err)
	}
	err = b.Clean()
	if err != nil {
		fmt.Println(err)
	}
}

func testRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cli.NewtUsage(cmd, nil)
	}

	t := target.GetTargets()[TARGET_TEST_NAME]
	if t == nil {
		cli.NewtUsage(cmd, util.NewNewtError("can't find unit test target: "+
			TARGET_TEST_NAME))
	}

	b, err := NewBuilder(t)
	if err != nil {
		fmt.Println(err)
	}

	pkgString := args[0]
	dep, err := pkg.NewDependency(nil, pkgString)
	if err != nil {
		cli.NewtUsage(cmd, err)
	}
	if dep == nil {
		cli.NewtUsage(cmd, util.NewNewtError("invalid package name"))
	}
	pack := project.GetProject().ResolveDependency(dep)
	if pack == nil {
		cli.NewtUsage(cmd, util.NewNewtError("invalid package name"))
	}

	err = b.Test(pack.(*pkg.LocalPackage))
	if err != nil {
		fmt.Println(err)
	}
}

func AddCommands(cmd *cobra.Command) {
	buildHelpText := ""
	buildHelpEx := ""
	buildCmd := &cobra.Command{
		Use:     "build",
		Short:   "Commands for building targets",
		Long:    buildHelpText,
		Example: buildHelpEx,
		Run:     buildRunCmd,
	}

	cmd.AddCommand(buildCmd)

	cleanHelpText := ""
	cleanHelpEx := ""
	cleanCmd := &cobra.Command{
		Use:     "clean",
		Short:   "Commands for cleaning targets",
		Long:    cleanHelpText,
		Example: cleanHelpEx,
		Run:     cleanRunCmd,
	}

	cmd.AddCommand(cleanCmd)

	testHelpText := ""
	testHelpEx := ""
	testCmd := &cobra.Command{
		Use:     "test",
		Short:   "Executes unit tests for one or more packages",
		Long:    testHelpText,
		Example: testHelpEx,
		Run:     testRunCmd,
	}

	cmd.AddCommand(testCmd)
}
