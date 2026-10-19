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
