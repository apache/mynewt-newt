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
	"strings"

	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/artifact/sec"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/manifest"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

type ImageProdOpts struct {
	LoaderSrcFilename string
	LoaderDstFilename string
	AppSrcFilename    string
	AppDstFilename    string
	EncKeyFilename    string
	Version           image.ImageVersion
	SigKeys           []sec.SignKey
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

func produceLoader(opts ImageProdOpts) (ProducedImage, error) {
	pi := ProducedImage{}

	igo := image.ImageCreateOpts{
		SrcBinFilename:    opts.LoaderSrcFilename,
		SrcEncKeyFilename: opts.EncKeyFilename,
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

	imgFile, err := os.OpenFile(opts.LoaderDstFilename,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return pi, util.FmtNewtError(
			"Can't open target image %s: %s",
			opts.LoaderDstFilename, err.Error())
	}
	defer imgFile.Close()

	if _, err := ri.Write(imgFile); err != nil {
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
		SrcEncKeyFilename: opts.EncKeyFilename,
		Version:           opts.Version,
		SigKeys:           opts.SigKeys,
		LoaderHash:        loaderHash,
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

	imgFile, err := os.OpenFile(opts.AppDstFilename,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return pi, util.FmtNewtError(
			"Can't open target image %s: %s", opts.AppDstFilename, err.Error())
	}
	defer imgFile.Close()

	if _, err := ri.Write(imgFile); err != nil {
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
	sigKeys []sec.SignKey, encKeyFilename string) ImageProdOpts {

	opts := ImageProdOpts{
		AppSrcFilename: b.AppBuilder.AppBinPath(),
		AppDstFilename: b.AppBuilder.AppImgPath(),
		EncKeyFilename: encKeyFilename,
		Version:        ver,
		SigKeys:        sigKeys,
	}

	if b.LoaderBuilder != nil {
		opts.LoaderSrcFilename = b.LoaderBuilder.AppBinPath()
		opts.LoaderDstFilename = b.LoaderBuilder.AppImgPath()
	}

	return opts
}

func ProduceAll(t *builder.TargetBuilder, ver image.ImageVersion,
	sigKeys []sec.SignKey, encKeyFilename string) error {

	popts := OptsFromTgtBldr(t, ver, sigKeys, encKeyFilename)
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
