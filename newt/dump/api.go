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

import "mynewt.apache.org/newt/newt/resolve"

func newApiMap(res *resolve.Resolution) map[string]string {
	m := make(map[string]string, len(res.ApiMap))
	for api, rpkg := range res.ApiMap {
		m[api] = rpkg.Lpkg.FullName()
	}

	return m
}

func newUnsatisfiedApis(res *resolve.Resolution) map[string][]string {
	m := make(map[string][]string, len(res.UnsatisfiedApis))
	for api, rpkgs := range res.UnsatisfiedApis {
		slice := make([]string, len(rpkgs))
		for i, rpkg := range rpkgs {
			slice[i] = rpkg.Lpkg.FullName()
		}
		m[api] = slice
	}

	return m
}

func newApiConflicts(res *resolve.Resolution) map[string][]string {
	m := make(map[string][]string, len(res.ApiConflicts))
	for _, c := range res.ApiConflicts {
		slice := make([]string, len(c.Pkgs))
		for i, rpkg := range c.Pkgs {
			slice[i] = rpkg.Lpkg.FullName()
		}
		m[c.Api] = slice
	}

	return m
}
