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

package mfg

import (
	"strings"

	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/artifact/sec"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
)

func loadDecodedMfg(basePath string) (DecodedMfg, error) {
	yc, err := newtutil.ReadConfig(basePath,
		strings.TrimSuffix(YAML_FILENAME, ".yml"))
	if err != nil {
		return DecodedMfg{}, err
	}

	dm, err := decodeMfg(yc)
	if err != nil {
		return DecodedMfg{}, err
	}

	return dm, nil
}

func LoadMfgEmitter(basePkg *pkg.LocalPackage,
	ver image.ImageVersion, keys []sec.SignKey) (MfgEmitter, error) {

	dm, err := loadDecodedMfg(basePkg.BasePath())
	if err != nil {
		return MfgEmitter{}, err
	}

	mb, err := newMfgBuilder(basePkg, dm, ver)
	if err != nil {
		return MfgEmitter{}, err
	}

	device, err := mb.calcDevice()
	if err != nil {
		return MfgEmitter{}, err
	}

	me, err := NewMfgEmitter(mb, basePkg.Name(), ver, device, keys)
	if err != nil {
		return MfgEmitter{}, err
	}

	return me, nil
}

func Upload(basePkg *pkg.LocalPackage) (string, error) {
	dm, err := loadDecodedMfg(basePkg.BasePath())
	if err != nil {
		return "", err
	}

	mb, err := newMfgBuilder(basePkg, dm, image.ImageVersion{})
	if err != nil {
		return "", err
	}

	envSettings := map[string]string{"MFG_IMAGE": "1"}
	binPath := MfgBinPath(basePkg.Name())
	basePath := strings.TrimSuffix(binPath, ".bin")

	if err := builder.Load(basePath, mb.Bsp, envSettings); err != nil {
		return "", err
	}

	return binPath, nil
}
