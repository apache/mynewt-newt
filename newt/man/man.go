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

package man

import (
	"fmt"
	"os/exec"
	"path"
	"path/filepath"

	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/newt/repo"
	"mynewt.apache.org/newt/util"
)

const MAN_DOXY_CONF = "man-pages.conf"
const MAN_REL_PATH = "/man"

func buildDoxyManPages(proj *project.Project, repo *repo.Repo) error {
	util.StatusMessage(util.VERBOSITY_DEFAULT,
		"Preparing man-pages, running doxygen for \"%s\"\n", repo.Name())

	doxyCmd := []string{
		"doxygen",
		path.Join(repo.Path(), MAN_DOXY_CONF),
	}

	env := map[string]string{
		"MANPATH":        path.Join(proj.BasePath, MAN_REL_PATH),
		"NEWT_PROJ_ROOT": proj.BasePath,
		"NEWT_REPO_ROOT": repo.Path(),
	}

	_, err := util.ShellCommand(doxyCmd, env)

	return err
}

func buildManDb(proj *project.Project) error {
	util.StatusMessage(util.VERBOSITY_DEFAULT, "Updating man-page index caches\n")

	manDbCmd := []string{
		"mandb",
	}

	env := map[string]string{
		"MANPATH": path.Join(proj.BasePath, MAN_REL_PATH),
	}

	_, err := util.ShellCommand(manDbCmd, env)

	return err
}

func BuildManPages(proj *project.Project) error {
	for _, repo := range proj.Repos() {
		manPath := path.Join(repo.Path(), MAN_DOXY_CONF)
		if util.NodeExist(manPath) {
			err := buildDoxyManPages(proj, repo)
			if err != nil {
				return util.NewNewtError(fmt.Sprintf("%s", err.Error()))
			}
		}
	}
	err := buildManDb(proj)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("%s", err.Error()))
	}

	return nil
}

func RunMan(proj *project.Project, args []string) error {
	binPath, err := exec.LookPath("man")
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("%s", err.Error()))
	}

	manCmd := []string{
		filepath.ToSlash(binPath),
		args[0],
	}

	env := map[string]string{
		"MANPATH": path.Join(proj.BasePath, MAN_REL_PATH),
	}

	err = util.ShellInteractiveCommand(manCmd, env, true)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("%s", err.Error()))
	}

	return nil
}

func RunApropos(proj *project.Project, args []string) error {
	aproposCmd := []string{
		"apropos",
		args[0],
	}

	env := map[string]string{
		"MANPATH": path.Join(proj.BasePath, MAN_REL_PATH),
	}

	output, err := util.ShellCommand(aproposCmd, env)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("%s", err.Error()))
	}

	fmt.Printf("%s", output)
	return nil
}
