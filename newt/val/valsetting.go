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

package val

import (
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/util"
)

// Value setting: represents a setting value as read from a YAML configuration
// file.  The setting may reference a syscfg setting via the `MYNEWT_VAL(...)`
// notation.
type ValSetting struct {
	// The exact text specified as the YAML map key.
	Text string

	// If this setting refers to a syscfg setting via the `MYNEWT_VAL(...)`
	// notation, this contains the name of the setting.  Otherwise, "".
	RefName string

	// The setting value, after setting references are resolved.
	Value string
}

// IntVal Extracts a setting's integer value.
func (vs *ValSetting) IntVal() (int, error) {
	iv, err := util.AtoiNoOct(vs.Value)
	if err != nil {
		return 0, util.ChildNewtError(err)
	}

	return iv, nil
}

// Constructs a setting from a YAML string.
func ResolveValSetting(s string, cfg *syscfg.Cfg) (ValSetting, error) {
	refName, val, err := cfg.ExpandRef(s)
	if err != nil {
		return ValSetting{},
			util.FmtNewtError("value \"%s\" references undefined setting", s)
	}

	return ValSetting{
		Text:    s,
		RefName: refName,
		Value:   val,
	}, nil
}
