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

package repo

import (
	"strings"
	"testing"
	"time"

	"mynewt.apache.org/newt/newt/newtutil"
)

func mustParseVer(t *testing.T, s string) newtutil.RepoVersion {
	t.Helper()
	v, err := newtutil.ParseRepoVersion(s)
	if err != nil {
		t.Fatalf("failed to parse version %q: %v", s, err)
	}
	return v
}

func normalizeWithTimeout(t *testing.T, r *Repo,
	ver newtutil.RepoVersion) (newtutil.RepoVersion, error) {

	t.Helper()

	type result struct {
		ver newtutil.RepoVersion
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := r.NormalizeVersion(ver)
		ch <- result{v, err}
	}()

	select {
	case res := <-ch:
		return res.ver, res.err
	case <-time.After(5 * time.Second):
		t.Fatalf("NormalizeVersion(%s) did not return (alias loop?)",
			ver.String())
		return ver, nil
	}
}

func TestNormalizeVersion(t *testing.T) {
	r := &Repo{
		name: "dummy",
		vers: map[newtutil.RepoVersion]string{
			mustParseVer(t, "1.2.3"):    "v1.2.3",
			mustParseVer(t, "1-latest"): "1.2.3",
			mustParseVer(t, "1-dev"):    "1-latest",
		},
	}

	v, err := normalizeWithTimeout(t, r, mustParseVer(t, "1-dev"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != mustParseVer(t, "1.2.3") {
		t.Fatalf("unexpected version: %s", v.String())
	}
}

func TestNormalizeVersionAliasLoop(t *testing.T) {
	tests := map[string]map[string]string{
		"self": {
			"1-dev": "1-dev",
		},
		"two": {
			"1-latest": "1-dev",
			"1-dev":    "1-latest",
		},
		"three": {
			"1-latest": "1-dev",
			"1-dev":    "1-stable",
			"1-stable":     "1-latest",
		},
	}

	for name, vers := range tests {
		t.Run(name, func(t *testing.T) {
			r := &Repo{
				name: "dummy",
				vers: map[newtutil.RepoVersion]string{},
			}
			for k, v := range vers {
				r.vers[mustParseVer(t, k)] = v
			}

			_, err := normalizeWithTimeout(t, r, mustParseVer(t, "1-dev"))
			if err == nil {
				t.Fatalf("expected error for version alias loop")
			}
			if !strings.Contains(err.Error(), "dummy") ||
				!strings.Contains(err.Error(), "loop") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
