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

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// newTestCompiler creates a compiler which is usable for tests that do not
// need a loaded compiler package.
func newTestCompiler() *Compiler {
	c := &Compiler{
		mutex:           &sync.Mutex{},
		objPathList:     map[string]bool{},
		extraDeps:       []string{},
		compileCommands: []CompileCommand{},
	}
	c.depTracker = NewDepTracker(c)

	return c
}

// Run with -race to detect unsynchronized access.
func TestCompilerConcurrentUpdates(t *testing.T) {
	c := newTestCompiler()
	dir := t.TempDir()

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		obj := filepath.Join(dir, fmt.Sprintf("f%d.o", i))
		if err := os.WriteFile(obj, nil, 0644); err != nil {
			t.Fatal(err)
		}

		wg.Add(3)
		go func() {
			defer wg.Done()
			c.addCompileCommand([]string{"cc", "-c", obj}, obj)
		}()
		go func() {
			defer wg.Done()
			c.depTracker.SetMostRecent(obj, time.Now())
		}()
		go func() {
			defer wg.Done()
			if err := c.depTracker.ProcessFileTime(obj); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if got := len(c.GetCompileCommands()); got != n {
		t.Errorf("expected %d compile commands, got %d", n, got)
	}
}
