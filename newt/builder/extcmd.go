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

package builder

import (
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/kballard/go-shellquote"
	log "github.com/sirupsen/logrus"
	"mynewt.apache.org/newt/newt/stage"
	"mynewt.apache.org/newt/util"
)

// replaceArtifactsIfChanged compares the artifacts just produced (temp
// directory) to those from the previous build (user bin directory).  If they
// are different, it replaces the old with the new so that they get relinked
// during this build.
func replaceArtifactsIfChanged(oldDir string, newDir string) error {
	eq, err := util.DirsAreEqual(oldDir, newDir)
	if err != nil {
		return err
	}

	if eq {
		// No changes detected.
		return nil
	}

	log.Debugf("changes detected; replacing %s with %s", oldDir, newDir)
	os.RemoveAll(oldDir)
	if err := util.MoveDir(newDir, oldDir); err != nil {
		return err
	}

	return nil
}

// createTempUserDirs creates a set of temporary directories for holding build
// inputs.  It returns:
//     * base-dir
//     * src-dir
//     * include-dir
func createTempUserDirs(label string) (string, string, string, error) {
	tmpDir, err := ioutil.TempDir("", "mynewt-user-"+label)
	if err != nil {
		return "", "", "", util.ChildNewtError(err)
	}
	log.Debugf("created user %s dir: %s", label, tmpDir)

	tmpSrcDir := UserTempSrcDir(tmpDir)
	log.Debugf("creating user %s src dir: %s", label, tmpSrcDir)
	if err := os.MkdirAll(tmpSrcDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		return "", "", "", util.ChildNewtError(err)
	}

	tmpIncDir := UserTempIncludeDir(tmpDir)
	log.Debugf("creating user %s include dir: %s", label, tmpIncDir)
	if err := os.MkdirAll(tmpIncDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		return "", "", "", util.ChildNewtError(err)
	}

	return tmpDir, tmpSrcDir, tmpIncDir, nil
}

// envVarsForCmd calculates the set of environment variables to export for the
// specified external command.
func (t *TargetBuilder) envVarsForCmd(sf stage.StageFunc, userSrcDir string,
	userIncDir string, workDir string) (map[string]string, error) {

	// Determine whether the owning package is part of the loader or the app.
	slot := 0
	buildName := "app"

	if t.LoaderBuilder != nil {
		rpkg := t.res.LpkgRpkgMap[sf.Pkg]
		if rpkg == nil {
			return nil, util.FmtNewtError(
				"resolution missing expected package: %s", sf.Pkg.FullName())
		}

		if t.LoaderBuilder.PkgMap[rpkg] != nil {
			buildName = "loader"
		} else {
			slot = 1
		}
	}

	env, err := t.AppBuilder.EnvVars(slot)
	if err != nil {
		return nil, err
	}

	p := UserEnvParams{
		Lpkg:         sf.Pkg,
		TargetName:   t.target.FullName(),
		BuildProfile: t.target.BuildProfile,
		AppName:      t.appPkg.FullName(),
		BuildName:    buildName,
		UserSrcDir:   userSrcDir,
		UserIncDir:   userIncDir,
		WorkDir:      workDir,
	}
	uenv := UserEnvVars(p)
	for k, v := range uenv {
		env[k] = v
	}

	c, err := t.NewCompiler("", "")
	if err != nil {
		return nil, err
	}
	tenv := ToolchainEnvVars(c)
	for k, v := range tenv {
		env[k] = v
	}

	return env, nil
}

