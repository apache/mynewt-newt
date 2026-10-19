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

package compat

import (
	"testing"

	"mynewt.apache.org/newt/newt/newtutil"
)

func compatTestVer(t *testing.T, s string) newtutil.Version {
	t.Helper()
	v, err := newtutil.ParseVersion(s)
	if err != nil {
		t.Fatalf("failed to parse version %q: %v", s, err)
	}
	return v
}

func TestParseNcTableDuplicateVersion(t *testing.T) {
	strMap := map[string]string{
		"1.0.0":  "good",
		"1.0.00": "error",
	}

	// The order of map iteration is random; repeat to cover both orders
	// of the duplicate entries.
	for i := 0; i < 20; i++ {
		tbl, err := ParseNcTable(strMap)
		if err == nil {
			// Without duplicate detection this could panic.
			tbl.CheckNewtVer(compatTestVer(t, "0.9.0"))
			t.Fatalf("expected error for duplicate newt version")
		}
	}
}

func TestCheckNewtVer(t *testing.T) {
	tbl, err := ParseNcTable(map[string]string{
		"1.0.0": "good",
		"2.0.0": "error",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		ver  newtutil.Version
		code NewtCompatCode
	}{
		{compatTestVer(t, "0.9.0"), NEWT_COMPAT_ERROR},
		{compatTestVer(t, "1.0.0"), NEWT_COMPAT_GOOD},
		{compatTestVer(t, "1.5.0"), NEWT_COMPAT_GOOD},
		{compatTestVer(t, "2.0.0"), NEWT_COMPAT_ERROR},
	}

	for _, tc := range tests {
		code, _ := tbl.CheckNewtVer(tc.ver)
		if code != tc.code {
			t.Errorf("newt %s: expected code %d, got %d",
				tc.ver.String(), tc.code, code)
		}
	}
}
