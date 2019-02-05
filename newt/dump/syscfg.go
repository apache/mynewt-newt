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

package dump

import (
	"sort"

	"mynewt.apache.org/newt/newt/syscfg"
)

type SyscfgPoint struct {
	Value string `json:"value"`
	Pkg   string `json:"package"`
}

type SyscfgRestriction struct {
	Code syscfg.CfgRestrictionCode `json:"code"`
	Expr string                    `json:"expr,omitempty"`
}

type SyscfgEntry struct {
	Type         syscfg.CfgSettingType  `json:"type"`
	History      []SyscfgPoint          `json:"history"`
	RefName      string                 `json:"ref_name,omitempty"`
	Restrictions []SyscfgRestriction    `json:"restrictions,omitempty"`
	State        syscfg.CfgSettingState `json:"state"`
}

type SyscfgPriority struct {
	Name      string `json:"name"`
	Definer   string `json:"definer"`
	Overrider string `json:"overrider"`
}

type SyscfgFlashConflict struct {
	Settings []string                    `json:"settings"`
	Code     syscfg.CfgFlashConflictCode `json:"code"`
}

type Syscfg struct {
	Settings        map[string]SyscfgEntry         `json:"settings"`
	PkgRestrictions map[string][]SyscfgRestriction `json:"pkg_restrictions"`
	Orphans         map[string][]SyscfgPoint       `json:"orphans"`
	Ambiguities     map[string][]SyscfgPoint       `json:"ambiguities"`
	SetViolations   map[string][]SyscfgRestriction `json:"set_violations"`
	PkgViolations   map[string][]SyscfgRestriction `json:"pkg_violations"`
	PrioViolations  []SyscfgPriority               `json:"prio_violations"`
	FlashConflicts  []SyscfgFlashConflict          `json:"flash_conflicts"`
	Redefines       map[string][]string            `json:"redefines"`
	Deprecated      []string                       `json:"deprecated"`
	Defunct         []string                       `json:"defunct"`
	UnresolvedRefs  []string                       `json:"unresolved_refs"`
}

func convPoint(p syscfg.CfgPoint) SyscfgPoint {
	return SyscfgPoint{
		Value: p.Value,
		Pkg:   p.Name(),
	}
}

func convPointSlice(ps []syscfg.CfgPoint) []SyscfgPoint {
	slice := make([]SyscfgPoint, len(ps))
	for i, p := range ps {
		slice[i] = convPoint(p)
	}
	return slice
}

func convStringMapPointSlice(
	src map[string][]syscfg.CfgPoint) map[string][]SyscfgPoint {

	dst := make(map[string][]SyscfgPoint, len(src))
	for k, ps := range src {
		dst[k] = convPointSlice(ps)
	}

	return dst
}

func convRestriction(r syscfg.CfgRestriction) SyscfgRestriction {
	return SyscfgRestriction{
		Code: r.Code,
		Expr: r.Expr,
	}
}

func convRestrictionSlice(rs []syscfg.CfgRestriction) []SyscfgRestriction {
	slice := make([]SyscfgRestriction, len(rs))
	for i, r := range rs {
		slice[i] = convRestriction(r)
	}
	return slice
}

func convStringMapRestrictionSlice(
	src map[string][]syscfg.CfgRestriction) map[string][]SyscfgRestriction {

	dst := make(map[string][]SyscfgRestriction, len(src))
	for k, rs := range src {
		dst[k] = convRestrictionSlice(rs)
	}

	return dst
}

func convPriority(p syscfg.CfgPriority) SyscfgPriority {
	return SyscfgPriority{
		Name:      p.SettingName,
		Definer:   p.PackageDef.FullName(),
		Overrider: p.PackageSrc.FullName(),
	}
}

func convPrioritySlice(ps []syscfg.CfgPriority) []SyscfgPriority {
	slice := make([]SyscfgPriority, len(ps))
	for i, p := range ps {
		slice[i] = convPriority(p)
	}
	return slice
}

func convFlashConflict(f syscfg.CfgFlashConflict) SyscfgFlashConflict {
	return SyscfgFlashConflict{
		Settings: f.SettingNames,
		Code:     f.Code,
	}
}

func convFlashConflictSlice(
	fs []syscfg.CfgFlashConflict) []SyscfgFlashConflict {

	slice := make([]SyscfgFlashConflict, len(fs))
	for i, f := range fs {
		slice[i] = convFlashConflict(f)
	}
	return slice
}

func convStringMapToSlice(m map[string]struct{}) []string {
	slice := make([]string, 0, len(m))
	for s, _ := range m {
		slice = append(slice, s)
	}

	sort.Strings(slice)
	return slice
}

func newSyscfg(cfg syscfg.Cfg) Syscfg {
	settings := make(map[string]SyscfgEntry, len(cfg.Settings))
	for name, ce := range cfg.Settings {
		history := convPointSlice(ce.History)

		restrictions := make([]SyscfgRestriction, len(ce.Restrictions))
		for i, r := range ce.Restrictions {
			restrictions[i] = SyscfgRestriction{
				Code: r.Code,
				Expr: r.Expr,
			}
		}

		settings[name] = SyscfgEntry{
			Type:         ce.SettingType,
			History:      history,
			RefName:      ce.ValueRefName,
			Restrictions: restrictions,
			State:        ce.State,
		}
	}

	redefines := make(map[string][]string, len(cfg.Redefines))
	for sname, pkgmap := range cfg.Redefines {
		for lpkg, _ := range pkgmap {
			redefines[sname] = append(redefines[sname], lpkg.FullName())
		}
	}

	return Syscfg{
		Settings:        settings,
		PkgRestrictions: convStringMapRestrictionSlice(cfg.PackageRestrictions),
		Orphans:         convStringMapPointSlice(cfg.Orphans),
		Ambiguities:     convStringMapPointSlice(cfg.Ambiguities),
		SetViolations:   convStringMapRestrictionSlice(cfg.SettingViolations),
		PkgViolations:   convStringMapRestrictionSlice(cfg.PackageViolations),
		PrioViolations:  convPrioritySlice(cfg.PriorityViolations),
		FlashConflicts:  convFlashConflictSlice(cfg.FlashConflicts),
		Redefines:       redefines,
		Deprecated:      convStringMapToSlice(cfg.Deprecated),
		Defunct:         convStringMapToSlice(cfg.Defunct),
		UnresolvedRefs:  convStringMapToSlice(cfg.UnresolvedValueRefs),
	}
}
