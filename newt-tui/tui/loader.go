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

package tui

import (
	"fmt"
	"sync"

	"mynewt.apache.org/newt/newt-tui/newtinfo"
)

// RunProject starts the browser for the project in root, reading it with
// "newt info --json" of the given newt binary.  The current directory must
// be root, as the newt commands the browser runs work on the project
// there.
func RunProject(newt string, root string) error {
	NewtPath = newt

	l := &loader{newt: newt, root: root}
	info, err := l.read("--brief")
	if err != nil {
		return err
	}
	l.first = info

	title, note := ProjectTitle(root, info.Project.Name)
	return Run(title, note, l.load, l.loadFull)
}

// loader reads the project for the browser.  Resolving the builds of all
// targets takes most of the time, so the browser reads the project without
// them first and then with them in the background.
type loader struct {
	newt string
	root string

	mu sync.Mutex
	// The project as read before the browser started, used for its first
	// load.
	first *newtinfo.Info
	// The targets as last resolved.
	resolved map[string]newtinfo.Target
}

func (l *loader) read(args ...string) (*newtinfo.Info, error) {
	return newtinfo.Load(l.newt, l.root, args...)
}

// load reads the project without resolving the targets.  Until they are
// resolved again, they show what they were last resolved to, e.g. whether
// they are bootloaders, but their syscfg is resolved again when needed.
func (l *loader) load() ([]*Category, error) {
	l.mu.Lock()
	info := l.first
	l.first = nil
	l.mu.Unlock()

	if info == nil {
		var err error
		if info, err = l.read("--brief"); err != nil {
			return nil, err
		}
	}

	l.mu.Lock()
	for i := range info.Targets {
		t := &info.Targets[i]
		if old, ok := l.resolved[t.Name]; ok {
			t.BootLoader, t.FlashArea = old.BootLoader, old.FlashArea
			t.Syscfg, t.Deps = old.Syscfg, old.Deps
			t.SyscfgErrors, t.SyscfgWarnings =
				old.SyscfgErrors, old.SyscfgWarnings
		}
	}
	l.mu.Unlock()

	return Load(info, l.resolveTarget), nil
}

// loadFull reads the project with all targets resolved.
func (l *loader) loadFull() ([]*Category, error) {
	info, err := l.read()
	if err != nil {
		return nil, err
	}

	resolved := map[string]newtinfo.Target{}
	for _, t := range info.Targets {
		resolved[t.Name] = t
	}
	l.mu.Lock()
	l.resolved = resolved
	l.mu.Unlock()

	return Load(info, l.resolveTarget), nil
}

// resolveTarget reads the resolved build of one target.
func (l *loader) resolveTarget(name string) (*newtinfo.Target, error) {
	info, err := l.read(name)
	if err != nil {
		return nil, err
	}
	if len(info.Targets) != 1 {
		return nil, fmt.Errorf("newt info --json %s: no such target", name)
	}
	return &info.Targets[0], nil
}
