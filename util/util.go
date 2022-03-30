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
	"encoding/json"
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

	log "github.com/sirupsen/logrus"

	"github.com/otiai10/copy"
)

var Verbosity int
var PrintShellCmds bool
var InjectSyscfg string
var ExecuteShell bool
var EscapeShellCmds bool
var ShallowCloneDepth int
var logFile *os.File
var SkipNewtCompat bool
var SkipSyscfgRepoHash bool

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

func FmtChildNewtError(parent error, format string,
	args ...interface{}) *NewtError {

	ne := ChildNewtError(parent)
	ne.Text = fmt.Sprintf(format, args...)
	return ne
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

// Escapes special characters for Windows builds (not in an MSYS environment).
func fixupCmdArgs(args []string) {
	if EscapeShellCmds {
		for i, _ := range args {
			args[i] = strings.Replace(args[i], "{", "\\{", -1)
			args[i] = strings.Replace(args[i], "}", "\\}", -1)
		}
	}
}

func LogShellCmd(cmdStrs []string, env map[string]string) {
	envLogStr := ""
	if len(env) > 0 {
		s := EnvVarsToSlice(env)
		envLogStr = strings.Join(s, " ") + " "
	}
	log.Debugf("%s%s", envLogStr, strings.Join(cmdStrs, " "))

	if PrintShellCmds {
		StatusMessage(VERBOSITY_DEFAULT, "%s\n", strings.Join(cmdStrs, " "))
	}
}

// EnvVarsToSlice converts an environment variable map into a slice of `k=v`
// strings.
func EnvVarsToSlice(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k, _ := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	slice := make([]string, 0, len(env))
	for _, key := range keys {
		slice = append(slice, fmt.Sprintf("%s=%s", key, env[key]))
	}

	return slice
}

// SliceToEnvVars converts a slice of `k=v` strings into an environment
// variable map.
func SliceToEnvVars(slc []string) (map[string]string, error) {
	m := make(map[string]string, len(slc))
	for _, s := range slc {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return nil, FmtNewtError("invalid env var string: \"%s\"", s)
		}

		m[parts[0]] = parts[1]
	}

	return m, nil
}

