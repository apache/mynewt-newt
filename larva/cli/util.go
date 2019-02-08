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
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/util"
)

var OptOutFilename string
var OptInPlace bool

func LarvaUsage(cmd *cobra.Command, err error) {
	if err != nil {
		sErr := err.(*util.NewtError)
		log.Debugf("%s", sErr.StackTrace)
		fmt.Fprintf(os.Stderr, "Error: %s\n", sErr.Text)
	}

	if cmd != nil {
		fmt.Printf("%s - ", cmd.Name())
		cmd.Help()
	}
	os.Exit(1)
}

func CalcOutFilename(inFilename string) (string, error) {
	if OptOutFilename != "" {
		if OptInPlace {
			return "", util.FmtNewtError(
				"Only one of --outfile (-o) or --inplace (-i) options allowed")
		}

		return OptOutFilename, nil
	}

	if !OptInPlace {
		return "", util.FmtNewtError(
			"--outfile (-o) or --inplace (-i) option required")
	}

	return inFilename, nil
}

func CopyDir(src string, dst string) error {
	if err := util.CopyDir(src, dst); err != nil {
		return util.FmtNewtError(
			"Failed to copy directory \"%s\" to \"%s\": %s",
			src, dst, err.Error())
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Copied directory \"%s\" to \"%s\"\n", src, dst)
	return nil
}

func EnsureOutDir(inDir, outDir string) error {
	if inDir != outDir {
		// Not an in-place operation; copy input directory.
		if err := CopyDir(inDir, outDir); err != nil {
			return err
		}
	}

	return nil
}

func CopyFile(src string, dst string) error {
	if err := util.CopyFile(src, dst); err != nil {
		return util.FmtNewtError(
			"Failed to copy file \"%s\" to \"%s\": %s",
			src, dst, err.Error())
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Copied file \"%s\" to \"%s\"\n", src, dst)
	return nil
}

func WriteFile(data []byte, filename string) error {
	if err := ioutil.WriteFile(filename, data, os.ModePerm); err != nil {
		return util.FmtNewtError(
			"Failed to write file \"%s\": %s", filename, err.Error())
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Wrote file \"%s\"\n", filename)
	return nil
}
