/*
 Copyright 2015 Stack Inc.
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
	"fmt"
	"github.com/hashicorp/logutils"
	"github.com/spf13/viper"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type StackError struct {
	Text       string
	StackTrace []byte
}

var Logger *log.Logger

func (se *StackError) Error() string {
	return se.Text + "\n" + string(se.StackTrace)
}

func NewStackError(msg string) *StackError {
	err := &StackError{
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
		Levels:   []logutils.LogLevel{"DEBUG", "VERBOSE", "INFO", "WARN", "ERROR"},
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
		return nil, NewStackError(err.Error())
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
		val += " " + v.GetString(key+"."+ident)
	}
	return val
}

func GetStringSliceIdentities(v *viper.Viper, t *Target, key string) []string {
	val := v.GetStringSlice(key)

	if t == nil {
		return val
	}

	idents := t.Identities

	for _, ident := range idents {
		val = append(val, v.GetStringSlice(key+"."+ident)...)
	}
	return val
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

// Execute the command specified by cmdStr on the shell and return results
func ShellCommand(cmdStr string) ([]byte, error) {
	log.Print("[VERBOSE] " + cmdStr)
	cmd := exec.Command("sh", "-c", cmdStr)

	o, err := cmd.CombinedOutput()
	log.Print("[VERBOSE] o=" + string(o))
	if err != nil {
		return o, NewStackError(err.Error())
	} else {
		return o, nil
	}
}

func fileClone(path string, dest string) error {
	_, err := ShellCommand("cp -rf " + path + " " + dest)
	if err != nil {
		return err
	}
	return nil
}

func gitCleanClone(urlLoc string, branch string, dest string) error {
	_, err := ShellCommand(fmt.Sprintf("git clone -b %s %s %s", branch, urlLoc, dest))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dest + "/.git/"); err != nil {
		return err
	}

	return nil
}

func gitSubmoduleClone(urlLoc string, branch string, dest string) error {
	_, err := ShellCommand(fmt.Sprintf("git submodule add -b %s %s %s", branch, urlLoc, dest))
	if err != nil {
		return err
	}
	return nil
}

func gitSubtreeClone(urlLoc string, branch string, dest string) error {
	// first, add urlLoc as a remote, name = filepath.Base(dest)
	remoteName := filepath.Base(dest)
	_, err := ShellCommand(fmt.Sprintf("git remote add %s %s", remoteName, urlLoc))
	if err != nil {
		return err
	}

	// then fetch the remote
	_, err = ShellCommand("git remote fetch " + remoteName)
	if err != nil {
		return err
	}

	// now, create the remote as a subtree
	_, err = ShellCommand(fmt.Sprintf("git subtree add --prefix %s %s %s", dest, remoteName, branch))
	if err != nil {
		return err
	}

	// XXX: append the remote name as a file to "<repo>/scripts/git-remotes.sh"

	return nil
}

func UrlPath(urlLoc string) (string, error) {
	url, err := url.Parse(urlLoc)
	if err != nil {
		return "", NewStackError(err.Error())
	}

	return filepath.Base(url.Path), nil
}

func CopyUrl(urlLoc string, branch string, dest string, installType int) error {
	url, err := url.Parse(urlLoc)
	if err != nil {
		return NewStackError(err.Error())
	}

	switch url.Scheme {
	case "file":
		if installType != 0 {
			return NewStackError("Can only do clean source imports for file URLs")
		}
		if err := fileClone(url.Path, dest); err != nil {
			return err
		}
	case "https", "http", "git": // non file schemes are assumed git repos
		switch installType {
		case 0:
			err = gitCleanClone(urlLoc, branch, dest)
		case 1:
			err = gitSubmoduleClone(urlLoc, branch, dest)
		case 2:
			err = gitSubtreeClone(urlLoc, branch, dest)
		default:
			return NewStackError("Unknown install type " + string(installType))
		}
	default:
		err = NewStackError("Unknown resource type: " + url.Scheme)
	}

	if err != nil {
		return err
	}

	return nil
}
