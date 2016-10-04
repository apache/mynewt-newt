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

package syscfg

import (
	"fmt"
	"strings"

	"mynewt.apache.org/newt/util"
)

type CfgRestrictionCode int

const (
	CFG_RESTRICTION_CODE_NOTNULL = iota
	CFG_RESTRICTION_CODE_EXPR
)

var cfgRestrictionNameCodeMap = map[string]CfgRestrictionCode{
	"$notnull": CFG_RESTRICTION_CODE_NOTNULL,
}

type CfgRestrictionExpr struct {
	ReqSetting string
	ReqVal     bool
	BaseVal    bool
}
type CfgRestriction struct {
	BaseSetting string
	Code        CfgRestrictionCode

	// Only used if Code is CFG_RESTRICTION_CODE_EXPR
	Expr CfgRestrictionExpr
}

func parseRestrictionExprConsequent(field string) (string, bool) {
	var val bool
	var name string

	if strings.HasPrefix(field, "!") {
		val = false
		name = strings.TrimPrefix(field, "!")
	} else {
		val = true
		name = field
	}

	return name, val
}

// Parses a restriction value.
//
// Currently, two forms of restrictions are supported:
// 1. "$notnull"
// 2. expression
//
// The "$notnull" string indicates that the setting must be set to something
// other than the empty string.
//
// An expression string indicates dependencies on other settings.  It would be
// better to have a real expression parser.  For now, only very simple
// expressions are supported.  A restriction expression must be of the
// following form:
//     [!]<req-setting> [if <base-val>]
//
// All setting values are interpreted as booleans.  If a setting is "0", "",
// or undefined, it is false; otherwise it is true.
//
// Examples:
//     # Can't enable this setting unless LOG_FCB is enabled.
//	   pkg.restrictions:
//         LOG_FCB
//
//     # Can't enable this setting unless LOG_FCB is disabled.
//	   pkg.restrictions:
//         !LOG_FCB
//
//     # Can't disable this setting unless LOG_FCB is enabled.
//	   pkg.restrictions:
//         LOG_FCB if 0
func readRestrictionExpr(text string) (CfgRestrictionExpr, error) {
	e := CfgRestrictionExpr{}

	fields := strings.Fields(text)
	switch len(fields) {
	case 1:
		e.ReqSetting, e.ReqVal = parseRestrictionExprConsequent(fields[0])
		e.BaseVal = true

	case 3:
		if fields[1] != "if" {
			return e, util.FmtNewtError("invalid restriction: %s", text)
		}
		e.ReqSetting, e.ReqVal = parseRestrictionExprConsequent(fields[0])
		e.BaseVal = ValueIsTrue(fields[2])

	default:
		return e, util.FmtNewtError("invalid restriction: %s", text)
	}

	return e, nil
}

func readRestriction(baseSetting string, text string) (CfgRestriction, error) {
	r := CfgRestriction{
		BaseSetting: baseSetting,
	}

	var ok bool
	if r.Code, ok = cfgRestrictionNameCodeMap[text]; !ok {
		// If the restriction text isn't a defined string, parse it as an
		// expression.
		r.Code = CFG_RESTRICTION_CODE_EXPR

		var err error
		if r.Expr, err = readRestrictionExpr(text); err != nil {
			return r, err
		}
	}

	return r, nil
}

func (cfg *Cfg) violationText(entry CfgEntry, r CfgRestriction) string {
	if r.Code == CFG_RESTRICTION_CODE_NOTNULL {
		return entry.Name + " must not be null"
	}

	str := fmt.Sprintf("%s=%s ", entry.Name, entry.Value)
	if r.Expr.ReqVal {
		str += fmt.Sprintf("requires %s be set", r.Expr.ReqSetting)
	} else {
		str += fmt.Sprintf("requires %s not be set", r.Expr.ReqSetting)
	}

	str += fmt.Sprintf(", but %s", r.Expr.ReqSetting)
	reqEntry, ok := cfg.Settings[r.Expr.ReqSetting]
	if !ok {
		str += "undefined"
	} else {
		str += fmt.Sprintf("=%s", reqEntry.Value)
	}

	return str
}

func (r *CfgRestriction) relevantSettingNames() []string {
	switch r.Code {
	case CFG_RESTRICTION_CODE_NOTNULL:
		return []string{r.BaseSetting}

	case CFG_RESTRICTION_CODE_EXPR:
		return []string{r.BaseSetting, r.Expr.ReqSetting}

	default:
		panic("Invalid restriction code: " + string(r.Code))
	}
}

func (cfg *Cfg) restrictionMet(r CfgRestriction) bool {
	baseEntry := cfg.Settings[r.BaseSetting]
	baseVal := baseEntry.IsTrue()

	switch r.Code {
	case CFG_RESTRICTION_CODE_NOTNULL:
		return baseEntry.Value != ""

	case CFG_RESTRICTION_CODE_EXPR:
		if baseVal != r.Expr.BaseVal {
			// Restriction does not apply.
			return true
		}

		reqEntry, ok := cfg.Settings[r.Expr.ReqSetting]
		reqVal := ok && reqEntry.IsTrue()

		return reqVal == r.Expr.ReqVal

	default:
		panic("Invalid restriction code: " + string(r.Code))
	}
}
