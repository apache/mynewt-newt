/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cli

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
)

const (
	INSTALLER_UNKNOWN = iota
	INSTALLER_FILE_TARBALL
	INSTALLER_FILE_DIRECTORY
	INSTALLER_HTTP_TARBALL
	INSTALLER_GIT_REPO
)

const (
	INSTALLER_GIT_UNKNOWN = iota
	INSTALLER_GIT_CLONE
	INSTALLER_GIT_CLONE_CLEAN
	INSTALLER_GIT_SUBTREE
)

type Installer struct {
	// Whether or not to fetch package dependencies when downloading a package
	FetchDeps bool
	// Whether or not to silently perform operation, or prompt
	Silent bool

	// Repository to install package into
	repo *Repo
	// System package manager to use for package installation
	pkgMgr *PkgMgr
}

// Allocate, return and initialize a new Installer object.
// @param r The Repository that this installer should point to
// @return Allocated Installer object on success, non-nil error on failure
func NewInstaller(r *Repo) (*Installer, error) {
	inst := &Installer{}
	if err := inst.Init(r); err != nil {
		return nil, err
	}

	return inst, nil
}

// Initialize the installer object.
// @param r The repository to initialize the installer in
// @return error is nil on success, non-nil on failure
func (inst *Installer) Init(r *Repo) error {
	var err error

	inst.repo = r

	inst.pkgMgr, err = NewPkgMgr(inst.repo, nil)
	if err != nil {
		return err
	}

	return nil
}

// Parse a URL into a URL type, which is one of the INSTALLER_*
// types defined in the package.
// @param urlPath The path of the URL to analyze
// @return urlType on success, non-nil error on failure
func (inst *Installer) parseUrlType(urlPath string) (int, error) {
	var urlType int

	parts, err := url.Parse(urlPath)
	if err != nil {
		return INSTALLER_UNKNOWN, err
	}

	urlType = INSTALLER_UNKNOWN
	switch parts.Scheme {
	case "git":
		urlType = INSTALLER_GIT_REPO
	case "http":
		fallthrough
	case "https":
		ext := filepath.Ext(parts.Path)
		if ext == ".git" {
			urlType = INSTALLER_GIT_REPO
		} else if ext == ".tar.gz" || ext == ".tgz" {
			urlType = INSTALLER_HTTP_TARBALL
		}
	case "file":
		urlPath = parts.Path
		fallthrough
	default:
		ext := filepath.Ext(urlPath)
		if ext == ".tar.gz" || ext == ".tgz" {
			urlType = INSTALLER_FILE_TARBALL
		} else {
			urlType = INSTALLER_FILE_DIRECTORY
		}
	}

	if urlType == INSTALLER_UNKNOWN {
		return 0, NewStackError("Unknown URL type " + parts.Scheme)
	}

	log.Printf("[DEBUG] Installer parsed URL %s (path: %s, scheme: %s, ext: %s, type: %d)",
		urlPath, parts.Path, parts.Scheme, filepath.Ext(parts.Path), urlType)

	return urlType, nil
}

// Copy a local file or directory into the temporary package directory.
// @param tmpDir The temporary directory to copy the file into
// @param node The file to copy
// @param nodeType The type of file location specified by node, can be either
//                 INSTALLER_FILE_TARBALL or INSTALLER_FILE_DIRECTORY.
// @return The temporary directory which the node has been copied into, or error is non-nil on failure
//
func (inst *Installer) copyLocal(tmpDir string, node string, nodeType int) (string, error) {
	// First copy local file to temporary directory
	// if its a tarball, ungz it
	// return the temporary path the package is installed into
	dest := tmpDir
	if nodeType == INSTALLER_FILE_TARBALL {
		dest = dest + ".tgz"
	} else {
		dest = dest + "/"
	}

	_, err := ShellCommand("cp -rf " + node + " " + dest)
	if err != nil {
		return "", err
	}

	localPkgDir := ""

	// if its a tarball, ungz it
	if nodeType == INSTALLER_FILE_TARBALL {
		if err := os.Chdir(filepath.Dir(dest)); err != nil {
			return "", err
		}

		_, err := ShellCommand("tar xf " + filepath.Base(dest) + " -C " + tmpDir + "/")
		if err != nil {
			return "", err
		}

		if err := os.Chdir(tmpDir); err != nil {
			return "", err
		}

		// find the directory that this decompressed into
		dirs, err := ioutil.ReadDir(".")
		if err != nil {
			return "", err
		}

		for _, dir := range dirs {
			if dir.IsDir() {
				localPkgDir = tmpDir + "/" + dir.Name()
				break
			}
		}
		if localPkgDir == "" {
			return "", NewStackError(fmt.Sprintf("No package found in %s", filepath.Dir(node)))
		}
	} else {
		localPkgDir = dest + "/" + filepath.Base(node)
	}

	return localPkgDir, nil
}

