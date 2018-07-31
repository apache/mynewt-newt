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

package ycfg

import (
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/parse"
)

// YAML configuration object.  This is a substitute for a viper configuration
// object, with the following newt-specific advantages:
// 1. Case sensitive.
// 2. Efficient conditionals based on syscfg values.
//
// A single YCfg setting is implemented as a tree of nodes.  Each word in the
// setting name represents a node; each "." in the name is a link in the tree.
// For example, the following syscfg lines:
//
// OS_MAIN_STACK_SIZE: 100
// OS_MAIN_STACK_SIZE.BLE_DEVICE: 200
// OS_MAIN_STACK_SIZE.SHELL_TASK: 300
//
// Is represented as the following tree:
//
//                      [OS_MAIN_STACK_SIZE (100)]
//                     /                          \
//            [BLE_DEVICE (200)]           [SHELL_TASK (300)]
//
// This allows us to quickly determine the value of OS_MAIN_STACK_SIZE.  After
// finding the OS_MAIN_STACK_SIZE node, the logic is something like this:
//
// Is BLE_DEVICE true? --> 200
// Is SHELL_TASK true? --> 300
// Else: --> 100
//
// The tree structure also allows for arbitrary expressions as conditionals, as
// opposed to simple setting names.  For example:
//
// OS_MAIN_STACK_SIZE: 100
// OS_MAIN_STACK_SIZE.'(BLE_DEVICE && !SHELL_TASK): 200
// OS_MAIN_STACK_SIZE.'(SHELL_TASK && !BLE_DEVICE): 300
// OS_MAIN_STACK_SIZE.'(SHELL_TASK && BLE_DEVICE):  400
//
// Since each expression is a child node of the setting in question, they are
// all known at the time of the lookup.  To determine the value of the setting,
// each expression is parsed, and only the one evaluating to true is selected.
type YCfg map[string]*YCfgNode

type YCfgEntry struct {
	Value interface{}
	Expr  string
}

type YCfgNode struct {
	Overwrite bool
	Name      string
	Value     interface{}
	Children  YCfg
	Parent    *YCfgNode
}

func (node *YCfgNode) addChild(name string) (*YCfgNode, error) {
	if node.Children == nil {
		node.Children = YCfg{}
	}

	if node.Children[name] != nil {
		return nil, fmt.Errorf("Duplicate YCfgNode: %s", name)
	}

	child := &YCfgNode{
		Name:   name,
		Parent: node,
	}
	node.Children[name] = child

	return child, nil
}

func (yc YCfg) Replace(key string, val interface{}) error {
	elems := strings.Split(key, ".")
	if len(elems) == 0 {
		return fmt.Errorf("Invalid ycfg key: \"\"")
	}

	var overwrite bool
	if elems[len(elems)-1] == "OVERWRITE" {
		overwrite = true
		elems = elems[:len(elems)-1]
	}

	var parent *YCfgNode
	for i, e := range elems {
		var parentChildren YCfg
		if parent == nil {
			parentChildren = yc
		} else {
			if parent.Children == nil {
				parent.Children = YCfg{}
			}
			parentChildren = parent.Children
		}
		child := parentChildren[e]
		if child == nil {
			var err error
			if parent != nil {
				child, err = parent.addChild(e)
				if err != nil {
					return err
				}
			} else {
				child = &YCfgNode{Name: e}
				parentChildren[e] = child
			}
		}

		if i == len(elems)-1 {
			child.Overwrite = overwrite
			child.Value = val
		}

		parent = child
	}

	return nil
}

func NewYCfg(kv map[string]interface{}) (YCfg, error) {
	yc := YCfg{}

	for k, v := range kv {
		if err := yc.Replace(k, v); err != nil {
			return nil, err
		}
	}

	return yc, nil
}

func (yc YCfg) find(key string) *YCfgNode {
	elems := strings.Split(key, ".")
	if len(elems) == 0 {
		return nil
	}

	cur := &YCfgNode{
		Children: yc,
	}
	for _, e := range elems {
		if cur.Children == nil {
			return nil
		}

		cur = cur.Children[e]
		if cur == nil {
			return nil
		}
	}

	return cur
}

