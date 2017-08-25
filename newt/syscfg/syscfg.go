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

package syscfg

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/flash"
	"mynewt.apache.org/newt/newt/interfaces"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/util"
)

const HEADER_PATH = "syscfg/syscfg.h"

const SYSCFG_PREFIX_SETTING = "MYNEWT_VAL_"

type CfgSettingType int

const (
	CFG_SETTING_TYPE_RAW CfgSettingType = iota
	CFG_SETTING_TYPE_TASK_PRIO
	CFG_SETTING_TYPE_INTERRUPT_PRIO
	CFG_SETTING_TYPE_FLASH_OWNER
)

const SYSCFG_PRIO_ANY = "any"

// Reserve last 16 priorities for the system (sanity, idle).
const SYSCFG_TASK_PRIO_MAX = 0xef

var cfgSettingNameTypeMap = map[string]CfgSettingType{
	"raw":           CFG_SETTING_TYPE_RAW,
	"task_priority": CFG_SETTING_TYPE_TASK_PRIO,
	"flash_owner":   CFG_SETTING_TYPE_FLASH_OWNER,
}

type CfgPoint struct {
	Value  string
	Source *pkg.LocalPackage
}

type CfgEntry struct {
	Name         string
	Value        string
	Description  string
	SettingType  CfgSettingType
	Restrictions []CfgRestriction
	PackageDef   *pkg.LocalPackage
	History      []CfgPoint
}

type CfgPriority struct {
	SettingName string
	PackageDef  *pkg.LocalPackage // package that define the setting.
	PackageSrc  *pkg.LocalPackage // package overriding setting value.
}

type CfgFlashConflictCode int

const (
	CFG_FLASH_CONFLICT_CODE_BAD_NAME CfgFlashConflictCode = iota
	CFG_FLASH_CONFLICT_CODE_NOT_UNIQUE
)

type CfgFlashConflict struct {
	SettingNames []string
	Code         CfgFlashConflictCode
}

type Cfg struct {
	Settings map[string]CfgEntry

	//// Errors
	// Overrides of undefined settings.
	Orphans map[string][]CfgPoint

	// Two packages of equal priority override a setting with different
	// values; not overridden by higher priority package.
	Ambiguities map[string][]CfgPoint

	// Setting restrictions not met.
	Violations map[string][]CfgRestriction

	// Attempted override by bottom-priority packages (libraries).
	PriorityViolations []CfgPriority

	FlashConflicts []CfgFlashConflict
}

func NewCfg() Cfg {
	return Cfg{
		Settings:           map[string]CfgEntry{},
		Orphans:            map[string][]CfgPoint{},
		Ambiguities:        map[string][]CfgPoint{},
		Violations:         map[string][]CfgRestriction{},
		PriorityViolations: []CfgPriority{},
		FlashConflicts:     []CfgFlashConflict{},
	}
}

func ValueIsTrue(val string) bool {
	if val == "" {
		return false
	}

	i, err := util.AtoiNoOct(val)
	if err == nil && i == 0 {
		return false
	}

	return true
}

func (cfg *Cfg) Features() map[string]bool {
	features := map[string]bool{}
	for k, v := range cfg.Settings {
		if v.IsTrue() {
			features[k] = true
		}
	}

	return features
}

func (cfg *Cfg) FeaturesForLpkg(lpkg *pkg.LocalPackage) map[string]bool {
	features := cfg.Features()

	for k, v := range lpkg.InjectedSettings() {
		_, ok := features[k]
		if ok {
			log.Warnf("Attempt to override syscfg setting %s with "+
				"injected feature from package %s", k, lpkg.Name())
		} else {
			if ValueIsTrue(v) {
				features[k] = true
			}
		}
	}

	return features
}

func (point CfgPoint) Name() string {
	if point.Source == nil {
		return "newt"
	} else {
		return point.Source.Name()
	}
}

func (point CfgPoint) IsInjected() bool {
	return point.Source == nil
}

func (entry *CfgEntry) IsTrue() bool {
	return ValueIsTrue(entry.Value)
}

