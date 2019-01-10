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
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"mynewt.apache.org/newt/artifact/flash"
	"mynewt.apache.org/newt/artifact/image"
	"mynewt.apache.org/newt/artifact/manifest"
	"mynewt.apache.org/newt/artifact/mfg"
	"mynewt.apache.org/newt/artifact/misc"
	"mynewt.apache.org/newt/artifact/sec"
	"mynewt.apache.org/newt/newt/builder"
	"mynewt.apache.org/newt/newt/flashmap"
	"mynewt.apache.org/newt/newt/target"
	"mynewt.apache.org/newt/util"
)

// Current manufacturing image binary format version.
const MANIFEST_FORMAT = 2

// Represents a file copy operation.
type CpEntry struct {
	From string
	To   string
}

type MfgEmitTarget struct {
	Name         string
	Offset       int
	IsBoot       bool
	BinPath      string
	ElfPath      string
	ManifestPath string
}

type MfgEmitRaw struct {
	Filename string
	Offset   int
}

type MfgEmitMetaMmr struct {
	Area flash.FlashArea
}

type MfgEmitMeta struct {
	Offset   int
	Hash     bool
	FlashMap bool
	Mmrs     []MfgEmitMetaMmr
}

type MfgEmitter struct {
	Name    string
	Ver     image.ImageVersion
	Targets []MfgEmitTarget
	Raws    []MfgEmitRaw
	Meta    *MfgEmitMeta
	Keys    []sec.SignKey

	Mfg      mfg.Mfg
	Device   int
	FlashMap flashmap.FlashMap
	BspName  string
}

// Calculates the source path of a target's binary.  Boot loader targets use
// `.bin` files; image targets use `.img`.
func targetSrcBinPath(t *target.Target, isBoot bool) string {
	if isBoot {
		return builder.AppBinPath(t.Name(), builder.BUILD_NAME_APP,
			t.App().Name())
	} else {
		return builder.AppImgPath(t.Name(), builder.BUILD_NAME_APP,
			t.App().Name())
	}
}

// Calculates the source path of a target's `.elf` file.
func targetSrcElfPath(t *target.Target) string {
	return builder.AppElfPath(t.Name(), builder.BUILD_NAME_APP, t.App().Name())
}

// Calculates the source path of a target's manifest file.
func targetSrcManifestPath(t *target.Target) string {
	return builder.ManifestPath(t.Name(), builder.BUILD_NAME_APP,
		t.App().Name())
}

func newMfgEmitTarget(bt MfgBuildTarget) (MfgEmitTarget, error) {
	return MfgEmitTarget{
		Name:    bt.Target.FullName(),
		Offset:  bt.Area.Offset + bt.Offset,
		IsBoot:  bt.IsBoot,
		BinPath: targetSrcBinPath(bt.Target, bt.IsBoot),
		ElfPath: targetSrcElfPath(bt.Target),
		ManifestPath: builder.ManifestPath(bt.Target.Name(),
			builder.BUILD_NAME_APP, bt.Target.App().Name()),
	}, nil
}

func newMfgEmitRaw(br MfgBuildRaw) MfgEmitRaw {
	return MfgEmitRaw{
		Filename: br.Filename,
		Offset:   br.Area.Offset + br.Offset,
	}
}

func newMfgEmitMeta(bm MfgBuildMeta, metaOff int) MfgEmitMeta {
	mmrs := []MfgEmitMetaMmr{}
	for _, bmmr := range bm.Mmrs {
		mmr := MfgEmitMetaMmr{
			Area: bmmr.Area,
		}
		mmrs = append(mmrs, mmr)
	}

	return MfgEmitMeta{
		Offset:   bm.Area.Offset + metaOff,
		Hash:     bm.Hash,
		FlashMap: bm.FlashMap,
		Mmrs:     mmrs,
	}
}

// NewMfgEmitter creates an mfg emitter from an mfg builder.
func NewMfgEmitter(mb MfgBuilder, name string, ver image.ImageVersion,
	device int, keys []sec.SignKey) (MfgEmitter, error) {

	me := MfgEmitter{
		Name:     name,
		Ver:      ver,
		Device:   device,
		Keys:     keys,
		FlashMap: mb.Bsp.FlashMap,
		BspName:  mb.Bsp.FullName(),
	}

	m, err := mb.Build()
	if err != nil {
		return me, err
	}
	me.Mfg = m

	for _, bt := range mb.Targets {
		et, err := newMfgEmitTarget(bt)
		if err != nil {
			return me, err
		}

		me.Targets = append(me.Targets, et)
	}

	for _, br := range mb.Raws {
		et := newMfgEmitRaw(br)
		me.Raws = append(me.Raws, et)
	}

	if mb.Meta != nil {
		mm := newMfgEmitMeta(*mb.Meta, me.Mfg.MetaOff)
		me.Meta = &mm
	}

	return me, nil
}

