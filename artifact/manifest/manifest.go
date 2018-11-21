package manifest

import (
	"encoding/json"
	"io"
	"io/ioutil"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/util"
)

/*
 * Data that's going to go to build manifest file
 */
type ManifestSizeArea struct {
	Name string `json:"name"`
	Size uint32 `json:"size"`
}

type ManifestSizeSym struct {
	Name  string              `json:"name"`
	Areas []*ManifestSizeArea `json:"areas"`
}

type ManifestSizeFile struct {
	Name string             `json:"name"`
	Syms []*ManifestSizeSym `json:"sym"`
}

type ManifestSizePkg struct {
	Name  string              `json:"name"`
	Files []*ManifestSizeFile `json:"files"`
}

type ManifestPkg struct {
	Name string `json:"name"`
	Repo string `json:"repo"`
}

type ManifestRepo struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty,omitempty"`
	URL    string `json:"url,omitempty"`
}

type Manifest struct {
	Name       string            `json:"name"`
	Date       string            `json:"build_time"`
	Version    string            `json:"build_version"`
	BuildID    string            `json:"id"`
	Image      string            `json:"image"`
	ImageHash  string            `json:"image_hash"`
	Loader     string            `json:"loader"`
	LoaderHash string            `json:"loader_hash"`
	Pkgs       []*ManifestPkg    `json:"pkgs"`
	LoaderPkgs []*ManifestPkg    `json:"loader_pkgs,omitempty"`
	TgtVars    []string          `json:"target"`
	Repos      []*ManifestRepo   `json:"repos"`
	FlashAreas []flash.FlashArea `json:"flash_map"`

	PkgSizes       []*ManifestSizePkg `json:"pkgsz"`
	LoaderPkgSizes []*ManifestSizePkg `json:"loader_pkgsz,omitempty"`
}

func ReadManifest(path string) (Manifest, error) {
	m := Manifest{}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return m, util.ChildNewtError(err)
	}

	if err := json.Unmarshal(content, &m); err != nil {
		return m, util.FmtNewtError(
			"Failure decoding manifest with path \"%s\": %s",
			path, err.Error())
	}

	return m, nil
}

func (m *Manifest) Write(w io.Writer) (int, error) {
	buffer, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return 0, util.FmtNewtError("Cannot encode manifest: %s", err.Error())
	}

	cnt, err := w.Write(buffer)
	if err != nil {
		return 0, util.FmtNewtError("Cannot write manifest: %s", err.Error())
	}

	return cnt, nil
}
