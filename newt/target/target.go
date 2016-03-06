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

package target

import (
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
)

type Target struct {
	basePkg      *pkg.LocalPackage
	CompilerName string
	BspName      string
	AppName      string
	Arch         string

	// Pointer to target.yml configuration structure
	v *viper.Viper
}

func NewTarget(basePkg *pkg.LocalPackage) (*Target, error) {
	target := &Target{}
	if err := target.Init(basePkg); err != nil {
		return nil, err
	}

	return target, nil
}

func (target *Target) Init(basePkg *pkg.LocalPackage) error {
	target.basePkg = basePkg

	v, err := util.ReadConfig(basePkg.BasePath(),
		strings.TrimSuffix(cli.TARGET_FILE_NAME, ".yml"))
	if err != nil {
		return err
	}
	target.v = v

	target.CompilerName = v.GetString("target.compiler")
	target.BspName = v.GetString("target.bsp")
	target.AppName = v.GetString("target.app")
	target.Arch = v.GetString("target.arch")

	// XXX: Verify required fields set?

	return nil
}

func (target *Target) Package() *pkg.LocalPackage {
	return target.basePkg
}

func (target *Target) Compiler() *pkg.LocalPackage {
	dep, _ := pkg.NewDependency(nil, target.CompilerName)
	pkg, _ := project.GetProject().ResolveDependency(dep)
	return pkg
}

func (t *Target) App() *pkg.LocalPackage {
	dep, _ := pkg.NewDependency(nil, t.APP)
	pkg, _ := project.GetProject().ResolveDependency(dep)
	return pkg
}

func (target *Target) App() *pkg.LocalPackage {
	dep, _ := pkg.NewDependency(nil, target.AppName)
	pkg, _ := project.GetProject().ResolveDependency(dep)
	return pkg
}

func TargetList() ([]*Target, error) {
	targets := []*Target{}

	packs := project.GetProject().PackageList()
	for _, packHash := range packs {
		for _, pack := range *packHash {
			if pack.Type() == pkg.PACKAGE_TYPE_TARGET {
				target, err := NewTarget(pack)
				if err != nil {
					return nil, err
				}
				targets = append(targets, target)
			}
		}
	}

	return targets, nil
}
