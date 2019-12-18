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

package resolve

import (
	"github.com/spf13/cast"
	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

func getExprMapStringSlice(
	yc ycfg.YCfg, key string, settings map[string]string) (
	map[*parse.Node][]string, string, error) {

	var warning string

	entries, getErr := yc.GetSlice(key, settings)
	if getErr != nil {
		warning = getErr.Error()
	}

	if len(entries) == 0 {
		return nil, warning, nil
	}

	m := make(map[*parse.Node][]string, len(entries))
	for _, e := range entries {
		slice, err := cast.ToStringSliceE(e.Value)
		if err != nil {
			return nil, warning, util.FmtNewtError(
				"ycfg node \"%s\" contains unexpected type; "+
					"have=%T want=[]string", e.Value)
		}

		m[e.Expr] = append(m[e.Expr], slice...)
	}

	return m, warning, nil
}

func revExprMapStringSlice(
	ems map[*parse.Node][]string) map[string][]*parse.Node {

	m := map[string][]*parse.Node{}

	for expr, vals := range ems {
		for _, val := range vals {
			m[val] = append(m[val], expr)
		}
	}

	return m
}

func readExprMap(yc ycfg.YCfg, key string, settings map[string]string) (
	parse.ExprMap, string, error) {

	ems, warning, err := getExprMapStringSlice(yc, key, settings)
	if err != nil {
		return nil, warning, err
	}

	em := parse.ExprMap{}

	rev := revExprMapStringSlice(ems)
	for v, exprs := range rev {
		sub := parse.ExprSet{}
		for _, expr := range exprs {
			sub[expr.String()] = expr
		}
		em[v] = sub
	}

	return em, warning, nil
}
