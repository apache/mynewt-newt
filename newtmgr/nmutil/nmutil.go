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

package nmutil

import (
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

var PacketTraceDir string
var traceFile *os.File

// @return                      true if the file can be used;
//                              false otherwise.
func ensureTraceFileOpen() bool {
	if traceFile != nil {
		return true
	}
	if PacketTraceDir == "" {
		return false
	}

	now := time.Now()
	secs := now.Unix()

	filename := fmt.Sprintf("%s/%d", PacketTraceDir, secs)

	var err error
	traceFile, err = os.Create(filename)
	if err != nil {
		return false
	}

	return true
}

func LogIncoming(bytes []byte) {
	if !ensureTraceFileOpen() {
		return
	}

	fmt.Fprintf(traceFile, "Incoming:\n%s\n", hex.Dump(bytes))
}

func LogOutgoing(bytes []byte) {
	if !ensureTraceFileOpen() {
		return
	}

	fmt.Fprintf(traceFile, "Outgoing:\n%s\n", hex.Dump(bytes))
}

func LogMessage(msg string) {
	fmt.Fprintf(traceFile, "Message: %s\n", msg)
}
