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

func operatorsTestSettings() *cfgv.Settings {
	return cfgv.NewSettingsFromMap(map[string]string{
		"A": "0",
		"B": "1",
		"X": "3",
	})
}

// Relational operators must be recognized without surrounding spaces.
func TestOperatorsNoSpaces(t *testing.T) {
	tests := []struct {
		expr string
		exp  bool
	}{
		{"A<1", true},
		{"A>1", false},
		{"X>=3", true},
		{"X<=2", false},
		{"X>A", true},
		{"(X>2)&&(A<1)", true},
		{"X==3", true},
		{"X!=3", false},
	}

	settings := operatorsTestSettings()
	for _, tt := range tests {
		got, err := ParseAndEval(tt.expr, settings)
		if err != nil {
			t.Errorf("%s: unexpected error: %s", tt.expr, err)
			continue
		}
		if got != tt.exp {
			t.Errorf("%s: got %v, expected %v", tt.expr, got, tt.exp)
		}
	}
}

// Tokens following a parenthesized expression must be rejected.
func TestOperatorsTrailingTokens(t *testing.T) {
	for _, expr := range []string{"(A) B", "!(A) B", "(A) (B)",
		"(A) B && X"} {

		if _, err := LexAndParse(expr); err == nil {
			t.Errorf("%s: parsed without error", expr)
		}
	}

	for _, expr := range []string{"(A)", "!(A)", "((A))", "(A) && (B)",
		"!(A) || B"} {

		if _, err := LexAndParse(expr); err != nil {
			t.Errorf("%s: unexpected error: %s", expr, err)
		}
	}
}
