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

package pkg

import "mynewt.apache.org/newt/newt/repo"

const PACKAGE_FILE_NAME = "pkg.yml"

const (
	PACKAGE_STABILITY_STABLE = "stable"
	PACKAGE_STABILITY_LATEST = "latest"
	PACKAGE_STABILITY_DEV    = "dev"
)

type PackageType int

const (
	PACKAGE_TYPE_LIB PackageType = iota
	PACKAGE_TYPE_BSP
	PACKAGE_TYPE_TARGET
	PACKAGE_TYPE_APP
)

const PACKAGE_TYPE_STR_LIB = "lib"
const PACKAGE_TYPE_STR_BSP = "bsp"
const PACKAGE_TYPE_STR_APP = "app"
const PACKAGE_TYPE_STR_TARGET = "target"

// An interface, representing information about a Package
// This interface is implemented by both packages in the
// local directory, but also packages that are stored in
// remote repositories.  It is abstracted so that routines
// that do package search & installation can work across
// both local & remote packages without needing to special
// case.
type Package interface {
	// Initialize the package, in the directory specified
	// by pkgDir
	Init(repo *repo.Repo, pkgDir string) error
	// The repository this package belongs to
	Repo() *repo.Repo
	// The name of this package
	Name() string
	// The type of package (lib, target, bsp, etc.)
	Type() PackageType
	// Hash of the contents of the package
	Hash() (string, error)
	// Description of this package
	Desc() *PackageDesc
	// Version of this package
	Vers() *Version
	// Dependency list for this package
	Deps() []*Dependency
	// APIs exported by this package
	Apis() []*Dependency
	// APIs required by this package
	ReqApis() []*Dependency
}

// Description of a package
type PackageDesc struct {
	// Author of the package
	Author string
	// Homepage of the package for more information
	Homepage    string
	Description string
	Keywords    []string
}

type BspPackage struct {
	Package
	LinkerScript   string
	DownloadScript string
	DebugScript    string
}
