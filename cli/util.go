/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/hashicorp/logutils"
	"github.com/spf13/viper"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type NewtError struct {
	Text       string
	StackTrace []byte
}

var Logger *log.Logger
var Verbosity int
var OK_STRING = " ok!\n"

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

// Initialize the CLI module
func Init(level string) {
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
}

func checkBoolMap(mapVar map[string]bool, item string) bool {
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
		return nil, NewNewtError(err.Error())
	} else {
		return v, nil
	}
}

func GetStringIdentities(v *viper.Viper, t *Target, key string) string {
	val := v.GetString(key)

	if t == nil {
		return val
	}

	idents := t.Identities

	for _, ident := range idents {
		overwriteVal := v.GetString(key + "." + ident + ".OVERWRITE")
		if overwriteVal != "" {
			val = strings.Trim(overwriteVal, "\n")
			break
		}

		appendVal := v.GetString(key + "." + ident)
		if appendVal != "" {
			val += " " + strings.Trim(appendVal, "\n")
		}
	}
	return strings.TrimSpace(val)
}

func GetStringSliceIdentities(v *viper.Viper, t *Target, key string) []string {
	val := v.GetStringSlice(key)

	// string empty items
	result := []string{}
	for _, item := range val {
		if item == "" || item == " " {
			continue
		}
		result = append(result, item)
	}

	if t == nil {
		return result
	}

	idents := t.Identities

	for _, ident := range idents {
		result = append(result, v.GetStringSlice(key+"."+ident)...)
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
			return epoch, NewNewtError(err.Error())
		}
	}

	return fileInfo.ModTime(), nil
}

// Execute the command specified by cmdStr on the shell and return results
func ShellCommand(cmdStr string) ([]byte, error) {
	log.Print("[VERBOSE] " + cmdStr)
	cmd := exec.Command("sh", "-c", cmdStr)

	o, err := cmd.CombinedOutput()
	log.Print("[VERBOSE] o=" + string(o))
	if err != nil {
		return o, NewNewtError(err.Error())
	} else {
		return o, nil
	}
}

func CopyFile(srcFile string, destFile string) error {
	if _, err := ShellCommand(fmt.Sprintf("cp -rf %s %s", srcFile,
		destFile)); err != nil {
		return err
	}
	return nil
}

func CopyDir(srcDir, destDir string) error {
	return CopyFile(srcDir, destDir)
}

// Print Silent, Quiet and Verbose aware status messages
func StatusMessage(level int, message string, args ...interface{}) {
	if Verbosity >= level {
		fmt.Printf(message, args...)
	}
}

// Reads each line from the specified text file into an array of strings.  If a
// line ends with a backslash, it is concatenated with the following line.
func ReadLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, NewNewtError(err.Error())
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
		return lines, NewNewtError(scanner.Err().Error())
	}

	return lines, nil
}

// Determines if a file was previously built with a command line invocation
// different from the one specified.
//
// @param dstFile               The output file whose build invocation is being
//                                  tested.
// @param cmd                   The command that would be used to generate the
//                                  specified destination file.
//
// @return                      true if the command has changed or if the
//                                  destination file was never built;
//                              false otherwise.
func CommandHasChanged(dstFile string, cmd string) bool {
	cmdFile := dstFile + ".cmd"
	prevCmd, err := ioutil.ReadFile(cmdFile)
	if err != nil {
		return true
	}

	return bytes.Compare(prevCmd, []byte(cmd)) != 0
}

// Writes a file containing the command-line invocation used to generate the
// specified file.  The file that this function writes can be used later to
// determine if the set of compiler options has changed.
//
// @param dstFile               The output file whose build invocation is being
//                                  recorded.
// @param cmd                   The command to write.
func WriteCommandFile(dstFile string, cmd string) error {
	cmdPath := dstFile + ".cmd"
	err := ioutil.WriteFile(cmdPath, []byte(cmd), 0644)
	if err != nil {
		return err
	}

	return nil
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
