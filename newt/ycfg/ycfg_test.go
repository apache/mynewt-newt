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

package ycfg

import (
	"fmt"
	"testing"

	"mynewt.apache.org/newt/newt/cfgv"
)

// Conditional children must be evaluated in the same (sorted) order on
// every call, e.g. so that "-include" flags from several conditions are
// always emitted in the same order.
func TestGetChildrenOrder(t *testing.T) {
	yc := NewYCfg("test")
	settings := cfgv.NewSettings(nil)

	expected := []string{}
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("SETTING_%d", i)
		val := fmt.Sprintf("-include %d.h", i)

		if err := yc.Replace("pkg.cflags."+name, val); err != nil {
			t.Fatalf("Replace failed: %s", err.Error())
		}
		settings.Set(name, "1")
		expected = append(expected, val)
	}

	for n := 0; n < 20; n++ {
		entries, err := yc.Get("pkg.cflags", settings)
		if err != nil {
			t.Fatalf("Get failed: %s", err.Error())
		}
		if len(entries) != len(expected) {
			t.Fatalf("expected %d entries, got %d", len(expected),
				len(entries))
		}
		for i, e := range entries {
			if e.Value != expected[i] {
				t.Fatalf("entry %d: expected %q, got %q", i, expected[i],
					e.Value)
			}
		}
	}
}
