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

package logcfg

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/newt/syscfg"
	"mynewt.apache.org/newt/newt/val"
	"mynewt.apache.org/newt/util"
)

const HEADER_PATH = "logcfg/logcfg.h"

type Log struct {
	// Log name; equal to the name of the YAML map that defines the log.
	Name string

	// The package that defines the log.
	Source *pkg.LocalPackage

	// The log's numeric module ID.
	Module val.ValSetting

	// The level assigned to this log.
	Level val.ValSetting
}

// Map of: [log-name] => log
type LogMap map[string]Log

// The log configuration of the target.
type LCfg struct {
	// [log-name] => log
	Logs LogMap

	// Strings describing errors encountered while parsing the log config.
	InvalidSettings []string

	// Contains sets of logs with conflicting module IDs.
	//     [module-ID] => <slice-of-logs-with-module-id>
	ModuleConflicts map[int][]Log
}

// Maps numeric log levels to their string representations.  Used when
// generating the C log macros.
var logLevelNames = []string{
	0: "DEBUG",
	1: "INFO",
	2: "WARN",
	3: "ERROR",
	4: "CRITICAL",
}

func LogLevelString(level int) string {
	if level < 0 || level >= len(logLevelNames) {
		return "???"
	}

	return logLevelNames[level]
}

func NewLCfg() LCfg {
	return LCfg{
		Logs:            map[string]Log{},
		ModuleConflicts: map[int][]Log{},
	}
}

// Parses a single log definition from a YAML map.  The `logMapItf` parameter
// should be a map with the following elements:
//     "module": <module-string>
//     "level": <level-string>
func parseOneLog(name string, lpkg *pkg.LocalPackage, logMapItf interface{},
	cfg *syscfg.Cfg) (Log, error) {

	cl := Log{
		Name:   name,
		Source: lpkg,
	}

	logMap := cast.ToStringMapString(logMapItf)
	if logMap == nil {
		return cl, util.FmtNewtError(
			"\"%s\" missing required field \"module\"", name)
	}

	modStr := logMap["module"]
	if modStr == "" {
		return cl, util.FmtNewtError(
			"\"%s\" missing required field \"module\"", name)
	}
	mod, err := val.ResolveValSetting(modStr, cfg)
	if err != nil {
		return cl, util.FmtNewtError(
			"\"%s\" contains invalid \"module\": %s",
			name, err.Error())
	}
	if _, err := mod.IntVal(); err != nil {
		return cl, util.FmtNewtError(
			"\"%s\" contains invalid \"module\": %s", name, err.Error())
	}

	levelStr := logMap["level"]
	if levelStr == "" {
		return cl, util.FmtNewtError(
			"\"%s\" missing required field \"level\"", name)
	}
	level, err := val.ResolveValSetting(levelStr, cfg)
	if err != nil {
		return cl, util.FmtNewtError(
			"\"%s\" contains invalid \"level\": %s",
			name, err.Error())
	}
	if _, err := level.IntVal(); err != nil {
		return cl, util.FmtNewtError(
			"\"%s\" contains invalid \"level\": %s", name, err.Error())
	}

	cl.Module = mod
	cl.Level = level

	return cl, nil
}

// Reads all the logs defined by the specified package.  The log definitions
// are read from the `syscfg.logs` map in the package's `syscfg.yml` file.
func (lcfg *LCfg) readOnePkg(lpkg *pkg.LocalPackage, cfg *syscfg.Cfg) {
	lsettings := cfg.AllSettingsForLpkg(lpkg)
	logMaps := lpkg.SyscfgY.GetValStringMap("syscfg.logs", lsettings)
	for name, logMapItf := range logMaps {
		cl, err := parseOneLog(name, lpkg, logMapItf, cfg)
		if err != nil {
			lcfg.InvalidSettings =
				append(lcfg.InvalidSettings, strings.TrimSpace(err.Error()))
		} else {
			lcfg.Logs[cl.Name] = cl
		}
	}
}

// Searches the log configuration for logs with identical module IDs.  The log
// configuration object is populated with the results.
func (lcfg *LCfg) detectModuleConflicts() {
	m := map[int][]Log{}

	for _, l := range lcfg.Logs {
		intMod, _ := l.Module.IntVal()
		m[intMod] = append(m[intMod], l)
	}

	for mod, logs := range m {
		if len(logs) > 1 {
			for _, l := range logs {
				lcfg.ModuleConflicts[mod] =
					append(lcfg.ModuleConflicts[mod], l)
			}
		}
	}
}