// Copy a remote git repository into the temporary package directory.
// @param tmpDir The temporary directory to copy the file into
// @param url The git URL to copy from
// @param gitType The type of git command to copy from (i.e. either a subtree, a clone or a
//                clean install.)
// @return The temporary directory which the node has been copied into, or error is non-nil on failure
//
func (inst *Installer) copyGit(tmpDir string, url string, gitType int) (string, error) {
	switch gitType {
	case INSTALLER_GIT_CLONE:
		fallthrough
	case INSTALLER_GIT_CLONE_CLEAN:
		// git clone url tmpDir/package
		// if clean { remove .git }
		_, err := ShellCommand(fmt.Sprintf("git clone %s %s", url, tmpDir))
		if err != nil {
			return "", NewStackError(err.Error())
		}
		if gitType == INSTALLER_GIT_CLONE_CLEAN {
			_, err := ShellCommand(fmt.Sprintf("rm -rf %s/.git", tmpDir))
			if err != nil {
				return "", err
			}
		}
	case INSTALLER_GIT_SUBTREE:
		// git fetch url
		// git subtree add prefix URL
	}

	return tmpDir, nil
}

// Fetch a package from a URL, and return the local temporary directory that stores the package
// and its dependencies
//
// @param instDirName Fetch a remote package specified by url
// @param url The URL to Fetch locally
// @param gitType The type of git command to use with git URLs
// @return The temporary directory the packages are copied into, error is non-nil on
//         failure.
func (inst *Installer) FetchPackage(instDirName string, url string, gitType int) (string,
	error) {
	urlType, err := inst.parseUrlType(url)
	if err != nil {
		return "", err
	}

	var localPkgDir string

	tmpDir, err := inst.repo.GetTmpDir(instDirName, "package")
	if err != nil {
		return "", err
	}

	switch urlType {
	case INSTALLER_FILE_TARBALL:
		fallthrough
	case INSTALLER_FILE_DIRECTORY:
		localPkgDir, err = inst.copyLocal(tmpDir, url, urlType)
		if err != nil {
			return "", err
		}
	case INSTALLER_GIT_REPO:
		localPkgDir, err = inst.copyGit(tmpDir, url, gitType)
		if err != nil {
			return "", err
		}
	}

	log.Printf("[DEBUG] Package from URL %s fetched to %s", url, localPkgDir)

	return localPkgDir, nil
}

// Install the URL into the local repository (set by instantiating an Installer object.)
//
// @param url The URL to install
// @param gitType The type of commands to use, if the URL is a git URL
// @return error is nil on success and non-nil on failure.
func (inst *Installer) Install(url string, gitType int) error {
	// First, create temporary package directory
	installDir, err := inst.repo.GetTmpDir(inst.repo.BasePath+"/.repo/tmp/", "install")
	if err != nil {
		return err
	}

	// First fetch the package to a local directory.
	pkgDir, err := inst.FetchPackage(installDir, url, gitType)
	if err != nil {
		return err
	}

	// Then, take the package, and verify its contents
	pkg, err := inst.pkgMgr.VerifyPackage(pkgDir)
	if err != nil {
		return err
	}

	fmt.Println(pkg.BasePath)

	// XXX: Ignore package dependencies for now
	// XXX: If configured, fetch dependencies, and verify their contents (recursively)
	// XXX: Then install dependencies

	// Then, install the package locally

	return nil
}
