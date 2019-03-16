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

package docs

import (
	"fmt"
	"io/ioutil"
	"os"

	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

type DocsBuilder struct {
	project *project.Project
}

type DocDescriptor struct {
	path    string
	name    string
	isLocal bool
}

func NewDocsBuilder() (*DocsBuilder, error) {
	db := &DocsBuilder{}
	db.project = project.GetProject()

	return db, nil
}

func (db *DocsBuilder) GetDocs() ([]*DocDescriptor, error) {
	docs := []*DocDescriptor{}

	for _, repo := range db.project.Repos() {
		name := repo.Name()

		if repo.IsLocal() {
			name = db.project.Name()
		}
		fmt.Println(name)

		descriptor := &DocDescriptor{
			path:    repo.Path() + "/docs",
			name:    name,
			isLocal: repo.IsLocal(),
		}

		docs = append(docs, descriptor)
	}

	return docs, nil
}

func (db *DocsBuilder) generateDoxygen(doc *DocDescriptor, tmpdir string) error {
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Preparing docs, running doxygen in %s\n", doc.path)

	util.CallInDir(doc.path, func() error {
		// Run doxygen
		doxygenGenerate := []string{
			"doxygen",
			"doxygen.xml",
		}

		util.ShellCommand(doxygenGenerate, nil)

		return nil
	})

	util.MoveDir(doc.path+"/_gen/_xml", tmpdir+"/"+doc.name+"-xml")
	os.RemoveAll(doc.path + "/_gen")

	return nil
}

func (db *DocsBuilder) Build(outdir string) error {
	// Get this project's base directory, plus all of the repo directories
	// and copy them to the scratch directory
	// Then run & generate build.
	docs, _ := db.GetDocs()

	tmpdir, _ := ioutil.TempDir("", "docs-repo")

	// Copy the docs into the generation directory one by one.
	for _, doc := range docs {
		if util.NodeNotExist(doc.path) {
			continue
		}

		if util.NodeExist(doc.path + "/doxygen.xml") {
			db.generateDoxygen(doc, tmpdir)
		}

		tmpPath := tmpdir + "/"
		if !doc.isLocal {
			tmpPath = tmpPath + doc.name
		}

		util.StatusMessage(util.VERBOSITY_DEFAULT,
			"Preparing docs, copying docs directory %s to %s\n", doc.path,
			tmpPath)

		util.CopyDir(doc.path, tmpPath)
	}

	// Change into the temporary directory to build it.
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Generating documentation in %s placing results in %s\n", tmpdir, outdir)

	util.CallInDir(tmpdir, func() error {
		// Time to run the build!
		sphinxBuild := []string{
			"sphinx-build",
			"-j", "auto",
			tmpdir,
			outdir,
		}

		util.ShellCommand(sphinxBuild, nil)
		return nil
	})

	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Cleaning up result of generated documentation\n")
	os.RemoveAll(tmpdir)

	return nil
}
