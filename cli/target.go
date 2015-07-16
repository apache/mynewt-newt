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
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

const TARGET_SECT_PREFIX = "_target_"

type Target struct {
	Vars map[string]string

	Name string

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

func (t *Target) SetDefaults() error {
	if t.Vars["project"] != "" && t.Vars["pkg"] != "" {
		return NewStackError("Target " + t.Vars["name"] + " cannot have a " +
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

	t.Name = t.Vars["name"]

	return nil
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
	err = t.SetDefaults()
	if err != nil {
		return nil, err
	}

	return t, nil
}

// Export a target, or all targets.  If exportAll is true, then all targets are exported, if false,
// then only the target represented by targetName is exported
func ExportTargets(r *Repo, name string, exportAll bool, fp *os.File) error {
	targets, err := GetTargets(r)
	if err != nil {
		return err
	}

	for _, target := range targets {
		log.Printf("[DEBUG] Exporting target %s", target.Name)

		if !exportAll && target.Name != name {
			continue
		}

		fmt.Fprintf(fp, "@target=%s\n", target.Name)

		for k, v := range target.GetVars() {
			fmt.Fprintf(fp, "%s=%s\n", k, v)
		}
	}
	fmt.Fprintf(fp, "@endtargets\n")

	return nil
}

func ImportTargets(r *Repo, name string, importAll bool, fp *os.File) error {
	s := bufio.NewScanner(fp)

	var currentTarget *Target = nil

	targets := make([]*Target, 0, 10)

	if importAll {
		log.Printf("[DEBUG] Importing all targets from %s", fp.Name())
	} else {
		log.Printf("[DEBUG] Importing target %s from %s", name, fp.Name())
	}

	for s.Scan() {
		line := s.Text()

		// scan lines
		// lines defining a target start with @
		if idx := strings.Index(line, "@"); idx == 0 {
			// save existing target if it exists
			if currentTarget != nil {
				targets = append(targets, currentTarget)
			}

			// look either for an end of target definitions, or a new target definition
			if line == "@endtargets" {
				break
			} else {
				// create a current target
				elements := strings.SplitN(line, "=", 2)
				// name is elements[0], and value is elements[1]
				currentTarget = &Target{
					Repo: r,
				}

				var err error
				currentTarget.Vars = map[string]string{}
				if err != nil {
					return err
				}

				currentTarget.Vars["name"] = elements[1]
			}
		} else {
			if currentTarget == nil {
				return NewStackError("No target present when variables being set in import file")
			}
			// target variables, set these on the current target
			elements := strings.SplitN(line, "=", 2)
			currentTarget.Vars[elements[0]] = elements[1]
		}
	}

	if err := s.Err(); err != nil {
		return err
	}

	for _, target := range targets {
		if err := target.SetDefaults(); err != nil {
			return err
		}

		if err := target.Save(); err != nil {
			return err
		}
	}

	return nil
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
		pm, err := NewPkgMgr(t.Repo)
		if err != nil {
			return err
		}

		err = pm.Build(t, t.Vars["pkg"])
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
		pm, err := NewPkgMgr(t.Repo)
		if err != nil {
			return err
		}
		err = pm.BuildClean(t, t.Vars["pkg"], cleanAll)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Target) Test(cmd string, flag bool) error {
	if t.Vars["project"] != "" {
		return NewStackError("Tests not supported on projects, only packages")
	}

	pm, err := NewPkgMgr(t.Repo)
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
		err = pm.Test(t, t.Vars["pkg"], flag, tests)
	case "testclean":
		err = pm.TestClean(t, t.Vars["pkg"], tests, flag)
	default:
		err = NewStackError("Unknown command to Test() " + cmd)
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
		return NewStackError("Cannot save a target without a name")
	}

	targetCfg := TARGET_SECT_PREFIX + t.Vars["name"]

	for k, v := range t.Vars {
		if err := r.SetConfig(targetCfg, k, v); err != nil {
			return err
		}
	}

	return nil
}

func (t *Target) Remove() error {
	r := t.Repo

	if _, ok := t.Vars["name"]; !ok {
		return NewStackError("Cannot remove a target without a name")
	}

	cfgSect := TARGET_SECT_PREFIX + t.Vars["name"]

	for k, _ := range t.Vars {
		if err := r.DelConfig(cfgSect, k); err != nil {
			return err
		}
	}

	return nil
}