func (entry *CfgEntry) appendValue(lpkg *pkg.LocalPackage, value interface{}) {
	strval := stringValue(value)
	point := CfgPoint{Value: strval, Source: lpkg}
	entry.History = append(entry.History, point)
	entry.Value = strval
}

func historyToString(history []CfgPoint) string {
	str := "["
	for i, _ := range history {
		if i != 0 {
			str += ", "
		}
		p := history[len(history)-i-1]
		str += fmt.Sprintf("%s:%s", p.Name(), p.Value)
	}
	str += "]"

	return str
}

func (entry *CfgEntry) ambiguities() []CfgPoint {
	diffVals := false
	var points []CfgPoint

	for i := 1; i < len(entry.History)-1; i++ {
		cur := entry.History[len(entry.History)-i-1]
		next := entry.History[len(entry.History)-i]

		// If either setting is injected, there is no ambiguity
		if cur.Source == nil || next.Source == nil {
			break
		}

		// If the two package have different priorities, there is no ambiguity.
		if normalizePkgType(cur.Source.Type()) !=
			normalizePkgType(next.Source.Type()) {

			break
		}

		if cur.Value != next.Value {
			diffVals = true
		}

		if len(points) == 0 {
			points = append(points, cur)
		}
		points = append(points, next)
	}

	// If all values are identical, there is no ambiguity
	if !diffVals {
		points = nil
	}

	return points
}

func (entry *CfgEntry) ambiguityText() string {
	points := entry.ambiguities()
	if len(points) == 0 {
		return ""
	}

	str := fmt.Sprintf("Setting: %s, Packages: [", entry.Name)
	for i, p := range points {
		if i > 0 {
			str += ", "
		}

		str += p.Source.Name()
	}
	str += "]"

	return str
}

func FeatureToCflag(featureName string) string {
	return fmt.Sprintf("-D%s=1", settingName(featureName))
}

func stringValue(val interface{}) string {
	return strings.TrimSpace(cast.ToString(val))
}

func readSetting(name string, lpkg *pkg.LocalPackage,
	vals map[interface{}]interface{}) (CfgEntry, error) {

	entry := CfgEntry{}

	entry.Name = name
	entry.PackageDef = lpkg
	entry.Description = stringValue(vals["description"])

	// The value field for setting definition is required.
	valueVal, valueExist := vals["value"]
	if valueExist {
		entry.Value = stringValue(valueVal)
	} else {
		return entry, util.FmtNewtError(
			"setting %s does not have required value field", name)
	}

	if vals["type"] == nil {
		entry.SettingType = CFG_SETTING_TYPE_RAW
	} else {
		var ok bool
		typename := stringValue(vals["type"])
		entry.SettingType, ok = cfgSettingNameTypeMap[typename]
		if !ok {
			return entry, util.FmtNewtError(
				"setting %s specifies invalid type: %s", name, typename)
		}
	}
	entry.appendValue(lpkg, entry.Value)

	entry.Restrictions = []CfgRestriction{}
	restrictionStrings := cast.ToStringSlice(vals["restrictions"])
	for _, rstring := range restrictionStrings {
		r, err := readRestriction(name, rstring)
		if err != nil {
			return entry,
				util.PreNewtError(err, "error parsing setting %s", name)
		}
		entry.Restrictions = append(entry.Restrictions, r)
	}

	return entry, nil
}

func (cfg *Cfg) readDefsOnce(lpkg *pkg.LocalPackage,
	features map[string]bool) error {
	v := lpkg.SyscfgV

	lfeatures := cfg.FeaturesForLpkg(lpkg)
	for k, v := range features {
		if v {
			lfeatures[k] = true
		}
	}
	for k, _ := range lfeatures {
		if _, ok := features[k]; ok == false {
			delete(lfeatures, k)
		}
	}

	settings := newtutil.GetStringMapFeatures(v, lfeatures, "syscfg.defs")
	if settings != nil {
		for k, v := range settings {
			vals := v.(map[interface{}]interface{})
			entry, err := readSetting(k, lpkg, vals)
			if err != nil {
				return util.FmtNewtError("Config for package %s: %s",
					lpkg.Name(), err.Error())
			}

			if oldEntry, exists := cfg.Settings[k]; exists {
				// Setting already defined.  Allow this only if the setting is
				// injected, in which case the injected value takes precedence.
				point := mostRecentPoint(oldEntry)
				if !point.IsInjected() {
					// XXX: Better error message.
					return util.FmtNewtError("setting %s redefined", k)
				}

				entry.History = append(entry.History, oldEntry.History...)
				entry.Value = oldEntry.Value
			}
			cfg.Settings[k] = entry
		}
	}

	return nil
}

