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

// imgprod - Image production.

package imgprod

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/apache/mynewt-artifact/flash"
	"github.com/apache/mynewt-artifact/image"
	"github.com/apache/mynewt-artifact/sec"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/manifest"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/toolchain"
	"mynewt.apache.org/newt/util"
)

type ImageProdOpts struct {
	LoaderSrcFilename string
	LoaderDstFilename string
	LoaderHexFilename string
	AppSrcFilename    string
	AppDstFilename    string
	AppHexFilename    string
	EncKeyFilename    string
	EncKeyIndex       int
	Sections          []image.Section
	Version           image.ImageVersion
	SigKeys           []sec.PrivSignKey
	BaseAddr          int
	HdrPad            int
	ImagePad          int
	DummyC            *toolchain.Compiler
	UseLegacyTLV      bool
}

type ProducedImage struct {
	Filename string
	Image    image.Image
	Hash     []byte
	FileSize int
}

type ProducedImageSet struct {
	Loader *ProducedImage
	App    ProducedImage
}

// writeImageFiles writes two image artifacts:
// * <name>.img
// * <name>.hex
func writeImageFiles(ri image.Image, imgFilename string, hexFilename string,
	baseAddr int, c *toolchain.Compiler) error {

	imgFile, err := os.OpenFile(imgFilename,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return util.FmtNewtError(
			"can't open image file \"%s\" %s", imgFilename, err.Error())
	}

	_, err = ri.Write(imgFile)
	imgFile.Close()
	if err != nil {
		return err
	}

	if err := c.ConvertBinToHex(imgFilename, hexFilename,
		baseAddr); err != nil {

		return err
	}

	return nil
}

