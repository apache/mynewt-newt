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

package util

import (
	"fmt"
	"github.com/hashicorp/logutils"
	"log"
	"mynewt.apache.org/newt/viper"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var Logger *log.Logger
var Verbosity int
var OK_STRING = " ok!\n"

func ParseEqualsPair(v string) (string, string, error) {
	s := strings.Split(v, "=")
	return s[0], s[1], nil
}

type NewtError struct {
	Text       string
	StackTrace []byte
}

const (
	VERBOSITY_SILENT  = 0
	VERBOSITY_QUIET   = 1
	VERBOSITY_DEFAULT = 2
	VERBOSITY_VERBOSE = 3
)

func (se *NewtError) Error() string {
	return se.Text + "\n" + string(se.StackTrace)
}

func NewNewtError(msg string) *NewtError {
	err := &NewtError{
		Text:       msg,
		StackTrace: make([]byte, 1<<16),
	}

	runtime.Stack(err.StackTrace, true)

	return err
}

func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// Initialize the CLI module
func Init(level string, silent bool, quiet bool, verbose bool) {
	if level == "" {
		level = "WARN"
	}

	filter := &logutils.LevelFilter{
		Levels: []logutils.LogLevel{"DEBUG", "VERBOSE", "INFO",
			"WARN", "ERROR"},
		MinLevel: logutils.LogLevel(level),
		Writer:   os.Stderr,
	}

	log.SetOutput(filter)

	if silent {
		Verbosity = VERBOSITY_SILENT
	} else if quiet {
		Verbosity = VERBOSITY_QUIET
	} else if verbose {
		Verbosity = VERBOSITY_VERBOSE
	} else {
		Verbosity = VERBOSITY_DEFAULT
	}
}

func CheckBoolMap(mapVar map[string]bool, item string) bool {
	v, ok := mapVar[item]
	return v && ok
}

// Read in the configuration file specified by name, in path
// return a new viper config object if successful, and error if not
func ReadConfig(path string, name string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName(name)
	v.AddConfigPath(path)

	err := v.ReadInConfig()
	if err != nil {
		return nil, NewNewtError(fmt.Sprintf("Error reading %s.yml: %s",
			filepath.Join(path, name), err.Error()))
	} else {
		return v, nil
	}
}
