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
	"log"

	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
	"github.com/mitchellh/go-homedir"
)

type ConnProfileMgr struct {
	cDb *util.CfgDb
}

type NewtmgrConnProfile interface {
	Name() string
	Type() string
	ConnString() string
}

type ConnProfile struct {
	MyName       string
	MyType       string
	MyConnString string
}

func NewConnProfileMgr() (*ConnProfileMgr, error) {
	cpm := &ConnProfileMgr{}

	if err := cpm.Init(); err != nil {
		return nil, err
	}

	return cpm, nil
}

func (cpm *ConnProfileMgr) Init() error {
	var err error

	dir, err := homedir.Dir()
	if err != nil {
		return util.NewNewtError(err.Error())
	}

	cpm.cDb, err = util.NewCfgDb("cp", dir+"/.newtmgr.cp.db")
	if err != nil {
		return err
	}

	return nil
}

func (cpm *ConnProfileMgr) GetConnProfileList() ([]*ConnProfile, error) {
	log.Printf("[DEBUG] Getting list of connection profiles")
	cpMap, err := cpm.cDb.GetSect("conn_profile_list")
	if err != nil {
		return nil, err
	}

	cpList := make([]*ConnProfile, 0)

	for _, profileName := range cpMap {
		cp, err := cpm.GetConnProfile(profileName)
		if err != nil {
			return nil, err
		}

		cpList = append(cpList, cp)
	}

	return cpList, nil
}

func (cpm *ConnProfileMgr) DeleteConnProfile(name string) error {
	if err := cpm.cDb.DeleteSect("_conn_profile_" + name); err != nil {
		return err
	}

	if err := cpm.cDb.DeleteKey("conn_profile_list", name); err != nil {
		return err
	}

	return nil
}

func (cpm *ConnProfileMgr) AddConnProfile(cp *ConnProfile) error {
	sect := "_conn_profile_" + cp.Name()
	cDb := cpm.cDb

	// First serialize the conn profile into the configuration database
	cDb.SetKey(sect, "name", cp.Name())
	cDb.SetKey(sect, "type", cp.Type())
	cDb.SetKey(sect, "connstring", cp.ConnString())

	// Then write the ConnProfile to the ConnProfileList
	cDb.SetKey("conn_profile_list", cp.Name(), cp.Name())

	return nil
}

func (cpm *ConnProfileMgr) GetConnProfile(pName string) (*ConnProfile, error) {
	// Each section is a connection profile, key values are the contents
	// of that section.
	if pName == "" {
		return nil, util.NewNewtError("Need to specify connection profile")
	}

	sectName := "_conn_profile_" + pName

	cpVals, err := cpm.cDb.GetSect(sectName)
	if err != nil {
		return nil, err
	}

	cp, err := NewConnProfile(pName)
	if err != nil {
		return nil, err
	}

	for k, v := range cpVals {
		switch k {
		case "name":
			cp.MyName = v
		case "type":
			cp.MyType = v
		case "connstring":
			cp.MyConnString = v
		default:
			return nil, util.NewNewtError(
				"Invalid key " + k + " with val " + v)
		}
	}

	return cp, nil
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

func NewConnProfile(pName string) (*ConnProfile, error) {
	cp := &ConnProfile{}
	cp.MyName = pName

	return cp, nil
}
