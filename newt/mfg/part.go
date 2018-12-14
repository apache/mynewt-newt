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
	"bytes"
	"io"
	"sort"

	"mynewt.apache.org/newt/util"
)

// A chunk of data in the manufacturing image.  Can be a firmware image or a
// raw entry (contents of a data file).
type Part struct {
	Name   string
	Offset int
	Data   []byte
}

type partSorter struct {
	parts []Part
}

func (s partSorter) Len() int {
	return len(s.parts)
}
func (s partSorter) Swap(i, j int) {
	s.parts[i], s.parts[j] = s.parts[j], s.parts[i]
}
func (s partSorter) Less(i, j int) bool {
	return s.parts[i].Offset < s.parts[j].Offset
}

func SortParts(parts []Part) []Part {
	sorter := partSorter{
		parts: parts,
	}

	sort.Sort(sorter)
	return sorter.parts
}

func WriteParts(parts []Part, w io.Writer, eraseVal byte) (int, error) {
	off := 0
	for _, p := range parts {
		if p.Offset < off {
			return off, util.FmtNewtError(
				"Invalid mfg parts: out of order")
		}

		// Pad the previous block up to the current offset.
		for off < p.Offset {
			if _, err := w.Write([]byte{eraseVal}); err != nil {
				return off, util.ChildNewtError(err)
			}
			off++
		}

		// Write the current block's data.
		size, err := w.Write(p.Data)
		if err != nil {
			return off, util.ChildNewtError(err)
		}
		off += size
	}

	// Note: the final block does not get padded.

	return off, nil
}

func PartsBytes(parts []Part) ([]byte, error) {
	b := &bytes.Buffer{}
	if _, err := WriteParts(parts, b, 0xff); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
