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

package toolchain

import "testing"

func TestSiblingToolPath(t *testing.T) {
	tests := []struct {
		odPath string
		tool   string
		exp    string
	}{
		{"arm-none-eabi-objdump", "nm", "arm-none-eabi-nm"},
		{"/opt/gcc/bin/riscv-none-embed-objdump", "addr2line",
			"/opt/gcc/bin/riscv-none-embed-addr2line"},
		{"objdump", "nm", "nm"},
		{"gobjdump", "nm", "gnm"},
		{`C:\gcc\bin\arm-none-eabi-objdump.exe`, "nm",
			`C:\gcc\bin\arm-none-eabi-nm.exe`},
		{"", "nm", "arm-none-eabi-nm"},
	}

	for _, tt := range tests {
		got := siblingToolPath(tt.odPath, tt.tool)
		if got != tt.exp {
			t.Errorf("siblingToolPath(%q, %q) = %q, expected %q",
				tt.odPath, tt.tool, got, tt.exp)
		}
	}
}
