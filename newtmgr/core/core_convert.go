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

package core

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"mynewt.apache.org/newt/util"
)

type CoreConvert struct {
	Source    *os.File
	Target    *os.File
	ImageHash []byte
	elfHdr    *elf.Header32
	phdrs     []*elf.Prog32
	data      [][]byte
}

const (
	COREDUMP_TLV_IMAGE = 1
	COREDUMP_TLV_MEM   = 2
	COREDUMP_TLV_REGS  = 3
)

const (
	COREDUMP_MAGIC = 0x690c47c3
)

type CoreDumpHdr struct {
	Magic uint32
	Size  uint32
}

type CoreDumpTlv struct {
	Type uint8
	pad  uint8
	Len  uint16
	Off  uint32
}

func NewCoreConvert() *CoreConvert {
	return &CoreConvert{}
}

func (cc *CoreConvert) readHdr() error {
	var hdr CoreDumpHdr

	hdr_buf := make([]byte, binary.Size(hdr))
	if hdr_buf == nil {
		return util.NewNewtError("Out of memory")
	}

	cnt, err := cc.Source.Read(hdr_buf)
	if err != nil {
		return util.NewNewtError(fmt.Sprintf("Error reading: %s", err.Error()))
	}
	if cnt != binary.Size(hdr) {
		return util.NewNewtError("Short read")
	}

	hdr.Magic = binary.LittleEndian.Uint32(hdr_buf[0:4])
	hdr.Size = binary.LittleEndian.Uint32(hdr_buf[4:8])

	if hdr.Magic != COREDUMP_MAGIC {
		return util.NewNewtError("Source file is not corefile")
	}
	return nil
}

func (cc *CoreConvert) readTlv() (*CoreDumpTlv, error) {
	var tlv CoreDumpTlv

	tlv_buf := make([]byte, binary.Size(tlv))
	if tlv_buf == nil {
		return nil, util.NewNewtError("Out of memory")
	}

	cnt, err := cc.Source.Read(tlv_buf)
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, util.NewNewtError(fmt.Sprintf("Error reading: %s",
			err.Error()))
	}
	if cnt == 0 {
		return nil, nil
	}
	if cnt != binary.Size(tlv) {
		return nil, util.NewNewtError("Short read")
	}

	tlv.Type = uint8(tlv_buf[0])
	tlv.pad = uint8(tlv_buf[1])
	tlv.Len = binary.LittleEndian.Uint16(tlv_buf[2:4])
	tlv.Off = binary.LittleEndian.Uint32(tlv_buf[4:8])

	return &tlv, nil
}

func (cc *CoreConvert) makeElfHdr() {
	var hdr elf.Header32
	var phdr elf.Prog32
	var shdr elf.Section32

	copy(hdr.Ident[:], elf.ELFMAG)
	hdr.Ident[elf.EI_CLASS] = byte(elf.ELFCLASS32)
	hdr.Ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	hdr.Ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	hdr.Ident[elf.EI_OSABI] = byte(elf.ELFOSABI_NONE)
	hdr.Ident[elf.EI_ABIVERSION] = 0
	hdr.Ident[elf.EI_PAD] = 0
	hdr.Type = uint16(elf.ET_CORE)
	hdr.Machine = uint16(elf.EM_ARM)
	hdr.Version = uint32(elf.EV_CURRENT)
	hdr.Entry = 0
	hdr.Phoff = uint32(binary.Size(hdr))
	hdr.Shoff = 0
	hdr.Flags = 0
	hdr.Ehsize = uint16(binary.Size(hdr))
	hdr.Phentsize = uint16(binary.Size(phdr))
	hdr.Phnum = uint16(len(cc.phdrs))
	hdr.Shentsize = uint16(binary.Size(shdr))
	hdr.Shnum = 0
	hdr.Shstrndx = uint16(elf.SHN_UNDEF)

	cc.elfHdr = &hdr
}

func (cc *CoreConvert) makeProgHdr(off uint32, mem []byte) {
	var phdr elf.Prog32

	memSz := uint32(len(mem))

	phdr.Type = uint32(elf.PT_LOAD)
	phdr.Off = 0 /* offset of data in file */
	phdr.Vaddr = off
	phdr.Paddr = 0
	phdr.Filesz = memSz
	phdr.Memsz = memSz
	phdr.Flags = uint32(elf.PF_R)
	phdr.Align = 4

	cc.phdrs = append(cc.phdrs, &phdr)
	if memSz%4 != 0 {
		pad := make([]byte, 4-memSz%4)
		mem = append(mem, pad...)
	}
	cc.data = append(cc.data, mem)
}

