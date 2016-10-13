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

package config

import (
	log "github.com/Sirupsen/logrus"
	"github.com/mitchellh/go-homedir"

	"encoding/json"
	"io/ioutil"
	"sort"

	"mynewt.apache.org/newt/util"
)

type ConnProfileMgr struct {
	profiles map[string]*ConnProfile
}

type NewtmgrConnProfile interface {
	Name() string
	Type() string
	ConnString() string
	DeviceAddress() []byte
	DeviceAddressType() uint8
}

type ConnProfile struct {
	MyName              string
	MyType              string
	MyConnString        string
	MyDeviceAddress     []byte
	MyDeviceAddressType uint8
}

func NewConnProfileMgr() (*ConnProfileMgr, error) {
	cpm := &ConnProfileMgr{
		profiles: map[string]*ConnProfile{},
	}

	if err := cpm.Init(); err != nil {
		return nil, err
	}

	return cpm, nil
}

func connProfileCfgFilename() (string, error) {
	dir, err := homedir.Dir()
	if err != nil {
		return "", util.NewNewtError(err.Error())
	}

	return dir + "/.newtmgr.cp.json", nil
}

func (cpm *ConnProfileMgr) Init() error {
	filename, err := connProfileCfgFilename()
	if err != nil {
		return err
	}

	// XXX: Should determine whether file exists by attempting to read it.
	if util.NodeExist(filename) {
		blob, err := ioutil.ReadFile(filename)
		if err != nil {
			return util.NewNewtError(err.Error())
		}

		var profiles []*ConnProfile
		err = json.Unmarshal(blob, &profiles)
		if err != nil {
			return util.FmtNewtError("error reading connection profile "+
				"config (%s): %s", filename, err.Error())
		}

		for _, p := range profiles {
			cpm.profiles[p.MyName] = p
		}
	}

	return nil
}

type connProfSorter struct {
	cps []*ConnProfile
}

func (s connProfSorter) Len() int {
	return len(s.cps)
}
func (s connProfSorter) Swap(i, j int) {
	s.cps[i], s.cps[j] = s.cps[j], s.cps[i]
}
func (s connProfSorter) Less(i, j int) bool {
	return s.cps[i].Name() < s.cps[j].Name()
}

func SortConnProfs(cps []*ConnProfile) []*ConnProfile {
	sorter := connProfSorter{
		cps: make([]*ConnProfile, 0, len(cps)),
	}

	for _, p := range cps {
		sorter.cps = append(sorter.cps, p)
	}

	sort.Sort(sorter)
	return sorter.cps
}

func (cpm *ConnProfileMgr) GetConnProfileList() ([]*ConnProfile, error) {
	log.Debugf("Getting list of connection profiles")

	cpList := make([]*ConnProfile, 0, len(cpm.profiles))
	for _, p := range cpm.profiles {
		cpList = append(cpList, p)
	}

	return SortConnProfs(cpList), nil
}

func (cpm *ConnProfileMgr) save() error {
	list, _ := cpm.GetConnProfileList()
	b, err := json.Marshal(list)
	if err != nil {
		return util.NewNewtError(err.Error())
	}

	filename, err := connProfileCfgFilename()
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filename, b, 0644)
	if err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}

func (cpm *ConnProfileMgr) DeleteConnProfile(name string) error {
	if cpm.profiles[name] == nil {
		return util.FmtNewtError("connection profile \"%s\" doesn't exist",
			name)
	}

	delete(cpm.profiles, name)

	err := cpm.save()
	if err != nil {
		return err
	}

	return nil
}

func (cpm *ConnProfileMgr) AddConnProfile(cp *ConnProfile) error {
	cpm.profiles[cp.Name()] = cp

	err := cpm.save()
	if err != nil {
		return err
	}

	return nil
}

func (cpm *ConnProfileMgr) GetConnProfile(pName string) (*ConnProfile, error) {
	// Each section is a connection profile, key values are the contents
	// of that section.
	if pName == "" {
		return nil, util.NewNewtError("Need to specify connection profile")
	}

	p := cpm.profiles[pName]
	if p == nil {
		return nil, util.FmtNewtError("connection profile \"%s\" doesn't "+
			"exist", pName)
	}

	return p, nil
}

func (cp *ConnProfile) Name() string {
	return cp.MyName
}

func (cp *ConnProfile) Type() string {
	return cp.MyType
}

func (cp *ConnProfile) ConnString() string {
	return cp.MyConnString
}

func (cp *ConnProfile) DeviceAddressType() uint8 {
	return cp.MyDeviceAddressType
}

func (cp *ConnProfile) DeviceAddress() []byte {
	return cp.MyDeviceAddress
}

func NewConnProfile(pName string) (*ConnProfile, error) {
	cp := &ConnProfile{}
	cp.MyName = pName

	return cp, nil
}
