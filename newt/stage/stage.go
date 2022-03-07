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

// stage - utility for generating C code consisting of a sequence of function
// calls ordered by stage number.
//
// This package is used by sysinit and sysdown.

package stage

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/val"
	"mynewt.apache.org/newt/util"
)

type StageFunc struct {
	Stage      val.ValSetting
	Name       string
	ReturnType string
	ArgList    string
	Pkg        *pkg.LocalPackage
	Deps       []*StageFunc
	DepsI      []*StageFunc
	Resolved   bool
	Resolving  bool
}

func NewStageFunc(name string, textVal string,
	p *pkg.LocalPackage, cfg *syscfg.Cfg) (StageFunc, error) {

	vs, err := val.ResolveValSetting(textVal, cfg)
	if err != nil {
		return StageFunc{}, err
	}

	// Ensure setting resolves to an integer.
	if _, err := vs.IntVal(); err != nil {
		return StageFunc{}, util.FmtNewtError("Invalid setting: \"%s: %s\"; "+
			"value \"%s\" does not resolve to an integer",
			name, textVal, textVal)
	}

	sf := StageFunc{
		Name:  name,
		Stage: vs,
		Pkg:   p,
	}

	return sf, nil
}

func NewStageFuncMultiDeps(name string, stageDeps []string,
	p *pkg.LocalPackage, cfg *syscfg.Cfg) (StageFunc, error) {
	sf := StageFunc{
		Name:  name,
		Pkg:   p,
		Stage: val.ValSetting{},
	}

	for _, stageDep := range stageDeps {
		s := strings.TrimPrefix(stageDep, "$before:")
		if s != stageDep {
			sf.Stage.Befores = append(sf.Stage.Befores, s)
		} else {
			s = strings.TrimPrefix(stageDep, "$after:")
			if s != stageDep {
				sf.Stage.Afters = append(sf.Stage.Afters, s)
			} else {
				return StageFunc{}, util.FmtNewtError("Invalid setting: \"%s: %s\"; "+
					"value should specify a $before or $after dependency.", name, stageDep)
			}
		}
	}

	return sf, nil
}

func ValIsDep(textVal string) bool {
	return strings.HasPrefix(textVal, "$before:") || strings.HasPrefix(textVal, "$after:")
}

// SortStageFuncs performs an in-place sort of the provided StageFunc slice.
func SortStageFuncs(sfs []StageFunc, entryType string) {
	sort.Slice(sfs, func(i int, j int) bool {
		a := sfs[i]
		b := sfs[j]

		inta, _ := a.Stage.IntVal()
		intb, _ := b.Stage.IntVal()

		// 1: Sort by stage number.
		if inta < intb {
			return true
		} else if intb < inta {
			return false
		}

		// 2: Sort by entry name.
		switch strings.Compare(a.Name, b.Name) {
		case -1:
			return true
		case 1:
			return false
		}

		// Same stage and function name?
		log.Warnf("Warning: Identical %s entries detected: %s",
			entryType, a.Name)

		return true
	})
}

func (f *StageFunc) ReturnTypeString() string {
	if f.ReturnType == "" {
		return "void"
	} else {
		return f.ReturnType
	}
}

func (f *StageFunc) ArgListString() string {
	if f.ArgList == "" {
		return "void"
	} else {
		return f.ArgList
	}
}

// WriteCalls emits C code: a list of function prototypes corresponding to the
// provided slice of stage functions.
func WritePrototypes(sortedFns []StageFunc, w io.Writer) {
	for _, f := range sortedFns {
		fmt.Fprintf(w, "%s %s(%s);\n",
			f.ReturnTypeString(), f.Name, f.ArgListString())
	}
}

// WriteCalls emits C code: a sequence of function calls corresponding to the
// provided slice of stage functions.
func WriteCalls(sortedFuncs []StageFunc, argList string, w io.Writer) {
	prevStage := -1
	dupCount := 0

	for i, f := range sortedFuncs {
		intStage, _ := f.Stage.IntVal()
		noStage := len(f.Stage.Afters) > 0 || len(f.Stage.Befores) > 0

		if intStage != prevStage {
			prevStage = intStage
			dupCount = 0

			if noStage {
				fmt.Fprintf(w, "\n")
			} else {
				if i != 0 {
					fmt.Fprintf(w, "\n")
				}
				fmt.Fprintf(w, "    /*** Stage %d */\n", intStage)
			}
		} else {
			dupCount += 1
		}

		if noStage {
			for _, s := range f.Stage.Afters {
				fmt.Fprintf(w, "    /* [$after:%s]: %s (%s) */\n",
					s, f.Name, f.Pkg.Name())
			}
			for _, s := range f.Stage.Befores {
				fmt.Fprintf(w, "    /* [$before:%s]: %s (%s) */\n",
					s, f.Name, f.Pkg.Name())
			}
		} else {
			fmt.Fprintf(w, "    /* %d.%d: %s (%s) */\n",
				intStage, dupCount, f.Name, f.Pkg.Name())
		}
		fmt.Fprintf(w, "    %s(%s);\n", f.Name, argList)
	}
}

// WriteArr emits C code: an array body of function pointers represented by the
// supplied slice.  The caller must 1) write the array declaration before
// calling this function, and 2) write "};" afterwards.
func WriteArr(sortedFuncs []StageFunc, w io.Writer) {
	prevStage := -1
	dupCount := 0

	for i, f := range sortedFuncs {
		intStage, _ := f.Stage.IntVal()

		if intStage != prevStage {
			prevStage = intStage
			dupCount = 0

			if i != 0 {
				fmt.Fprintf(w, "\n")
			}
			fmt.Fprintf(w, "    /*** Stage %d */\n", intStage)
		} else {
			dupCount += 1
		}

		fmt.Fprintf(w, "    /* %d.%d: %s (%s) */\n",
			intStage, dupCount, f.Name, f.Pkg.Name())
		fmt.Fprintf(w, "    %s,\n", f.Name)
	}
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "    /*** Array terminator. */\n")
	fmt.Fprintf(w, "    0\n")
}

// EnsureWritten writes the specified file if its contents differ from those of
// the supplied byte slice.
func EnsureWritten(path string, contents []byte) error {
	unchanged, err := util.FileContains(contents, path)
	if err != nil {
		return err
	}

	if unchanged {
		log.Debugf("file unchanged; not writing src file (%s).", path)
		return nil
	}

	log.Debugf("file changed; writing src file (%s).", path)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return util.NewNewtError(err.Error())
	}

	if err := ioutil.WriteFile(path, contents, 0644); err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}
