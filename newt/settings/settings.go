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
	"os/user"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/ycfg"
)

const NEWTRC_DIR string = ".newt"
const REPOS_FILENAME string = "repos.yml"

// Contains general newt settings read from $HOME/.newt
var newtrc ycfg.YCfg

func readNewtrc() ycfg.YCfg {
	usr, err := user.Current()
	if err != nil {
		return ycfg.YCfg{}
	}

	dir := usr.HomeDir + "/" + NEWTRC_DIR
	yc, err := newtutil.ReadConfig(dir,
		strings.TrimSuffix(REPOS_FILENAME, ".yml"))
	if err != nil {
		log.Debugf("Failed to read %s/%s file", dir, REPOS_FILENAME)
		return ycfg.YCfg{}
	}

	return yc
}

func Newtrc() ycfg.YCfg {
	if newtrc != nil {
		return newtrc
	}

	newtrc = readNewtrc()
	return newtrc
}
