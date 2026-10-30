/*
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

package tui

import (
	"testing"
)

func TestThemes(t *testing.T) {
	for setting, want := range map[string]Theme{"": terminalTheme,
		"terminal": terminalTheme, "hal": halTheme} {
		if th, err := loadTheme(setting); err != nil || th != want {
			t.Errorf("theme %q: %v", setting, err)
		}
	}
	if th, err := loadTheme("nord"); err == nil || th != terminalTheme {
		t.Errorf("an unknown theme should fail and use the terminal's")
	}

	// The styles follow the theme.
	setTheme(halTheme)
	defer setTheme(terminalTheme)
	if styleSep.GetForeground() != halTheme.Border ||
		styleSel.GetBackground() != halTheme.SelBg ||
		kindStyle(KindBsp).GetForeground() != halTheme.Bsp {
		t.Errorf("styles do not use the theme")
	}
}
