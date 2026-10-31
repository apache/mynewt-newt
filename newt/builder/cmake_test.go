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

package builder

import (
	"bytes"
	"strings"
	"testing"

	"mynewt.apache.org/newt/newt/toolchain"
)

// The C++ compiler is required by the generated project only if there are
// C++ sources.
func TestCmakeHeaderWriteCxx(t *testing.T) {
	tests := []struct {
		name      string
		hasCxx    bool
		languages string
	}{
		{
			name:      "no c++ sources",
			hasCxx:    false,
			languages: "LANGUAGES C ASM)",
		},
		{
			name:      "c++ sources",
			hasCxx:    true,
			languages: "LANGUAGES C CXX ASM)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			CmakeHeaderWrite(&b, &toolchain.Compiler{}, "test", tt.hasCxx)
			out := b.String()

			if !strings.Contains(out, tt.languages) {
				t.Errorf("project languages %q not found in:\n%s",
					tt.languages, out)
			}

			hasCxxCompiler := strings.Contains(out, "CMAKE_CXX_COMPILER")
			if hasCxxCompiler != tt.hasCxx {
				t.Errorf("CMAKE_CXX_COMPILER set: %t, expected %t",
					hasCxxCompiler, tt.hasCxx)
			}
		})
	}
}
