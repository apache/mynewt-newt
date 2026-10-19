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
	"testing"
)

func valueRefsCfg(vals map[string]string) Cfg {
	cfg := NewCfg()
	for k, v := range vals {
		cfg.Settings[k] = CfgEntry{Name: k, Value: v}
	}

	return cfg
}

// Chained references resolve to the final value regardless of map
// iteration order.
func TestResolveValueRefsChain(t *testing.T) {
	// Map iteration order is random; repeat to cover different orders.
	for i := 0; i < 50; i++ {
		cfg := valueRefsCfg(map[string]string{
			"A": "MYNEWT_VAL(B)",
			"B": "MYNEWT_VAL(C)",
			"C": "MYNEWT_VAL(D)",
			"D": "5",
		})
		cfg.ResolveValueRefs()

		for _, k := range []string{"A", "B", "C", "D"} {
			if v := cfg.Settings[k].Value; v != "5" {
				t.Fatalf("setting %s: got %q, want \"5\"", k, v)
			}
		}
		if cfg.Settings["A"].ValueRefName != "B" {
			t.Fatalf("setting A: got ref %q, want \"B\"",
				cfg.Settings["A"].ValueRefName)
		}
		if len(cfg.UnresolvedValueRefs) != 0 {
			t.Fatalf("unexpected unresolved refs: %v", cfg.UnresolvedValueRefs)
		}
	}
}

// Circular references and references to undefined settings are reported as
// unresolved.
func TestResolveValueRefsUnresolved(t *testing.T) {
	for i := 0; i < 50; i++ {
		cfg := valueRefsCfg(map[string]string{
			"SELF":  "MYNEWT_VAL(SELF)",
			"MUT_A": "MYNEWT_VAL(MUT_B)",
			"MUT_B": "MYNEWT_VAL(MUT_A)",
			"UNDEF": "MYNEWT_VAL(NOT_DEFINED)",
			"CHAIN": "MYNEWT_VAL(UNDEF)",
			"OK":    "1",
		})
		cfg.ResolveValueRefs()

		for _, k := range []string{"SELF", "MUT_A", "MUT_B", "UNDEF",
			"CHAIN"} {

			if _, ok := cfg.UnresolvedValueRefs[k]; !ok {
				t.Fatalf("setting %s not reported as unresolved", k)
			}
			if v := cfg.Settings[k].Value; v != "0" {
				t.Fatalf("setting %s: got %q, want \"0\"", k, v)
			}
		}
		if _, ok := cfg.UnresolvedValueRefs["OK"]; ok {
			t.Fatalf("setting OK reported as unresolved")
		}
	}
}
