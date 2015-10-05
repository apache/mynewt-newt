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
	"fmt"
	"os"
	"path/filepath"
)

type RemoteNest struct {
	// Nestsitory associated with the Eggs
	Nest *Nest

	Clutch *Clutch

	Name string

	RemoteLoc string

	LocalLoc string
}

// Allocate a new  structure, and initialize it.
func NewRemoteNest(clutch *Clutch) (*RemoteNest, error) {
	remoteNest := &RemoteNest{
		Name : clutch.Name,
		RemoteLoc: clutch.RemoteUrl,
		LocalLoc: "",
	}

	err := remoteNest.Download()
	if err != nil {
		return nil, err
	}
	return remoteNest, nil
}

// Download it
func (remoteNest *RemoteNest) Download() error {
	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Downloading %s from %s/"+
		"master...", remoteNest.Name, remoteNest.RemoteLoc)

	dir, err := dl.GetRepo(remoteNest.RemoteLoc, "master")
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	remoteNest.LocalLoc = dir

	nest, err := NewNestWithDir(dir)
	if err != nil {
		return err
	}
	remoteNest.Nest = nest

	clutch, err := NewClutch(nest)
	if err != nil {
		return err
	}

	err = clutch.LoadConfigs(nil, false)
	if err != nil {
		return err
	}
	remoteNest.Clutch = clutch

	return nil
}

func (remoteNest *RemoteNest) ResolveEggName(eggName string) (*Egg, error) {
	if remoteNest.Clutch == nil {
		return nil, NewNewtError(fmt.Sprintf("RemoteNest %s not downloaded yet!",
					remoteNest.Name))
	}
	return remoteNest.Clutch.ResolveEggName(eggName)
}

func (remoteNest *RemoteNest) fetchEgg(eggName string, tgtBase string) error {
	egg, err := remoteNest.ResolveEggName(eggName)
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Installing %s\n", egg.FullName)

	srcDir := filepath.Join(remoteNest.LocalLoc, egg.FullName)
	tgtDir := filepath.Join(tgtBase, egg.FullName)

	err = CopyDir(srcDir, tgtDir)
	return err
}

// Remove local copy
func (remoteNest *RemoteNest) Remove() error {
	if remoteNest.LocalLoc != "" {
		err := os.RemoveAll(remoteNest.LocalLoc)
		return err
	}
	return nil
}
