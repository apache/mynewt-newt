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

	"mynewt.apache.org/newt/newt/extcmd"
	"mynewt.apache.org/newt/util"
)

type ExtCmdFunc struct {
	Cmd     string `json:"cmd"`
	Stage   int    `json:"stage"`
	PkgName string `json:"package"`
}

type ExtCmd struct {
	Cmds []ExtCmdFunc `json:"cmds"`
}

func newExtCmd(scfg extcmd.ExtCmdCfg) (ExtCmd, error) {
	cmds := make([]ExtCmdFunc, len(scfg.StageFuncs))
	for i, f := range scfg.StageFuncs {
		stage, err := strconv.Atoi(f.Stage.Value)
		if err != nil {
			return ExtCmd{}, util.ChildNewtError(err)
		}
		cmds[i] = ExtCmdFunc{
			Cmd:     f.Name,
			Stage:   stage,
			PkgName: f.Pkg.FullName(),
		}
	}

	return ExtCmd{
		Cmds: cmds,
	}, nil
}
