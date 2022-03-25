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

package expr

import (
	"go/ast"
	"go/parser"
	"go/token"
	"mynewt.apache.org/newt/util"
	"strconv"
)

type RawString struct {
	S string
}

type ExprQuery interface {
	ExprGetValue(name string) (string, bool)
	ExprGetValueChoices(name string) ([]string, bool)
	ExprSetValue(name string, value interface{}, err error)
	ExprQueryPkg(name string, pkgName string) bool
}

type exprEntry struct {
	val    interface{}
	failed bool
	done   bool
}

type exprCtx struct {
	q         ExprQuery
	ees       map[string]*exprEntry
	entryName string
}

func int2bool(x int) bool {
	return x != 0
}

func bool2int(b bool) int {
	if b {
		return 1
	}

	return 0
}

func (expr *exprCtx) evalBasicLit(e *ast.BasicLit) (interface{}, error) {
	kind := e.Kind
	val := e.Value

	switch kind {
	case token.INT:
		v, err := strconv.ParseInt(val, 0, 0)
		return int(v), err
	case token.STRING:
		v, err := strconv.Unquote(val)
		return string(v), err
	case token.FLOAT:
		return 0, util.FmtNewtError("Unsupported non-integer number (%s) literal found, "+
			"consider using integer division instead", e.Value)
	}

	return 0, util.FmtNewtError("Invalid literal used in expression")
}

func (expr *exprCtx) evalBinaryExpr(e *ast.BinaryExpr) (int, error) {
	switch e.Op {
	case token.ADD:
	case token.SUB:
	case token.MUL:
	case token.QUO:
	case token.REM:
	case token.LAND:
	case token.LOR:
	case token.EQL:
	case token.LSS:
	case token.GTR:
	case token.NEQ:
	case token.LEQ:
	case token.GEQ:
	default:
		return 0, util.FmtNewtError("Invalid \"%s\" operator in expression", e.Op.String())
	}

	var x interface{}
	var y interface{}
	var err error

	x, err = expr.evalNode(e.X, false)
	if err != nil {
		return 0, err
	}
	y, err = expr.evalNode(e.Y, false)
	if err != nil {
		return 0, err
	}

	xv, xok := x.(int)
	yv, yok := y.(int)

	if xok != yok {
		return 0, util.FmtNewtError("Mismatched types for \"%s\" operator in expression", e.Op.String())
	}

	ret := 0

	if xok {
		switch e.Op {
		case token.ADD:
			ret = xv + yv
		case token.SUB:
			ret = xv - yv
		case token.MUL:
			ret = xv * yv
		case token.QUO:
			ret = xv / yv
		case token.REM:
			ret = xv % yv
		case token.LAND:
			ret = bool2int(int2bool(xv) && int2bool(yv))
		case token.LOR:
			ret = bool2int(int2bool(xv) || int2bool(yv))
		case token.EQL:
			ret = bool2int(xv == yv)
		case token.LSS:
			ret = bool2int(xv < yv)
		case token.GTR:
			ret = bool2int(xv > yv)
		case token.NEQ:
			ret = bool2int(xv != yv)
		case token.LEQ:
			ret = bool2int(xv <= yv)
		case token.GEQ:
			ret = bool2int(xv >= yv)
		}
	} else {
		// Each node is evaluated to int/string only so below assertions
		// should never fail
		switch e.Op {
		case token.EQL:
			ret = bool2int(x.(string) == y.(string))
		case token.NEQ:
			ret = bool2int(x.(string) != y.(string))
		default:
			return 0, util.FmtNewtError("Operator \"%s\" not supported for string literals",
				e.Op.String())
		}
	}

	return ret, nil
}

func (expr *exprCtx) evalUnaryExpr(e *ast.UnaryExpr) (int, error) {
	if e.Op != token.NOT && e.Op != token.SUB {
		return 0, util.FmtNewtError("Invalid \"%s\" operator in expression", e.Op.String())
	}

	x, err := expr.evalNode(e.X, false)
	if err != nil {
		return 0, err
	}

	xv, ok := x.(int)
	if !ok {
		return 0, util.FmtNewtError("String literals not applicable for \"%s\" operator", e.Op.String())
	}

	var ret int

	switch e.Op {
	case token.NOT:
		ret = bool2int(!int2bool(xv))
	case token.SUB:
		ret = -xv
	}

	return ret, nil
}

