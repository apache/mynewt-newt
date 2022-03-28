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
	"mynewt.apache.org/newt/newt/cfgv"
	"strings"

	"github.com/spf13/cast"

	"mynewt.apache.org/newt/newt/parse"
	"mynewt.apache.org/newt/util"
	"mynewt.apache.org/newt/yaml"
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
type YCfg struct {
	// Name of config; typically a YAML filename.
	name string

	// The settings.
	tree YCfgTree
}

type YCfgEntry struct {
	Value interface{}
	Expr  *parse.Node
}

type YCfgNode struct {
	Overwrite bool
	Name      string
	Value     interface{}
	Children  YCfgTree
	Parent    *YCfgNode
	FileInfo  *util.FileInfo
}

type YCfgTree map[string]*YCfgNode

func (yc *YCfg) Tree() YCfgTree {
	return yc.tree
}

func NewYCfgNode() *YCfgNode {
	return &YCfgNode{Children: YCfgTree{}}
}

func (node *YCfgNode) addChild(name string) (*YCfgNode, error) {
	if node.Children == nil {
		node.Children = YCfgTree{}
	}

	if node.Children[name] != nil {
		return nil, fmt.Errorf("Duplicate YCfgNode: %s", name)
	}

	child := NewYCfgNode()
	child.Name = name
	child.Parent = node

	node.Children[name] = child

	return child, nil
}

func (yc *YCfg) ReplaceFromFile(key string, val interface{},
	fileInfo *util.FileInfo) error {

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
		var parentChildren YCfgTree
		if parent == nil {
			parentChildren = yc.tree
		} else {
			if parent.Children == nil {
				parent.Children = YCfgTree{}
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
				child = NewYCfgNode()
				child.Name = e
				parentChildren[e] = child
			}
		}

		if i == len(elems)-1 {
			child.Overwrite = overwrite
			child.Value = val
		}
		child.FileInfo = fileInfo

		parent = child
	}

	return nil
}

// MergeFromFile merges the given value into a tree node.  Only two value types
// can be merged:
//
//     map[interface{}]interface{}
//     []interface{}
//
// The node's current value must have the same type as the value being merged.
// In the map case, each key-value pair in the given value is inserted into the
// current value, overwriting as necessary.  In the slice case, the given value
// is appended to the current value.
//
// If no node with the specified key exists, a new node is created containing
// the given value.
func (yc *YCfg) MergeFromFile(key string, val interface{},
	fileInfo *util.FileInfo) error {

	// Find the node being merged into.
	node := yc.find(key)

	// If the node doesn't exist, create one with the new value.
	if node == nil || node.Value == nil {
		return yc.ReplaceFromFile(key, val, fileInfo)
	}

	// The null value gets interpreted as an empty string during YAML
	// parsing.  A null merge is a no-op.
	if s, ok := val.(string); ok && s == "" {
		val = nil
	}
	if val == nil {
		return nil
	}

	mergeErr := func() error {
		return util.FmtNewtError(
			"can't merge type %T into cfg node \"%s\"; node type: %T",
			val, key, node.Value)
	}

	switch nodeVal := node.Value.(type) {
	case map[interface{}]interface{}:
		newVal, ok := val.(map[interface{}]interface{})
		if !ok {
			return mergeErr()
		}
		for k, v := range newVal {
			nodeVal[k] = v
		}

	case []interface{}:
		newVal, ok := val.([]interface{})
		if !ok {
			return mergeErr()
		}
		node.Value = append(nodeVal, newVal...)

	default:
		return mergeErr()
	}

	return nil
}

func (yc *YCfg) Replace(key string, val interface{}) error {
	return yc.ReplaceFromFile(key, val, nil)
}

func NewYCfg(name string) YCfg {
	return YCfg{
		name: name,
		tree: YCfgTree{},
	}
}

