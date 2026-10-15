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

package sysinit

import (
	"slices"
	"testing"

	"mynewt.apache.org/newt/newt/stage"
	"mynewt.apache.org/newt/newt/val"
)

func stagedFunc(name string, stg string) stage.StageFunc {
	return stage.StageFunc{
		Name:  name,
		Stage: val.ValSetting{Text: stg, Value: stg},
	}
}

func checkOrder(t *testing.T, sfs []stage.StageFunc, expected []string) {
	t.Helper()

	sorted, err := ResolveStageFuncsOrder(sfs)
	if err != nil {
		t.Fatal(err)
	}

	names := []string{}
	for _, sf := range sorted {
		names = append(names, sf.Name)
	}

	if !slices.Equal(names, expected) {
		t.Errorf("unexpected order: got %v, expected %v", names, expected)
	}
}

func TestResolveStageFuncsOrderEmpty(t *testing.T) {
	checkOrder(t, nil, []string{})
}

func TestResolveStageFuncsOrderNoStages(t *testing.T) {
	a := stage.StageFunc{Name: "a"}
	a.Stage.Afters = []string{"b"}
	b := stage.StageFunc{Name: "b"}

	checkOrder(t, []stage.StageFunc{a, b}, []string{"b", "a"})
}

func TestResolveStageFuncsOrderStages(t *testing.T) {
	checkOrder(t, []stage.StageFunc{
		stagedFunc("c", "200"),
		stagedFunc("b", "100"),
		stagedFunc("a", "100"),
	}, []string{"a", "b", "c"})
}

func TestResolveStageFuncsOrderMixed(t *testing.T) {
	d := stage.StageFunc{Name: "d"}
	d.Stage.Befores = []string{"b"}

	checkOrder(t, []stage.StageFunc{
		stagedFunc("b", "200"),
		stagedFunc("a", "100"),
		d,
	}, []string{"a", "d", "b"})
}
