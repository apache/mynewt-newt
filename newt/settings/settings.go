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

package settings

import (
	"fmt"
	"os/user"
	"strconv"

	log "github.com/sirupsen/logrus"

	"mynewt.apache.org/newt/newt/config"
	"mynewt.apache.org/newt/newt/ycfg"
	"mynewt.apache.org/newt/util"
)

const NEWTRC_DIR string = ".newt"
const REPOS_FILENAME string = "repos.yml"
const NEWTRC_FILENAME string = "newtrc.yml"

// Contains general newt settings read from $HOME/.newt
var newtrc *ycfg.YCfg

func processNewtrc(yc ycfg.YCfg) {
	s := yc.GetValString("escape_shell", nil)
	if s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			log.Warnf(".newtrc contains invalid \"escape_shell\" value: %s; "+
				"expected \"true\" or \"false\"", s)
		} else {
			util.EscapeShellCmds = b
		}
	}
}

func readNewtrc() ycfg.YCfg {
	usr, err := user.Current()
	if err != nil {
		return ycfg.YCfg{}
	}

	yc := ycfg.NewYCfg("newtrc")
	for _, filename := range []string{NEWTRC_FILENAME, REPOS_FILENAME} {
		path := fmt.Sprintf("%s/%s/%s", usr.HomeDir, NEWTRC_DIR, filename)
		sub, err := config.ReadFile(path)
		if err != nil && !util.IsNotExist(err) {
			log.Warnf("Failed to read %s file", path)
			return ycfg.YCfg{}
		}

		fi := util.FileInfo{
			Path:   path,
			Parent: nil,
		}
		for k, v := range sub.AllSettings() {
			if err := yc.MergeFromFile(k, v, &fi); err != nil {
				log.Warnf("Failed to read %s file: %s", path, err.Error())
				return ycfg.YCfg{}
			}
		}
	}

	processNewtrc(yc)

	return yc
}

func Newtrc() ycfg.YCfg {
	if newtrc != nil {
		return *newtrc
	}

	yc := readNewtrc()
	newtrc = &yc
	return yc
}
