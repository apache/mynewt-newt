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

package toolchain

import (
	"sort"
	"strings"

	"mynewt.apache.org/newt/util"
)

// Options which take their argument as a separate command line argument
// (e.g., "-include file.h").
var flagsWithArg = map[string]bool{
	"--param":            true,
	"-D":                 true,
	"-I":                 true,
	"-L":                 true,
	"-MF":                true,
	"-MQ":                true,
	"-MT":                true,
	"-T":                 true,
	"-U":                 true,
	"-Xassembler":        true,
	"-Xlinker":           true,
	"-Xpreprocessor":     true,
	"-idirafter":         true,
	"-imacros":           true,
	"-imultilib":         true,
	"-include":           true,
	"-iprefix":           true,
	"-iquote":            true,
	"-isysroot":          true,
	"-isystem":           true,
	"-iwithprefix":       true,
	"-iwithprefixbefore": true,
	"-o":                 true,
	"-u":                 true,
	"-x":                 true,
	"-z":                 true,
}

// flagTakesArg indicates whether flags[i] is an option followed by its
// separate argument.
func flagTakesArg(flags []string, i int) bool {
	return flagsWithArg[flags[i]] && i+1 < len(flags)
}

// SortFlags splits the specified strings on white space, sorts the resulting
// flags and removes duplicates.  Options taking a separate argument are kept
// together with their argument; these pairs are placed after all other flags
// in their original order and are not deduplicated, because their order and
// count may be significant (e.g., "-include a.h -include b.h" or
// "-Xlinker -z -Xlinker relro").
func SortFlags(flags ...string) []string {
	tokens := []string{}
	for _, f := range flags {
		tokens = append(tokens, strings.Fields(f)...)
	}

	singles := []string{}
	pairs := []string{}
	for i := 0; i < len(tokens); i++ {
		if flagTakesArg(tokens, i) {
			pairs = append(pairs, tokens[i], tokens[i+1])
			i++
		} else {
			singles = append(singles, tokens[i])
		}
	}

	singles = util.UniqueStrings(singles)
	sort.Strings(singles)

	return append(singles, pairs...)
}

// FlagMacro returns the name of the macro defined or undefined by the
// specified flag (e.g., "-DFOO=1" or "-UFOO"), or "" if the flag is not -D or
// -U.
func FlagMacro(flag string) string {
	if !strings.HasPrefix(flag, "-D") && !strings.HasPrefix(flag, "-U") {
		return ""
	}

	name := flag[2:]
	if i := strings.IndexByte(name, '='); i >= 0 {
		name = name[:i]
	}
	return name
}

// flagUnit is a single flag or an option with its separate argument.
type flagUnit []string

// splitFlags splits the specified strings on white space into flag units.
func splitFlags(flags []string) []flagUnit {
	tokens := []string{}
	for _, f := range flags {
		tokens = append(tokens, strings.Fields(f)...)
	}

	units := []flagUnit{}
	for i := 0; i < len(tokens); i++ {
		if flagTakesArg(tokens, i) {
			units = append(units, flagUnit(tokens[i:i+2]))
			i++
		} else {
			units = append(units, flagUnit(tokens[i:i+1]))
		}
	}
	return units
}

// MacroFlags returns the definitions and undefinitions of macros (-D, -U)
// in the specified flags, in their order.  Options with a separate argument
// are joined with it (e.g., "-D FOO=1" becomes "-DFOO=1").
func MacroFlags(flags ...string) []string {
	macros := []string{}
	for _, u := range splitFlags(flags) {
		if f := strings.Join(u, ""); FlagMacro(f) != "" {
			macros = append(macros, f)
		}
	}
	return macros
}

// MergeFlags splits the specified strings on white space and returns the
// resulting flags in their original order.  Flags given later have higher
// priority, as with the compiler, which uses the last of conflicting options
// (e.g., -O2 -Os, -fno-common -fcommon).  Only flags which would be redundant
// or would make the compiler complain are removed:
//   - an option is removed if it is repeated later (e.g., "-Wall -Wextra
//     -Wall" becomes "-Wextra -Wall"),
//   - a definition or undefinition of a macro (-D, -U) is removed if the same
//     macro is defined or undefined later, because redefining a macro with a
//     different value causes a warning.
//
// Other options taking a separate argument (e.g., "-include a.h") are kept
// together with their argument and are never removed, because their order
// and count may be significant.
func MergeFlags(flags ...string) []string {
	units := splitFlags(flags)

	// A unit is removed if a later unit has the same key.  Units without a
	// key are always kept.
	keys := make([]string, len(units))
	for i, u := range units {
		if m := FlagMacro(strings.Join(u, "")); m != "" {
			keys[i] = "macro:" + m
		} else if len(u) == 1 {
			keys[i] = "flag:" + u[0]
		}
	}

	seen := map[string]bool{}
	keep := make([]bool, len(units))
	for i := len(units) - 1; i >= 0; i-- {
		if keys[i] == "" || !seen[keys[i]] {
			keep[i] = true
			seen[keys[i]] = true
		}
	}

	merged := []string{}
	for i, u := range units {
		if keep[i] {
			merged = append(merged, u...)
		}
	}
	return merged
}
