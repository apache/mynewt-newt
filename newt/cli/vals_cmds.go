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
	"strings"

	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/util"
)

func valsRunCmd(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		NewtUsage(cmd, nil)
	}

	allVals := [][]string{}
	for _, elemType := range args {
		vals, err := VarValues(elemType)
		if err != nil {
			NewtUsage(cmd, err)
		}

		allVals = append(allVals, vals)
	}

	for i, vals := range allVals {
		if i != 0 {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "\n")
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT, "%s names:\n", args[i])
		for _, val := range vals {
			util.StatusMessage(util.VERBOSITY_DEFAULT, "    %s\n", val)
		}
	}
}

func AddValsCommands(cmd *cobra.Command) {
	valsShortHelp := "Display valid values for the specified element type(s)"

	valsLongHelp := valsShortHelp + ".\n\nElement types:\n    " +
		strings.Join(VarTypes(), "\n    ")

	valsCmd := &cobra.Command{
		Use:   "vals <element-type> [element-types...]",
		Short: valsShortHelp,
		Long:  valsLongHelp,
		Run:   valsRunCmd,
	}

	cmd.AddCommand(valsCmd)
}
