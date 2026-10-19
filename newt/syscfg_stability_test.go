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

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var syscfgStabilityDir = filepath.Join("testdata", "syscfg_stability")

// syscfgStabilityCheckNoPanic fails the test if newt output indicates a
// crash.
func syscfgStabilityCheckNoPanic(t *testing.T, out string) {
	t.Helper()

	if strings.Contains(out, "panic:") ||
		strings.Contains(out, "goroutine ") {
		t.Fatalf("newt crashed\n%s", out)
	}
}

// Overriding a setting injected by newt (e.g., TARGET_NAME) is an error.
func TestSyscfgStabilityNewtSettingOverride(t *testing.T) {
	out, _ := newtCombinedOutput(syscfgStabilityDir, nil, "target",
		"config", "show", "newt_override")
	syscfgStabilityCheckNoPanic(t, out)

	msg := "Package: targets/newt_override overriding setting: " +
		"TARGET_NAME which is set by newt and cannot be overridden"
	if !strings.Contains(out, msg) {
		t.Errorf("missing priority violation error\n%s", out)
	}
}

// Deprecated and experimental settings overridden on the command line
// produce warnings naming newt as the source.
func TestSyscfgStabilityCmdlineDeprecated(t *testing.T) {
	dir := copyToTemp(t, syscfgStabilityDir)

	out, _ := newtCombinedOutput(dir, nil, "build", "deprecated",
		"-S", "FOO_DEPRECATED=2:FOO_EXPERIMENTAL=2")
	syscfgStabilityCheckNoPanic(t, out)

	for _, msg := range []string{
		"Use of deprecated setting FOO_DEPRECATED in newt",
		"Use of experimental setting FOO_EXPERIMENTAL in newt",
	} {
		if !strings.Contains(out, msg) {
			t.Errorf("missing warning \"%s\"\n%s", msg, out)
		}
	}
}

// A target inside an external repo can use a BSP and compiler from the
// local project.
func TestSyscfgStabilityExtRepoTarget(t *testing.T) {
	out, err := newtCombinedOutput(syscfgStabilityDir, nil, "target", "dep",
		"@apache-mynewt-core/targets/ext")
	syscfgStabilityCheckNoPanic(t, out)
	if err != nil {
		t.Fatalf("newt target dep failed: %s\n%s", err, out)
	}

	for _, p := range []string{"hw/bsp/loc", "compiler/loc"} {
		if !strings.Contains(out, "* "+p+" -->") {
			t.Errorf("missing package %s in dependency graph\n%s", p, out)
		}
	}
}

// syscfgStabilityRunTimeout runs newt and kills it if it does not finish
// within the timeout. Returns combined output and whether newt timed out.
func syscfgStabilityRunTimeout(t *testing.T, dir string,
	timeout time.Duration, args ...string) (string, bool) {

	t.Helper()

	cmd := newtExecCmd(dir, nil, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		return out.String(), false
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		return out.String(), true
	}
}

// Conditional overrides that contradict each other are reported as an
// error instead of making newt loop forever.
func TestSyscfgStabilityOscillation(t *testing.T) {
	tests := []struct {
		target string
		msgs   []string
	}{
		{"osc_self", []string{"OSC_X: "}},
		{"osc_dep", []string{"OSC_Y: ", "@apache-mynewt-core/sys/oscbar"}},
	}

	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			out, timedOut := syscfgStabilityRunTimeout(t, syscfgStabilityDir,
				30*time.Second, "target", "config", "show", tc.target)
			if timedOut {
				t.Fatalf("newt target config show did not finish\n%s", out)
			}
			syscfgStabilityCheckNoPanic(t, out)

			msgs := append([]string{"Syscfg does not converge"}, tc.msgs...)
			for _, msg := range msgs {
				if !strings.Contains(out, msg) {
					t.Errorf("missing \"%s\" in output\n%s", msg, out)
				}
			}
		})
	}
}
