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

package builder

import (
	"bytes"
	"sort"
	"strings"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/newt/resolve"
)

func TestTargetName(testPkgName string) string {
	return strings.Replace(testPkgName, "/", "_", -1)
}

func (b *Builder) FeatureString() string {
	var buffer bytes.Buffer

	settingMap := b.cfg.SettingValues()
	featureSlice := make([]string, 0, len(settingMap))
	for k, v := range settingMap {
		if parse.ValueIsTrue(v) {
			featureSlice = append(featureSlice, k)
		}
	}
	sort.Strings(featureSlice)

	for i, feature := range featureSlice {
		if i != 0 {
			buffer.WriteString(" ")
		}

		buffer.WriteString(feature)
	}
	return buffer.String()
}

type bpkgSorter struct {
	bpkgs []*BuildPackage
}

func (b bpkgSorter) Len() int {
	return len(b.bpkgs)
}
func (b bpkgSorter) Swap(i, j int) {
	b.bpkgs[i], b.bpkgs[j] = b.bpkgs[j], b.bpkgs[i]
}
func (b bpkgSorter) Less(i, j int) bool {
	return b.bpkgs[i].rpkg.Lpkg.Name() < b.bpkgs[j].rpkg.Lpkg.Name()
}

func (b *Builder) sortedBuildPackages() []*BuildPackage {
	sorter := bpkgSorter{
		bpkgs: make([]*BuildPackage, 0, len(b.PkgMap)),
	}

	for _, bpkg := range b.PkgMap {
		sorter.bpkgs = append(sorter.bpkgs, bpkg)
	}

	sort.Sort(sorter)
	return sorter.bpkgs
}

func (b *Builder) SortedRpkgs() []*resolve.ResolvePackage {
	bpkgs := b.sortedBuildPackages()

	rpkgs := make([]*resolve.ResolvePackage, len(bpkgs), len(bpkgs))
	for i, bpkg := range bpkgs {
		rpkgs[i] = bpkg.rpkg
	}

	return rpkgs
}

func logDepInfo(res *resolve.Resolution) {
	// Log API set.
	apis := []string{}
	for api, _ := range res.ApiMap {
		apis = append(apis, api)
	}
	sort.Strings(apis)

	log.Debugf("API set:")
	for _, api := range apis {
		rpkg := res.ApiMap[api]
		log.Debugf("    * " + api + " (" + rpkg.Lpkg.FullName() + ")")
	}

	// Log dependency graph.
	dg, err := depGraph(res.MasterSet)
	if err != nil {
		log.Debugf("Error while constructing dependency graph: %s\n",
			err.Error())
	} else {
		log.Debugf("%s", DepGraphText(dg))
	}

	// Log reverse dependency graph.
	rdg, err := revdepGraph(res.MasterSet)
	if err != nil {
		log.Debugf("Error while constructing reverse dependency graph: %s\n",
			err.Error())
	} else {
		log.Debugf("%s", RevdepGraphText(rdg))
	}
}
