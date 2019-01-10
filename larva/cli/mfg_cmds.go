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
	"mynewt.apache.org/newt/artifact/mfg"
	"mynewt.apache.org/newt/artifact/misc"
	"mynewt.apache.org/newt/larva/lvmfg"
	"mynewt.apache.org/newt/util"
)

func readMfgBin(filename string) ([]byte, error) {
	bin, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, util.FmtChildNewtError(err,
			"Failed to read manufacturing image: %s", err.Error())
	}

	return bin, nil
}

func readManifest(mfgDir string) (manifest.MfgManifest, error) {
	return manifest.ReadMfgManifest(mfgDir + "/" + mfg.MANIFEST_FILENAME)
}

func extractFlashAreas(mman manifest.MfgManifest) ([]flash.FlashArea, error) {
	areas := flash.SortFlashAreasByDevOff(mman.FlashAreas)

	if len(areas) == 0 {
		LarvaUsage(nil, util.FmtNewtError(
			"Boot loader manifest does not contain flash map"))
	}

	overlaps, conflicts := flash.DetectErrors(areas)
	if len(overlaps) > 0 || len(conflicts) > 0 {
		return nil, util.NewNewtError(flash.ErrorText(overlaps, conflicts))
	}

	if err := lvmfg.VerifyAreas(areas); err != nil {
		return nil, err
	}

	log.Debugf("Successfully read flash areas: %+v", areas)
	return areas, nil
}

func createNameBlobMap(binDir string,
	areas []flash.FlashArea) (lvmfg.NameBlobMap, error) {

	mm := lvmfg.NameBlobMap{}

	for _, area := range areas {
		filename := fmt.Sprintf("%s/%s.bin", binDir, area.Name)
		bin, err := readMfgBin(filename)
		if err != nil {
			if !util.IsNotExist(err) {
				return nil, util.ChildNewtError(err)
			}
		} else {
			mm[area.Name] = bin
		}
	}

	return mm, nil
}

func runMfgShowCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		LarvaUsage(cmd, nil)
	}
	inFilename := args[0]

	metaEndOff, err := util.AtoiNoOct(args[1])
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"invalid meta offset \"%s\"", args[1]))
	}

	bin, err := readMfgBin(inFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	m, err := mfg.Parse(bin, metaEndOff, 0xff)
	if err != nil {
		LarvaUsage(nil, err)
	}

	if m.Meta == nil {
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Manufacturing image %s does not contain an MMR\n", inFilename)
	} else {
		s, err := m.Meta.Json(metaEndOff)
		if err != nil {
			LarvaUsage(nil, err)
		}
		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Manufacturing image %s contains an MMR with "+
				"the following properties:\n%s\n", inFilename, s)
	}
}

func runSplitCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		LarvaUsage(cmd, nil)
	}

	mfgDir := args[0]
	outDir := args[1]

	mm, err := readManifest(mfgDir)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	areas, err := extractFlashAreas(mm)
	if err != nil {
		LarvaUsage(nil, err)
	}

	binPath := fmt.Sprintf("%s/%s", mfgDir, mm.BinPath)
	bin, err := readMfgBin(binPath)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Failed to read \"%s\": %s", binPath, err.Error()))
	}

	nbmap, err := lvmfg.Split(bin, mm.Device, areas, 0xff)
	if err != nil {
		LarvaUsage(nil, err)
	}

	if err := os.Mkdir(outDir, os.ModePerm); err != nil {
		LarvaUsage(nil, util.ChildNewtError(err))
	}

	for name, data := range nbmap {
		filename := fmt.Sprintf("%s/%s.bin", outDir, name)
		if err := WriteFile(data, filename); err != nil {
			LarvaUsage(nil, err)
		}
	}

	mfgDstDir := fmt.Sprintf("%s/mfg", outDir)
	if err := CopyDir(mfgDir, mfgDstDir); err != nil {
		LarvaUsage(nil, err)
	}
}

func runJoinCmd(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		LarvaUsage(cmd, nil)
	}

	splitDir := args[0]
	outDir := args[1]

	if util.NodeExist(outDir) {
		LarvaUsage(nil, util.FmtNewtError(
			"Destination \"%s\" already exists", outDir))
	}

	mm, err := readManifest(splitDir + "/mfg")
	if err != nil {
		LarvaUsage(cmd, err)
	}
	areas, err := extractFlashAreas(mm)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	nbmap, err := createNameBlobMap(splitDir, areas)
	if err != nil {
		LarvaUsage(nil, err)
	}

	bin, err := lvmfg.Join(nbmap, 0xff, areas)
	if err != nil {
		LarvaUsage(nil, err)
	}

	m, err := mfg.Parse(bin, mm.Meta.EndOffset, 0xff)
	if err != nil {
		LarvaUsage(nil, err)
	}

	infos, err := ioutil.ReadDir(splitDir + "/mfg")
	if err != nil {
		LarvaUsage(nil, util.FmtNewtError(
			"Error reading source mfg directory: %s", err.Error()))
	}
	for _, info := range infos {
		if info.Name() != mfg.MFG_IMG_FILENAME {
			src := splitDir + "/mfg/" + info.Name()
			dst := outDir + "/" + info.Name()
			if info.IsDir() {
				err = CopyDir(src, dst)
			} else {
				err = CopyFile(src, dst)
			}
			if err != nil {
				LarvaUsage(nil, err)
			}
		}
	}

	finalBin, err := m.Bytes(0xff)
	if err != nil {
		LarvaUsage(nil, err)
	}

	binPath := fmt.Sprintf("%s/%s", outDir, mfg.MFG_IMG_FILENAME)
	if err := WriteFile(finalBin, binPath); err != nil {
		LarvaUsage(nil, err)
	}
}

