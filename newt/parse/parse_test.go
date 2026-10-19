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

package parse

import (
	"testing"

	"mynewt.apache.org/newt/newt/cfgv"
)

func TestParseEmptyParens(t *testing.T) {
	settings := cfgv.NewSettingsFromMap(map[string]string{"X": "1"})

	exprs := []string{
		"()",
		"X == ()",
		"() == 1",
		"X != ()",
		"X < ()",
		"() >= X",
		"X && ()",
		"!()",
		"(())",
		"X == (())",
	}
	for _, expr := range exprs {
		_, err := ParseAndEval(expr, settings)
		if err == nil {
			t.Errorf("expected error for expression [%s]", expr)
		}
	}
}

func TestParseValid(t *testing.T) {
	settings := cfgv.NewSettingsFromMap(map[string]string{
		"X": "1",
		"Y": "0",
	})

	exprs := map[string]bool{
		"":             true,
		"X":            true,
		"(X)":          true,
		"((X) == (1))": true,
		"X == (Y)":     false,
		"!(Y)":         true,
		"X > Y":        true,
	}
	for expr, want := range exprs {
		got, err := ParseAndEval(expr, settings)
		if err != nil {
			t.Errorf("unexpected error for expression [%s]: %v", expr, err)
			continue
		}
		if got != want {
			t.Errorf("expression [%s]: got %v, want %v", expr, got, want)
		}
	}
}
