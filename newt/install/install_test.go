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

package install

import (
	"bufio"
	"strings"
	"testing"
)

func TestAskProceed(t *testing.T) {
	long := strings.Repeat("x", 10000)

	tests := []struct {
		name    string
		input   string
		proceed bool
		err     bool
	}{
		{"empty", "\n", true, false},
		{"yes", "y\n", true, false},
		{"no", "n\n", false, false},
		{"yes without newline", "Y", true, false},
		{"eof", "", false, true},
		{"invalid then no", "foo\nn\n", false, false},
		{"long line then yes", long + "\ny\n", true, false},
		{"long line then no", long + "\nno\n", false, false},
		{"long yes line", "y" + long + "\n", true, false},
		{"long line then eof", long, false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rd := bufio.NewReader(strings.NewReader(tc.input))
			proceed, err := askProceed(rd)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if proceed != tc.proceed {
				t.Fatalf("expected proceed=%v, got %v",
					tc.proceed, proceed)
			}
		})
	}
}
