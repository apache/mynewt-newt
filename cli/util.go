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
	"fmt"
	"github.com/hashicorp/logutils"
	"github.com/spf13/viper"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type NewtError struct {
	Text       string
	StackTrace []byte
}

var Logger *log.Logger

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
