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

package syscfg

import (
	"encoding/json"

	"mynewt.apache.org/newt/util"
)

var cfgSettingNameTypeMap = map[string]CfgSettingType{
	"raw":           CFG_SETTING_TYPE_RAW,
	"task_priority": CFG_SETTING_TYPE_TASK_PRIO,
	"flash_owner":   CFG_SETTING_TYPE_FLASH_OWNER,
}

var cfgSettingNameStateMap = map[string]CfgSettingState{
	"good":       CFG_SETTING_STATE_GOOD,
	"deprecated": CFG_SETTING_STATE_DEPRECATED,
	"defunct":    CFG_SETTING_STATE_DEFUNCT,
}

var cfgFlashConflictNameCodeMap = map[string]CfgFlashConflictCode{
	"bad_name":   CFG_FLASH_CONFLICT_CODE_BAD_NAME,
	"not_unique": CFG_FLASH_CONFLICT_CODE_NOT_UNIQUE,
}

func (t CfgSettingType) String() string {
	for k, v := range cfgSettingNameTypeMap {
		if v == t {
			return k
		}
	}
	return "???"
}

func CfgSettingTypeFromString(s string) (CfgSettingType, error) {
	if t, ok := cfgSettingNameTypeMap[s]; ok {
		return t, nil
	}

	return 0, util.FmtNewtError("cannot parse syscfg setting type: \"%s\"", s)
}

func (t CfgSettingType) MarshalJSON() ([]byte, error) {
	s := t.String()
	j, err := json.Marshal(s)
	if err != nil {
		return nil, util.ChildNewtError(err)
	}

	return j, nil
}

func (t *CfgSettingType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return util.ChildNewtError(err)
	}

	x, err := CfgSettingTypeFromString(s)
	if err != nil {
		return err
	}

	*t = x
	return nil
}

func (t CfgSettingState) String() string {
	for k, v := range cfgSettingNameStateMap {
		if v == t {
			return k
		}
	}
	return "???"
}

func CfgSettingStateFromString(s string) (CfgSettingState, error) {
	if t, ok := cfgSettingNameStateMap[s]; ok {
		return t, nil
	}

	return 0, util.FmtNewtError("cannot parse syscfg setting state: \"%s\"", s)
}

func (t CfgSettingState) MarshalJSON() ([]byte, error) {
	s := t.String()
	j, err := json.Marshal(s)
	if err != nil {
		return nil, util.ChildNewtError(err)
	}

	return j, nil
}

func (t *CfgSettingState) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return util.ChildNewtError(err)
	}

	x, err := CfgSettingStateFromString(s)
	if err != nil {
		return err
	}

	*t = x
	return nil
}

func (t CfgFlashConflictCode) String() string {
	for k, v := range cfgFlashConflictNameCodeMap {
		if v == t {
			return k
		}
	}
	return "???"
}

func CfgFlashConflictCodeFromString(s string) (CfgFlashConflictCode, error) {
	if t, ok := cfgFlashConflictNameCodeMap[s]; ok {
		return t, nil
	}

	return 0, util.FmtNewtError("cannot parse flash conflict code : \"%s\"", s)
}

func (t CfgFlashConflictCode) MarshalJSON() ([]byte, error) {
	s := t.String()
	j, err := json.Marshal(s)
	if err != nil {
		return nil, util.ChildNewtError(err)
	}

	return j, nil
}

func (t *CfgFlashConflictCode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return util.ChildNewtError(err)
	}

	x, err := CfgFlashConflictCodeFromString(s)
	if err != nil {
		return err
	}

	*t = x
	return nil
}
