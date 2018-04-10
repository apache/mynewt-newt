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

// Currently, two forms of restrictions are supported:
// 1. "$notnull"
// 2. expression
//
// The "$notnull" string indicates that the setting must be set to something
// other than the empty string.
//
// An expression string can take two forms:
// 1. Full expression
// 2. Shorthand
//
// A full expression has the same syntax as syscfg expressions.
//
// A shorthand expression has one of the following forms:
//     [!]<req-setting>
//     (DEPRECATED) [!]<req-setting> if <base-val>
//     (DEPRECATED) [!]<req-setting> if <expression>
//
// Examples:
//     # Can't enable this setting unless LOG_FCB is enabled.
//     # (shorthand)
//     pkg.restrictions:
//         - LOG_FCB
//
//     # Can't enable this setting unless LOG_FCB is disabled.
//     # (shorthand)
//     pkg.restrictions:
//         - !LOG_FCB
//
//     # Can't enable this setting (`MYSETTING`) unless LOG_FCB is enabled and
//     # CONSOLE_UART is set to "uart0".
//     # (full expression)
//     pkg.restrictions:
//         - '(LOG_FCB && CONSOLE_UART == "uart0") || !MYSETTING

package syscfg

import (
	"fmt"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/parse"
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

type CfgRestriction struct {
	BaseSetting string
	Code        CfgRestrictionCode

	// Only used if Code is CFG_RESTRICTION_CODE_EXPR
	Expr string
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
		r.Expr = text
	}

	return r, nil
}

func translateShorthandExpr(expr string, baseSetting string) string {
	tokens, err := parse.Lex(expr)
	if err != nil {
		return ""
	}

	ifi := -1
	var ift *parse.Token
	for i, t := range tokens {
		if t.Code == parse.TOKEN_IDENT && t.Text == "if" {
			ifi = i
			ift = &t
			break
		}
	}

	if ifi == 0 || ifi == len(tokens)-1 {
		return ""
	}

	if ifi == -1 {
		if parse.FindBinaryToken(tokens) == -1 {
			// [!]<req-setting>
			return fmt.Sprintf("(%s) || !%s", expr, baseSetting)
		} else {
			// Full expression
			return ""
		}
	}

	if parse.FindBinaryToken(tokens[ifi+1:]) == -1 {
		// [!]<req-setting> if <base-val>
		return fmt.Sprintf("(%s) || %s != (%s)",
			expr[:ift.Offset], baseSetting, expr[tokens[ifi+1].Offset:])
	} else {
		// [!]<req-setting> if <expression>
		return fmt.Sprintf("(%s) || !(%s)",
			expr[:ift.Offset], expr[tokens[ifi+1].Offset:])
	}
}

func normalizeExpr(expr string, baseSetting string) string {
	shexpr := translateShorthandExpr(expr, baseSetting)
	if shexpr != "" {
		log.Debugf("Translating shorthand restriction: `%s` ==> `%s`",
			expr, shexpr)
		expr = shexpr
	}

	return expr
}

func (cfg *Cfg) settingViolationText(entry CfgEntry, r CfgRestriction) string {
	prefix := fmt.Sprintf("Setting %s(%s) ", entry.Name, entry.Value)
	if r.Code == CFG_RESTRICTION_CODE_NOTNULL {
		return prefix + "must not be null"
	} else {
		return prefix + "requires: " + r.Expr
	}
}

func (cfg *Cfg) packageViolationText(pkgName string, r CfgRestriction) string {
	return fmt.Sprintf("Package %s requires: %s", pkgName, r.Expr)
}

func (r *CfgRestriction) relevantSettingNames() []string {
	var names []string

	if r.BaseSetting != "" {
		names = append(names, r.BaseSetting)
	}

	if r.Code == CFG_RESTRICTION_CODE_EXPR {
		tokens, _ := parse.Lex(normalizeExpr(r.Expr, r.BaseSetting))
		for _, token := range tokens {
			if token.Code == parse.TOKEN_IDENT {
				names = append(names, token.Text)
			}
		}
	}

	return names
}

func (cfg *Cfg) restrictionMet(
	r CfgRestriction, settings map[string]string) bool {

	baseEntry := cfg.Settings[r.BaseSetting]

	switch r.Code {
	case CFG_RESTRICTION_CODE_NOTNULL:
		return baseEntry.Value != ""

	case CFG_RESTRICTION_CODE_EXPR:
		var expr string
		if r.BaseSetting != "" {
			expr = normalizeExpr(r.Expr, r.BaseSetting)
		} else {
			expr = r.Expr
		}

		val, err := parse.ParseAndEval(expr, settings)
		if err != nil {
			util.StatusMessage(util.VERBOSITY_QUIET,
				"WARNING: ignoring illegal expression for setting \"%s\": "+
					"`%s` %s\n", r.BaseSetting, r.Expr, err.Error())
			return true
		}
		return val

	default:
		panic("Invalid restriction code: " + string(r.Code))
	}
}