// Calculates the necessary file copy operations for emitting an mfg image.
func (me *MfgEmitter) calcCpEntries() []CpEntry {
	entries := []CpEntry{}
	for i, mt := range me.Targets {
		var binTo string
		if mt.IsBoot {
			binTo = MfgTargetBinPath(i)
		} else {
			binTo = MfgTargetImgPath(i)
		}

		entry := CpEntry{
			From: mt.BinPath,
			To:   MfgBinDir(me.Name) + "/" + binTo,
		}
		entries = append(entries, entry)

		entry = CpEntry{
			From: mt.ElfPath,
			To: MfgBinDir(me.Name) + "/" +
				MfgTargetElfPath(i),
		}
		entries = append(entries, entry)

		entry = CpEntry{
			From: mt.ManifestPath,
			To: MfgBinDir(me.Name) + "/" +
				MfgTargetManifestPath(i),
		}
		entries = append(entries, entry)
	}

	return entries
}

func copyBinFiles(entries []CpEntry) error {
	for _, entry := range entries {
		if err := os.MkdirAll(filepath.Dir(entry.To), 0755); err != nil {
			return util.ChildNewtError(err)
		}

		util.StatusMessage(util.VERBOSITY_VERBOSE, "copying file %s --> %s\n",
			entry.From, entry.To)

		if err := util.CopyFile(entry.From, entry.To); err != nil {
			return err
		}
	}

	return nil
}

func (me *MfgEmitter) createSigs() ([]manifest.MfgManifestSig, error) {
	hashBytes, err := me.Mfg.Hash()
	if err != nil {
		return nil, err
	}

	var sigs []manifest.MfgManifestSig
	for _, k := range me.Keys {
		sig, err := image.GenerateSig(k, hashBytes)
		if err != nil {
			return nil, err
		}

		pubKey, err := k.PubBytes()
		if err != nil {
			return nil, err
		}
		keyHash := sec.RawKeyHash(pubKey)

		sigs = append(sigs, manifest.MfgManifestSig{
			Key: hex.EncodeToString(keyHash),
			Sig: hex.EncodeToString(sig),
		})
	}

	return sigs, nil
}

// emitManifest generates an mfg manifest.
func (me *MfgEmitter) emitManifest() ([]byte, error) {
	hashBytes, err := me.Mfg.Hash()
	if err != nil {
		return nil, err
	}

	sigs, err := me.createSigs()
	if err != nil {
		return nil, err
	}

	mm := manifest.MfgManifest{
		Name:       me.Name,
		BuildTime:  time.Now().Format(time.RFC3339),
		Format:     MANIFEST_FORMAT,
		MfgHash:    misc.HashString(hashBytes),
		Version:    me.Ver.String(),
		Device:     me.Device,
		BinPath:    mfg.MFG_IMG_FILENAME,
		Signatures: sigs,
		FlashAreas: me.FlashMap.SortedAreas(),
		Bsp:        me.BspName,
	}

	for i, t := range me.Targets {
		mmt := manifest.MfgManifestTarget{
			Name:         t.Name,
			ManifestPath: MfgTargetManifestPath(i),
			Offset:       t.Offset,
		}

		if t.IsBoot {
			mmt.BinPath = MfgTargetBinPath(i)
		} else {
			mmt.ImagePath = MfgTargetImgPath(i)
		}

		mm.Targets = append(mm.Targets, mmt)
	}

	if me.Meta != nil {
		mmm := manifest.MfgManifestMeta{
			EndOffset: me.Mfg.MetaOff + int(me.Mfg.Meta.Footer.Size),
			Size:      int(me.Mfg.Meta.Footer.Size),
		}

		mmm.Hash = me.Meta.Hash
		mmm.FlashMap = me.Meta.FlashMap

		for _, mmr := range me.Meta.Mmrs {
			mmm.Mmrs = append(mmm.Mmrs, manifest.MfgManifestMetaMmr{
				Area:      mmr.Area.Name,
				Device:    mmr.Area.Device,
				EndOffset: mmr.Area.Offset + mmr.Area.Size,
			})
		}

		mm.Meta = &mmm
	}

	return mm.MarshalJson()
}

// @return                      [source-paths], [dest-paths], error
func (me *MfgEmitter) Emit() ([]string, []string, error) {
	if err := me.Mfg.RecalcHash(0xff); err != nil {
		return nil, nil, err
	}

	mbin, err := me.Mfg.Bytes(0xff)
	if err != nil {
		return nil, nil, err
	}

	cpEntries := me.calcCpEntries()
	if err := copyBinFiles(cpEntries); err != nil {
		return nil, nil, err
	}

	// Write mfgimg.bin
	binPath := MfgBinPath(me.Name)
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		return nil, nil, util.ChildNewtError(err)
	}
	if err := ioutil.WriteFile(binPath, mbin, 0644); err != nil {
		return nil, nil, err
	}

	// Write manifest.
	manifest, err := me.emitManifest()
	if err != nil {
		return nil, nil, err
	}

	manifestPath := MfgManifestPath(me.Name)
	if err := ioutil.WriteFile(manifestPath, manifest, 0644); err != nil {
		return nil, nil, util.FmtNewtError(
			"Failed to write mfg manifest file: %s", err.Error())
	}

	srcPaths := []string{}
	dstPaths := []string{
		binPath,
		manifestPath,
	}
	for _, entry := range cpEntries {
		srcPaths = append(srcPaths, entry.From)
		dstPaths = append(dstPaths, entry.To)
	}

	return srcPaths, dstPaths, nil
}
