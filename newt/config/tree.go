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
	"sort"
	"strings"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/util"
)

type Node struct {
	Entry    *FileEntry
	Children []*Node
}

type nodeSorter struct {
	nodes []*Node
}

func (s nodeSorter) Len() int {
	return len(s.nodes)
}
func (s nodeSorter) Swap(i, j int) {
	s.nodes[i], s.nodes[j] = s.nodes[j], s.nodes[i]
}
func (s nodeSorter) Less(i, j int) bool {
	return s.nodes[i].Entry.FileInfo.Path < s.nodes[j].Entry.FileInfo.Path
}

func SortNodes(nodes []*Node) {
	sorter := nodeSorter{
		nodes: nodes,
	}
	sort.Sort(sorter)
}

func SortTree(root *Node) {
	SortNodes(root.Children)
	for _, n := range root.Children {
		SortTree(n)
	}
}

func BuildTree(entries []FileEntry) (*Node, error) {
	// Create a node for each entry.
	m := make(map[*util.FileInfo]*Node, len(entries))
	for i, _ := range entries {
		e := &entries[i]
		m[e.FileInfo] = &Node{
			Entry: e,
		}
	}

	// Fill each node's `Children` slice.
	var root *Node
	for _, n := range m {
		if n.Entry.FileInfo.Parent == nil {
			if root != nil {
				return nil, util.FmtNewtError(
					"config tree contains two roots: %s, %s",
					root.Entry.FileInfo.Path, n.Entry.FileInfo.Path)
			}
			root = n
		} else {
			parentInfo := n.Entry.FileInfo.Parent
			parentNode := m[parentInfo]
			parentNode.Children = append(parentNode.Children, n)
		}
	}

	SortTree(root)
	return root, nil
}

func TreeString(tree *Node) string {
	var lines []string

	var appendLines func(n *Node, nestLevel int)
	appendLines = func(n *Node, nestLevel int) {
		indent := strings.Repeat(" ", nestLevel*4)
		path := newtutil.ProjRelPath(n.Entry.FileInfo.Path)
		lines = append(lines, indent+path)

		for _, child := range n.Children {
			appendLines(child, nestLevel+1)
		}
	}

	appendLines(tree, 1)
	return strings.Join(lines, "\n")
}
