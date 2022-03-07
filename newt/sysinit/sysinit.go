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

package sysinit

import (
	"bytes"
	"fmt"
	"github.com/spf13/cast"
	"io"
	"mynewt.apache.org/newt/util"
	"sort"
	"strings"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/stage"
	"mynewt.apache.org/newt/newt/syscfg"
)

type SysinitCfg struct {
	// Sorted in call order (stage-num,function-name).
	StageFuncs []stage.StageFunc

	// Strings describing errors encountered while parsing the sysinit config.
	InvalidSettings []string

	// Contains sets of entries with conflicting function names.
	//     [function-name] => <slice-of-stages-with-function-name>
	Conflicts map[string][]stage.StageFunc
}

func (scfg *SysinitCfg) readOnePkg(lpkg *pkg.LocalPackage, cfg *syscfg.Cfg) {
	settings := cfg.AllSettingsForLpkg(lpkg)
	initMap := lpkg.InitFuncs(settings)
	for name, stageDef := range initMap {
		var sf stage.StageFunc
		var err error

		switch stageDef.(type) {
		default:
			stageStr := cast.ToString(stageDef)

			if stage.ValIsDep(stageStr) {
				stageDeps := strings.Split(stageStr, ",")
				sf, err = stage.NewStageFuncMultiDeps(name, stageDeps, lpkg, cfg)
			} else {
				sf, err = stage.NewStageFunc(name, stageStr, lpkg, cfg)
			}
		case []interface{}:
			var stageDeps []string

			for _, stageDepIf := range stageDef.([]interface{}) {
				stageDeps = append(stageDeps, cast.ToString(stageDepIf))
			}

			sf, err = stage.NewStageFuncMultiDeps(name, stageDeps, lpkg, cfg)
		}

		if err != nil {
			scfg.InvalidSettings = append(scfg.InvalidSettings, err.Error())
		} else {
			scfg.StageFuncs = append(scfg.StageFuncs, sf)
		}
	}
}

// Searches the sysinit configuration for entries with identical function
// names.  The sysinit configuration object is populated with the results.
func (scfg *SysinitCfg) detectConflicts() {
	m := map[string][]stage.StageFunc{}

	for _, sf := range scfg.StageFuncs {
		m[sf.Name] = append(m[sf.Name], sf)
	}

	for name, sfs := range m {
		if len(sfs) > 1 {
			scfg.Conflicts[name] = sfs
		}
	}
}

func stageFuncResolve(sfRef *stage.StageFunc, stack *[]stage.StageFunc) error {
	if sfRef.Resolved {
		return nil
	}
	if sfRef.Resolving {
		return util.FmtNewtError("Circular dependency detected while resolving \"%s (%s)\".",
			sfRef.Name, sfRef.Pkg.FullName())
	}

	sfRef.Resolving = true
	for _, sfDepRef := range sfRef.Deps {
		err := stageFuncResolve(sfDepRef, stack)
		if err != nil {
			return err
		}
	}
	sfRef.Resolving = false

	sfRef.Resolved = true
	*stack = append([]stage.StageFunc{*sfRef}, *stack...)

	return nil
}

func ResolveStageFuncsOrder(sfs []stage.StageFunc) ([]stage.StageFunc, error) {
	var nodes []*stage.StageFunc
	var nodesQ []*stage.StageFunc
	nodesByStage := make(map[int][]*stage.StageFunc)
	nodesByName := make(map[string]*stage.StageFunc)

	for idx, _ := range sfs {
		sfRef := &sfs[idx]
		nodesByName[sfRef.Name] = sfRef
		if len(sfRef.Stage.Befores) == 0 && len(sfRef.Stage.Afters) == 0 {
			stage, _ := sfRef.Stage.IntVal()
			nodesByStage[stage] = append(nodesByStage[stage], sfRef)
			nodes = append(nodes, sfRef)
		} else {
			nodesQ = append(nodesQ, sfRef)
		}
	}

	// Put nodes without stages first, so they are resolved and put to
	// stack first - we do not want them to precede all nodes with stages.
	// While technically correct, it's better not to start sysinit with
	// nodes that do not have any stage since this will likely happen
	// before os init packages.
	nodes = append(nodesQ, nodes...)

	var stages []int
	for stage := range nodesByStage {
		stages = append(stages, stage)
	}
	sort.Ints(stages)

	// Add implicit dependencies for nodes with stages. We need to add
	// direct dependencies between each node of stage X to each node of
	// stage Y to make sure they can be resolved properly and reordered
	// if needed due to other dependencies.
	sfsPrev := nodesByStage[stages[0]]
	stages = stages[1:]
	for _, stage := range stages {
		sfsCurr := nodesByStage[stage]

		for _, sfsP := range sfsPrev {
			for _, sfsC := range sfsCurr {
				sfsP.Deps = append(sfsP.Deps, sfsC)
				// Keep separate list of implicit dependencies
				// This is only used for Graphviz output
				sfsP.DepsI = append(sfsP.DepsI, sfsC)
			}
		}

		sfsPrev = sfsCurr
	}

	// Now add other dependencies, i.e. $after and $before
	for _, sf := range nodesQ {
		for _, depStr := range sf.Stage.Afters {
			depSf := nodesByName[depStr]
			if depSf == nil {
				return []stage.StageFunc{},
					util.FmtNewtError("Unknown depdendency (\"%s\") for \"%s (%s)\".",
						depStr, sf.Name, sf.Pkg.FullName())
			}
			depSf.Deps = append(depSf.Deps, sf)
		}
		for _, depStr := range sf.Stage.Befores {
			depSf := nodesByName[depStr]
			if depSf == nil {
				return []stage.StageFunc{},
					util.FmtNewtError("Unknown depdendency (\"%s\") for \"%s (%s)\".",
						depStr, sf.Name, sf.Pkg.FullName())
			}
			sf.Deps = append(sf.Deps, depSf)
		}
	}

	// Now we can resolve order of functions by sorting dependency graph
	// topologically. This will also detect circular dependencies.
	sfs = []stage.StageFunc{}
	for _, sfRef := range nodes {
		err := stageFuncResolve(sfRef, &sfs)
		if err != nil {
			return []stage.StageFunc{}, err
		}
	}

	return sfs, nil
}

