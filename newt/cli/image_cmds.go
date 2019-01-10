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
	"strconv"

	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/artifact/sec"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/imgprod"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

var useV1 bool
var useV2 bool
var encKeyFilename string

// @return                      keys, key ID, error
func parseKeyArgs(args []string) ([]sec.SignKey, uint8, error) {
	if len(args) == 0 {
		return nil, 0, nil
	}

	var keyId uint8
	var keyFilenames []string

	if len(args) == 1 {
		keyFilenames = append(keyFilenames, args[0])
	} else if useV1 {
		keyIdUint, err := strconv.ParseUint(args[1], 10, 8)
		if err != nil {
			return nil, 0, util.NewNewtError("Key ID must be between 0-255")
		}
		keyId = uint8(keyIdUint)
		keyFilenames = args[:1]
	} else {
		keyId = 0
		keyFilenames = args
	}

	keys, err := sec.ReadKeys(keyFilenames)
	if err != nil {
		return nil, 0, err
	}

	return keys, keyId, nil
}

func createImageRunCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		NewtUsage(cmd, util.NewNewtError("Must specify target and version"))
	}

	if useV1 && useV2 {
		NewtUsage(cmd, util.NewNewtError("Either -1, or -2, but not both"))
	}

	if !useV2 {
		useV1 = true
	}

	TryGetProject()

	targetName := args[0]
	t := ResolveTarget(targetName)
	if t == nil {
		NewtUsage(cmd, util.NewNewtError("Invalid target name: "+targetName))
	}

	ver, err := image.ParseVersion(args[1])
	if err != nil {
		NewtUsage(cmd, err)
	}

	b, err := builder.NewTargetBuilder(t)
	if err != nil {
		NewtUsage(nil, err)
	}

	keys, _, err := parseKeyArgs(args[2:])
	if err != nil {
		NewtUsage(cmd, err)
	}

	if err := b.Build(); err != nil {
		NewtUsage(nil, err)
	}

	if useV1 {
		err = imgprod.ProduceAllV1(b, ver, keys, encKeyFilename)
	} else {
		err = imgprod.ProduceAll(b, ver, keys, encKeyFilename)
	}
	if err != nil {
		NewtUsage(nil, err)
	}
}

func AddImageCommands(cmd *cobra.Command) {
	createImageHelpText := "Create an image by adding an image header to the " +
		"binary file created for <target-name>. Version number in the header " +
		"is set to be <version>.\n\n"

	createImageHelpText += "To use version 1 of image format, specify -1 on " +
		"command line.\n"
	createImageHelpText += "To sign version 1 of the image format give private " +
		"key as <signing-key> and an optional key-id.\n\n"
	createImageHelpText += "To use version 2 of image format, specify -2 on " +
		"command line.\n"
	createImageHelpText += "To sign version 2 of the image format give private " +
		"key as <signing-key> (no key-id needed).\n\n"

	createImageHelpText += "Default image format is version 1.\n"

	createImageHelpText += "To encrypt the image, specify -e passing it a public" +
		"key\n\n"

	createImageHelpEx := "  newt create-image my_target1 1.3.0\n"
	createImageHelpEx += "  newt create-image my_target1 1.3.0.3\n"
	createImageHelpEx += "  newt create-image my_target1 1.3.0.3 private.pem\n"
	createImageHelpEx +=
		"  newt create-image -2 my_target1 1.3.0.3 private-1.pem private-2.pem\n"

	createImageCmd := &cobra.Command{
		Use: "create-image <target-name> <version> [signing-key-1] " +
			"[signing-key-2] [...]",
		Short:   "Add image header to target binary",
		Long:    createImageHelpText,
		Example: createImageHelpEx,
		Run:     createImageRunCmd,
	}

	createImageCmd.PersistentFlags().BoolVarP(&newtutil.NewtForce,
		"force", "f", false,
		"Ignore flash overflow errors during image creation")
	createImageCmd.PersistentFlags().BoolVar(&image.UseRsaPss,
		"rsa-pss", false,
		"Use RSA-PSS instead of PKCS#1 v1.5 for RSA sig. "+
			"Meaningful for version 1 image format.")
	createImageCmd.PersistentFlags().BoolVarP(&useV1,
		"1", "1", false, "Use old image header format")
	createImageCmd.PersistentFlags().BoolVarP(&useV2,
		"2", "2", false, "Use new image header format")
	createImageCmd.PersistentFlags().StringVarP(&encKeyFilename,
		"encrypt", "e", "", "Encrypt image using this public key")

	cmd.AddCommand(createImageCmd)
	AddTabCompleteFn(createImageCmd, targetList)

	resignImageHelpText :=
		"This command is obsolete; use the `larva` tool to resign images."

	resignImageCmd := &cobra.Command{
		Use:   "resign-image",
		Short: "Obsolete",
		Long:  resignImageHelpText,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cmd.AddCommand(resignImageCmd)
}
