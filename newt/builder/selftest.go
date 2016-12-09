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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/project"
	"mynewt.apache.org/newt/util"
)

func (b *Builder) SelfTestLink(p *pkg.LocalPackage) error {
	testBpkg := b.PkgMap[p]
	testPath := b.TestExePath(testBpkg)
	if err := b.link(testPath, nil, nil); err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) SelfTestCreateExe() error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	if err := t.AppBuilder.Build(); err != nil {
		return err
	}

	if err := t.AppBuilder.SelfTestLink(t.testPkg); err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) SelfTestExecute() error {
	if err := t.SelfTestCreateExe(); err != nil {
		return err
	}

	if err := t.AppBuilder.SelfTestExecute(t.testPkg); err != nil {
		return err
	}

	return nil
}

func (t *TargetBuilder) SelfTestDebug() error {
	if err := t.PrepBuild(); err != nil {
		return err
	}

	lpkg := t.GetTestPkg()
	if lpkg == nil {
		panic("internal error: attempt to debug target builder with no test " +
			"package")
	}

	bpkg := t.AppBuilder.PkgMap[lpkg]
	if bpkg == nil {
		panic("internal error: local package \"" + lpkg.FullName() +
			"\" not built")
	}

	return t.AppBuilder.debugBin(
		strings.TrimSuffix(t.AppBuilder.TestExePath(bpkg), ".elf"),
		"", false, false)
}

func (b *Builder) testOwner(p *BuildPackage) *BuildPackage {
	if p.Type() != pkg.PACKAGE_TYPE_UNITTEST {
		panic("Expected unittest package; got: " + p.Name())
	}

	curPath := p.BasePath()

	for {
		parentPath := filepath.Dir(curPath)
		if parentPath == project.GetProject().BasePath || parentPath == "." {
			return nil
		}

		parentPkg := b.pkgWithPath(parentPath)
		if parentPkg != nil && parentPkg.Type() != pkg.PACKAGE_TYPE_UNITTEST {
			return parentPkg
		}

		curPath = parentPath
	}
}

func (b *Builder) SelfTestExecute(p *pkg.LocalPackage) error {
	testBpkg := b.PkgMap[p]
	if testBpkg == nil {
		panic("internal error; package-under-test \"" + p.FullName() +
			"\" not in builder")
	}

	testPath := b.TestExePath(testBpkg)
	if err := os.Chdir(filepath.Dir(testPath)); err != nil {
		return err
	}

	util.StatusMessage(util.VERBOSITY_DEFAULT, "Executing test: %s\n",
		testPath)
	cmd := []string{testPath}
	if _, err := util.ShellCommand(cmd, nil); err != nil {
		newtError := err.(*util.NewtError)
		newtError.Text = fmt.Sprintf("Test failure (%s):\n%s", p.Name(),
			newtError.Text)
		return newtError
	}

	return nil
}
