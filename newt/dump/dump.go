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
	"bytes"
	"encoding/json"

	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/util"
)

type Report struct {
	TargetName      string              `json:"target_name"`
	DepGraph        DepGraph            `json:"dep_graph"`
	RevdepGraph     DepGraph            `json:"revdep_graph"`
	Syscfg          Syscfg              `json:"syscfg"`
	Sysinit         Sysinit             `json:"sysinit"`
	Sysdown         Sysdown             `json:"sysdown"`
	Logcfg          Logcfg              `json:"logcfg"`
	ApiMap          map[string]string   `json:"api_map"`
	UnsatisfiedApis map[string][]string `json:"unsatisfied_apis"`
	ApiConflicts    map[string][]string `json:"api_conflicts"`
	FlashMap        FlashMap            `json:"flash_map"`
}

func NewReport(tb *builder.TargetBuilder) (Report, error) {
	report := Report{}

	res, err := tb.Resolve()
	if err != nil {
		return report, err
	}

	report.TargetName = tb.GetTarget().FullName()

	dg, err := tb.CreateDepGraph()
	if err != nil {
		return report, err
	}
	report.DepGraph = newDepGraph(dg)

	rdg, err := tb.CreateRevdepGraph()
	if err != nil {
		return report, err
	}
	report.RevdepGraph = newDepGraph(rdg)

	report.Syscfg = newSyscfg(res.Cfg)

	si, err := newSysinit(res.SysinitCfg)
	if err != nil {
		return report, err
	}
	report.Sysinit = si

	sd, err := newSysdown(res.SysdownCfg)
	if err != nil {
		return report, err
	}
	report.Sysdown = sd

	lc, err := newLogcfg(res.LCfg)
	if err != nil {
		return report, err
	}
	report.Logcfg = lc

	report.ApiMap = newApiMap(res)
	report.UnsatisfiedApis = newUnsatisfiedApis(res)
	report.ApiConflicts = newApiConflicts(res)

	report.FlashMap = newFlashMap(tb.BspPkg().FlashMap)

	return report, nil
}

func (r *Report) JSON() (string, error) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "    ")

	// Don't escape ampersand.
	enc.SetEscapeHTML(false)

	if err := enc.Encode(r); err != nil {
		return "", util.ChildNewtError(err)
	}

	return buf.String(), nil
}
