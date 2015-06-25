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
	"errors"
	"github.com/hashicorp/logutils"
	"github.com/spf13/viper"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
)

var Logger *log.Logger

// Initialize the CLI module
func Init(level string) {
	if level == "" {
		level = "WARN"
	}

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR"},
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
		return nil, err
	} else {
		return v, err
	}
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
	log.Print("[DEBUG] " + cmdStr)
	cmd := exec.Command("sh", "-c", cmdStr)

	o, err := cmd.CombinedOutput()
	log.Print("[DEBUG] o=" + string(o))
	return o, err
}

func fileClone(path string, dest string) error {
	_, err := ShellCommand("cp -rf " + path + " " + dest)
	if err != nil {
		return err
	}
	return nil
}

func gitClone(urlLoc string, dest string) error {
	_, err := ShellCommand("git clone " + urlLoc + " " + dest)
	if err != nil {
		return err
	}
	return nil
}

func UrlPath(urlLoc string) (string, error) {
	url, err := url.Parse(urlLoc)
	if err != nil {
		return "", err
	}

	return filepath.Base(url.Path), err
}

func CopyUrl(urlLoc string, dest string) error {
	url, err := url.Parse(urlLoc)
	if err != nil {
		return err
	}

	switch url.Scheme {
	case "file":
		err = fileClone(url.Path, dest)
	case "https", "http", "git": // non file schemes are assumed git repos
		err = gitClone(urlLoc, dest)
	default:
		err = errors.New("Unknown resource type: " + url.Scheme)
	}

	if err != nil {
		return err
	}

	return nil
}
