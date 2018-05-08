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

// Contains all repo version detection functionality.

package repo

import (
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

var versionYmlMissing = util.FmtNewtError("version.yml missing")
var versionYmlBad = util.FmtNewtError("version.yml bad")

func versString(vers []newtutil.RepoVersion) string {
	s := "["

	for _, v := range vers {
		if len(s) > 1 {
			s += ","
		}
		s += v.String()
	}

	s += "]"
	return s
}

func (r *Repo) DepsForVersion(ver newtutil.RepoVersion) []*RepoDependency {
	// If the project uses a specific commit of this repo rather than a version
	// specifier, ignore the commit string when calculating dependencies.
	// Repos specify dependencies for tags and branches corresponding to
	// version numbers rather than this particular commit.
	ver.Commit = ""

	commit, err := r.CommitFromVer(ver)
	if err != nil {
		return nil
	}

	return r.deps[commit]
}

// Removes extra information from a git commit string.  This throws away
// information and causes some ambiguity, but it allows git commits to be
// specified in a user-friendly manner (e.g., "mynewt_1_3_0_tag" rather than
// "tags/mynewt_1_3_0_tag").
func normalizeCommit(commit string) string {
	commit = strings.TrimPrefix(commit, "tags/")
	commit = strings.TrimPrefix(commit, "origin/")
	commit = strings.TrimPrefix(commit, "heads/")
	return commit
}

// Retrieves the repo's currently checked-out hash.
func (r *Repo) CurrentHash() (string, error) {
	dl := r.downloader
	commit, err := dl.HashFor(r.Path(), "HEAD")
	if err != nil {
		return "",
			util.FmtNewtError("Error finding current hash for \"%s\": %s",
				r.Name(), err.Error())
	}
	return commit, nil
}

// Retrieves all commit strings corresponding to the repo's current state.
func (r *Repo) CurrentCommits() ([]string, error) {
	commits, err := r.downloader.CommitsFor(r.Path(), "HEAD")
	if err != nil {
		return nil, err
	}

	return commits, nil
}

// Retrieves the commit that the specified version maps to in `repository.yml`.
// Note: This returns the specific commit in `repository.yml`; there may be
// other commits that refer to the same point in the repo's history.
func (r *Repo) CommitFromVer(ver newtutil.RepoVersion) (string, error) {
	if ver.Commit != "" {
		return ver.Commit, nil
	}

	nver, err := r.NormalizeVersion(ver)
	if err != nil {
		return "", err
	}

	commit := r.vers[nver]
	if commit == "" {
		return "",
			util.FmtNewtError(
				"repo \"%s\" version %s does not map to a commit",
				r.Name(), nver.String())
	}

	return commit, nil
}

// Determines whether the two specified commits refer to the same point in the
// repo's history.
func (r *Repo) CommitsEquivalent(c1 string, c2 string) (bool, error) {
	if c1 == "" {
		return c2 == "", nil
	} else if c2 == "" {
		return false, nil
	}

	commits, err := r.downloader.CommitsFor(r.Path(), c1)
	if err != nil {
		return false, err
	}

	for _, c := range commits {
		if c == c2 {
			return true, nil
		}
	}

	return false, nil
}

// Retrieves the unique commit hash corresponding to the specified repo
// version.
func (r *Repo) HashFromVer(ver newtutil.RepoVersion) (string, error) {
	commit, err := r.CommitFromVer(ver)
	if err != nil {
		return "", err
	}

	hash, err := r.downloader.HashFor(r.Path(), commit)
	if err != nil {
		return "", err
	}

	return hash, nil
}

// Retrieves all versions that map to the specified commit string.
// Note: This function only considers the specified commit.  If any equivalent
// commits exist, they are not considered here.
func (r *Repo) VersFromCommit(commit string) []newtutil.RepoVersion {
	var vers []newtutil.RepoVersion

	for v, c := range r.vers {
		if c == commit {
			vers = append(vers, v)
		}
	}

	newtutil.SortVersions(vers)
	return vers
}

// Retrieves all versions that map to any of the specified commit strings.
// Note: This function only considers the specified commits.  If any equivalent
// commits exist, they are not considered here.
func (r *Repo) VersFromCommits(commits []string) []newtutil.RepoVersion {
	var vers []newtutil.RepoVersion
	for _, c := range commits {
		vers = append(vers, r.VersFromCommit(normalizeCommit(c))...)
	}

	newtutil.SortVersions(vers)
	return vers
}

// Retrieves all versions that map to the specified commit.  Versions that map
// to equivalent commits are also included.
func (r *Repo) VersFromEquivCommit(
	commit string) ([]newtutil.RepoVersion, error) {

	commits, err := r.downloader.CommitsFor(r.Path(), commit)
	if err != nil {
		return nil, err
	}

	return r.VersFromCommits(commits), nil
}

// Converts the specified versions to their equivalent x.x.x forms for this
// repo.  For example, this might convert "0-dev" to "0.0.0" (depending on the
// `repository.yml` file contents).
func (r *Repo) NormalizedVersions() ([]newtutil.RepoVersion, error) {
	verMap := map[newtutil.RepoVersion]struct{}{}

	for ver, _ := range r.vers {
		nver, err := r.NormalizeVersion(ver)
		if err != nil {
			return nil, err
		}
		verMap[nver] = struct{}{}
	}

	vers := make([]newtutil.RepoVersion, 0, len(verMap))
	for ver, _ := range verMap {
		vers = append(vers, ver)
	}

	return vers, nil
}

// Converts the specified version to its equivalent x.x.x form for this repo.
// For example, this might convert "0-dev" to "0.0.0" (depending on the
// `repository.yml` file contents).
func (r *Repo) NormalizeVersion(
	ver newtutil.RepoVersion) (newtutil.RepoVersion, error) {

	origVer := ver
	for {
		if ver.Stability == "" ||
			ver.Stability == newtutil.VERSION_STABILITY_NONE {
			return ver, nil
		}

		verStr := r.vers[ver]
		if verStr == "" {
			return ver, util.FmtNewtError(
				"cannot normalize version \"%s\" for repo \"%s\"; "+
					"no mapping to numeric version",
				origVer.String(), r.Name())
		}

		nextVer, err := newtutil.ParseRepoVersion(verStr)
		if err != nil {
			return ver, err
		}
		ver = nextVer
	}
}

// Normalizes the version component of a version requirement.
func (r *Repo) NormalizeVerReq(verReq newtutil.RepoVersionReq) (
	newtutil.RepoVersionReq, error) {

	ver, err := r.NormalizeVersion(verReq.Ver)
	if err != nil {
		return verReq, err
	}

	verReq.Ver = ver
	return verReq, nil
}

// Normalizes the version component of each specified version requirement.
func (r *Repo) NormalizeVerReqs(verReqs []newtutil.RepoVersionReq) (
	[]newtutil.RepoVersionReq, error) {

	result := make([]newtutil.RepoVersionReq, len(verReqs))
	for i, verReq := range verReqs {
		n, err := r.NormalizeVerReq(verReq)
		if err != nil {
			return nil, err
		}
		result[i] = n
	}

	return result, nil
}

// Compares the two specified versions for equality.  Two versions are equal if
// they ultimately map to the same commit object.
func (r *Repo) VersionsEqual(v1 newtutil.RepoVersion,
	v2 newtutil.RepoVersion) bool {

	if newtutil.CompareRepoVersions(v1, v2) == 0 {
		return true
	}

	h1, err := r.HashFromVer(v1)
	if err != nil {
		return false
	}

	h2, err := r.HashFromVer(v2)
	if err != nil {
		return false
	}

	return h1 == h2
}

// Parses the `version.yml` file at the specified path.  On success, the parsed
// version is returned.
func parseVersionYml(path string) (newtutil.RepoVersion, error) {
	yc, err := newtutil.ReadConfigPath(path)
	if err != nil {
		if util.IsNotExist(err) {
			return newtutil.RepoVersion{}, versionYmlMissing
		} else {
			return newtutil.RepoVersion{}, versionYmlBad
		}
	}

	verString := yc.GetValString("repo.version", nil)
	if verString == "" {
		return newtutil.RepoVersion{}, versionYmlBad
	}

	ver, err := newtutil.ParseRepoVersion(verString)
	if err != nil || !ver.IsNormalized() {
		return newtutil.RepoVersion{}, versionYmlBad
	}

	return ver, nil
}

// Reads and parses the `version.yml` file belonging to an installed repo.
func (r *Repo) installedVersionYml() (*newtutil.RepoVersion, error) {
	ver, err := parseVersionYml(r.Path() + "/" + REPO_VER_FILE_NAME)
	if err != nil {
		return nil, err
	}

	return &ver, nil
}

// Downloads and parses the `version.yml` file from the repo at the specified
// commit.
func (r *Repo) nonInstalledVersionYml(
	commit string) (*newtutil.RepoVersion, error) {

	filePath, err := r.downloadFile(commit, REPO_VER_FILE_NAME)
	if err != nil {
		// The download failed.  Determine if the commit string is bad or if
		// the file just doesn't exist in that commit.
		if _, e2 := r.downloader.CommitType(r.localPath, commit); e2 != nil {
			// Bad commit string.
			return nil, err
		}

		// The commit exists, but it doesn't contain a `version.yml` file.
		// Assume the commit corresponds to version 0.0.0.
		return nil, versionYmlMissing
	}

	ver, err := parseVersionYml(filePath)
	if err != nil {
		return nil, err
	}

	return &ver, nil
}

// Tries to determine which repo version meets the specified criteria:
//  * Maps to the specified commit string (or an equivalent commit).
//  * Is equal to the specified version read from `version.yml` (if not-null).
func (r *Repo) inferVersion(commit string, vyVer *newtutil.RepoVersion) (
	*newtutil.RepoVersion, error) {

	// Search `repository.yml` for versions that the specified commit maps to.
	ryVers, err := r.VersFromEquivCommit(commit)
	if err != nil {
		return nil, err
	}

	// If valid versions were derived from both `version.yml` and the specified
	// commit+`repository.yml`, look for a common version.
	if vyVer != nil {
		if len(ryVers) > 0 {
			for _, cv := range ryVers {
				if newtutil.CompareRepoVersions(*vyVer, cv) == 0 {
					return vyVer, nil
				}
			}

			util.StatusMessage(util.VERBOSITY_QUIET,
				"WARNING: Version mismatch in %s:%s; "+
					"repository.yml:%s, version.yml:%s\n",
				r.Name(), commit, versString(ryVers), vyVer.String())
		} else {
			// If the set of commits don't match a version from
			// `repository.yml`, record the commit hash in the version
			// specifier.  This will distinguish the returned version from its
			// corresponding official release.
			hash, err := r.downloader.HashFor(r.Path(), commit)
			if err != nil {
				return nil, err
			}
			vyVer.Commit = hash
		}

		// Always prefer the version in `version.yml`.
		log.Debugf("Inferred version %s from %s:%s from version.yml",
			vyVer.String(), r.Name(), commit)
		return vyVer, nil
	}

	if len(ryVers) > 0 {
		log.Debugf("Inferred version %s for %s:%s from repository.yml",
			ryVers[0].String(), r.Name(), commit)
		return &ryVers[0], nil
	}

	return nil, nil
}

// Retrieves the installed version of the repo.  Returns nil if the version
// cannot be detected.
func (r *Repo) InstalledVersion() (*newtutil.RepoVersion, error) {
	vyVer, err := r.installedVersionYml()
	if err != nil && err != versionYmlMissing && err != versionYmlBad {
		return nil, err
	}

	hash, err := r.CurrentHash()
	if err != nil {
		return nil, err
	}

	ver, err := r.inferVersion(hash, vyVer)
	if err != nil {
		return nil, err
	}

	return ver, nil
}

// Retrieves the repo version corresponding to the specified commit.  Returns
// nil if the version cannot be detected.
func (r *Repo) NonInstalledVersion(
	commit string) (*newtutil.RepoVersion, error) {

	ver, versionYmlErr := r.nonInstalledVersionYml(commit)
	if versionYmlErr != nil &&
		versionYmlErr != versionYmlMissing &&
		versionYmlErr != versionYmlBad {

		return nil, versionYmlErr
	}

	ver, err := r.inferVersion(commit, ver)
	if err != nil {
		return nil, err
	}

	if ver == nil {
		if versionYmlErr == versionYmlMissing {
			util.StatusMessage(util.VERBOSITY_QUIET,
				"WARNING: %s:%s does not contain a `version.yml` file.\n",
				r.Name(), commit)
		} else if versionYmlErr == versionYmlBad {
			util.StatusMessage(util.VERBOSITY_QUIET,
				"WARNING: %s:%s contains a malformed `version.yml` file.\n",
				r.Name(), commit)
		}
	}

	return ver, nil
}