func produceLoader(opts ImageProdOpts) (ProducedImage, error) {
	pi := ProducedImage{}

	igo := image.ImageCreateOpts{
		SrcBinFilename:    opts.LoaderSrcFilename,
		Sections:          opts.Sections,
		SrcEncKeyFilename: opts.EncKeyFilename,
		SrcEncKeyIndex:    opts.EncKeyIndex,
		Version:           opts.Version,
		SigKeys:           opts.SigKeys,
	}

	ri, err := image.GenerateImage(igo)
	if err != nil {
		return pi, err
	}

	hash, err := ri.Hash()
	if err != nil {
		return pi, err
	}

	fileSize, err := ri.TotalSize()
	if err != nil {
		return pi, err
	}

	if err := writeImageFiles(ri, opts.LoaderDstFilename,
		opts.LoaderHexFilename, opts.BaseAddr, opts.DummyC); err != nil {

		return pi, err
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Loader image successfully generated: %s\n", opts.LoaderDstFilename)

	pi.Filename = opts.LoaderDstFilename
	pi.Image = ri
	pi.Hash = hash
	pi.FileSize = fileSize

	return pi, nil
}

func produceApp(opts ImageProdOpts, loaderHash []byte) (ProducedImage, error) {
	pi := ProducedImage{}

	igo := image.ImageCreateOpts{
		SrcBinFilename:    opts.AppSrcFilename,
		Sections:          opts.Sections,
		SrcEncKeyFilename: opts.EncKeyFilename,
		SrcEncKeyIndex:    opts.EncKeyIndex,
		Version:           opts.Version,
		SigKeys:           opts.SigKeys,
		LoaderHash:        loaderHash,
		HdrPad:            opts.HdrPad,
		ImagePad:          opts.ImagePad,
		UseLegacyTLV:      opts.UseLegacyTLV,
	}

	ri, err := image.GenerateImage(igo)
	if err != nil {
		return pi, err
	}

	hash, err := ri.Hash()
	if err != nil {
		return pi, err
	}

	fileSize, err := ri.TotalSize()
	if err != nil {
		return pi, err
	}

	if err := writeImageFiles(ri, opts.AppDstFilename, opts.AppHexFilename,
		opts.BaseAddr, opts.DummyC); err != nil {

		return pi, err
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"App image successfully generated: %s\n", opts.AppDstFilename)

	pi.Filename = opts.AppDstFilename
	pi.Image = ri
	pi.Hash = hash
	pi.FileSize = fileSize

	return pi, nil
}

// Verifies that each already-built image leaves enough room for a boot trailer
// a the end of its slot.
func verifyImgSizes(pset ProducedImageSet, maxSizes []int) error {
	errLines := []string{}
	slot := 0

	if pset.Loader != nil {
		if overflow := int(pset.Loader.FileSize) - maxSizes[0]; overflow > 0 {
			errLines = append(errLines,
				fmt.Sprintf("loader overflows slot-0 by %d bytes "+
					"(image=%d max=%d)",
					overflow, pset.Loader.FileSize, maxSizes[0]))
		}
		slot++
	}

	if overflow := int(pset.App.FileSize) - maxSizes[slot]; overflow > 0 {
		errLines = append(errLines,
			fmt.Sprintf("app overflows slot-%d by %d bytes "+
				"(image=%d max=%d)",
				slot, overflow, pset.App.FileSize, maxSizes[slot]))

	}

	if len(errLines) > 0 {
		if !newtutil.NewtForce {
			return util.NewNewtError(strings.Join(errLines, "; "))
		} else {
			for _, e := range errLines {
				util.StatusMessage(util.VERBOSITY_QUIET,
					"* Warning: %s (ignoring due to force flag)\n", e)
			}
		}
	}

	return nil
}

func ProduceImages(opts ImageProdOpts) (ProducedImageSet, error) {
	pset := ProducedImageSet{}

	var loaderHash []byte
	if opts.LoaderSrcFilename != "" {
		pi, err := produceLoader(opts)
		if err != nil {
			return pset, err
		}
		loaderHash = pi.Hash

		pset.Loader = &pi
	}

	pi, err := produceApp(opts, loaderHash)
	if err != nil {
		return pset, err
	}
	pset.App = pi

	return pset, nil
}

func ProduceManifest(opts manifest.ManifestCreateOpts) error {
	m, err := manifest.CreateManifest(opts)
	if err != nil {
		return err
	}

	file, err := os.Create(opts.TgtBldr.AppBuilder.ManifestPath())
	if err != nil {
		return util.FmtNewtError("Cannot create manifest file %s: %s",
			opts.TgtBldr.AppBuilder.ManifestPath(), err.Error())
	}
	defer file.Close()

	if _, err := m.Write(file); err != nil {
		return err
	}

	return nil
}

func OptsFromTgtBldr(b *builder.TargetBuilder, ver image.ImageVersion,
	sigKeys []sec.PrivSignKey, encKeyFilename string, encKeyIndex int,
	hdrPad int, imagePad int, sections []image.Section, useLegacyTLV bool) (ImageProdOpts, error) {

	// This compiler is just used for converting .img files to .hex files, so
	// dummy paths are OK.
	c, err := b.NewCompiler("", "")
	if err != nil {
		return ImageProdOpts{}, err
	}

	// If there is no flash area for slot 0, default to a base address of 0.
	img0Area := b.BspPkg().FlashMap.Areas[flash.FLASH_AREA_NAME_IMAGE_0]
	baseAddr := img0Area.Offset

	// If there is not a cmd line override, use the BSP values
	// for header pad and image pad
	if hdrPad <= 0 {
		hdrPad = b.BspPkg().ImageOffset
	}
	if imagePad <= 0 {
		imagePad = b.BspPkg().ImagePad
	}

	opts := ImageProdOpts{
		AppSrcFilename: b.AppBuilder.AppBinPath(),
		AppDstFilename: b.AppBuilder.AppImgPath(),
		AppHexFilename: b.AppBuilder.AppHexPath(),
		EncKeyFilename: encKeyFilename,
		EncKeyIndex:    encKeyIndex,
		Version:        ver,
		SigKeys:        sigKeys,
		DummyC:         c,
		BaseAddr:       baseAddr,
		HdrPad:         hdrPad,
		ImagePad:       imagePad,
		Sections:       sections,
		UseLegacyTLV:   useLegacyTLV,
	}

	if b.LoaderBuilder != nil {
		opts.LoaderSrcFilename = b.LoaderBuilder.AppBinPath()
		opts.LoaderDstFilename = b.LoaderBuilder.AppImgPath()
		opts.LoaderHexFilename = b.LoaderBuilder.AppHexPath()
	}

	return opts, nil
}

func ProduceAll(t *builder.TargetBuilder, ver image.ImageVersion,
	sigKeys []sec.PrivSignKey, encKeyFilename string, encKeyIndex int,
	hdrPad int, imagePad int, sectionString string, useLegacyTLV bool) error {

	elfPath := t.AppBuilder.AppElfPath()

	cmdName := "arm-none-eabi-objdump"
	cmdOut, err := exec.Command(cmdName, elfPath, "-hw").Output()
	if err != nil {
		return err
	}

	var sections []image.Section
	section_list := strings.Split(sectionString, ",")
	lines := strings.Split(string(cmdOut), "\n")
	var imgBase int
	for _, line := range lines {
		fields := strings.Fields(strings.Replace(line, "\t", " ", -1))
		if len(fields) >= 8 {
			_, err := strconv.ParseUint(fields[0], 16, 64)
			if err != nil {
				continue
			}
			if fields[1] == ".imghdr" {
				base, err := strconv.ParseInt(fields[3], 16, 32)
				if err != nil {
					continue
				}
				imgBase = int(base)
			}

			for _, section := range section_list {
				if fields[1] == section {
					offset, _ := strconv.ParseInt(fields[3], 16, 32)
					size, _ := strconv.ParseInt(fields[2], 16, 32)
					s := image.Section{Name: section,
						Size:   int(size),
						Offset: int(offset)}
					sections = append(sections, s)
				}
			}
		}
	}

	// update sections offset by subtracting off start of app image
	for s := range sections {
		sections[s].Offset = sections[s].Offset - imgBase
	}

	popts, err := OptsFromTgtBldr(t, ver, sigKeys, encKeyFilename, encKeyIndex,
		hdrPad, imagePad, sections, useLegacyTLV)
	if err != nil {
		return err
	}

	pset, err := ProduceImages(popts)
	if err != nil {
		return err
	}

	var loaderHash []byte
	if pset.Loader != nil {
		loaderHash = pset.Loader.Hash
	}

	mopts, err := manifest.OptsForImage(t, ver, pset.App.Hash, loaderHash)
	if err != nil {
		return err
	}

	if err := ProduceManifest(mopts); err != nil {
		return err
	}

	if err := verifyImgSizes(pset, mopts.TgtBldr.MaxImgSizes()); err != nil {
		return err
	}

	return nil
}
