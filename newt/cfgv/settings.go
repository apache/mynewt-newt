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

package cfgv

type Settings struct {
	settings map[string]string
}

func NewSettings(settings *Settings) *Settings {
	var m map[string]string

	if settings != nil {
		m = make(map[string]string, len(settings.settings))

		for k, v := range settings.settings {
			m[k] = v
		}
	} else {
		m = make(map[string]string)
	}

	return &Settings{settings: m}
}

func NewSettingsPrealloc(count int) *Settings {
	return &Settings{settings: make(map[string]string, count)}
}

func NewSettingsFromMap(init map[string]string) *Settings {
	m := make(map[string]string, len(init))

	for k, v := range init {
		m[k] = v
	}

	return &Settings{settings: m}
}

func (s *Settings) Get(name string) string {
	if s == nil {
		return ""
	}

	value, _ := s.settings[name]

	return value
}

func (s *Settings) GetOk(name string) (string, bool) {
	value, ok := s.settings[name]

	return value, ok
}

func (s *Settings) Set(name string, value string) {
	s.settings[name] = value
}

func (s *Settings) Count() int {
	return len(s.settings)
}

func (s *Settings) Exists(name string) bool {
	_, ok := s.settings[name]

	return ok
}

func (s *Settings) Names() []string {
	var ks []string

	for k := range s.settings {
		ks = append(ks, k)
	}

	return ks
}

func (s *Settings) ToMap() map[string]string {
	m := make(map[string]string, len(s.settings))

	for k, v := range s.settings {
		m[k] = v
	}

	return m
}
