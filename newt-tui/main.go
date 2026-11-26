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

// newt-tui is a terminal browser for newt projects: their targets, apps,
// BSPs, libraries and repos.  It reads the project with "newt info --json"
// and runs newt for everything else, so it works with any newt that
// supports that command and newt does not depend on it.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mynewt.apache.org/newt/newt-tui/tui"
)

func main() {
	newt := flag.String("newt", "newt", "the newt binary to run")
	dir := flag.String("C", ".", "the project directory, or a directory in it")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: newt-tui [flags]\n\n"+
			"Browse the targets, apps, BSPs and other packages of the newt\n"+
			"project in the current directory.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*newt, *dir); err != nil {
		fmt.Fprintf(os.Stderr, "newt-tui: %s\n", err)
		os.Exit(1)
	}
}

func run(newt string, dir string) error {
	root, err := projectRoot(dir)
	if err != nil {
		return err
	}

	// newt runs from the project's directory, wherever newt-tui started.
	if err := os.Chdir(root); err != nil {
		return err
	}

	return tui.RunProject(newt, root)
}

// projectRoot returns the directory with the project.yml of the project
// that dir is in.
func projectRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "project.yml")); err == nil {
			return d, nil
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("%s is not in a newt project "+
				"(no project.yml found)", abs)
		}
	}
}
