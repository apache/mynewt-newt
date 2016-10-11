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

type PackageWriter struct {
	downloader *downloader.GithubDownloader
	repo       string
	targetPath string
	template   string
	fullName   string
	project    *Project
}

var TemplateRepoMap = map[string]string{
	"SDK": "incubator-incubator-mynewt-pkg-sdk",
	"BSP": "incubator-incubator-mynewt-pkg-bsp",
	"PKG": "incubator-incubator-mynewt-pkg-pkg",
}

const PACKAGEWRITER_GITHUB_DOWNLOAD_USER = "apache"
const PACKAGEWRITER_GITHUB_DOWNLOAD_BRANCH = "master"

func (pw *PackageWriter) ConfigurePackage(template string, loc string) error {
	str, ok := TemplateRepoMap[template]
	if !ok {
		return util.NewNewtError(fmt.Sprintf("Cannot find matching "+
			"repository for template %s", template))
	}
	pw.repo = str

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
	f, err := os.Open(pfile)
	if err != nil {
		return util.ChildNewtError(err)
	}
	defer f.Close()

	data, _ := ioutil.ReadAll(f)

	// Search & replace file contents
	re := regexp.MustCompile("your-pkg-name")
	res := re.ReplaceAllString(string(data), pw.fullName)

	if err := ioutil.WriteFile(pfile, []byte(res), 0666); err != nil {
		return util.ChildNewtError(err)
	}

	return nil
}

func (pw *PackageWriter) fixupPKG() error {
	pkgBase := path.Base(pw.fullName)

	// Move include file to name after package name
	if err := util.MoveFile(pw.targetPath+"/include/your-path/your-file.h",
		pw.targetPath+"/include/your-path/"+pkgBase+".h"); err != nil {
		return err
	}

	// Move source file
	if err := util.MoveFile(pw.targetPath+"/src/your-source.c",
		pw.targetPath+"/src/"+pkgBase+".c"); err != nil {
		return err
	}

	if err := util.CopyDir(pw.targetPath+"/include/your-path/",
		pw.targetPath+"/include/"+pkgBase+"/"); err != nil {
		return err
	}

	if err := os.RemoveAll(pw.targetPath + "/include/your-path/"); err != nil {
		return util.ChildNewtError(err)
	}

	if err := pw.cleanupPackageFile(pw.targetPath + "/pkg.yml"); err != nil {
		return err
	}

	return nil
}

func (pw *PackageWriter) WritePackage() error {
	dl := pw.downloader

	dl.User = PACKAGEWRITER_GITHUB_DOWNLOAD_USER
	dl.Repo = pw.repo

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Download package template for package type %s.\n",
		strings.ToLower(pw.template))

	tmpdir, err := dl.DownloadRepo(PACKAGEWRITER_GITHUB_DOWNLOAD_BRANCH)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(tmpdir + "/.git/"); err != nil {
		return util.NewNewtError(err.Error())
	}

	if err := util.CopyDir(tmpdir, pw.targetPath); err != nil {
		return err
	}

	switch pw.template {
	case "PKG":
		if err := pw.fixupPKG(); err != nil {
			return err
		}
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
