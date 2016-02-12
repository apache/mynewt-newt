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
	"fmt"
)

type BuiltPkg struct {
	Name    string
	Hash    string
	Version string
}

var BuiltPkgs []*BuiltPkg

func NewBuiltPkg(pkg *Pkg) (*BuiltPkg, error) {
	var verStr string
	hash, err := pkg.GetHash()
	if err != nil {
		return nil, NewNewtError(fmt.Sprintf("Unable to get hash for %s: %s",
			pkg.FullName, err.Error()))
	}
	if pkg.Version != nil {
		verStr = pkg.Version.String()
	} else {
		verStr = ""
	}

	builtpkg := &BuiltPkg{
		Name:    pkg.FullName,
		Hash:    hash,
		Version: verStr,
	}
	return builtpkg, nil
}
