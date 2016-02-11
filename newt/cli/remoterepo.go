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
	"os"
	"path/filepath"
)

type RemoteRepo struct {
	// Repository associated with the Pkgs
	Repo *Repo

	PkgList *PkgList

	Name string

	RemoteLoc string

	LocalLoc string
}

// Allocate a new  structure, and initialize it.
func NewRemoteRepo(pkgList *PkgList, branch string) (*RemoteRepo, error) {
	remoteRepo := &RemoteRepo{
		Name:      pkgList.Name,
		RemoteLoc: pkgList.RemoteUrl,
		LocalLoc:  "",
	}

	err := remoteRepo.Download(branch)
	if err != nil {
		return nil, err
	}
	return remoteRepo, nil
}

// Download it
func (remoteRepo *RemoteRepo) Download(branch string) error {
	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Downloading %s from %s/"+
		"%s...", remoteRepo.Name, remoteRepo.RemoteLoc, branch)

	dir, err := dl.GetRepo(remoteRepo.RemoteLoc, branch)
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	remoteRepo.LocalLoc = dir

	repo, err := NewRepoWithDir(dir)
	if err != nil {
		return err
	}
	remoteRepo.Repo = repo

	pkgList, err := NewPkgList(repo)
	if err != nil {
		return err
	}

	err = pkgList.LoadConfigs(nil, false)
	if err != nil {
		return err
	}
	remoteRepo.PkgList = pkgList

	return nil
}

func (remoteRepo *RemoteRepo) ResolvePkgName(pkgName string) (*Pkg, error) {
	if remoteRepo.PkgList == nil {
		return nil, NewNewtError(fmt.Sprintf("RemoteRepo %s not downloaded yet!",
			remoteRepo.Name))
	}
	return remoteRepo.PkgList.ResolvePkgName(pkgName)
}

func (remoteRepo *RemoteRepo) fetchPkg(pkgName string, tgtBase string) error {
	pkg, err := remoteRepo.ResolvePkgName(pkgName)
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Installing %s\n", pkg.FullName)

	srcDir := filepath.Join(remoteRepo.LocalLoc, pkg.FullName)
	tgtDir := filepath.Join(tgtBase, pkg.FullName)

	err = CopyDir(srcDir, tgtDir)
	return err
}

// Remove local copy
func (remoteRepo *RemoteRepo) Remove() error {
	if remoteRepo.LocalLoc != "" {
		err := os.RemoveAll(remoteRepo.LocalLoc)
		return err
	}
	return nil
}