// Reads all log definitions for each of the specified packages.  The
// returned LCfg object is populated with the result of this operation.
func Read(lpkgs []*pkg.LocalPackage, cfg *syscfg.Cfg) LCfg {
	lcfg := NewLCfg()

	for _, lpkg := range lpkgs {
		lcfg.readOnePkg(lpkg, cfg)
	}

	lcfg.detectModuleConflicts()

	return lcfg
}

// If any errors were encountered while parsing log definitions, this function
// returns a string indicating the errors.  If no errors were encountered, ""
// is returned.
func (lcfg *LCfg) ErrorText() string {
	str := ""

	if len(lcfg.InvalidSettings) > 0 {
		str += "Invalid log definitions detected:"
		for _, e := range lcfg.InvalidSettings {
			str += "\n    " + e
		}
	}

	if len(lcfg.ModuleConflicts) > 0 {
		str += "Log module conflicts detected:\n"
		for mod, logs := range lcfg.ModuleConflicts {
			for _, l := range logs {
				str += fmt.Sprintf("    Module=%d Log=%s Package=%s\n",
					mod, l.Name, l.Source.FullName())
			}
		}

		str +=
			"\nResolve the problem by assigning unique module IDs to each log."
	}

	return str
}

// Retrieves a sorted slice of logs from the receiving log configuration.
func (lcfg *LCfg) sortedLogs() []Log {
	names := make([]string, 0, len(lcfg.Logs))

	for n, _ := range lcfg.Logs {
		names = append(names, n)
	}
	sort.Strings(names)

	logs := make([]Log, 0, len(names))
	for _, n := range names {
		logs = append(logs, lcfg.Logs[n])
	}

	return logs
}

// Writes a no-op stub log C macro definition.
func writeLogStub(logName string, levelStr string, w io.Writer) {
	fmt.Fprintf(w, "#define %s_%s(...) IGNORE(__VA_ARGS__)\n",
		logName, levelStr)
}

// Writes a log C macro definition.
func writeLogMacro(logName string, module int, levelStr string, w io.Writer) {
	fmt.Fprintf(w,
		"#define %s_%s(...) MODLOG_%s(%d, __VA_ARGS__)\n",
		logName, levelStr, levelStr, module)
}

// Write log C macro definitions for each log in the log configuration.
func (lcfg *LCfg) writeLogMacros(w io.Writer) {
	logs := lcfg.sortedLogs()
	for _, l := range logs {
		fmt.Fprintf(w, "\n")

		levelInt, _ := util.AtoiNoOct(l.Level.Value)
		for i, levelStr := range logLevelNames {
			if i < levelInt {
				writeLogStub(l.Name, levelStr, w)
			} else {
				modInt, _ := l.Module.IntVal()
				writeLogMacro(l.Name, modInt, levelStr, w)
			}
		}
	}
}

// Writes a logcfg header file to the specified writer.
func (lcfg *LCfg) write(w io.Writer) {
	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	fmt.Fprintf(w, "#ifndef H_MYNEWT_LOGCFG_\n")
	fmt.Fprintf(w, "#define H_MYNEWT_LOGCFG_\n\n")

	if len(lcfg.Logs) > 0 {
		fmt.Fprintf(w, "#include \"modlog/modlog.h\"\n")
		fmt.Fprintf(w, "#include \"log_common/log_common.h\"\n")

		lcfg.writeLogMacros(w)
		fmt.Fprintf(w, "\n")
	}

	fmt.Fprintf(w, "#endif\n")
}

// Ensures an up-to-date logcfg header is written for the target.
func (lcfg *LCfg) EnsureWritten(includeDir string) error {
	buf := bytes.Buffer{}
	lcfg.write(&buf)

	path := includeDir + "/" + HEADER_PATH

	writeReqd, err := util.FileContentsChanged(path, buf.Bytes())
	if err != nil {
		return err
	}
	if !writeReqd {
		log.Debugf("logcfg unchanged; not writing header file (%s).", path)
		return nil
	}

	log.Debugf("logcfg changed; writing header file (%s).", path)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return util.NewNewtError(err.Error())
	}

	if err := ioutil.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}
