/*
 Copyright 2015 Stack Inc.
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
	"errors"
	"strings"
)

const TARGET_SECT_PREFIX = "_target_"

type Target struct {
	Vars map[string]string

	Arch string
	Cdef string

	Bsp string

	Repo *Repo
}

// Check if the target specified by name exists for the Repo specified by
// r
func TargetExists(r *Repo, name string) bool {
	_, err := r.GetConfig(TARGET_SECT_PREFIX+name, "name")
	if err == nil {
		return true
	} else {
		return false
	}
}

// Load the target specified by name for the repository specified by r
func LoadTarget(r *Repo, name string) (*Target, error) {
	t := &Target{
		Repo: r,
	}

	var err error

	t.Vars, err = r.GetConfigSect(TARGET_SECT_PREFIX + name)
	if err != nil {
		return nil, err
	}

	// Cannot have both a project and package set
	if t.Vars["project"] != "" && t.Vars["pkg"] != "" {
		return nil, errors.New("Target " + t.Vars["name"] + " cannot have a " +
			"project and package set.")
	}

	// Must have an architecture set, default to sim.
	if t.Vars["arch"] == "" {
		t.Vars["arch"] = "sim"
		t.Arch = "sim"
	} else {
		t.Arch = t.Vars["arch"]
	}

	t.Cdef = t.Vars["compiler_def"]
	if t.Cdef == "" {
		t.Cdef = "default"
	}

	t.Bsp = t.Vars["bsp"]

	return t, nil
}

// Get a list of targets for the repository specified by r
func GetTargets(r *Repo) ([]*Target, error) {
	targets := []*Target{}
	for sect, _ := range r.Config {
		if strings.HasPrefix(sect, TARGET_SECT_PREFIX) {
			target, err := LoadTarget(r, sect[len(TARGET_SECT_PREFIX):len(sect)])
			if err != nil {
				return nil, err
			}

			targets = append(targets, target)
		}
	}
	return targets, nil
}

// Get a map[] of variables for this target
func (t *Target) GetVars() map[string]string {
	return t.Vars
}

// Return the compiler definition file for this target
func (t *Target) GetCompiler() string {
	path := t.Repo.BasePath + "/compiler/"
	if t.Vars["compiler"] != "" {
		path += t.Vars["compiler"]
	} else {
		path += t.Arch
	}
	path += "/"

	return path
}

// Build the target
func (t *Target) Build() error {
	if t.Vars["project"] != "" {
		// Now load and build the project.
		p, err := LoadProject(t.Repo, t, t.Vars["project"])
		if err != nil {
			return err
		}
		// The project is the target, and builds itself.
		if err = p.Build(); err != nil {
			return err
		}
	} else if t.Vars["pkg"] != "" {
		pm, err := NewPkgMgr(t.Repo, t)
		if err != nil {
			return err
		}

		err = pm.Build(t.Vars["pkg"])
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Target) BuildClean(cleanAll bool) error {
	if t.Vars["project"] != "" {
		p, err := LoadProject(t.Repo, t, t.Vars["project"])
		if err != nil {
			return err
		}

		// The project is the target, and build cleans itself.
		if err = p.BuildClean(cleanAll); err != nil {
			return err
		}
	} else if t.Vars["pkg"] != "" {
		pm, err := NewPkgMgr(t.Repo, t)
		if err != nil {
			return err
		}
		err = pm.BuildClean(t.Vars["pkg"], cleanAll)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Target) Test(cmd string, flag bool) error {
	if t.Vars["project"] != "" {
		return errors.New("Tests not supported on projects, only packages")
	}

	pm, err := NewPkgMgr(t.Repo, t)
	if err != nil {
		return err
	}

	var tests []string
	if t.Vars["tests"] != "" {
		tests = strings.Split(t.Vars["tests"], " ")
	} else {
		tests, err = pm.GetPackageTests(t.Vars["pkg"])
		if err != nil {
			return err
		}
	}

	switch cmd {
	case "test":
		err = pm.Test(t.Vars["pkg"], flag, tests)
	case "testclean":
		err = pm.TestClean(t.Vars["pkg"], tests, flag)
	default:
		err = errors.New("Unknown command to Test() " + cmd)
	}
	if err != nil {
		return err
	}

	return nil
}

// Save the target's configuration elements
func (t *Target) Save() error {
	r := t.Repo

	if _, ok := t.Vars["name"]; !ok {
		return errors.New("Cannot save a target without a name")
	}

	targetCfg := TARGET_SECT_PREFIX + t.Vars["name"]

	for k, v := range t.Vars {
		r.SetConfig(targetCfg, k, v)
	}

	return nil
}
