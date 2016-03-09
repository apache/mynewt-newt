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

package cli

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/logutils"
	"github.com/spf13/cobra"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/viper"
)

const (
	VERBOSITY_SILENT  = 0
	VERBOSITY_QUIET   = 1
	VERBOSITY_DEFAULT = 2
	VERBOSITY_VERBOSE = 3
)

var Logger *log.Logger
var Verbosity int = VERBOSITY_DEFAULT
var Force bool
var OK_STRING = " ok!\n"

func NewtUsage(cmd *cobra.Command, err error) {
	if err != nil {
		sErr := err.(*util.NewtError)
		log.Printf("[DEBUG] %s", sErr.StackTrace)
		fmt.Fprintf(os.Stderr, "Error: %s\n", sErr.Text)
	}

	if cmd != nil {
		fmt.Printf("\n")
		fmt.Printf("%s - ", cmd.Name())
		cmd.Help()
	}
	os.Exit(1)
}

// Display help text with a max line width of 79 characters
func FormatHelp(text string) string {
	// first compress all new lines and extra spaces
	words := regexp.MustCompile("\\s+").Split(text, -1)
	linelen := 0
	fmtText := ""
	for _, word := range words {
		word = strings.Trim(word, "\n ") + " "
		tmplen := linelen + len(word)
		if tmplen >= 80 {
			fmtText += "\n"
			linelen = 0
		}
		fmtText += word
		linelen += len(word)
	}
	return fmtText
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

func GetStringFeatures(v *viper.Viper, features map[string]bool,
	key string) string {
	val := v.GetString(key)

	// Process the features in alphabetical order to ensure consistent
	// results across repeated runs.
	var featureKeys []string
	for feature, _ := range features {
		featureKeys = append(featureKeys, feature)
	}
	sort.Strings(featureKeys)

	for _, feature := range featureKeys {
		overwriteVal := v.GetString(key + "." + feature + ".OVERWRITE")
		if overwriteVal != "" {
			val = strings.Trim(overwriteVal, "\n")
			break
		}

		appendVal := v.GetString(key + "." + feature)
		if appendVal != "" {
			val += " " + strings.Trim(appendVal, "\n")
		}
	}
	return strings.TrimSpace(val)
}

func GetStringSliceFeatures(v *viper.Viper, features map[string]bool,
	key string) []string {

	val := v.GetStringSlice(key)

	// string empty items
	result := []string{}
	for _, item := range val {
		if item == "" || item == " " {
			continue
		}
		result = append(result, item)
	}

	for item, _ := range features {
		result = append(result, v.GetStringSlice(key+"."+item)...)
	}

	return result
}

func NodeExist(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else {
		return false
	}
}

// Check whether the node (either dir or file) specified by path exists
func NodeNotExist(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	} else {
		return false
	}
}

func FileModificationTime(path string) (time.Time, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		epoch := time.Unix(0, 0)
		if os.IsNotExist(err) {
			return epoch, nil
		} else {
			return epoch, util.NewNewtError(err.Error())
		}
	}

	return fileInfo.ModTime(), nil
}

func ChildDirs(path string) ([]string, error) {
	children, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, util.NewNewtError(err.Error())
	}

	childDirs := []string{}
	for _, child := range children {
		name := child.Name()
		if !filepath.HasPrefix(name, ".") &&
			!filepath.HasPrefix(name, "..") &&
			child.IsDir() {

			childDirs = append(childDirs, name)
		}
	}

	return childDirs, nil
}

func DescendantDirsOfParent(rootPath string, parentName string, fullPath bool) ([]string, error) {
	rootPath = path.Clean(rootPath)

	if NodeNotExist(rootPath) {
		return []string{}, nil
	}

	children, err := ChildDirs(rootPath)
	if err != nil {
		return nil, err
	}

	dirs := []string{}
	if path.Base(rootPath) == parentName {
		for _, child := range children {
			if fullPath {
				child = rootPath + "/" + child
			}

			dirs = append(dirs, child)
		}
	} else {
		for _, child := range children {
			childPath := rootPath + "/" + child
			subDirs, err := DescendantDirsOfParent(childPath, parentName,
				fullPath)
			if err != nil {
				return nil, err
			}

			dirs = append(dirs, subDirs...)
		}
	}

	return dirs, nil
}

