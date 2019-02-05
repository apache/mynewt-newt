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

package dump

import (
	"strconv"

	"mynewt.apache.org/newt/newt/logcfg"
	"mynewt.apache.org/newt/util"
)

type Log struct {
	Package string `json:"package"`
	Module  int    `json:"module"`
	Level   int    `json:"level"`
}

type Logcfg struct {
	Logs map[string]Log `json:"logs"`
	// XXX: InvalidSettings
	// XXX: ModuleConflicts
}

func newLogcfg(lcfg logcfg.LCfg) (Logcfg, error) {
	lm := make(map[string]Log, len(lcfg.Logs))
	for _, llog := range lcfg.Logs {
		mod, err := strconv.Atoi(llog.Module.Value)
		if err != nil {
			return Logcfg{}, util.ChildNewtError(err)
		}

		level, err := strconv.Atoi(llog.Level.Value)
		if err != nil {
			return Logcfg{}, util.ChildNewtError(err)
		}

		lm[llog.Name] = Log{
			Package: llog.Source.FullName(),
			Module:  mod,
			Level:   level,
		}
	}

	return Logcfg{
		Logs: lm,
	}, nil
}
