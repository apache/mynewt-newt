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

package protocol

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"mynewt.apache.org/newt/util"
)

const (
	IMGMGR_NMGR_ID_STATE    = 0
	IMGMGR_NMGR_ID_UPLOAD   = 1
	IMGMGR_NMGR_ID_CORELIST = 3
	IMGMGR_NMGR_ID_CORELOAD = 4
	IMGMGR_NMGR_ID_ERASE	= 5
)

type SplitStatus int

const (
	NOT_APPLICABLE SplitStatus = iota
	NOT_MATCHING
	MATCHING
)

/* returns the enum as a string */
func (sm SplitStatus) String() string {
	names := map[SplitStatus]string{
		NOT_APPLICABLE: "N/A",
		NOT_MATCHING:   "non-matching",
		MATCHING:       "matching",
	}

	str := names[sm]
	if str == "" {
		return "Unknown!"
	}
	return str
}

func HashDecode(src string) (string, error) {
	imgHex, err := base64.StdEncoding.DecodeString(src)
	if err != nil {
		return "", util.NewNewtError(fmt.Sprintf("Hash decode error: %s",
			err.Error()))
	}
	return hex.EncodeToString(imgHex), nil
}

func HashEncode(src string) (string, error) {
	imgHex, err := hex.DecodeString(src)
	if err != nil {
		return "", util.NewNewtError(fmt.Sprintf("Hash encode error: %s",
			err.Error()))
	}
	return base64.StdEncoding.EncodeToString(imgHex), nil
}
