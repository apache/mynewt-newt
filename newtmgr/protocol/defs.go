/*
 Copyright 2016 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package protocol

// First 64 groups are reserved for system level newtmgr commands.
// Per-user commands are then defined after group 64.

const (
	NMGR_GROUP_ID_DEFAULT = 0
	NMGR_GROUP_ID_IMAGE   = 1
	NMGR_GROUP_ID_STATS   = 2
	NMGR_GROUP_ID_CONFIG  = 3
	NMGR_GROUP_ID_LOGS    = 4
	NMGR_GROUP_ID_PERUSER = 64
)

// Ids for default group commands

const (
	NMGR_ID_ECHO           = 0
	NMGR_ID_CONS_ECHO_CTRL = 1
	NMGR_ID_TASKSTATS      = 2
	NMGR_ID_MPSTATS        = 3
	NMGR_ID_DATETIME_STR   = 4
	NMGR_ID_RESET          = 5
)
