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
	"path/filepath"
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
		name:   "mynewt-pkg-sdk",
		branch: "master",
	},
	"BSP": templateRepo{
		owner:  "apache",
		name:   "mynewt-pkg-bsp",
		branch: "master",
	},
	"LIB": templateRepo{
		owner:  "apache",
		name:   "mynewt-pkg-pkg",
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
		name:   "mynewt-pkg-pkg",
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

// Creates a table of search-replace pairs.  These pairs are simple
// substitution rules (i.e., not regexes) that get applied to filenames,
// directory names, and the contents of YAML files.
func (pw *PackageWriter) replacementTable() [][]string {
	pkgBase := path.Base(pw.fullName)

	return [][]string{
		{`$$pkgfullname`, pw.fullName},
		{`$$pkgdir`, path.Dir(pw.fullName)},
		{`$$pkgname`, path.Base(pw.fullName)},

		// Legacy.
		{`your-pkg-name`, `"` + pw.fullName + `"`},
		{`your-path`, pkgBase},
		{`your-source`, pkgBase},
		{`your-file`, pkgBase},
	}
}

// Applies all the substitution rules in the supplied table to a string.
func replaceText(s string, table [][]string) string {
	for _, r := range table {
		s = strings.Replace(s, r[0], r[1], -1)
	}

	return s
}

// Applies all the substitution rules in the supplied table to the contents of
// a file.  If the file contents change as a result, the file gets rewritten.
func fixupFileText(path string, table [][]string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return util.ChildNewtError(err)
	}

	s1 := string(data)
	s2 := replaceText(s1, table)

	if s2 != s1 {
		if err := ioutil.WriteFile(path, []byte(s2), 0666); err != nil {
			return util.ChildNewtError(err)
		}
	}

	return nil
}

// Retrieves the names of all child files and directories.
//
// @param path                  The root directory where the traversal starts.
//
// @return []string             All descendent files.
//         []string             All descendent directories.
//         error                Error
func collectPaths(path string) ([]string, []string, error) {
	files := []string{}
	dirs := []string{}

	collect := func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if f.IsDir() {
			dirs = append(dirs, path)
		} else {
			files = append(files, path)
		}
		return nil
	}
	if err := filepath.Walk(path, collect); err != nil {
		return nil, nil, util.ChildNewtError(err)
	}

	return files, dirs, nil
}

// Customizes a template package.  Renames generic files and directories and
// substitutes text in YAML files.
func (pw *PackageWriter) fixupPkg() error {
	table := pw.replacementTable()
	pkgDir := pw.targetPath

	// Apply the replacement patterns to directory names.
	_, dirs, err := collectPaths(pkgDir)
	if err != nil {
		return err
	}
	for _, d1 := range dirs {
		d2 := replaceText(d1, table)
		if d1 != d2 {
			// Make parent directory to allow multiple replacements in path.
			if err := os.MkdirAll(filepath.Dir(d2), os.ModePerm); err != nil {
				return util.ChildNewtError(err)
			}
			if err := os.Rename(d1, d2); err != nil {
				return util.ChildNewtError(err)
			}
		}
	}

	// Replace text inside YAML files.
	files, _, err := collectPaths(pkgDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".yml") {
			if err := fixupFileText(f, table); err != nil {
				return err
			}
		}
	}

	// Apply the replacement patterns to file names.
	for _, f1 := range files {
		f2 := replaceText(f1, table)
		if f2 != f1 {
			if err := os.Rename(f1, f2); err != nil {
				return util.ChildNewtError(err)
			}
		}
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
