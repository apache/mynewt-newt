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

package yaml

import (
	"fmt"
	"sort"
	"strings"
)

func EscapeString(s string) string {
	special := ":{}[],&*#?|-<>=!%@\\\"'"
	if strings.ContainsAny(s, special) {
		return "\"" + strings.Replace(s, "\"", "\\\"", -1) + "\""
	} else {
		return s
	}
}

// Converts a key-value pair to a YAML string.
func KvToYaml(key string, val interface{}, indent int) string {
	s := ""

	if key != "" {
		s += fmt.Sprintf("%*s%s:", indent, "", EscapeString(key))
	}

	switch v := val.(type) {
	case []interface{}:
		s += "\n"
		for _, elem := range v {
			s += fmt.Sprintf("%*s- %s\n", indent+4, "", KvToYaml("", elem, 0))
		}

	case map[interface{}]interface{}:
		s += "\n"

		subKeys := make([]string, 0, len(v))
		for sk, _ := range v {
			subKeys = append(subKeys, sk.(string))
		}
		sort.Strings(subKeys)

		for _, sk := range subKeys {
			s += KvToYaml(sk, v[sk], indent+4)
		}

	default:
		valStr := EscapeString(fmt.Sprintf("%v", v))
		s += fmt.Sprintf(" %v\n", valStr)
	}

	return s
}

func MapToYaml(m map[string]interface{}) string {
	keys := make([]string, 0, len(m))
	for k, _ := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	s := ""
	for _, k := range keys {
		s += KvToYaml(k, m[k], 0)
	}

	return s
}