// EnvironAsMap gathers the current process's set of environment variables and
// returns them as a map.
func EnvironAsMap() (map[string]string, error) {
	slc := os.Environ()

	m, err := SliceToEnvVars(slc)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Execute the specified process and block until it completes.  Additionally,
// the amount of combined stdout+stderr output to be logged to the debug log
// can be restricted to a maximum number of characters.
//
// @param cmdStrs               The "argv" strings of the command to execute.
// @param env                   Additional key,value pairs to inject into the
//                                  child process's environment.  Specify null
//                                  to just inherit the parent environment.
// @param logCmd                Whether to log the command being executed.
// @param maxDbgOutputChrs      The maximum number of combined stdout+stderr
//                                  characters to write to the debug log.
//                                  Specify -1 for no limit; 0 for no output.
//
// @return []byte               Combined stdout and stderr output of process.
// @return error                NewtError on failure.  Use IsExit() to
//                                  determine if the command failed to execute
//                                  or if it just returned a non-zero exit
//                                  status.
func ShellCommandLimitDbgOutput(
	cmdStrs []string, env map[string]string, logCmd bool,
	maxDbgOutputChrs int) ([]byte, error) {

	var name string
	var args []string

	// Escape special characters for Windows.
	fixupCmdArgs(cmdStrs)

	if logCmd {
		LogShellCmd(cmdStrs, env)
	}

	if ExecuteShell && (runtime.GOOS == "linux" || runtime.GOOS == "darwin") {
		cmd := strings.Join(cmdStrs, " ")
		name = "/bin/sh"
		cmd = strings.Replace(cmd, "\"", "\\\"", -1)
		cmd = strings.Replace(cmd, "<", "\\<", -1)
		cmd = strings.Replace(cmd, ">", "\\>", -1)
		args = []string{"-c", cmd}
	} else {
		if strings.HasSuffix(cmdStrs[0], ".sh") {
			var newt_sh = os.Getenv("NEWT_SH")
			if newt_sh == "" {
				newt_sh = "bash"
			}
			name = newt_sh
			args = cmdStrs
		} else {
			name = cmdStrs[0]
			args = cmdStrs[1:]
		}
	}
	cmd := exec.Command(name, args...)

	if env != nil {
		m, err := EnvironAsMap()
		if err != nil {
			return nil, err
		}

		for k, v := range env {
			m[k] = v
		}
		cmd.Env = EnvVarsToSlice(m)
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
		err = ChildNewtError(err)
		log.Debugf("err=%s", err.Error())
		if len(o) > 0 {
			err.(*NewtError).Text = string(o)
		}
		return o, err
	} else {
		return o, nil
	}
}

// Execute the specified process and block until it completes.
//
// @param cmdStrs               The "argv" strings of the command to execute.
// @param env                   Additional key,value pairs to inject into the
//                                  child process's environment.  Specify null
//                                  to just inherit the parent environment.
//
// @return []byte               Combined stdout and stderr output of process.
// @return error                NewtError on failure.
func ShellCommand(cmdStrs []string, env map[string]string) ([]byte, error) {
	return ShellCommandLimitDbgOutput(cmdStrs, env, true, -1)
}

// Run interactive shell command
func ShellInteractiveCommand(cmdStr []string, env map[string]string,
	flagBlock bool) error {

	// Escape special characters for Windows.
	fixupCmdArgs(cmdStr)

	var newt_sh string
	if runtime.GOOS == "windows" && strings.HasSuffix(cmdStr[0], ".sh") {
		newt_sh = os.Getenv("NEWT_SH")
		if newt_sh == "" {
			bash, err := exec.LookPath("bash")
			if err != nil {
				return err
			}
			newt_sh = bash
		}
		cmdStr = append([]string{newt_sh}, cmdStr...)
	}
	log.Print("[VERBOSE] " + cmdStr[0])

	c := make(chan os.Signal, 1)
	// Block SIGINT, at least.
	// Otherwise Ctrl-C meant for gdb would kill newt.
	if flagBlock == false {
		signal.Notify(c, os.Interrupt)
		signal.Notify(c, syscall.SIGTERM)

		go func() {
			<-c
		}()
	}

	m, err := EnvironAsMap()
	if err != nil {
		return err
	}

	for k, v := range env {
		m[k] = v
	}
	envSlice := EnvVarsToSlice(m)

	// Transfer stdin, stdout, and stderr to the new process
	// and also set target directory for the shell to start in.
	// and set the additional environment variables
	pa := os.ProcAttr{
		Env:   envSlice,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}

	// Start up a new shell.
	proc, err := os.StartProcess(cmdStr[0], cmdStr, &pa)
	if err != nil {
		signal.Stop(c)
		return NewNewtError(err.Error())
	}

	// Release and exit
	state, err := proc.Wait()
	signal.Stop(c)

	if err != nil {
		return NewNewtError(err.Error())
	}
	if state.ExitCode() != 0 {
		return FmtNewtError(
			"command %v exited with nonzero status %d",
			cmdStr, state.ExitCode())
	}

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
	opt := copy.Options{
		OnSymlink: func(src string) copy.SymlinkAction {
			return copy.Shallow
		},
	}

	err := copy.Copy(srcDirStr, dstDirStr, opt)

	if err != nil {
		return ChildNewtError(err)
	}

	return nil
}

func MoveFile(srcFile string, destFile string) error {
	// First, attempt a rename.  This will succeed if the source and
	// destination are on the same disk.
	if err := os.Rename(srcFile, destFile); err == nil {
		return nil
	}

	// Otherwise, copy the file and delete the old path.
	if err := CopyFile(srcFile, destFile); err != nil {
		return err
	}

	if err := os.RemoveAll(srcFile); err != nil {
		return ChildNewtError(err)
	}

	return nil
}

func MoveDir(srcDir string, destDir string) error {
	// First, attempt a rename.  This will succeed if the source and
	// destination are on the same disk.
	if err := os.Rename(srcDir, destDir); err == nil {
		return nil
	}

	// Otherwise, copy the directory and delete the old path.
	if err := CopyDir(srcDir, destDir); err != nil {
		return err
	}

	if err := os.RemoveAll(srcDir); err != nil {
		return ChildNewtError(err)
	}

	return nil
}

func CallInDir(path string, execFunc func() error) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	os.Chdir(path)

	err = execFunc()

	os.Chdir(wd)

	return err
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

// Converts the specified string to an integer.  The string can be in base-10
// or base-16.  This is equivalent to the "0" base used in the standard
// conversion functions, except octal is not supported (a leading zero implies
// decimal).
//
// The second return value is true on success.
func AtoiNoOctTry(s string) (int, bool) {
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
		return 0, false
	}

	return int(i), true
}

// Converts the specified string to an integer.  The string can be in base-10
// or base-16.  This is equivalent to the "0" base used in the standard
// conversion functions, except octal is not supported (a leading zero implies
// decimal).
func AtoiNoOct(s string) (int, error) {
	val, success := AtoiNoOctTry(s)
	if !success {
		return 0, FmtNewtError("Invalid number: \"%s\"", s)
	}

	return val, nil
}

func IsNotExist(err error) bool {
	newtErr, ok := err.(*NewtError)
	if ok {
		err = newtErr.Parent
	}

	return os.IsNotExist(err)
}