func (expr *exprCtx) evalCallExpr(e *ast.CallExpr) (interface{}, error) {
	f := e.Fun.(*ast.Ident)
	expectedArgc := -1
	minArgc := -1

	switch f.Name {
	case "raw", "has_pkg":
		expectedArgc = 1
	case "min", "max":
		expectedArgc = 2
	case "in_range", "clamp", "ite":
		expectedArgc = 3
	case "in_set":
		minArgc = 2
	default:
		return 0, util.FmtNewtError("Invalid function in expression: \"%s\"", f.Name)
	}

	argc := len(e.Args)

	if expectedArgc > 0 && argc != expectedArgc {
		return 0, util.FmtNewtError("Invalid number of arguments for \"%s\": expected %d, got %d",
			f.Name, expectedArgc, argc)
	}

	if minArgc > 0 && argc < minArgc {
		return 0, util.FmtNewtError("Invalid number of arguments for \"%s\": expected at least %d, got %d",
			f.Name, minArgc, argc)
	}

	var argv []interface{}
	for _, node := range e.Args {
		arg, err := expr.evalNode(node, false)
		if err != nil {
			return 0, err
		}

		argv = append(argv, arg)
	}

	var ret interface{}

	switch f.Name {
	case "raw":
		s, _ := argv[0].(string)
		rs := RawString{s}
		return rs, nil
	case "has_pkg":
		ret = bool2int(expr.q.ExprQueryPkg(expr.entryName, argv[0].(string)))
	case "min":
		a, ok1 := argv[0].(int)
		b, ok2 := argv[1].(int)
		if !ok1 || !ok2 {
			return 0, util.FmtNewtError("Invalid argument type for \"%s\"", f.Name)
		}
		ret = util.Min(a, b)
	case "max":
		a, ok1 := argv[0].(int)
		b, ok2 := argv[1].(int)
		if !ok1 || !ok2 {
			return 0, util.FmtNewtError("Invalid argument type for \"%s\"", f.Name)
		}
		ret = util.Max(a, b)
	case "clamp":
		v, ok1 := argv[0].(int)
		a, ok2 := argv[1].(int)
		b, ok3 := argv[2].(int)
		if !ok1 || !ok2 || !ok3 {
			return 0, util.FmtNewtError("Invalid argument type for \"%s\"", f.Name)
		}
		if v < a {
			ret = a
		} else if v > b {
			ret = b
		} else {
			ret = v
		}
	case "ite":
		v, ok1 := argv[0].(int)
		if !ok1 {
			return 0, util.FmtNewtError("Invalid argument type for \"%s\"", f.Name)
		}
		if v != 0 {
			ret = argv[1]
		} else {
			ret = argv[2]
		}
	case "in_range":
		v, ok1 := argv[0].(int)
		a, ok2 := argv[1].(int)
		b, ok3 := argv[2].(int)
		if !ok1 || !ok2 || !ok3 {
			return 0, util.FmtNewtError("Invalid argument type for \"%s\"", f.Name)
		}
		ret = bool2int(v >= a && v <= b)
	case "in_set":
		m := make(map[interface{}]struct{})
		for _, arg := range argv[1:] {
			m[arg] = struct{}{}
		}
		_, ok := m[argv[0]]
		ret = bool2int(ok)
	default:
		panic("This should never happen :>")
	}

	return ret, nil
}

func (expr *exprCtx) evalIdent(node *ast.Ident, direct bool) (interface{}, error) {
	name := node.Name

	if direct {
		vs, ok := expr.q.ExprGetValueChoices(expr.entryName)
		if ok {
			for _, v := range vs {
				if v == name {
					return v, nil
				}
			}
		}
	}

	ee, err := expr.evalEntry(name)
	if err != nil {
		return nil, err
	}

	return ee.val, err
}

func (expr *exprCtx) evalNode(node ast.Node, direct bool) (interface{}, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		return expr.evalBasicLit(n)
	case *ast.BinaryExpr:
		return expr.evalBinaryExpr(n)
	case *ast.UnaryExpr:
		return expr.evalUnaryExpr(n)
	case *ast.CallExpr:
		return expr.evalCallExpr(n)
	case *ast.Ident:
		return expr.evalIdent(n, direct)
	case *ast.ParenExpr:
		return expr.evalNode(n.X, false)
	}

	return 0, util.FmtNewtError("Invalid token in expression")
}

func (expr *exprCtx) evalEntry(name string) (*exprEntry, error) {
	ee, ok := expr.ees[name]
	if ok {
		if !ee.done {
			return ee, util.FmtNewtError("Circular dependency")
		}
		if ee.failed {
			// Return an empty error here. This can be used to detect case
			// when entry value cannot be evaluated because of an error in
			// another value entry. This can prevent returning the same error
			// for each entry that references invalid entry, but otherwise
			// is likely valid.
			return ee, util.FmtNewtError("")
		}
		return ee, nil
	}

	ee = &exprEntry{}
	expr.ees[name] = ee

	sval, ok := expr.q.ExprGetValue(name)
	if !ok {
		return ee, util.FmtNewtError("Unknown identifier referenced: %s", name)
	}

	prevEntryName := expr.entryName
	expr.entryName = name

	var val interface{} = nil
	var err error = nil

	if len(sval) > 0 {
		node, _ := parser.ParseExpr(sval)
		val, err = expr.evalNode(node, true)
		if err != nil {
			ee.failed = true
		}
	}

	expr.entryName = prevEntryName

	ee.val = val
	ee.done = true
	expr.q.ExprSetValue(name, ee.val, err)

	return ee, err
}

func (expr *exprCtx) Evaluate(s string) (interface{}, error) {
	ee, err := expr.evalEntry(s)

	return ee.val, err
}

func CreateCtx(q ExprQuery) *exprCtx {
	return &exprCtx{q: q, ees: make(map[string]*exprEntry)}
}