func (yc *YCfg) find(key string) *YCfgNode {
	elems := strings.Split(key, ".")
	if len(elems) == 0 {
		return nil
	}

	cur := &YCfgNode{
		Children: yc.tree,
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

func (yc *YCfg) HasKey(key string) bool {
	return yc.find(key) != nil
}

// Get retrieves all nodes with the specified key.  If it encounters a parse
// error in the tree, it ignores the bad node and continues the search.  All
// bad nodes are indicated in the returned error.  In this sense, the returned
// object is valid even if there is an error, and the error can be thought of
// as a set of warnings.
func (yc *YCfg) Get(key string,
	settings *cfgv.Settings) ([]YCfgEntry, error) {

	node := yc.find(key)
	if node == nil {
		return nil, nil
	}

	entries := []YCfgEntry{}

	if node.Value != nil {
		entry := YCfgEntry{Value: node.Value}
		entries = append(entries, entry)
	}

	var errLines []string
	for _, child := range node.Children {
		expr, err := parse.LexAndParse(child.Name)
		if err != nil {
			errLines = append(errLines,
				fmt.Sprintf("%s: %s", yc.name, err.Error()))
			continue
		}
		val, err := parse.Eval(expr, settings)
		if err != nil {
			errLines = append(errLines,
				fmt.Sprintf("%s: %s", yc.name, err.Error()))
			continue
		}
		if val {
			entry := YCfgEntry{
				Value: child.Value,
				Expr:  expr,
			}
			if child.Overwrite {
				entries = []YCfgEntry{entry}
				break
			}

			entries = append(entries, entry)
		}
	}

	if len(errLines) > 0 {
		return entries, util.NewNewtError(strings.Join(errLines, "\n"))
	} else {
		return entries, nil
	}
}

// GetSlice retrieves all entries with the specified key and coerces their
// values to type []interface{}.  The returned []YCfgEntry is formed from the
// union of all these slices.  The returned error is a set of warnings just as
// in `Get`.
func (yc *YCfg) GetSlice(key string, settings *cfgv.Settings) ([]YCfgEntry, error) {
	sliceEntries, getErr := yc.Get(key, settings)
	if len(sliceEntries) == 0 {
		return nil, getErr
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

	return result, getErr
}

// GetValSlice retrieves all entries with the specified key and coerces their
// values to type []interface{}.  The returned slice is the union of all these
// slices. The returned error is a set of warnings just as in `Get`.
func (yc *YCfg) GetValSlice(
	key string, settings *cfgv.Settings) ([]interface{}, error) {

	entries, getErr := yc.GetSlice(key, settings)
	if len(entries) == 0 {
		return nil, getErr
	}

	vals := make([]interface{}, len(entries))
	for i, e := range entries {
		vals[i] = e.Value
	}

	return vals, getErr
}

// GetStringSlice retrieves all entries with the specified key and coerces
// their values to type []string.  The returned []YCfgEntry is formed from the
// union of all these slices.  The returned error is a set of warnings just as
// in `Get`.
func (yc *YCfg) GetStringSlice(key string,
	settings *cfgv.Settings) ([]YCfgEntry, error) {

	sliceEntries, getErr := yc.Get(key, settings)
	if len(sliceEntries) == 0 {
		return nil, getErr
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

	return result, getErr
}

// GetValStringSlice retrieves all entries with the specified key and coerces
// their values to type []string.  The returned []string is the union of all
// these slices.  The returned error is a set of warnings just as in `Get`.
func (yc *YCfg) GetValStringSlice(
	key string, settings *cfgv.Settings) ([]string, error) {

	entries, getErr := yc.GetStringSlice(key, settings)
	if len(entries) == 0 {
		return nil, getErr
	}

	vals := make([]string, len(entries))
	for i, e := range entries {
		if e.Value != nil {
			vals[i] = cast.ToString(e.Value)
		}
	}

	return vals, getErr
}

// GetValStringSliceNonempty retrieves all entries with the specified key and
// coerces their values to type []string.  The returned []string is the union
// of all these slices.  Empty strings are excluded from this union.  The
// returned error is a set of warnings just as in `Get`.
func (yc *YCfg) GetValStringSliceNonempty(
	key string, settings *cfgv.Settings) ([]string, error) {

	strs, getErr := yc.GetValStringSlice(key, settings)
	filtered := make([]string, 0, len(strs))
	for _, s := range strs {
		if s != "" {
			filtered = append(filtered, s)
		}
	}

	return filtered, getErr
}

// GetStringMap retrieves all entries with the specified key and coerces their
// values to type map[string]interface{}.  The returned map[string]YCfgEntry is
// formed from the union of all these maps.  The returned error is a set of
// warnings just as in `Get`.
func (yc *YCfg) GetStringMap(
	key string, settings *cfgv.Settings) (map[string]YCfgEntry, error) {

	mapEntries, getErr := yc.Get(key, settings)
	if len(mapEntries) == 0 {
		return nil, getErr
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

	return result, getErr
}

// GetValStringMap retrieves all entries with the specified key and coerces
// their values to type map[string]interface{}.  The returned
// map[string]YCfgEntry is the union of all these maps.  The returned error is
// a set of warnings just as in `Get`.
func (yc *YCfg) GetValStringMap(
	key string, settings *cfgv.Settings) (map[string]interface{}, error) {

	entryMap, getErr := yc.GetStringMap(key, settings)

	smap := make(map[string]interface{}, len(entryMap))
	for k, v := range entryMap {
		if v.Value != nil {
			smap[k] = v.Value
		}
	}

	return smap, getErr
}

// GetFirst retrieves the first entry with the specified key.  The bool return
// value is true if a matching entry was found.  The returned error is a set of
// warnings just as in `Get`.
func (yc *YCfg) GetFirst(key string,
	settings *cfgv.Settings) (YCfgEntry, bool, error) {

	entries, getErr := yc.Get(key, settings)
	if len(entries) == 0 {
		return YCfgEntry{}, false, getErr
	}

	return entries[0], true, getErr
}

// GetFirstVal retrieves the first entry with the specified key and returns its
// value.  It returns nil if no matching entry is found.  The returned error is
// a set of warnings just as in `Get`.
func (yc *YCfg) GetFirstVal(key string,
	settings *cfgv.Settings) (interface{}, error) {

	entry, ok, getErr := yc.GetFirst(key, settings)
	if !ok {
		return nil, getErr
	}

	return entry.Value, getErr
}

// GetValString retrieves the first entry with the specified key and returns
// its value coerced to a string.  It returns "" if no matching entry is found.
// The returned error is a set of warnings just as in `Get`.
func (yc *YCfg) GetValString(key string,
	settings *cfgv.Settings) (string, error) {

	entry, ok, getErr := yc.GetFirst(key, settings)
	if !ok {
		return "", getErr
	} else {
		return cast.ToString(entry.Value), getErr
	}
}

// GetValInt retrieves the first entry with the specified key and returns its
// value coerced to an int.  It returns 0 if no matching entry is found.  The
// returned error is a set of warnings just as in `Get`.
func (yc *YCfg) GetValInt(key string, settings *cfgv.Settings) (int, error) {
	entry, ok, getErr := yc.GetFirst(key, settings)
	if !ok {
		return 0, getErr
	} else {
		return cast.ToInt(entry.Value), getErr
	}
}

// GetValIntDflt retrieves the first entry with the specified key and returns its
// value coerced to an int.  It returns the specified default if no matching entry
// is found.  The returned error is a set of warnings just as in `Get`.
func (yc *YCfg) GetValIntDflt(key string, settings *cfgv.Settings, dflt int) (int, error) {
	entry, ok, getErr := yc.GetFirst(key, settings)
	if !ok {
		return dflt, getErr
	} else {
		return cast.ToInt(entry.Value), getErr
	}
}

// GetValBoolDflt retrieves the first entry with the specified key and returns
// its value coerced to a bool.  It returns the specified default if no
// matching entry is found.  The returned error is a set of warnings just as in
// `Get`.
func (yc *YCfg) GetValBoolDflt(key string, settings *cfgv.Settings,
	dflt bool) (bool, error) {

	entry, ok, getErr := yc.GetFirst(key, settings)
	if !ok {
		return dflt, getErr
	} else {
		return cast.ToBool(entry.Value), getErr
	}
}

// GetValBoolDflt retrieves the first entry with the specified key and returns
// its value coerced to a bool.  It returns false if no matching entry is
// found.  The returned error is a set of warnings just as in `Get`.
func (yc *YCfg) GetValBool(key string,
	settings *cfgv.Settings) (bool, error) {

	return yc.GetValBoolDflt(key, settings, false)
}

// GetStringMapString retrieves all entries with the specified key and coerces
// their values to type map[string]string.  The returned map[string]YCfgEntry
// is formed from the union of all these maps.  The returned error is a set of
// warnings just as in `Get`.
func (yc *YCfg) GetStringMapString(key string,
	settings *cfgv.Settings) (map[string]YCfgEntry, error) {

	mapEntries, getErr := yc.Get(key, settings)
	if len(mapEntries) == 0 {
		return nil, getErr
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

	return result, getErr
}

// GetStringMapString retrieves all entries with the specified key and coerces
// their values to type map[string]string.  The returned map[string]YCfgEntry
// is the union of all these maps.  The returned error is a set of warnings
// just as in `Get`.
func (yc *YCfg) GetValStringMapString(key string,
	settings *cfgv.Settings) (map[string]string, error) {

	entryMap, getErr := yc.GetStringMapString(key, settings)

	valMap := make(map[string]string, len(entryMap))
	for k, v := range entryMap {
		if v.Value != nil {
			valMap[k] = cast.ToString(v.Value)
		}
	}

	return valMap, getErr
}

// FullName calculates a node's name with the following form:
//     [...].<grandparent>.<parent>.<node>
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

// Delete deletes all entries with the specified key.
func (yc *YCfg) Delete(key string) {
	delete(yc.tree, key)
}

// Clear removes all entries from the YCfg.
func (yc *YCfg) Clear() {
	yc.tree = YCfgTree{}
}

// Traverse performs an in-order traversal of the YCfg tree.  The specified
// function is applied to each node.
func (yc *YCfg) Traverse(cb func(node *YCfgNode, depth int)) {
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

	for _, n := range yc.tree {
		traverseLevel(n, cb, 0)
	}
}

// AllSettings converts the YCfg into a map with the following form:
//     <node-full-name>: <node-value>
func (yc *YCfg) AllSettings() map[string]interface{} {
	settings := map[string]interface{}{}

	yc.Traverse(func(node *YCfgNode, depth int) {
		if node.Value != nil {
			settings[node.FullName()] = node.Value
		}
	})

	return settings
}

// AllSettingsAsStrings converts the YCfg into a map with the following form:
//     <node-full-name>: <node-value>
//
// All values in the map have been coerced to strings.
func (yc *YCfg) AllSettingsAsStrings() map[string]string {
	settings := yc.AllSettings()
	smap := make(map[string]string, len(settings))
	for k, v := range settings {
		smap[k] = fmt.Sprintf("%v", v)
	}

	return smap
}

// String produces a user-friendly string representation of the YCfg.
func (yc *YCfg) String() string {
	lines := make([]string, 0, len(yc.tree))
	yc.Traverse(func(node *YCfgNode, depth int) {
		line := strings.Repeat(" ", depth*4) + node.Name
		if node.Value != nil {
			line += fmt.Sprintf(": %+v", node.Value)
		}
		lines = append(lines, line)
	})

	return strings.Join(lines, "\n")
}

// YAML converts the YCfg to a map and encodes the map as YAML.
func (yc *YCfg) YAML() string {
	return yaml.MapToYaml(yc.AllSettings())
}
