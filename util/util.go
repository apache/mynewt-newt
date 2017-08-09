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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/viper"
)

var Verbosity int
var PrintShellCmds bool
var ExecuteShell bool
var logFile *os.File

func ParseEqualsPair(v string) (string, string, error) {
	s := strings.Split(v, "=")
	return s[0], s[1], nil
}

type NewtError struct {
	Parent     error
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
	return se.Text
}

func NewNewtError(msg string) *NewtError {
	err := &NewtError{
		Text:       msg,
		StackTrace: make([]byte, 65536),
	}

	stackLen := runtime.Stack(err.StackTrace, true)
	err.StackTrace = err.StackTrace[:stackLen]

	return err
}

func FmtNewtError(format string, args ...interface{}) *NewtError {
	return NewNewtError(fmt.Sprintf(format, args...))
}

func PreNewtError(err error, format string, args ...interface{}) *NewtError {
	baseErr := err.(*NewtError)
	baseErr.Text = fmt.Sprintf(format, args...) + "; " + baseErr.Text

	return baseErr
}

func ChildNewtError(parent error) *NewtError {
	for {
		newtErr, ok := parent.(*NewtError)
		if !ok || newtErr == nil || newtErr.Parent == nil {
			break
		}
		parent = newtErr.Parent
	}

	newtErr := NewNewtError(parent.Error())
	newtErr.Parent = parent
	return newtErr
}

// Print Silent, Quiet and Verbose aware status messages to stdout.
func WriteMessage(f *os.File, level int, message string,
	args ...interface{}) {

	if Verbosity >= level {
		str := fmt.Sprintf(message, args...)
		f.WriteString(str)
		f.Sync()

		if logFile != nil {
			logFile.WriteString(str)
		}
	}
}

// Print Silent, Quiet and Verbose aware status messages to stdout.
func StatusMessage(level int, message string, args ...interface{}) {
	WriteMessage(os.Stdout, level, message, args...)
}

