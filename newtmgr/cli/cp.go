/*
 Copyright 2015 Runtime Inc.
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

package cli

import (
	"git-wip-us.apache.org/repos/asf/incubator-mynewt-newt/util"
	"github.com/mitchellh/go-homedir"
	"log"
)

type CpMgr struct {
	cDb *util.CfgDb
}

type ConnProfile struct {
	Name       string
	Type       string
	ConnString string
}

func NewCpMgr() (*CpMgr, error) {
	cpm := &CpMgr{}

	if err := cpm.Init(); err != nil {
		return nil, err
	}

	return cpm, nil
}

func (cpm *CpMgr) Init() error {
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

func (cpm *CpMgr) GetConnProfileList() ([]*ConnProfile, error) {
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

func (cpm *CpMgr) DeleteConnProfile(name string) error {
	if err := cpm.cDb.DeleteSect("_conn_profile_" + name); err != nil {
		return err
	}

	if err := cpm.cDb.DeleteKey("conn_profile_list", name); err != nil {
		return err
	}

	return nil
}

func (cpm *CpMgr) AddConnProfile(cp *ConnProfile) error {
	sect := "_conn_profile_" + cp.Name
	cDb := cpm.cDb

	// First serialize the conn profile into the configuration database
	cDb.SetKey(sect, "name", cp.Name)
	cDb.SetKey(sect, "type", cp.Type)
	cDb.SetKey(sect, "connstring", cp.ConnString)

	// Then write the ConnProfile to the ConnProfileList
	cDb.SetKey("conn_profile_list", cp.Name, cp.Name)

	return nil
}

func (cpm *CpMgr) GetConnProfile(pName string) (*ConnProfile, error) {
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
			cp.Name = v
		case "type":
			cp.Type = v
		case "connstring":
			cp.ConnString = v
		default:
			return nil, util.NewNewtError(
				"Invalid key " + k + " with val " + v)
		}
	}

	return cp, nil
}

func NewConnProfile(pName string) (*ConnProfile, error) {
	cp := &ConnProfile{}
	cp.Name = pName

	return cp, nil
}