// Execute the command specified by cmdStr on the shell and return results
func ShellCommand(cmdStr string) ([]byte, error) {
	log.Print("[VERBOSE] " + cmdStr)
	cmd := exec.Command("sh", "-c", cmdStr)

	o, err := cmd.CombinedOutput()
	log.Print("[VERBOSE] o=" + string(o))
	if err != nil {
		return o, util.NewNewtError(err.Error())
	} else {
		return o, nil
	}
}

// Run interactive shell command
func ShellInteractiveCommand(cmdStr []string) error {
	log.Print("[VERBOSE] " + cmdStr[0])

	//
	// Block SIGINT, at least.
	// Otherwise Ctrl-C meant for gdb would kill newt.
	//
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
	}()

	// Transfer stdin, stdout, and stderr to the new process
	// and also set target directory for the shell to start in.
	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}

	// Start up a new shell.
	proc, err := os.StartProcess(cmdStr[0], cmdStr, &pa)
	if err != nil {
		signal.Stop(c)
		return util.NewNewtError(err.Error())
	}

	// Release and exit
	_, err = proc.Wait()
	if err != nil {
		signal.Stop(c)
		return util.NewNewtError(err.Error())
	}
	signal.Stop(c)
	return nil
}

func CopyFile(srcFile string, destFile string) error {
	_, err := ShellCommand(fmt.Sprintf("mkdir -p %s", filepath.Dir(destFile)))
	if err != nil {
		return err
	}
	if _, err := ShellCommand(fmt.Sprintf("cp -Rf %s %s", srcFile,
		destFile)); err != nil {
		return err
	}
	return nil
}

func CopyDir(srcDir, destDir string) error {
	return CopyFile(srcDir, destDir)
}

// Print Silent, Quiet and Verbose aware status messages to stdout.
func StatusMessage(level int, message string, args ...interface{}) {
	if Verbosity >= level {
		fmt.Printf(message, args...)
	}
}

// Print Silent, Quiet and Verbose aware status messages to stderr.
func ErrorMessage(level int, message string, args ...interface{}) {
	if Verbosity >= level {
		fmt.Fprintf(os.Stderr, message, args...)
	}
}

// Reads each line from the specified text file into an array of strings.  If a
// line ends with a backslash, it is concatenated with the following line.
func ReadLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, util.NewNewtError(err.Error())
	}
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		concatted := false

		if len(lines) != 0 {
			prevLine := lines[len(lines)-1]
			if len(prevLine) > 0 && prevLine[len(prevLine)-1:] == "\\" {
				prevLine = prevLine[:len(prevLine)-1]
				prevLine += line
				lines[len(lines)-1] = prevLine

				concatted = true
			}
		}

		if !concatted {
			lines = append(lines, line)
		}
	}

	if scanner.Err() != nil {
		return lines, util.NewNewtError(scanner.Err().Error())
	}

	return lines, nil
}

// Removes all duplicate strings from the specified array, while preserving
// order.
func UniqueStrings(elems []string) []string {
	set := make(map[string]bool)
	result := make([]string, 0)

	for _, elem := range elems {
		if !set[elem] {
			result = append(result, elem)
			set[elem] = true
		}
	}

	return result
}

// Sorts whitespace-delimited lists of strings.
//
// @param wsSepStrings          A list of strings; each string contains one or
//                                  more whitespace-delimited tokens.
//
// @return                      A slice containing all the input tokens, sorted
//                                  alphabetically.
func SortFields(wsSepStrings ...string) []string {
	slice := []string{}

	for _, s := range wsSepStrings {
		slice = append(slice, strings.Fields(s)...)
	}

	slice = UniqueStrings(slice)
	sort.Strings(slice)
	return slice
}