// Print Silent, Quiet and Verbose aware status messages to stderr.
func ErrorMessage(level int, message string, args ...interface{}) {
	WriteMessage(os.Stderr, level, message, args...)
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

func ChildDirs(path string) ([]string, error) {
	children, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, NewNewtError(err.Error())
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

type logFormatter struct{}

func (f *logFormatter) Format(entry *log.Entry) ([]byte, error) {
	// 2016/03/16 12:50:47 [DEBUG]

	b := &bytes.Buffer{}

	b.WriteString(entry.Time.Format("2006/01/02 15:04:05.000 "))
	b.WriteString("[" + strings.ToUpper(entry.Level.String()) + "] ")
	b.WriteString(entry.Message)
	b.WriteByte('\n')

	return b.Bytes(), nil
}

func initLog(level log.Level, logFilename string) error {
	log.SetLevel(level)

	var writer io.Writer
	if logFilename == "" {
		writer = os.Stderr
	} else {
		var err error
		logFile, err = os.Create(logFilename)
		if err != nil {
			return NewNewtError(err.Error())
		}

		writer = io.MultiWriter(os.Stderr, logFile)
	}

	log.SetOutput(writer)
	log.SetFormatter(&logFormatter{})

	return nil
}

// Initialize the util module
func Init(logLevel log.Level, logFile string, verbosity int) error {
	// Configure logging twice.  First just configure the filter for stderr;
	// second configure the logfile if there is one.  This needs to happen in
	// two steps so that the log level is configured prior to the attempt to
	// open the log file.  The correct log level needs to be applied to file
	// error messages.
	if err := initLog(logLevel, ""); err != nil {
		return err
	}
	if logFile != "" {
		if err := initLog(logLevel, logFile); err != nil {
			return err
		}
	}

	Verbosity = verbosity
	PrintShellCmds = false
	ExecuteShell = false

	return nil
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

// Execute the specified process and block until it completes.  Additionally,
// the amount of combined stdout+stderr output to be logged to the debug log
// can be restricted to a maximum number of characters.
//
// @param cmdStrs               The "argv" strings of the command to execute.
// @param env                   Additional key=value pairs to inject into the
//                                  child process's environment.  Specify null
//                                  to just inherit the parent environment.
// @param maxDbgOutputChrs      The maximum number of combined stdout+stderr
//                                  characters to write to the debug log.
//                                  Specify -1 for no limit; 0 for no output.
//
// @return []byte               Combined stdout and stderr output of process.
// @return error                NewtError on failure.
func ShellCommandLimitDbgOutput(
	cmdStrs []string, env []string, maxDbgOutputChrs int) ([]byte, error) {
	var name string
	var args []string

	envLogStr := ""
	if env != nil {
		envLogStr = strings.Join(env, " ") + " "
	}
	log.Debugf("%s%s", envLogStr, strings.Join(cmdStrs, " "))

	if PrintShellCmds {
		StatusMessage(VERBOSITY_SILENT, "%s\n", strings.Join(cmdStrs, " "))
	}

	if ExecuteShell && ((runtime.GOOS == "linux") || (runtime.GOOS == "darwin")) {
		cmd := strings.Join(cmdStrs, " ")
		name = "/bin/sh"
		args = []string{"-c", strings.Replace(cmd, "\"", "\\\"", -1)}
	} else {
		name = cmdStrs[0]
		args = cmdStrs[1:]
	}
	cmd := exec.Command(name, args...)

	if env != nil {
		cmd.Env = append(env, os.Environ()...)
	}

	o, err := cmd.CombinedOutput()

	if maxDbgOutputChrs < 0 || len(o) <= maxDbgOutputChrs {
		dbgStr := string(o)
		log.Debugf("o=%s", dbgStr)
	} else if maxDbgOutputChrs != 0 {
		dbgStr := string(o[:maxDbgOutputChrs]) + "[...]"
		log.Debugf("o=%s", dbgStr)
	}

	if err != nil {
		log.Debugf("err=%s", err.Error())
		if len(o) > 0 {
			return o, NewNewtError(string(o))
		} else {
			return o, NewNewtError(err.Error())
		}
	} else {
		return o, nil
	}
}

// Execute the specified process and block until it completes.
//
// @param cmdStrs               The "argv" strings of the command to execute.
// @param env                   Additional key=value pairs to inject into the
//                                  child process's environment.  Specify null
//                                  to just inherit the parent environment.
//
// @return []byte               Combined stdout and stderr output of process.
// @return error                NewtError on failure.
func ShellCommand(cmdStrs []string, env []string) ([]byte, error) {
	return ShellCommandLimitDbgOutput(cmdStrs, env, -1)
}

// Run interactive shell command
func ShellInteractiveCommand(cmdStr []string, env []string) error {
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

	if env != nil {
		env = append(env, os.Environ()...)
	}

	// Transfer stdin, stdout, and stderr to the new process
	// and also set target directory for the shell to start in.
	// and set the additional environment variables
	pa := os.ProcAttr{
		Env:   env,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}

	// Start up a new shell.
	proc, err := os.StartProcess(cmdStr[0], cmdStr, &pa)
	if err != nil {
		signal.Stop(c)
		return NewNewtError(err.Error())
	}

	// Release and exit
	_, err = proc.Wait()
	if err != nil {
		signal.Stop(c)
		return NewNewtError(err.Error())
	}
	signal.Stop(c)
	return nil
}

func CopyFile(srcFile string, dstFile string) error {
	in, err := os.Open(srcFile)
	if err != nil {
		return ChildNewtError(err)
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return ChildNewtError(err)
	}

	dstDir := filepath.Dir(dstFile)
	if err := os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return ChildNewtError(err)
	}

	out, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		info.Mode())
	if err != nil {
		return ChildNewtError(err)
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return ChildNewtError(err)
	}

	return nil
}

func CopyDir(srcDirStr, dstDirStr string) error {
	srcDir, err := os.Open(srcDirStr)
	if err != nil {
		return ChildNewtError(err)
	}

	info, err := srcDir.Stat()
	if err != nil {
		return ChildNewtError(err)
	}

	if err := os.MkdirAll(filepath.Dir(dstDirStr), info.Mode()); err != nil {
		return ChildNewtError(err)
	}

	infos, err := srcDir.Readdir(-1)
	if err != nil {
		return ChildNewtError(err)
	}

	for _, info := range infos {
		src := srcDirStr + "/" + info.Name()
		dst := dstDirStr + "/" + info.Name()
		if info.IsDir() {
			if err := CopyDir(src, dst); err != nil {
				return err
			}
		} else {
			if err := CopyFile(src, dst); err != nil {
				return err
			}
		}
	}

	return nil
}

func MoveFile(srcFile string, destFile string) error {
	if err := CopyFile(srcFile, destFile); err != nil {
		return err
	}

	if err := os.RemoveAll(srcFile); err != nil {
		return ChildNewtError(err)
	}

	return nil
}

func MoveDir(srcDir string, destDir string) error {
	if err := CopyDir(srcDir, destDir); err != nil {
		return err
	}

	if err := os.RemoveAll(srcDir); err != nil {
		return ChildNewtError(err)
	}

	return nil
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

func AtoiNoOct(s string) (int, error) {
	var runLen int
	for runLen = 0; runLen < len(s)-1; runLen++ {
		if s[runLen] != '0' || s[runLen+1] == 'x' {
			break
		}
	}

	if runLen > 0 {
		s = s[runLen:]
	}

	i, err := strconv.ParseInt(s, 0, 0)
	if err != nil {
		return 0, NewNewtError(err.Error())
	}

	return int(i), nil
}

func IsNotExist(err error) bool {
	newtErr, ok := err.(*NewtError)
	if ok {
		err = newtErr.Parent
	}

	return os.IsNotExist(err)
}

func FileContentsChanged(path string, newContents []byte) (bool, error) {
	oldContents, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist; write required.
			return true, nil
		}

		return true, NewNewtError(err.Error())
	}

	rc := bytes.Compare(oldContents, newContents)
	return rc != 0, nil
}

func CIdentifier(s string) string {
	s = strings.Replace(s, "/", "_", -1)
	s = strings.Replace(s, "-", "_", -1)
	s = strings.Replace(s, " ", "_", -1)

	return s
}

func FilenameFromPath(s string) string {
	s = strings.Replace(s, "/", "_", -1)
	s = strings.Replace(s, " ", "_", -1)
	s = strings.Replace(s, "\t", "_", -1)
	s = strings.Replace(s, "\n", "_", -1)

	return s
}

func IntMax(a, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

func IntMin(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func PrintStacks() {
	buf := make([]byte, 1024*1024)
	stacklen := runtime.Stack(buf, true)
	fmt.Printf("*** goroutine dump\n%s\n*** end\n", buf[:stacklen])
}