func (yc YCfg) Get(key string, settings map[string]string) []YCfgEntry {
	node := yc.find(key)
	if node == nil {
		return nil
	}

	entries := []YCfgEntry{}

	if node.Value != nil {
		entry := YCfgEntry{Value: node.Value}
		entries = append(entries, entry)
	}

	for _, child := range node.Children {
		val, err := parse.ParseAndEval(child.Name, settings)
		if err != nil {
			log.Error(err.Error())
		} else if val {
			entry := YCfgEntry{
				Value: child.Value,
				Expr:  child.Name,
			}
			if child.Overwrite {
				return []YCfgEntry{entry}
			}

			entries = append(entries, entry)
		}
	}

	return entries
}

func (yc YCfg) GetSlice(key string, settings map[string]string) []YCfgEntry {
	sliceEntries := yc.Get(key, settings)
	if len(sliceEntries) == 0 {
		return nil
	}

	result := []YCfgEntry{}
	for _, sliceEntry := range sliceEntries {
		if sliceEntry.Value != nil {
			slice, err := cast.ToSliceE(sliceEntry.Value)
			if err != nil {
				// Not a slice.  Put the single value in a new slice.
				slice = []interface{}{sliceEntry.Value}
			}
			for _, v := range slice {
				entry := YCfgEntry{
					Value: v,
					Expr:  sliceEntry.Expr,
				}
				result = append(result, entry)
			}
		}
	}

	return result
}

func (yc YCfg) GetValSlice(
	key string, settings map[string]string) []interface{} {

	entries := yc.GetSlice(key, settings)
	if len(entries) == 0 {
		return nil
	}

	vals := make([]interface{}, len(entries))
	for i, e := range entries {
		vals[i] = e.Value
	}

	return vals
}

func (yc YCfg) GetStringSlice(key string,
	settings map[string]string) []YCfgEntry {

	sliceEntries := yc.Get(key, settings)
	if len(sliceEntries) == 0 {
		return nil
	}

	result := []YCfgEntry{}
	for _, sliceEntry := range sliceEntries {
		if sliceEntry.Value != nil {
			slice, err := cast.ToStringSliceE(sliceEntry.Value)
			if err != nil {
				// Not a slice.  Put the single value in a new slice.
				slice = []string{cast.ToString(sliceEntry.Value)}
			}
			for _, v := range slice {
				entry := YCfgEntry{
					Value: v,
					Expr:  sliceEntry.Expr,
				}
				result = append(result, entry)
			}
		}
	}

	return result
}

func (yc YCfg) GetValStringSlice(
	key string, settings map[string]string) []string {

	entries := yc.GetStringSlice(key, settings)
	if len(entries) == 0 {
		return nil
	}

	vals := make([]string, len(entries))
	for i, e := range entries {
		if e.Value != nil {
			vals[i] = cast.ToString(e.Value)
		}
	}

	return vals
}

func (yc YCfg) GetValStringSliceNonempty(
	key string, settings map[string]string) []string {

	strs := yc.GetValStringSlice(key, settings)
	filtered := make([]string, 0, len(strs))
	for _, s := range strs {
		if s != "" {
			filtered = append(filtered, s)
		}
	}

	return filtered
}

func (yc YCfg) GetStringMap(
	key string, settings map[string]string) map[string]YCfgEntry {

	mapEntries := yc.Get(key, settings)
	if len(mapEntries) == 0 {
		return nil
	}

	result := map[string]YCfgEntry{}

	for _, mapEntry := range mapEntries {
		for k, v := range cast.ToStringMap(mapEntry.Value) {
			entry := YCfgEntry{
				Value: v,
				Expr:  mapEntry.Expr,
			}

			// XXX: Report collisions?
			result[k] = entry
		}
	}

	return result
}

func (yc YCfg) GetValStringMap(
	key string, settings map[string]string) map[string]interface{} {

	entryMap := yc.GetStringMap(key, settings)

	smap := make(map[string]interface{}, len(entryMap))
	for k, v := range entryMap {
		if v.Value != nil {
			smap[k] = v.Value
		}
	}

	return smap
}

