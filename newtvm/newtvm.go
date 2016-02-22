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

// newtvm is a Windows wrapper for the newt tool.  It runs the specified
// commands within a Linux docker instance, giving access to newt and the
// utilities that it depends on.

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const debug bool = false
const dockerMachineName string = "default"
const newtvmImage = "mynewt/mynewt"
const newtvmVersion = "0.0.3"

// Sets the necessary environment variables to allow docker to run.
func configEnv() error {
	re, err := regexp.Compile("^SET ([^ ]+)=(.+)")
	if err != nil {
		return err
	}

	cmd := exec.Command("docker-machine", "env", "--shell", "cmd", "default")
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	output := string(outputBytes[:])
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if matches != nil {
			if debug {
				fmt.Fprintf(os.Stderr, "os.Setenv(\"%s\", \"%s\")\n",
					matches[1], matches[2])
			}
			err = os.Setenv(matches[1], matches[2])
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Calculates a virtualbox-compatible representation of the present working
// directory.
//
// E.g.,
//     C:\Users\me\Documents
//
// becomes:
//     /c/Users/me/documents
func fixedPwd() (string, error) {
	var pwd string
	var err error

	if pwd, err = os.Getwd(); err != nil {
		return "", err
	}

	// Begin with a slash; convert drive letter to lowercase.
	pwd = "/" + strings.ToLower(pwd[0:1]) + pwd[2:]

	// Replace backslashes with slashes.
	pwd = strings.Replace(pwd, "\\", "/", -1)

	return pwd, nil
}

// Constructs a command that will run the specified shell tokens in the
// docker environment.
//
// E.g., the args parameter might be: { "echo", "'hello", "world'" }
func buildCmd(args []string) (*exec.Cmd, error) {
	pwd, err := fixedPwd()
	if err != nil {
		return nil, err
	}

	fullArgs := []string{
		"run", "--device=/dev/bus/usb", "--rm=true",
		"-v", fmt.Sprintf("%s:/larva", pwd),
		"-w", "/larva", fmt.Sprintf("%s:%s", newtvmImage, newtvmVersion),
		"script", "-qc",
		strings.Join(args, " "), "/dev/null"}

	return exec.Command("docker", fullArgs...), nil
}

// Executes the specified command, displaying output as it is generated.
func execCmd(cmd *exec.Cmd) error {
	// Reader / scanner pair for printing stdout output.
	stdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stdoutScanner := bufio.NewScanner(stdoutReader)
	go func() {
		for stdoutScanner.Scan() {
			fmt.Println(stdoutScanner.Text())
		}
	}()

	// Reader / scanner pair for printing stderr output.
	stderrReader, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	stderrScanner := bufio.NewScanner(stderrReader)
	go func() {
		for stderrScanner.Scan() {
			fmt.Fprintln(os.Stderr, stderrScanner.Text())
		}
	}()

	// Execute command.
	err = cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}

	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: newtvm <command> [arg-1] [arg-2] [...]\n")
}

func usageErr(msg string, rc int) {
	if msg != "" {
		fmt.Fprintf(os.Stderr, "* error: %s\n", msg)
	}

	printUsage(os.Stderr)

	os.Exit(rc)
}

func main() {
	if len(os.Args) < 2 {
		usageErr("", 1)
	}

	cmd, err := buildCmd(os.Args[1:])
	if err != nil {
		usageErr(err.Error(), 1)
	}

	err = configEnv()
	if err != nil {
		usageErr(err.Error(), 1)
	}

	err = execCmd(cmd)
	if err != nil {
		usageErr(err.Error(), 1)
	}
}