func genSwapKeyCmd(cmd *cobra.Command, args []string, isKek bool) {
	if len(args) < 3 {
		LarvaUsage(cmd, nil)
	}

	mfgimgFilename := args[0]
	okeyFilename := args[1]
	nkeyFilename := args[2]

	outFilename, err := CalcOutFilename(mfgimgFilename)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	bin, err := readMfgBin(mfgimgFilename)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Failed to read mfgimg file: %s", err.Error()))
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

	if isKek {
		err = lvmfg.ReplaceKek(bin, okey, nkey)
	} else {
		err = lvmfg.ReplaceIsk(bin, okey, nkey)
	}
	if err != nil {
		LarvaUsage(nil, err)
	}

	if err := WriteFile(bin, outFilename); err != nil {
		LarvaUsage(nil, err)
	}
}

func runSwapIskCmd(cmd *cobra.Command, args []string) {
	genSwapKeyCmd(cmd, args, false)
}

func runSwapKekCmd(cmd *cobra.Command, args []string) {
	genSwapKeyCmd(cmd, args, true)
}

func runRehashCmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		LarvaUsage(cmd, nil)
	}

	mfgDir := args[0]

	outDir, err := CalcOutFilename(mfgDir)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	// Read manifest and mfgimg.bin.
	mman, err := readManifest(mfgDir)
	if err != nil {
		LarvaUsage(cmd, err)
	}

	binPath := fmt.Sprintf("%s/%s", mfgDir, mman.BinPath)
	bin, err := readMfgBin(binPath)
	if err != nil {
		LarvaUsage(cmd, util.FmtNewtError(
			"Failed to read \"%s\": %s", binPath, err.Error()))
	}

	// Calculate accurate hash.
	metaOff := -1
	if mman.Meta != nil {
		metaOff = mman.Meta.EndOffset
	}
	m, err := mfg.Parse(bin, metaOff, 0xff)
	if err != nil {
		LarvaUsage(nil, err)
	}

	if err := m.RecalcHash(0xff); err != nil {
		LarvaUsage(nil, err)
	}

	hash, err := m.Hash()
	if err != nil {
		LarvaUsage(nil, err)
	}

	// Update manifest.
	mman.MfgHash = misc.HashString(hash)

	// Write new artifacts.
	if outDir != mfgDir {
		// Not an in-place operation; copy input directory.
		if err := CopyDir(mfgDir, outDir); err != nil {
			LarvaUsage(nil, err)
		}
		binPath = fmt.Sprintf("%s/%s", outDir, mman.BinPath)
	}

	newBin, err := m.Bytes(0xff)
	if err != nil {
		LarvaUsage(nil, err)
	}
	if err := WriteFile(newBin, binPath); err != nil {
		LarvaUsage(nil, err)
	}

	json, err := mman.MarshalJson()
	if err != nil {
		LarvaUsage(nil, err)
	}

	manPath := fmt.Sprintf("%s/%s", outDir, mfg.MANIFEST_FILENAME)
	if err := WriteFile(json, manPath); err != nil {
		LarvaUsage(nil, err)
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

	showCmd := &cobra.Command{
		Use:   "show <mfgimg.bin> <meta-end-offset>",
		Short: "Displays JSON describing a manufacturing image",
		Run:   runMfgShowCmd,
	}

	mfgCmd.AddCommand(showCmd)

	splitCmd := &cobra.Command{
		Use:   "split <mfgimage-dir> <out-dir>",
		Short: "Splits a Mynewt mfg section into several files",
		Run:   runSplitCmd,
	}

	mfgCmd.AddCommand(splitCmd)

	joinCmd := &cobra.Command{
		Use:   "join <split-dir> <out-dir>",
		Short: "Joins a split mfg section into a single file",
		Run:   runJoinCmd,
	}

	mfgCmd.AddCommand(joinCmd)

	swapIskCmd := &cobra.Command{
		Use:   "swapisk <mfgimg-bin> <cur-key-der> <new-key-der>",
		Short: "Replaces an image-signing key in a manufacturing image",
		Run:   runSwapIskCmd,
	}

	swapIskCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o",
		"", "File to write to")
	swapIskCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	mfgCmd.AddCommand(swapIskCmd)

	swapKekCmd := &cobra.Command{
		Use:   "swapkek <mfgimg-bin> <cur-key-der> <new-key-der>",
		Short: "Replaces a key-encrypting key in a manufacturing image",
		Run:   runSwapKekCmd,
	}

	swapKekCmd.PersistentFlags().StringVarP(&OptOutFilename, "outfile", "o",
		"", "File to write to")
	swapKekCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input file")

	mfgCmd.AddCommand(swapKekCmd)

	rehashCmd := &cobra.Command{
		Use:   "rehash <mfgimage-dir>",
		Short: "Replaces an outdated mfgimage hash with an accurate one",
		Run:   runRehashCmd,
	}
	rehashCmd.PersistentFlags().StringVarP(&OptOutFilename, "outdir", "o",
		"", "Directory to write to")
	rehashCmd.PersistentFlags().BoolVarP(&OptInPlace, "inplace", "i", false,
		"Replace input files")

	mfgCmd.AddCommand(rehashCmd)
}
