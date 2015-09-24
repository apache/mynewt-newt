/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cli

import (
	"github.com/spf13/viper"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type Clutch struct {
	Name string

	LarvaFile string

	EggShells []*EggShell

	RemoteUrl string

	nest *Nest
}

type EggShell struct {
	FullName string
	Url      string
	Version  *Version
	Deps     []*DependencyRequirement
	Caps     []*DependencyRequirement
	ReqCaps  []*DependencyRequirement
}

type Nest struct {
	// Path to the Nest
	Path string

	// Store of Larvas
	Clutches map[string]*Clutch

	// Repository for this Nest
	repo *Repo
}

func NewNest(repo *Repo) (*Nest, error) {
	nest := &Nest{}

	nest.Path = repo.BasePath + "/.nest/"
	if NodeNotExist(nest.Path) {
		if err := os.MkdirAll(nest.Path, 0755); err != nil {
			return nil, err
		}
	}

	nest.Clutches = map[string]*Clutch{}
	nest.repo = repo

	if err := nest.LoadNest(); err != nil {
		return nil, err
	}

	return nest, nil
}

func NewClutch(nest *Nest) (*Clutch, error) {
	clutch := &Clutch{}

	clutch.nest = nest

	return clutch, nil
}

func NewEggShell() (*EggShell, error) {
	eShell := &EggShell{}

	return eShell, nil
}

func (nest *Nest) LoadNest() error {
	files, err := ioutil.ReadDir(nest.Path)
	if err != nil {
		return err
	}
	for _, fileInfo := range files {
		file := fileInfo.Name()
		if filepath.Ext(file) == ".yml" {
			name := file[:len(filepath.Base(file))-len(".yml")]
			log.Printf("[DEBUG] Loading Clutch %s", name)
			clutch, err := NewClutch(nest)
			if err != nil {
				return err
			}
			if err := clutch.Load(name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (nest *Nest) GetClutches() (map[string]*Clutch, error) {
	return nest.Clutches, nil
}

func (cl *Clutch) strSliceToDr(list []string) ([]*DependencyRequirement, error) {
	drList := []*DependencyRequirement{}

	for _, name := range list {
		req, err := NewDependencyRequirementParseString(name)
		if err != nil {
			return nil, err
		}
		drList = append(drList, req)
	}

	if len(drList) == 0 {
		return nil, nil
	} else {
		return drList, nil
	}
}

func (cl *Clutch) fileToPackageList(cfg *viper.Viper) ([]*EggShell, error) {
	pkgMap := cfg.GetStringMap("pkgs")

	pkgList := []*EggShell{}

	for name, _ := range pkgMap {
		lpkg, err := NewEggShell()
		if err != nil {
			return nil, err
		}
		lpkg.FullName = name

		pkgDef := cfg.GetStringMap("pkgs." + name)
		lpkg.Url = pkgDef["url"].(string)
		lpkg.Version, err = NewVersParseString(pkgDef["vers"].(string))
		if err != nil {
			return nil, err
		}

		lpkg.Deps, err = cl.strSliceToDr(cfg.GetStringSlice("pkgs." + name + ".deps"))
		if err != nil {
			return nil, err
		}

		lpkg.Caps, err = cl.strSliceToDr(cfg.GetStringSlice("pkgs." + name + ".caps"))
		if err != nil {
			return nil, err
		}

		lpkg.ReqCaps, err = cl.strSliceToDr(cfg.GetStringSlice("pkgs." + name +
			".req_caps"))
		if err != nil {
			return nil, err
		}

		pkgList = append(pkgList, lpkg)
	}

	return pkgList, nil
}

// Create the manifest file name, it's the manifest dir + manifest name and a
// .yml extension
func (cl *Clutch) GetClutchFile(name string) string {
	return cl.nest.Path + name + ".yml"
}

func (cl *Clutch) Load(name string) error {
	cfg, err := ReadConfig(cl.nest.Path, name)
	if err != nil {
		return nil
	}

	cl.EggShells, err = cl.fileToPackageList(cfg)
	if err != nil {
		return err
	}

	cl.nest.Clutches[name] = cl

	return nil
}

func (cl *Clutch) Install(name string, url string) error {
	clutchFile := cl.GetClutchFile(name)

	// XXX: Should warn if file already exists, and require force option
	os.Remove(clutchFile)

	// Download the manifest
	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	if err := dl.DownloadFile(url, clutchFile); err != nil {
		return err
	}

	// Load the manifest, and ensure that it is in the correct format
	if err := cl.Load(name); err != nil {
		return err
	}

	return nil
}