// Indicates whether the provided error is of type *exec.ExitError (raised when
// a child process exits with a non-zero status code).
func IsExit(err error) bool {
	newtErr, ok := err.(*NewtError)
	if ok {
		err = newtErr.Parent
	}

	_, ok = err.(*exec.ExitError)
	return ok
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

// Attempts to convert the specified absolute path into a relative path
// (relative to the current working directory).  If the path cannot be
// converted, it is returned unchanged.
func TryRelPath(full string) string {
	pwd, err := os.Getwd()
	if err != nil {
		return full
	}

	rel, err := filepath.Rel(pwd, full)
	if err != nil {
		return full
	}

	return rel
}

// StringMapStringToItfMapItf converts a map[string]string to the more generic
// map[interface{}]interface{} type.
func StringMapStringToItfMapItf(
	sms map[string]string) map[interface{}]interface{} {

	imi := map[interface{}]interface{}{}
	for k, v := range sms {
		imi[k] = v
	}

	return imi
}

// FileContains indicates whether the specified file's contents are equal to
// the provided byte slice.
func FileContains(contents []byte, path string) (bool, error) {
	oldSrc, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist; contents aren't equal.
			return false, nil
		}

		return false, NewNewtError(err.Error())
	}

	rc := bytes.Compare(oldSrc, contents)
	return rc == 0, nil
}

// Keeps track of warnings that have already been reported.
// [warning-text] => struct{}
var warnings = map[string]struct{}{}

// Displays the specified warning if it has not been displayed yet.
func OneTimeWarning(text string, args ...interface{}) {
	body := fmt.Sprintf(text, args...)
	if _, ok := warnings[body]; !ok {
		warnings[body] = struct{}{}

		body := fmt.Sprintf(text, args...)
		ErrorMessage(VERBOSITY_QUIET, "WARNING: %s\n", body)
	}
}

// OneTimeWarningError displays the text of the specified error as a warning if
// it has not been displayed yet.  No-op if nil is passed in.
func OneTimeWarningError(err error) {
	if err != nil {
		OneTimeWarning("%s", err.Error())
	}
}

func MarshalJSONStringer(sr fmt.Stringer) ([]byte, error) {
	s := sr.String()
	j, err := json.Marshal(s)
	if err != nil {
		return nil, ChildNewtError(err)
	}

	return j, nil
}

// readDirRecursive recursively reads the contents of a directory.  It returns
// [dir-paths],[file-paths].  All returned strings are relative to the provided
// base directory.
func readDirRecursive(path string) ([]string, []string, error) {
	var dirs []string
	var files []string

	var iter func(crumbs string) error
	iter = func(crumbs string) error {
		var crumbsPath string
		if crumbs != "" {
			crumbsPath = "/" + crumbs
		}

		f, err := os.Open(path + crumbsPath)
		if err != nil {
			return ChildNewtError(err)
		}
		defer f.Close()

		infos, err := f.Readdir(-1)
		if err != nil {
			return ChildNewtError(err)
		}

		for _, info := range infos {
			name := fmt.Sprintf("%s/%s", crumbs, info.Name())

			if info.IsDir() {
				dirs = append(dirs, name)
				if err := iter(name); err != nil {
					return err
				}
			} else {
				files = append(files, name)
			}
		}

		return nil
	}

	if err := iter(""); err != nil {
		return nil, nil, err
	}

	return dirs, files, nil
}

// DirsAreEqual compares the contents of two directories.  Directories are
// equal if 1) their subdirectory structures are identical, and 2) they contain
// the exact same set of files (same names and contents).
func DirsAreEqual(dira string, dirb string) (bool, error) {
	dirsa, filesa, err := readDirRecursive(dira)
	if err != nil {
		return false, err
	}

	dirsb, filesb, err := readDirRecursive(dirb)
	if err != nil {
		return false, err
	}

	if len(dirsa) != len(dirsb) || len(filesa) != len(filesb) {
		return false, nil
	}

	// Returns the intersection of two sets of strings.
	intersection := func(a []string, b []string) map[string]struct{} {
		ma := make(map[string]struct{}, len(a))
		for _, p := range a {
			ma[p] = struct{}{}
		}

		isect := map[string]struct{}{}
		for _, p := range b {
			if _, ok := ma[p]; ok {
				isect[p] = struct{}{}
			}
		}

		return isect
	}

	// If the intersection lengths are equal, both directories have the same
	// structure.

	isectDirs := intersection(dirsa, dirsb)
	if len(isectDirs) != len(dirsa) {
		return false, nil
	}

	isectFiles := intersection(filesa, filesb)
	if len(isectFiles) != len(filesa) {
		return false, nil
	}

	// Finally, compare the contents of files in each directory.
	for _, p := range filesa {
		patha := fmt.Sprintf("%s/%s", dira, p)
		bytesa, err := ioutil.ReadFile(patha)
		if err != nil {
			return false, ChildNewtError(err)
		}

		pathb := fmt.Sprintf("%s/%s", dirb, p)
		unchanged, err := FileContains(bytesa, pathb)
		if err != nil {
			return false, err
		}

		if !unchanged {
			return false, nil
		}
	}

	return true, nil
}