func (cfg *Cfg) readValsOnce(lpkg *pkg.LocalPackage,
	features map[string]bool) error {
	v := lpkg.SyscfgV

	lfeatures := cfg.FeaturesForLpkg(lpkg)
	for k, v := range features {
		if v {
			lfeatures[k] = true
		}
	}
	for k, _ := range lfeatures {
		if _, ok := features[k]; ok == false {
			delete(lfeatures, k)
		}
	}

	values := newtutil.GetStringMapFeatures(v, lfeatures, "syscfg.vals")
	for k, v := range values {
		entry, ok := cfg.Settings[k]
		if ok {
			sourcetype := normalizePkgType(lpkg.Type())
			deftype := normalizePkgType(entry.PackageDef.Type())

			// Overrides must come from a higher priority package, with one
			// exception: a package can override its own setting.
			if lpkg != entry.PackageDef && sourcetype <= deftype {
				priority := CfgPriority{
					PackageDef:  entry.PackageDef,
					PackageSrc:  lpkg,
					SettingName: k,
				}
				cfg.PriorityViolations = append(cfg.PriorityViolations, priority)
			} else {
				entry.appendValue(lpkg, v)
				cfg.Settings[k] = entry
			}
		} else {
			orphan := CfgPoint{
				Value:  stringValue(v),
				Source: lpkg,
			}
			cfg.Orphans[k] = append(cfg.Orphans[k], orphan)
		}
	}

	return nil
}