func (cc *CoreConvert) makeRegData(regs []byte) []byte {
	type Elf32_Note struct {
		Namesz uint32
		Descsz uint32
		Ntype  uint32
	}

	type Elf32_Prstatus struct {
		Dummy  [18]uint32
		Regs   [18]uint32
		Dummy2 uint32
	}

	var note Elf32_Note
	var sts Elf32_Prstatus

	idx := 0
	for off := 0; off < len(regs); off += 4 {
		reg := binary.LittleEndian.Uint32(regs[off : off+4])
		sts.Regs[idx] = reg
		idx++
		if idx >= 18 {
			break
		}
	}

	noteName := ".reg"
	noteLen := len(noteName) + 1
	if noteLen%4 != 0 {
		noteLen = noteLen + 4 - (noteLen % 4)
	}
	noteBytes := make([]byte, noteLen)
	copy(noteBytes[:], noteName)

	note.Namesz = uint32(len(noteName) + 1) /* include terminating '\0' */
	note.Descsz = uint32(binary.Size(sts))
	note.Ntype = uint32(elf.NT_PRSTATUS)

	buffer := new(bytes.Buffer)
	binary.Write(buffer, binary.LittleEndian, note)
	buffer.Write(noteBytes)
	binary.Write(buffer, binary.LittleEndian, sts)
	return buffer.Bytes()
}

func (cc *CoreConvert) makeRegInfo(regs []byte) {
	var phdr elf.Prog32

	phdr.Type = uint32(elf.PT_NOTE)
	phdr.Off = 0
	phdr.Vaddr = 0
	phdr.Paddr = 0
	phdr.Filesz = 0
	phdr.Memsz = 0
	phdr.Flags = 0
	phdr.Align = 4

	data := cc.makeRegData(regs)
	phdr.Filesz = uint32(len(data))

	cc.phdrs = append(cc.phdrs, &phdr)
	cc.data = append(cc.data, data)
}

func (cc *CoreConvert) setProgHdrOff() {
	off := binary.Size(cc.elfHdr)
	off += len(cc.phdrs) * binary.Size(cc.phdrs[0])

	for idx, phdr := range cc.phdrs {
		phdr.Off = uint32(off)
		off += len(cc.data[idx])
	}
}

func (cc *CoreConvert) Convert() error {
	if cc.Source == nil || cc.Target == nil {
		return util.NewNewtError("Missing file parameters")
	}

	err := cc.readHdr()
	if err != nil {
		return err
	}

	for {
		tlv, err := cc.readTlv()
		if err != nil {
			return err
		}
		if tlv == nil {
			break
		}
		data_buf := make([]byte, tlv.Len)
		cnt, err := cc.Source.Read(data_buf)
		if err != nil {
			return util.NewNewtError(fmt.Sprintf("Error reading: %s",
				err.Error()))
		}
		if cnt != int(tlv.Len) {
			return util.NewNewtError("Short file")
		}
		switch tlv.Type {
		case COREDUMP_TLV_MEM:
			cc.makeProgHdr(tlv.Off, data_buf)
		case COREDUMP_TLV_IMAGE:
			cc.ImageHash = data_buf
		case COREDUMP_TLV_REGS:
			if tlv.Len%4 != 0 {
				return util.NewNewtError("Invalid register area size")
			}
			cc.makeRegInfo(data_buf)
		default:
			return util.NewNewtError("Unknown TLV type")
		}
	}
	cc.makeElfHdr()
	if err != nil {
		return err
	}
	cc.setProgHdrOff()

	binary.Write(cc.Target, binary.LittleEndian, cc.elfHdr)
	for _, phdr := range cc.phdrs {
		binary.Write(cc.Target, binary.LittleEndian, phdr)
	}
	for _, data := range cc.data {
		cc.Target.Write(data)
	}
	return nil
}

func ConvertFilenames(srcFilename string,
	dstFilename string) (*CoreConvert, error) {

	coreConvert := NewCoreConvert()

	var err error

	coreConvert.Source, err = os.OpenFile(srcFilename, os.O_RDONLY, 0)
	if err != nil {
		return coreConvert, util.FmtNewtError("Cannot open file %s - %s",
			srcFilename, err.Error())
	}
	defer coreConvert.Source.Close()

	coreConvert.Target, err = os.OpenFile(dstFilename,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		return coreConvert, util.FmtNewtError("Cannot open file %s - %s",
			dstFilename, err.Error())
	}
	defer coreConvert.Target.Close()

	if err := coreConvert.Convert(); err != nil {
		return coreConvert, err
	}

	return coreConvert, nil
}