// execExtCmds executes a set of user scripts.
func (t *TargetBuilder) execExtCmds(sf stage.StageFunc, userSrcDir string,
	userIncDir string, workDir string) error {

	env, err := t.envVarsForCmd(sf, userSrcDir, userIncDir, workDir)
	if err != nil {
		return err
	}

	toks, err := shellquote.Split(sf.Name)
	if err != nil {
		return util.FmtNewtError(
			"invalid command string: \"%s\": %s", sf.Name, err.Error())
	}

	// Replace environment variables in command string.
	for i, tok := range toks {
		toks[i] = os.ExpandEnv(tok)
	}

	// If the command is in the user's PATH, expand it to its real location.
	cmd, err := exec.LookPath(toks[0])
	if err == nil {
		toks[0] = cmd
	}

	// Execute the commands from the package's directory.
	pwd, err := os.Getwd()
	if err != nil {
		return util.ChildNewtError(err)
	}
	if err := os.Chdir(sf.Pkg.BasePath()); err != nil {
		return util.ChildNewtError(err)
	}
	defer os.Chdir(pwd)

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Executing %s\n", sf.Name)
	if err := util.ShellInteractiveCommand(toks, env, true); err != nil {
		return err
	}

	return nil
}

// execPreBuildCmds runs the target's set of pre-build user commands.  It is an
// error if any command fails (exits with a nonzero status).
func (t *TargetBuilder) execPreBuildCmds(workDir string) error {
	// Create temporary directories where scripts can put build inputs.
	tmpDir, tmpSrcDir, tmpIncDir, err := createTempUserDirs("pre-build")
	if err != nil {
		return err
	}
	defer func() {
		log.Debugf("removing user pre-build dir: %s", tmpDir)
		os.RemoveAll(tmpDir)
	}()

	for _, sf := range t.res.PreBuildCmdCfg.StageFuncs {
		if err := t.execExtCmds(sf, tmpSrcDir, tmpIncDir, workDir); err != nil {
			return err
		}
	}

	srcDir := UserPreBuildSrcDir(t.target.FullName())
	if err := replaceArtifactsIfChanged(srcDir, tmpSrcDir); err != nil {
		return err
	}

	incDir := UserPreBuildIncludeDir(t.target.FullName())
	if err := replaceArtifactsIfChanged(incDir, tmpIncDir); err != nil {
		return err
	}

	return nil
}

// execPreLinkCmds runs the target's set of post-build user commands.  It is
// an error if any command fails (exits with a nonzero status).
func (t *TargetBuilder) execPreLinkCmds(workDir string) error {
	// Create temporary directories where scripts can put build inputs.
	tmpDir, tmpSrcDir, _, err := createTempUserDirs("pre-link")
	if err != nil {
		return err
	}
	defer func() {
		log.Debugf("removing user pre-link dir: %s", tmpDir)
		os.RemoveAll(tmpDir)
	}()

	for _, sf := range t.res.PreLinkCmdCfg.StageFuncs {
		if err := t.execExtCmds(sf, tmpSrcDir, "", workDir); err != nil {
			return err
		}
	}

	srcDir := UserPreLinkSrcDir(t.target.FullName())
	err = replaceArtifactsIfChanged(srcDir, tmpSrcDir)
	if err != nil {
		return err
	}

	return nil
}

// execPostLinkCmds runs the target's set of post-build user commands.  It is
// an error if any command fails (exits with a nonzero status).
func (t *TargetBuilder) execPostLinkCmds(workDir string) error {
	for _, sf := range t.res.PostLinkCmdCfg.StageFuncs {
		if err := t.execExtCmds(sf, "", "", workDir); err != nil {
			return err
		}
	}

	return nil
}

// makeUserDir creates a temporary directory where scripts can put build
// inputs.
func makeUserDir() (string, error) {
	tmpDir, err := ioutil.TempDir("", "mynewt-user")
	if err != nil {
		return "", util.ChildNewtError(err)
	}
	log.Debugf("created user dir: %s", tmpDir)

	return tmpDir, nil
}

func makeUserWorkDir() (string, error) {
	tmpDir, err := ioutil.TempDir("", "mynewt-user-work")
	if err != nil {
		return "", util.ChildNewtError(err)
	}
	log.Debugf("created user work dir: %s", tmpDir)

	return tmpDir, nil
}