func (yc YCfg) GetFirst(key string, settings map[string]string) (YCfgEntry, bool) {
	entries := yc.Get(key, settings)
	if len(entries) == 0 {
		return YCfgEntry{}, false
	}

	return entries[0], true
}

func (yc YCfg) GetFirstVal(key string, settings map[string]string) interface{} {
	entry, ok := yc.GetFirst(key, settings)
	if !ok {
		return nil
	}

	return entry.Value
}

func (yc YCfg) GetValString(key string, settings map[string]string) string {
	entry, ok := yc.GetFirst(key, settings)
	if !ok {
		return ""
	} else {
		return cast.ToString(entry.Value)
	}
}

func (yc YCfg) GetValInt(key string, settings map[string]string) int {
	entry, ok := yc.GetFirst(key, settings)
	if !ok {
		return 0
	} else {
		return cast.ToInt(entry.Value)
	}
}

func (yc YCfg) GetValBoolDflt(key string, settings map[string]string,
	dflt bool) bool {

	entry, ok := yc.GetFirst(key, settings)
	if !ok {
		return dflt
	} else {
		return cast.ToBool(entry.Value)
	}
}

func (yc YCfg) GetValBool(key string, settings map[string]string) bool {
	return yc.GetValBoolDflt(key, settings, false)
}

func (yc YCfg) GetStringMapString(key string,
	settings map[string]string) map[string]YCfgEntry {

	mapEntries := yc.Get(key, settings)
	if len(mapEntries) == 0 {
		return nil
	}

	result := map[string]YCfgEntry{}

	for _, mapEntry := range mapEntries {
		for k, v := range cast.ToStringMapString(mapEntry.Value) {
			entry := YCfgEntry{
				Value: v,
				Expr:  mapEntry.Expr,
			}

			// XXX: Report collisions?
			result[k] = entry
		}
	}

	return result
}

func (yc YCfg) GetValStringMapString(key string,
	settings map[string]string) map[string]string {

	entryMap := yc.GetStringMapString(key, settings)

	valMap := make(map[string]string, len(entryMap))
	for k, v := range entryMap {
		if v.Value != nil {
			valMap[k] = cast.ToString(v.Value)
		}
	}

	return valMap
}

func (node *YCfgNode) FullName() string {
	tokens := []string{}

	for n := node; n != nil; n = n.Parent {
		tokens = append(tokens, n.Name)
	}

	last := len(tokens) - 1
	for i := 0; i < len(tokens)/2; i++ {
		tokens[i], tokens[last-i] = tokens[last-i], tokens[i]
	}

	return strings.Join(tokens, ".")
}

func (yc YCfg) Delete(name string) {
	delete(yc, name)
}

func (yc YCfg) Traverse(cb func(node *YCfgNode, depth int)) {
	var traverseLevel func(
		node *YCfgNode,
		cb func(node *YCfgNode, depth int),
		depth int)

	traverseLevel = func(
		node *YCfgNode,
		cb func(node *YCfgNode, depth int),
		depth int) {

		cb(node, depth)
		for _, child := range node.Children {
			traverseLevel(child, cb, depth+1)
		}
	}

	for _, n := range yc {
		traverseLevel(n, cb, 0)
	}
}

func (yc YCfg) AllSettings() map[string]interface{} {
	settings := map[string]interface{}{}

	yc.Traverse(func(node *YCfgNode, depth int) {
		if node.Value != nil {
			settings[node.FullName()] = node.Value
		}
	})

	return settings
}

func (yc YCfg) AllSettingsAsStrings() map[string]string {
	settings := yc.AllSettings()
	smap := make(map[string]string, len(settings))
	for k, v := range settings {
		smap[k] = fmt.Sprintf("%v", v)
	}

	return smap
}

func (yc YCfg) String() string {
	lines := make([]string, 0, len(yc))
	yc.Traverse(func(node *YCfgNode, depth int) {
		line := strings.Repeat(" ", depth*4) + node.Name
		if node.Value != nil {
			line += fmt.Sprintf(": %+v", node.Value)
		}
		lines = append(lines, line)
	})

	return strings.Join(lines, "\n")
}
