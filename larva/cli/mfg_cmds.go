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

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/artifact/manifest"
	"mynewt.apache.org/newt/larva/mfg"
	"mynewt.apache.org/newt/util"
)

var optDeviceNum int

func readManifest(filename string) (manifest.Manifest, error) {
	man, err := manifest.ReadManifest(filename)
	if err != nil {
		return man, err
	}

	log.Debugf("Successfully read manifest %s", filename)
	return man, nil
}

func readFlashAreas(manifestFilename string) ([]flash.FlashArea, error) {
	man, err := readManifest(manifestFilename)
	if err != nil {
		return nil, err
	}

	areas := flash.SortFlashAreasByDevOff(man.FlashAreas)

	overlaps, conflicts := flash.DetectErrors(areas)
	if len(overlaps) > 0 || len(conflicts) > 0 {
		return nil, util.NewNewtError(flash.ErrorText(overlaps, conflicts))
	}

	if err := mfg.VerifyAreas(areas, optDeviceNum); err != nil {
		return nil, err
	}

	log.Debugf("Successfully read flash areas: %+v", areas)
	return areas, nil
}

func createMfgMap(binDir string, areas []flash.FlashArea) (mfg.MfgMap, error) {
	mm := mfg.MfgMap{}

	for _, area := range areas {
		filename := fmt.Sprintf("%s/%s.bin", binDir, area.Name)
		bin, err := ioutil.ReadFile(filename)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, util.ChildNewtError(err)
			}
		} else {
			mm[area.Name] = bin
		}
	}

	return mm, nil
}

func runSplitCmd(cmd *cobra.Command, args []string) {
	if len(args) < 3 {
		LarvaUsage(cmd, nil)
	}

	imgFilename := args[0]
	manFilename := args[1]
	outDir := args[2]

	mfgBin, err := ioutil.ReadFile(imgFilename)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Failed to read manufacturing image: %s", err.Error()))
	}

	areas, err := readFlashAreas(manFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	mm, err := mfg.Split(mfgBin, optDeviceNum, areas)
	if err != nil {
		LarvaUsage(nil, err)
	}

	if err := os.Mkdir(outDir, os.ModePerm); err != nil {
		LarvaUsage(nil, util.ChildNewtError(err))
	}

	for name, data := range mm {
		filename := fmt.Sprintf("%s/%s.bin", outDir, name)
		if err := ioutil.WriteFile(filename, data, os.ModePerm); err != nil {
			LarvaUsage(nil, util.ChildNewtError(err))
		}
	}
}

func runJoinCmd(cmd *cobra.Command, args []string) {
	if len(args) < 3 {
		LarvaUsage(cmd, nil)
	}

	binDir := args[0]
	manFilename := args[1]
	outFilename := args[2]

	areas, err := readFlashAreas(manFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	mm, err := createMfgMap(binDir, areas)
	if err != nil {
		LarvaUsage(nil, err)
	}

	mfgBin, err := mfg.Join(mm, 0xff, areas)
	if err != nil {
		LarvaUsage(nil, err)
	}

	if err := ioutil.WriteFile(outFilename, mfgBin, os.ModePerm); err != nil {
		LarvaUsage(nil, util.ChildNewtError(err))
	}
}

func runBootKeyCmd(cmd *cobra.Command, args []string) {
	if len(args) < 3 {
		LarvaUsage(cmd, nil)
	}

	sec0Filename := args[0]
	okeyFilename := args[1]
	nkeyFilename := args[2]

	outFilename, err := CalcOutFilename(sec0Filename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	sec0, err := ioutil.ReadFile(sec0Filename)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Failed to read sec0 file: %s", err.Error()))
	}

	okey, err := ioutil.ReadFile(okeyFilename)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Failed to read old key der: %s", err.Error()))
	}

	nkey, err := ioutil.ReadFile(nkeyFilename)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Failed to read new key der: %s", err.Error()))
	}

	if err := mfg.ReplaceBootKey(sec0, okey, nkey); err != nil {
		LarvaUsage(nil, err)
	}

	if err := ioutil.WriteFile(outFilename, sec0, os.ModePerm); err != nil {
		LarvaUsage(nil, util.ChildNewtError(err))
	}
}

func AddMfgCommands(cmd *cobra.Command) {
	mfgCmd := &cobra.Command{
		Use:   "mfg",
		Short: "Manipulates Mynewt manufacturing images",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(mfgCmd)

	splitCmd := &cobra.Command{
		Use:   "split <mfg-image> <manifest> <out-dir>",
		Short: "Splits a Mynewt mfg section into several files",
		Run:   runSplitCmd,
	}

	splitCmd.PersistentFlags().IntVarP(&optDeviceNum, "device", "d", 0,
		"Flash device number")

	mfgCmd.AddCommand(splitCmd)

	joinCmd := &cobra.Command{
		Use:   "join <bin-dir> <manifest> <out-mfg-image>",
		Short: "Joins a split mfg section into a single file",
		Run:   runJoinCmd,
	}

	joinCmd.PersistentFlags().IntVarP(&optDeviceNum, "device", "d", 0,
		"Flash device number")

	mfgCmd.AddCommand(joinCmd)

	bootKeyCmd := &cobra.Command{
		Use:   "bootkey <sec0-bin> <cur-key-der> <new-key-der>",
		Short: "Replaces the boot key in a manufacturing image",
		Run:   runBootKeyCmd,
	}

	bootKeyCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o", "",
		"File to write to")
	bootKeyCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	mfgCmd.AddCommand(bootKeyCmd)
}
