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

import "mynewt.apache.org/newt/newt/interfaces"

const PACKAGE_FILE_NAME = "pkg.yml"
const SYSCFG_YAML_FILENAME = "syscfg.yml"

const (
	PACKAGE_STABILITY_STABLE = "stable"
	PACKAGE_STABILITY_LATEST = "latest"
	PACKAGE_STABILITY_DEV    = "dev"
)

// Define constants with values of increasing priority.
const (
	PACKAGE_TYPE_COMPILER interfaces.PackageType = iota
	PACKAGE_TYPE_MFG
	PACKAGE_TYPE_SDK
	PACKAGE_TYPE_GENERATED
	PACKAGE_TYPE_LIB
	PACKAGE_TYPE_TRANSIENT
	PACKAGE_TYPE_BSP
	PACKAGE_TYPE_UNITTEST
	PACKAGE_TYPE_APP
	PACKAGE_TYPE_TARGET
)

var PackageTypeNames = map[interfaces.PackageType]string{
	PACKAGE_TYPE_COMPILER:  "compiler",
	PACKAGE_TYPE_MFG:       "mfg",
	PACKAGE_TYPE_SDK:       "sdk",
	PACKAGE_TYPE_GENERATED: "generated",
	PACKAGE_TYPE_LIB:       "lib",
	PACKAGE_TYPE_TRANSIENT: "transient",
	PACKAGE_TYPE_BSP:       "bsp",
	PACKAGE_TYPE_UNITTEST:  "unittest",
	PACKAGE_TYPE_APP:       "app",
	PACKAGE_TYPE_TARGET:    "target",
}

// An interface, representing information about a Package
// This interface is implemented by both packages in the
// local directory, but also packages that are stored in
// remote repositories.  It is abstracted so that routines
// that do package search & installation can work across
// both local & remote packages without needing to special
// case.
type Package interface {
	// The repository this package belongs to
	Repo() interfaces.RepoInterface
	// The name of this package
	Name() string
	// The full name of this package, including repo
	FullName() string
	// The type of package (lib, target, bsp, etc.)
	Type() interfaces.PackageType
	// BasePath is the path on disk if it's a local package
	BasePath() string
	// Hash of the contents of the package
	Hash() (string, error)
	// Description of this package
	Desc() *PackageDesc
	// APIs exported by this package
	Apis() []string
	// APIs required by this package
	ReqApis() []string
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
