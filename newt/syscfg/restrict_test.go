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
	"strings"
	"testing"

	"mynewt.apache.org/newt/newt/cfgv"
)

func TestCreateRangeRestrictionInvalid(t *testing.T) {
	exprs := []string{
		"",
		" ",
		"0..1,",
		",0..1",
		"1,,3",
		"..5",
		"5..",
		"1..2..3",
		"1.. ,3",
	}
	for _, expr := range exprs {
		_, err := createRangeRestriction("FOO", expr)
		if err == nil {
			t.Errorf("expected error for range \"%s\"", expr)
			continue
		}
		if !strings.Contains(err.Error(), "FOO") {
			t.Errorf("error for range \"%s\" does not name setting: %s",
				expr, err.Error())
		}
	}
}

func TestCreateRangeRestrictionValid(t *testing.T) {
	tests := []struct {
		expr string
		val  string
		met  bool
	}{
		{"0..10", "5", true},
		{"0..10", "11", false},
		{"0..1, 5", "5", true},
		{" 0 .. 1 , 5 ", "4", false},
		{"1,2,3", "2", true},
		{"MIN..MAX", "7", true},
	}
	for _, tc := range tests {
		r, err := createRangeRestriction("FOO", tc.expr)
		if err != nil {
			t.Errorf("unexpected error for range \"%s\": %v", tc.expr, err)
			continue
		}

		cfg := NewCfg()
		cfg.Settings["FOO"] = CfgEntry{Name: "FOO", Value: tc.val}
		settings := cfgv.NewSettingsFromMap(map[string]string{
			"FOO": tc.val,
			"MIN": "3",
			"MAX": "8",
		})
		if met := cfg.restrictionMet(r, settings); met != tc.met {
			t.Errorf("range \"%s\" value %s: got %v, want %v",
				tc.expr, tc.val, met, tc.met)
		}
	}
}