func Read(lpkgs []*pkg.LocalPackage, cfg *syscfg.Cfg) SysinitCfg {
	scfg := SysinitCfg{
		Conflicts: map[string][]stage.StageFunc{},
	}

	for _, lpkg := range lpkgs {
		scfg.readOnePkg(lpkg, cfg)
	}

	scfg.detectConflicts()

	// Don't try to resolve order if there are name conflicts since that
	// process depends on unique names for each function and could return
	// other confusing errors.
	if len(scfg.Conflicts) > 0 {
		return scfg
	}

	var err error
	scfg.StageFuncs, err = ResolveStageFuncsOrder(scfg.StageFuncs)
	if err != nil {
		scfg.InvalidSettings = append(scfg.InvalidSettings, err.Error())
	}

	return scfg
}

func (scfg *SysinitCfg) filter(lpkgs []*pkg.LocalPackage) []stage.StageFunc {
	m := make(map[*pkg.LocalPackage]struct{}, len(lpkgs))

	for _, lpkg := range lpkgs {
		m[lpkg] = struct{}{}
	}

	filtered := []stage.StageFunc{}
	for _, sf := range scfg.StageFuncs {
		if _, ok := m[sf.Pkg]; ok {
			filtered = append(filtered, sf)
		}
	}

	return filtered
}

// If any errors were encountered while parsing sysinit definitions, this
// function returns a string indicating the errors.  If no errors were
// encountered, "" is returned.
func (scfg *SysinitCfg) ErrorText() string {
	str := ""

	if len(scfg.InvalidSettings) > 0 {
		str += "Invalid sysinit definitions detected:"
		for _, e := range scfg.InvalidSettings {
			str += "\n    " + e
		}
	}

	if len(scfg.Conflicts) > 0 {
		str += "Sysinit function name conflicts detected:\n"
		for name, sfs := range scfg.Conflicts {
			for _, sf := range sfs {
				str += fmt.Sprintf("    Function=%s Package=%s\n",
					name, sf.Pkg.FullName())
			}
		}

		str += "\nResolve the problem by assigning unique function names " +
			"to each entry."
	}

	return str
}

func (scfg *SysinitCfg) write(lpkgs []*pkg.LocalPackage, isLoader bool,
	w io.Writer) error {

	var sfs []stage.StageFunc
	if lpkgs == nil {
		sfs = scfg.StageFuncs
	} else {
		sfs = scfg.filter(lpkgs)
	}

	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	if isLoader {
		fmt.Fprintf(w, "#if SPLIT_LOADER\n\n")
	} else {
		fmt.Fprintf(w, "#if !SPLIT_LOADER\n\n")
	}

	stage.WritePrototypes(sfs, w)

	var fnName string
	if isLoader {
		fnName = "sysinit_loader"
	} else {
		fnName = "sysinit_app"
	}

	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "void\n%s(void)\n{\n", fnName)

	stage.WriteCalls(sfs, "", w)

	fmt.Fprintf(w, "}\n\n")
	fmt.Fprintf(w, "#endif\n")

	return nil
}

func (scfg *SysinitCfg) EnsureWritten(lpkgs []*pkg.LocalPackage, srcDir string,
	targetName string, isLoader bool) error {

	buf := bytes.Buffer{}
	if err := scfg.write(lpkgs, isLoader, &buf); err != nil {
		return err
	}

	var path string
	if isLoader {
		path = fmt.Sprintf("%s/%s-sysinit-loader.c", srcDir, targetName)
	} else {
		path = fmt.Sprintf("%s/%s-sysinit-app.c", srcDir, targetName)
	}

	return stage.EnsureWritten(path, buf.Bytes())
}
