package imgprod

import (
	"fmt"
	"os"
	"strings"

	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/manifest"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

type ProducedImageV1 struct {
	Filename string
	Image    image.ImageV1
	Hash     []byte
	FileSize int
}

type ProducedImageSetV1 struct {
	Loader *ProducedImageV1
	App    ProducedImageV1
}

func produceLoaderV1(opts ImageProdOpts) (ProducedImageV1, error) {
	pi := ProducedImageV1{}

	igo := image.ImageCreateOpts{
		SrcBinFilename:    opts.LoaderSrcFilename,
		SrcEncKeyFilename: opts.EncKeyFilename,
		Version:           opts.Version,
		SigKeys:           opts.SigKeys,
	}

	img, err := image.GenerateV1Image(igo)
	if err != nil {
		return pi, err
	}

	hash, err := img.Hash()
	if err != nil {
		return pi, err
	}

	fileSize, err := img.TotalSize()
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

	if _, err := img.Write(imgFile); err != nil {
		return pi, err
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"V1 loader image successfully generated: %s\n", opts.LoaderDstFilename)

	pi.Filename = opts.LoaderDstFilename
	pi.Image = img
	pi.Hash = hash
	pi.FileSize = fileSize

	return pi, nil
}

func produceAppV1(opts ImageProdOpts,
	loaderHash []byte) (ProducedImageV1, error) {

	pi := ProducedImageV1{}

	igo := image.ImageCreateOpts{
		SrcBinFilename:    opts.AppSrcFilename,
		SrcEncKeyFilename: opts.EncKeyFilename,
		Version:           opts.Version,
		SigKeys:           opts.SigKeys,
		LoaderHash:        loaderHash,
	}

	img, err := image.GenerateV1Image(igo)
	if err != nil {
		return pi, err
	}

	hash, err := img.Hash()
	if err != nil {
		return pi, err
	}

	fileSize, err := img.TotalSize()
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

	if _, err := img.Write(imgFile); err != nil {
		return pi, err
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"App image successfully generated: %s\n", opts.AppDstFilename)

	pi.Filename = opts.AppDstFilename
	pi.Image = img
	pi.Hash = hash
	pi.FileSize = fileSize

	return pi, nil
}

// Verifies that each already-built image leaves enough room for a boot trailer
// a the end of its slot.
func verifyImgSizesV1(pset ProducedImageSetV1, maxSizes []int) error {
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

func ProduceImagesV1(opts ImageProdOpts) (ProducedImageSetV1, error) {
	pset := ProducedImageSetV1{}

	var loaderHash []byte
	if opts.LoaderSrcFilename != "" {
		pi, err := produceLoaderV1(opts)
		if err != nil {
			return pset, err
		}
		loaderHash = pi.Hash

		pset.Loader = &pi
	}

	pi, err := produceAppV1(opts, loaderHash)
	if err != nil {
		return pset, err
	}
	pset.App = pi

	return pset, nil
}

func ProduceAllV1(t *builder.TargetBuilder, ver image.ImageVersion,
	sigKeys []image.ImageSigKey, encKeyFilename string) error {

	popts := OptsFromTgtBldr(t, ver, sigKeys, encKeyFilename)
	pset, err := ProduceImagesV1(popts)
	if err != nil {
		return err
	}

	mopts := manifest.ManifestCreateOpts{
		TgtBldr:    t,
		AppHash:    pset.App.Hash,
		Version:    ver,
		BuildID:    fmt.Sprintf("%x", pset.App.Hash),
		FlashAreas: t.BspPkg().FlashMap.SortedAreas(),
	}

	if pset.Loader != nil {
		mopts.LoaderHash = pset.Loader.Hash
	}

	if err := ProduceManifest(mopts); err != nil {
		return err
	}

	if err := verifyImgSizesV1(pset, mopts.TgtBldr.MaxImgSizes()); err != nil {
		return err
	}

	return nil
}
