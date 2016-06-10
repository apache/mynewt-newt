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

package cli

import (
	"mynewt.apache.org/newt/newtmgr/config"
	"mynewt.apache.org/newt/newtmgr/protocol"
	"mynewt.apache.org/newt/newtmgr/transport"
)

func getTargetCmdRunner() (*protocol.CmdRunner, error) {
	cpm, err := config.NewConnProfileMgr()
	if err != nil {
		return nil, err
	}

	profile, err := cpm.GetConnProfile(ConnProfileName)
	if err != nil {
		return nil, err
	}

	conn, err := transport.NewConn(profile)
	if err != nil {
		return nil, err
	}

	runner, err := protocol.NewCmdRunner(conn)
	if err != nil {
		return nil, err
	}
	return runner, nil
}
