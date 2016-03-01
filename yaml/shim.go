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

package yaml

import (
	"errors"
)

// This function just wraps DecodeStream with the name "Unmarshal."  This
// allows this library to be used in place of other YAML decoding libraries.
func Unmarshal(src []byte, dst interface{}) error {
	// The destination type must be a string->interface{} map or a pointer to
	// one.  If the destination is something else, return an error.
	mapping, ok := dst.(map[string]interface{})
	if !ok {
		mPtr, ok := dst.(*map[string]interface{})
		if !ok {
			return errors.New("invalid dst type; expected " +
				"map[string]interface{} or *map[string]interface{}")
		}
		mapping = *mPtr
	}

	return DecodeStream(src, mapping)
}
