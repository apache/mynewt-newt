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

package interfaces

type PackageInterface interface {
	Name() string
	FullName() string
	BasePath() string
	Repo() RepoInterface
	Type() PackageType
}

type PackageType int

type RepoInterface interface {
	Name() string
	IsLocal() bool
	Path() string
}

type PackageList map[string]*map[string]PackageInterface

type DependencyInterface interface {
	SatisfiesDependency(pkg PackageInterface) bool
	String() string
}

type ProjectInterface interface {
	Name() string
	Path() string
	ResolveDependency(dep DependencyInterface) PackageInterface
	ResolvePath(basePath string, name string) (string, error)
	PackageList() PackageList
	FindRepoPath(rname string) string
	RepoIsInstalled(rname string) bool
}

var globalProject ProjectInterface

func GetProject() ProjectInterface {
	return globalProject
}

func SetProject(proj ProjectInterface) {
	globalProject = proj
}