func (cfg *Cfg) Log() {
	keys := make([]string, len(cfg.Settings))
	i := 0
	for k, _ := range cfg.Settings {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	log.Debugf("syscfg settings (%d entries):", len(cfg.Settings))
	for _, k := range keys {
		entry := cfg.Settings[k]

		log.Debugf("    %s=%s %s", k, entry.Value,
			historyToString(entry.History))
	}
}

func (cfg *Cfg) settingsOfType(typ CfgSettingType) []CfgEntry {
	entries := []CfgEntry{}

	for _, entry := range cfg.Settings {
		if entry.SettingType == typ {
			entries = append(entries, entry)
		}
	}

	return entries
}

func (cfg *Cfg) detectViolations() {
	for _, entry := range cfg.Settings {
		var ev []CfgRestriction
		for _, r := range entry.Restrictions {
			if !cfg.restrictionMet(r) {
				ev = append(ev, r)
			}
		}

		if ev != nil {
			cfg.Violations[entry.Name] = ev
		}
	}
}

func (cfg *Cfg) detectFlashConflicts(flashMap flash.FlashMap) {
	entries := cfg.settingsOfType(CFG_SETTING_TYPE_FLASH_OWNER)

	areaEntryMap := map[string][]CfgEntry{}

	for _, entry := range entries {
		if entry.Value != "" {
			area, ok := flashMap.Areas[entry.Value]
			if !ok {
				conflict := CfgFlashConflict{
					SettingNames: []string{entry.Name},
					Code:         CFG_FLASH_CONFLICT_CODE_BAD_NAME,
				}
				cfg.FlashConflicts = append(cfg.FlashConflicts, conflict)
			} else {
				areaEntryMap[area.Name] =
					append(areaEntryMap[area.Name], entry)
			}
		}
	}

	// Settings with type flash_owner must have unique values.
	for _, entries := range areaEntryMap {
		if len(entries) > 1 {
			conflict := CfgFlashConflict{
				SettingNames: []string{},
				Code:         CFG_FLASH_CONFLICT_CODE_NOT_UNIQUE,
			}

			for _, entry := range entries {
				conflict.SettingNames =
					append(conflict.SettingNames, entry.Name)
			}

			cfg.FlashConflicts = append(cfg.FlashConflicts, conflict)
		}
	}
}

func (cfg *Cfg) flashConflictErrorText(conflict CfgFlashConflict) string {
	entry := cfg.Settings[conflict.SettingNames[0]]

	switch conflict.Code {
	case CFG_FLASH_CONFLICT_CODE_BAD_NAME:
		return fmt.Sprintf("Setting %s specifies unknown flash area: %s\n",
			entry.Name, entry.Value)

	case CFG_FLASH_CONFLICT_CODE_NOT_UNIQUE:
		return fmt.Sprintf(
			"Multiple flash_owner settings specify the same flash area\n"+
				"          settings: %s\n"+
				"        flash area: %s\n",
			strings.Join(conflict.SettingNames, ", "),
			entry.Value)

	default:
		panic("Invalid flash conflict code: " + string(conflict.Code))
	}
}

func historyTextOnce(settingName string, points []CfgPoint) string {
	return fmt.Sprintf("    %s: %s\n", settingName, historyToString(points))
}

func historyText(historyMap map[string][]CfgPoint) string {
	if len(historyMap) == 0 {
		return ""
	}

	str := "Setting history (newest -> oldest):\n"
	names := make([]string, 0, len(historyMap))
	for name, _ := range historyMap {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		points := historyMap[name]
		str += historyTextOnce(name, points)
	}

	return str
}

func (cfg *Cfg) ErrorText() string {
	str := ""

	historyMap := map[string][]CfgPoint{}

	if len(cfg.Violations) > 0 {
		str += "Syscfg restriction violations detected:\n"
		for settingName, rslice := range cfg.Violations {
			baseEntry := cfg.Settings[settingName]
			historyMap[settingName] = baseEntry.History
			for _, r := range rslice {
				for _, name := range r.relevantSettingNames() {
					reqEntry := cfg.Settings[name]
					historyMap[name] = reqEntry.History
				}
				str += "    " + cfg.violationText(baseEntry, r) + "\n"
			}
		}
	}

	if len(cfg.Ambiguities) > 0 {
		str += "Syscfg ambiguities detected:\n"

		settingNames := make([]string, 0, len(cfg.Ambiguities))
		for k, _ := range cfg.Ambiguities {
			settingNames = append(settingNames, k)
		}
		sort.Strings(settingNames)

		for _, name := range settingNames {
			entry := cfg.Settings[name]
			historyMap[entry.Name] = entry.History
			str += "    " + entry.ambiguityText()
		}
	}

	if len(cfg.PriorityViolations) > 0 {
		str += "Priority violations detected (Packages can only override " +
			"settings defined by packages of lower priority):\n"
		for _, priority := range cfg.PriorityViolations {
			entry := cfg.Settings[priority.SettingName]
			historyMap[priority.SettingName] = entry.History

			str += fmt.Sprintf("    Package: %s overriding setting: %s defined by %s\n",
				priority.PackageSrc.Name(), priority.SettingName, priority.PackageDef.Name())
		}
	}

	if len(cfg.FlashConflicts) > 0 {
		str += "Flash errors detected:\n"
		for _, conflict := range cfg.FlashConflicts {
			for _, name := range conflict.SettingNames {
				entry := cfg.Settings[name]
				historyMap[name] = entry.History
			}

			str += "    " + cfg.flashConflictErrorText(conflict)
		}
	}

	if str == "" {
		return ""
	}

	str += "\n" + historyText(historyMap)

	return str
}

func (cfg *Cfg) WarningText() string {
	str := ""

	historyMap := map[string][]CfgPoint{}

	if len(cfg.Orphans) > 0 {
		settingNames := make([]string, len(cfg.Orphans))
		i := 0
		for k, _ := range cfg.Orphans {
			settingNames[i] = k
			i++
		}
		sort.Strings(settingNames)

		str += "Ignoring override of undefined settings:"
		for _, n := range settingNames {
			historyMap[n] = cfg.Orphans[n]
			str += fmt.Sprintf("\n    %s", n)
		}
	}

	if str == "" {
		return ""
	}

	str += "\n" + historyText(historyMap)

	return str
}

func escapeStr(s string) string {
	return strings.ToUpper(util.CIdentifier(s))
}

func settingName(setting string) string {
	return SYSCFG_PREFIX_SETTING + escapeStr(setting)
}

func normalizePkgType(typ interfaces.PackageType) interfaces.PackageType {
	switch typ {
	case pkg.PACKAGE_TYPE_TARGET:
		return pkg.PACKAGE_TYPE_TARGET
	case pkg.PACKAGE_TYPE_APP:
		return pkg.PACKAGE_TYPE_APP
	case pkg.PACKAGE_TYPE_UNITTEST:
		return pkg.PACKAGE_TYPE_UNITTEST
	case pkg.PACKAGE_TYPE_BSP:
		return pkg.PACKAGE_TYPE_BSP
	default:
		return pkg.PACKAGE_TYPE_LIB
	}
}

func categorizePkgs(
	lpkgs []*pkg.LocalPackage) map[interfaces.PackageType][]*pkg.LocalPackage {

	pmap := map[interfaces.PackageType][]*pkg.LocalPackage{
		pkg.PACKAGE_TYPE_TARGET:   []*pkg.LocalPackage{},
		pkg.PACKAGE_TYPE_APP:      []*pkg.LocalPackage{},
		pkg.PACKAGE_TYPE_UNITTEST: []*pkg.LocalPackage{},
		pkg.PACKAGE_TYPE_BSP:      []*pkg.LocalPackage{},
		pkg.PACKAGE_TYPE_LIB:      []*pkg.LocalPackage{},
	}

	for _, lpkg := range lpkgs {
		typ := normalizePkgType(lpkg.Type())
		pmap[typ] = append(pmap[typ], lpkg)
	}

	for k, v := range pmap {
		pmap[k] = pkg.SortLclPkgs(v)
	}

	return pmap
}

func (cfg *Cfg) readDefsForPkgType(lpkgs []*pkg.LocalPackage,
	features map[string]bool) error {

	for _, lpkg := range lpkgs {
		if err := cfg.readDefsOnce(lpkg, features); err != nil {
			return err
		}
	}
	return nil
}
func (cfg *Cfg) readValsForPkgType(lpkgs []*pkg.LocalPackage,
	features map[string]bool) error {

	for _, lpkg := range lpkgs {
		if err := cfg.readValsOnce(lpkg, features); err != nil {
			return err
		}
	}

	return nil
}

func (cfg *Cfg) detectAmbiguities() {
	for _, entry := range cfg.Settings {
		if points := entry.ambiguities(); len(points) > 0 {
			cfg.Ambiguities[entry.Name] = points
		}
	}
}

func Read(lpkgs []*pkg.LocalPackage, apis []string,
	injectedSettings map[string]string, features map[string]bool,
	flashMap flash.FlashMap) (Cfg, error) {

	cfg := NewCfg()
	for k, v := range injectedSettings {
		cfg.Settings[k] = CfgEntry{
			Name:        k,
			Description: "Injected setting",
			Value:       v,
			History: []CfgPoint{{
				Value:  v,
				Source: nil,
			}},
		}

		if ValueIsTrue(v) {
			features[k] = true
		}
	}

	// Read system configuration files.  In case of conflicting settings, the
	// higher priority pacakge's setting wins.  Package priorities are assigned
	// as follows (highest priority first):
	//     * target
	//     * app (if present)
	//     * unittest (if no app)
	//     * bsp
	//     * everything else (lib, sdk, compiler)

	lpkgMap := categorizePkgs(lpkgs)

	for _, ptype := range []interfaces.PackageType{
		pkg.PACKAGE_TYPE_LIB,
		pkg.PACKAGE_TYPE_BSP,
		pkg.PACKAGE_TYPE_UNITTEST,
		pkg.PACKAGE_TYPE_APP,
		pkg.PACKAGE_TYPE_TARGET,
	} {
		if err := cfg.readDefsForPkgType(lpkgMap[ptype], features); err != nil {
			return cfg, err
		}
	}

	for _, ptype := range []interfaces.PackageType{
		pkg.PACKAGE_TYPE_LIB,
		pkg.PACKAGE_TYPE_BSP,
		pkg.PACKAGE_TYPE_UNITTEST,
		pkg.PACKAGE_TYPE_APP,
		pkg.PACKAGE_TYPE_TARGET,
	} {
		if err := cfg.readValsForPkgType(lpkgMap[ptype], features); err != nil {
			return cfg, err
		}
	}

	cfg.detectAmbiguities()
	cfg.detectViolations()
	cfg.detectFlashConflicts(flashMap)

	return cfg, nil
}

func mostRecentPoint(entry CfgEntry) CfgPoint {
	if len(entry.History) == 0 {
		panic("invalid cfg entry; len(history) == 0")
	}

	return entry.History[len(entry.History)-1]
}

func calcPriorities(cfg Cfg, settingType CfgSettingType, max int,
	allowDups bool) error {

	// setting-name => entry
	anyEntries := map[string]CfgEntry{}

	// priority-value => entry
	valEntries := map[int]CfgEntry{}

	for name, entry := range cfg.Settings {
		if entry.SettingType == settingType {
			if entry.Value == SYSCFG_PRIO_ANY {
				anyEntries[name] = entry
			} else {
				prio, err := util.AtoiNoOct(entry.Value)
				if err != nil {
					return util.FmtNewtError(
						"invalid priority value: setting=%s value=%s pkg=%s",
						name, entry.Value, entry.History[0].Name())
				}

				if prio > max {
					return util.FmtNewtError(
						"invalid priority value: value too great (> %d); "+
							"setting=%s value=%s pkg=%s",
						max, entry.Name, entry.Value,
						mostRecentPoint(entry).Name())
				}

				if !allowDups {
					if oldEntry, ok := valEntries[prio]; ok {
						return util.FmtNewtError(
							"duplicate priority value: setting1=%s "+
								"setting2=%s pkg1=%s pkg2=%s value=%s",
							oldEntry.Name, entry.Name, entry.Value,
							oldEntry.History[0].Name(),
							entry.History[0].Name())
					}
				}

				valEntries[prio] = entry
			}
		}
	}

	greatest := 0
	for prio, _ := range valEntries {
		if prio > greatest {
			greatest = prio
		}
	}

	anyNames := make([]string, 0, len(anyEntries))
	for name, _ := range anyEntries {
		anyNames = append(anyNames, name)
	}
	sort.Strings(anyNames)

	for _, name := range anyNames {
		entry := anyEntries[name]

		greatest++
		if greatest > max {
			return util.FmtNewtError("could not assign 'any' priority: "+
				"value too great (> %d); setting=%s value=%s pkg=%s",
				max, name, greatest,
				mostRecentPoint(entry).Name())
		}

		entry.Value = strconv.Itoa(greatest)
		cfg.Settings[name] = entry
	}

	return nil
}

func writeCheckMacros(w io.Writer) {
	s := `/**
 * This macro exists to ensure code includes this header when needed.  If code
 * checks the existence of a setting directly via ifdef without including this
 * header, the setting macro will silently evaluate to 0.  In contrast, an
 * attempt to use these macros without including this header will result in a
 * compiler error.
 */
#define MYNEWT_VAL(x)                           MYNEWT_VAL_ ## x
`
	fmt.Fprintf(w, "%s\n", s)
}

func writeComment(entry CfgEntry, w io.Writer) {
	if len(entry.History) > 1 {
		fmt.Fprintf(w, "/* Overridden by %s (defined by %s) */\n",
			mostRecentPoint(entry).Name(),
			entry.History[0].Name())
	}
}

func writeDefine(key string, value string, w io.Writer) {
	if value == "" {
		fmt.Fprintf(w, "#undef %s\n", key)
	} else {
		fmt.Fprintf(w, "#ifndef %s\n", key)
		fmt.Fprintf(w, "#define %s (%s)\n", key, value)
		fmt.Fprintf(w, "#endif\n")
	}
}

func EntriesByPkg(cfg Cfg) map[string][]CfgEntry {
	pkgEntries := map[string][]CfgEntry{}
	for _, v := range cfg.Settings {
		name := v.History[0].Name()
		pkgEntries[name] = append(pkgEntries[name], v)
	}
	return pkgEntries
}

func writeSettingsOnePkg(cfg Cfg, pkgName string, pkgEntries []CfgEntry,
	w io.Writer) {

	names := make([]string, len(pkgEntries), len(pkgEntries))
	for i, entry := range pkgEntries {
		names[i] = entry.Name
	}

	if len(names) == 0 {
		return
	}
	sort.Strings(names)

	fmt.Fprintf(w, "/*** %s */\n", pkgName)

	first := true
	for _, n := range names {
		entry := cfg.Settings[n]
		if first {
			first = false
		} else {
			fmt.Fprintf(w, "\n")
		}

		writeComment(entry, w)
		writeDefine(settingName(n), entry.Value, w)
	}
}

func writeSettings(cfg Cfg, w io.Writer) {
	// Group settings by package name so that the generated header file is
	// easier to read.
	pkgEntries := EntriesByPkg(cfg)

	pkgNames := make([]string, 0, len(pkgEntries))
	for name, _ := range pkgEntries {
		pkgNames = append(pkgNames, name)
	}
	sort.Strings(pkgNames)

	for _, name := range pkgNames {
		fmt.Fprintf(w, "\n")
		entries := pkgEntries[name]
		writeSettingsOnePkg(cfg, name, entries, w)
	}
}

func write(cfg Cfg, w io.Writer) {
	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	fmt.Fprintf(w, "#ifndef H_MYNEWT_SYSCFG_\n")
	fmt.Fprintf(w, "#define H_MYNEWT_SYSCFG_\n\n")

	writeCheckMacros(w)
	fmt.Fprintf(w, "\n")

	writeSettings(cfg, w)
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "#endif\n")
}

