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
	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/newt/flashmap"
)

type FlashMap struct {
	Areas       map[string]flash.FlashArea `json:"areas"`
	Overlaps    [][]string                 `json:"overlaps"`
	IdConflicts [][]string                 `json:"id_conflicts"`
}

func convFlashArea2Slice(fa2s [][]flash.FlashArea) [][]string {
	outer := make([][]string, len(fa2s))

	for i, fas := range fa2s {
		inner := make([]string, len(fas))
		for j, fa := range fas {
			inner[j] = fa.Name
		}
		outer[i] = inner
	}

	return outer
}

func newFlashMap(fm flashmap.FlashMap) FlashMap {
	return FlashMap{
		Areas:       fm.Areas,
		Overlaps:    convFlashArea2Slice(fm.Overlaps),
		IdConflicts: convFlashArea2Slice(fm.IdConflicts),
	}
}
