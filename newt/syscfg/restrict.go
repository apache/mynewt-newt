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
	"encoding/json"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/util"
)

type CfgRestrictionCode int

const (
	CFG_RESTRICTION_CODE_NOTNULL = iota
	CFG_RESTRICTION_CODE_EXPR
	CFG_RESTRICTION_CODE_CHOICE
	CFG_RESTRICTION_CODE_RANGE
)

var cfgRestrictionNameCodeMap = map[string]CfgRestrictionCode{
	"$notnull": CFG_RESTRICTION_CODE_NOTNULL,
	"expr":     CFG_RESTRICTION_CODE_EXPR,
	"choice":   CFG_RESTRICTION_CODE_CHOICE,
	"range":    CFG_RESTRICTION_CODE_RANGE,
}

type CfgRestrictionRange struct {
	LExpr string
	RExpr string
}

type CfgRestriction struct {
	BaseSetting string
	Code        CfgRestrictionCode

	// Only used if Code is either CFG_RESTRICTION_CODE_EXPR or CFG_RESTRICTION_CODE_RANGE
	Expr string

	// Only used if Code is CFG_RESTRICTION_CODE_RANGE
	Ranges []CfgRestrictionRange
}

func (c CfgRestrictionCode) String() string {
	for s, code := range cfgRestrictionNameCodeMap {
		if code == c {
			return s
		}
	}

	return "???"
}

func parseCfgRestrictionCode(s string) (CfgRestrictionCode, error) {
	if c, ok := cfgRestrictionNameCodeMap[s]; ok {
		return c, nil
	}

	return 0, util.FmtNewtError("cannot parse cfg restriction code \"%s\"", s)
}

func (c CfgRestrictionCode) MarshalJSON() ([]byte, error) {
	return util.MarshalJSONStringer(c)
}

func (c *CfgRestrictionCode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return util.ChildNewtError(err)
	}

	x, err := parseCfgRestrictionCode(s)
	if err != nil {
		return err
	}

	*c = x
	return nil
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

func (r *CfgRestriction) validateRangesBounds(settings map[string]string) bool {
	for _, rtoken := range r.Ranges {
		if len(rtoken.RExpr) > 0 {
			expr := fmt.Sprintf("(%s) <= (%s)", rtoken.LExpr, rtoken.RExpr)
			val, err := parse.ParseAndEval(expr, settings)
			if !val || err != nil {
				return false
			}
		}
	}

	return true
}

func (r *CfgRestriction) createRangeExpr() string {
	exprOutTokens := []string{}

	for _, rtoken := range r.Ranges {
		if len(rtoken.RExpr) > 0 {
			exprOutTokens = append(exprOutTokens,
				fmt.Sprintf("(((%s) >= (%s)) && ((%s) <= (%s)))",
					r.BaseSetting, rtoken.LExpr, r.BaseSetting, rtoken.RExpr))
		} else {
			exprOutTokens = append(exprOutTokens, fmt.Sprintf("((%s) == (%s))",
				r.BaseSetting, rtoken.LExpr))
		}
	}

	return strings.Join(exprOutTokens," || ")
}

func (cfg *Cfg) settingViolationText(entry CfgEntry, r CfgRestriction) string {
	prefix := fmt.Sprintf("Setting %s(%s) ", entry.Name, entry.Value)
	if r.Code == CFG_RESTRICTION_CODE_NOTNULL {
		return prefix + "must not be null"
	} else if r.Code == CFG_RESTRICTION_CODE_CHOICE {
		return prefix + "must be one of defined choices (see definition)"
	} else if r.Code == CFG_RESTRICTION_CODE_RANGE {
		return prefix + "must be in range: " + r.Expr
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
	} else if r.Code == CFG_RESTRICTION_CODE_RANGE {
		tokens, _ := parse.Lex(r.createRangeExpr())
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

	case CFG_RESTRICTION_CODE_CHOICE:
		if baseEntry.Value == "" {
			// Assume empty value a valid choice (use $notnull if need otherwise)
			return true
		}
		value := strings.ToLower(baseEntry.Value)
		for _, choice := range baseEntry.ValidChoices {
			if strings.ToLower(choice) == value {
				return true
			}
		}
		return false


	case CFG_RESTRICTION_CODE_RANGE:
		expr := r.createRangeExpr()
		if expr == "" {
			util.OneTimeWarning(
				"Ignoring illegal range expression for setting \"%s\": "+
					"`%s`\n", r.BaseSetting, r.Expr)
			return true
		}

		val, err := parse.ParseAndEval(expr, settings)
		if err != nil {
			util.OneTimeWarning(
				"Ignoring illegal range expression for setting \"%s\": "+
					"`%s`\n", r.BaseSetting, r.Expr)
			return true
		}

		// invalid bounds may or may not result in an error so just emit a warning
		if !r.validateRangesBounds(settings) {
			util.OneTimeWarning(
				"Invalid bounds (lval > rval) for range expression for setting \"%s\": "+
					"`%s`\n", r.BaseSetting, r.Expr)
		}

		return val
	case CFG_RESTRICTION_CODE_EXPR:
		var expr string
		if r.BaseSetting != "" {
			expr = normalizeExpr(r.Expr, r.BaseSetting)
		} else {
			expr = r.Expr
		}

		val, err := parse.ParseAndEval(expr, settings)
		if err != nil {
			util.OneTimeWarning(
				"Ignoring illegal expression for setting \"%s\": "+
					"`%s` %s\n", r.BaseSetting, r.Expr, err.Error())
			return true
		}
		return val

	default:
		panic("Invalid restriction code: " + string(r.Code))
	}
}

func createRangeRestriction(baseSetting string, expr string) (CfgRestriction, error) {
	r := CfgRestriction{
		BaseSetting: baseSetting,
		Code: CFG_RESTRICTION_CODE_RANGE,
		Expr: expr,
		Ranges: []CfgRestrictionRange{},
	}

	exprTokens := strings.Split(expr, ",")
	for _,token := range exprTokens {
		rtoken := CfgRestrictionRange{}

		limits := strings.Split(token, "..")
		if len(limits) == 1 {
			rtoken.LExpr = limits[0]
		} else if len(limits) == 2 && len(strings.TrimSpace(limits[1])) > 0 {
			rtoken.LExpr = limits[0]
			rtoken.RExpr = limits[1]
		} else {
			return r, util.FmtNewtError("invalid token in range expression \"%s\"", token)
		}

		r.Ranges = append(r.Ranges, rtoken)
	}

	return r, nil
}