func EnsureWritten(cfg Cfg, includeDir string) error {
	// XXX: Detect these problems at error text generation time.
	if err := calcPriorities(cfg, CFG_SETTING_TYPE_TASK_PRIO,
		SYSCFG_TASK_PRIO_MAX, false); err != nil {

		return err
	}

	buf := bytes.Buffer{}
	write(cfg, &buf)

	path := includeDir + "/" + HEADER_PATH

	writeReqd, err := util.FileContentsChanged(path, buf.Bytes())
	if err != nil {
		return err
	}
	if !writeReqd {
		log.Debugf("syscfg unchanged; not writing header file (%s).", path)
		return nil
	}

	log.Debugf("syscfg changed; writing header file (%s).", path)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return util.NewNewtError(err.Error())
	}

	if err := ioutil.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}

func KeyValueFromStr(str string) (map[string]string, error) {
	vals := map[string]string{}

	if strings.TrimSpace(str) == "" {
		return vals, nil
	}

	// Separate syscfg vals are delimited by ':'.
	fields := strings.Split(str, ":")

	// Key-value pairs are delimited by '='.  If no '=' is present, assume the
	// string is the key name and the value is 1.
	for _, f := range fields {
		if _, err := util.AtoiNoOct(f); err == nil {
			return nil, util.FmtNewtError(
				"Invalid setting name \"%s\"; must not be a number", f)
		}

		kv := strings.SplitN(f, "=", 2)
		switch len(kv) {
		case 1:
			vals[f] = "1"
		case 2:
			vals[kv[0]] = kv[1]
		}
	}

	return vals, nil
}

func KeyValueToStr(syscfgKv map[string]string) string {
	str := ""

	names := make([]string, 0, len(syscfgKv))
	for k, _ := range syscfgKv {
		names = append(names, k)
	}
	sort.Strings(names)

	for i, name := range names {
		if i != 0 {
			str += ":"
		}

		str += fmt.Sprintf("%s=%s", name, syscfgKv[name])
	}

	return str
}
