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

package cache

import (
	"encoding/gob"
	"fmt"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
	"os"
	"path/filepath"
)

type ProjectCache struct {
	baseDir string
}

func InitCache(projDir string) *ProjectCache {
	pc := ProjectCache{}
	pc.baseDir = filepath.Join(projDir, ".cache")

	if _, err := os.Stat(pc.baseDir); os.IsNotExist(err) {
		if err := os.Mkdir(pc.baseDir, 0754); err != nil {
			return nil
		}
	}

	return &pc
}

func (pc *ProjectCache) getPackagesFile(repo *repo.Repo) string {
	return fmt.Sprintf(filepath.Join(pc.baseDir, repo.Name()))
}

func (pc *ProjectCache) AddPackages(repo *repo.Repo, pkgMap map[string]interfaces.PackageInterface) {
	cacheName := pc.getPackagesFile(repo)
	var dirList []string

	hash, err := repo.CurrentHash()
	if err != nil {
		return
	}

	for _, v := range pkgMap {
		dirList = append(dirList, v.BasePath())
	}

	f, err := os.Create(cacheName)
	if err != nil {
		util.OneTimeWarning("Failed to create cache file for \"%s\"", repo.Name())
		return
	}

	defer f.Close()

	enc := gob.NewEncoder(f)
	enc.Encode(hash)
	enc.Encode(dirList)
}

func (pc *ProjectCache) GetPackagesDirs(repo *repo.Repo) []string {
	cacheName := pc.getPackagesFile(repo)
	var dirList []string

	f, err := os.Open(cacheName)
	if err != nil {
		if !os.IsNotExist(err) {
			util.OneTimeWarning("Failed to open cache file for \"%s\"", repo.Name())
		}
		return nil
	}

	defer f.Close()

	var hash string

	enc := gob.NewDecoder(f)
	err = enc.Decode(&hash)
	if err != nil {
		util.OneTimeWarning("Failed to read cache for \"%s\"", repo.Name())
		return nil
	}

	currHash, _ := repo.CurrentHash()
	if hash != currHash {
		return nil
	}

	err = enc.Decode(&dirList)
	if err != nil {
		util.OneTimeWarning("Failed to read cache for \"%s\"", repo.Name())
		return nil
	}

	return dirList
}
