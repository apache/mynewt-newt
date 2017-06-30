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

package project

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"

	"mynewt.apache.org/newt/newt/downloader"
	"mynewt.apache.org/newt/util"
)

type templateRepo struct {
	owner  string
	name   string
	branch string
}

type PackageWriter struct {
	downloader *downloader.GithubDownloader
	repo       templateRepo
	targetPath string
	template   string
	fullName   string
	project    *Project
}

var TemplateRepoMap = map[string]templateRepo{
	"APP": templateRepo{
		owner:  "runtimeco",
		name:   "mynewt-pkg-app",
		branch: "master",
	},
	"SDK": templateRepo{
		owner:  "apache",
		name:   "incubator-incubator-mynewt-pkg-sdk",
		branch: "master",
	},
	"BSP": templateRepo{
		owner:  "apache",
		name:   "incubator-incubator-mynewt-pkg-bsp",
		branch: "master",
	},
	"LIB": templateRepo{
		owner:  "apache",
		name:   "incubator-incubator-mynewt-pkg-pkg",
		branch: "master",
	},
	"UNITTEST": templateRepo{
		owner:  "runtimeco",
		name:   "mynewt-pkg-unittest",
		branch: "master",
	},

	// Type=pkg is identical to type=lib for backwards compatibility.
	"PKG": templateRepo{
		owner:  "apache",
		name:   "incubator-incubator-mynewt-pkg-pkg",
		branch: "master",
	},
}

func (pw *PackageWriter) ConfigurePackage(template string, loc string) error {
	tr, ok := TemplateRepoMap[template]
	if !ok {
		return util.NewNewtError(fmt.Sprintf("Cannot find matching "+
			"repository for template %s", template))
	}
	pw.repo = tr

	pw.fullName = path.Clean(loc)
	path := pw.project.Path()
	path = path + "/" + pw.fullName

	if util.NodeExist(path) {
		return util.NewNewtError(fmt.Sprintf("Cannot place a new package in "+
			"%s, path already exists.", path))
	}

	pw.template = template
	pw.targetPath = path

	return nil
}

func (pw *PackageWriter) cleanupPackageFile(pfile string) error {
	data, err := ioutil.ReadFile(pfile)
	if err != nil {
		return util.ChildNewtError(err)
	}

	// Search & replace file contents
	re := regexp.MustCompile("your-pkg-name")
	res := re.ReplaceAllString(string(data), "\""+pw.fullName+"\"")

	if err := ioutil.WriteFile(pfile, []byte(res), 0666); err != nil {
		return util.ChildNewtError(err)
	}

	return nil
}

func (pw *PackageWriter) fixupPkg() error {
	pkgBase := path.Base(pw.fullName)

	// Move include file to name after package name
	if err := util.MoveFile(pw.targetPath+"/include/your-path/your-file.h",
		pw.targetPath+"/include/your-path/"+pkgBase+".h"); err != nil {

		if !util.IsNotExist(err) {
			return err
		}
	}

	// Move source file
	if err := util.MoveFile(pw.targetPath+"/src/your-source.c",
		pw.targetPath+"/src/"+pkgBase+".c"); err != nil {

		if !util.IsNotExist(err) {
			return err
		}
	}

	if err := util.CopyDir(pw.targetPath+"/include/your-path/",
		pw.targetPath+"/include/"+pkgBase+"/"); err != nil {

		if !util.IsNotExist(err) {
			return err
		}
	}

	if err := os.RemoveAll(pw.targetPath + "/include/your-path/"); err != nil {
		if !util.IsNotExist(err) {
			return util.ChildNewtError(err)
		}
	}

	if err := pw.cleanupPackageFile(pw.targetPath + "/pkg.yml"); err != nil {
		return err
	}

	return nil
}

func (pw *PackageWriter) WritePackage() error {
	dl := pw.downloader

	dl.User = pw.repo.owner
	dl.Repo = pw.repo.name

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Download package template for package type %s.\n",
		strings.ToLower(pw.template))

	tmpdir, err := dl.DownloadRepo(pw.repo.branch)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(tmpdir + "/.git/"); err != nil {
		return util.NewNewtError(err.Error())
	}

	if err := util.CopyDir(tmpdir, pw.targetPath); err != nil {
		return err
	}

	if err := pw.fixupPkg(); err != nil {
		return err
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Package successfuly installed into %s.\n", pw.targetPath)

	return nil
}

/**
 * Create new PackageWriter structure, and return it
 */
func NewPackageWriter() *PackageWriter {
	proj := GetProject()

	pw := &PackageWriter{
		project:    proj,
		downloader: downloader.NewGithubDownloader(),
	}

	return pw
}
