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
	"path/filepath"
	"strings"
	"testing"
)

// Failure to run nm or objdump must be reported as an error instead of
// exiting newt.
func TestSizeReportToolErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-tool")
	elf := filepath.Join(t.TempDir(), "app.elf")

	_, err := runNmCommand(missing, elf)
	if err == nil || !strings.Contains(err.Error(), "compiler.path.nm") {
		t.Errorf("unexpected nm error: %v", err)
	}

	_, err = runObjdumpCommand(missing, elf, "-hw")
	if err == nil ||
		!strings.Contains(err.Error(), "compiler.path.objdump") {

		t.Errorf("unexpected objdump error: %v", err)
	}

	_, err = runObjdumpCommand("", elf, "-hw")
	if err == nil || !strings.Contains(err.Error(),
		"compiler.path.objdump is not set") {

		t.Errorf("unexpected objdump error: %v", err)
	}
}
