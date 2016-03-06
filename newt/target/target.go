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
	BSP      string
	APP      string
	ARCH     string
	COMPILER string
}

func (t *Target) Bsp() *pkg.LocalPackage {
	dep, _ := pkg.NewDependency(nil, t.BSP)
	pkg, _ := project.GetProject().ResolveDependency(dep)
	return pkg
}

func (t *Target) App() *pkg.LocalPackage {
	dep, _ := pkg.NewDependency(nil, t.APP)
	pkg, _ := project.GetProject().ResolveDependency(dep)
	return pkg
}

func (t *Target) Compiler() *pkg.LocalPackage {
	dep, _ := pkg.NewDependency(nil, t.COMPILER)
	pkg, _ := project.GetProject().ResolveDependency(dep)
	return pkg
}
