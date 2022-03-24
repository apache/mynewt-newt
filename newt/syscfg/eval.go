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

package syscfg

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"mynewt.apache.org/newt/util"
	"strconv"
)

func int2bool(x int) bool {
	return x != 0
}

func bool2int(b bool) int {
	if b {
		return 1
	}

	return 0
}

func (cfg *Cfg) exprEvalLiteral(e *ast.BasicLit) (interface{}, error) {
	kind := e.Kind
	val := e.Value

	switch kind {
	case token.INT:
		v, err := strconv.ParseInt(val, 0, 0)
		return int(v), err
	case token.STRING:
		return val, nil
	}

	return 0, util.FmtNewtError("Invalid exprEvalLiteral used in expression")
}

func (cfg *Cfg) exprEvalBinaryExpr(e *ast.BinaryExpr) (int, error) {
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

	x, err = cfg.exprEvalNode(e.X)
	if err != nil {
		return 0, err
	}
	y, err = cfg.exprEvalNode(e.Y)
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

func (cfg *Cfg) exprEvalUnaryExpr(e *ast.UnaryExpr) (int, error) {
	if e.Op != token.NOT && e.Op != token.SUB {
		return 0, util.FmtNewtError("Invalid \"%s\" operator in expression", e.Op.String())
	}

	x, err := cfg.exprEvalNode(e.X)
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

func (cfg *Cfg) exprEvalCallExpr(e *ast.CallExpr) (interface{}, error) {
	f := e.Fun.(*ast.Ident)
	expectedArgc := -1
	minArgc := -1

	switch f.Name {
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

	argv := []interface{}{}
	argvs := []string{}
	for _, node := range e.Args {
		arg, err := cfg.exprEvalNode(node)
		if err != nil {
			return 0, err
		}

		argv = append(argv, arg)
		argvs = append(argvs, fmt.Sprintf("%v", arg))
	}

	var ret interface{}

	switch f.Name {
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
	}

	return ret, nil
}

func (cfg *Cfg) exprEvalIdentifier(e *ast.Ident) (interface{}, error) {
	name := e.Name

	entry, ok := cfg.Settings[name]
	if !ok {
		return 0, util.FmtNewtError("Undefined identifier referenced: %s", name)
	}

	var val interface{}
	var err error

	switch entry.EvalState {
	case CFG_EVAL_STATE_NONE:
		entry, err = cfg.evalEntry(entry)
		val = entry.EvalValue
	case CFG_EVAL_STATE_RUNNING:
		err = util.FmtNewtError("Circular identifier dependency in expression")
	case CFG_EVAL_STATE_SUCCESS:
		val = entry.EvalValue
	case CFG_EVAL_STATE_FAILED:
		err = util.FmtNewtError("")
	}

	return val, err
}

func (cfg *Cfg) exprEvalNode(node ast.Node) (interface{}, error) {
	switch e := node.(type) {
	case *ast.BasicLit:
		return cfg.exprEvalLiteral(e)
	case *ast.BinaryExpr:
		return cfg.exprEvalBinaryExpr(e)
	case *ast.UnaryExpr:
		return cfg.exprEvalUnaryExpr(e)
	case *ast.CallExpr:
		return cfg.exprEvalCallExpr(e)
	case *ast.Ident:
		return cfg.exprEvalIdentifier(e)
	case *ast.ParenExpr:
		return cfg.exprEvalNode(e.X)
	}

	return 0, util.FmtNewtError("Invalid token in expression")
}

func (cfg *Cfg) evalEntry(entry CfgEntry) (CfgEntry, error) {
	name := entry.Name

	if entry.EvalState != CFG_EVAL_STATE_NONE {
		panic("This should never happen :>")
	}

	entry.EvalState = CFG_EVAL_STATE_RUNNING
	cfg.Settings[name] = entry

	entry.EvalOrigValue = entry.Value

	node, _ := parser.ParseExpr(entry.Value)
	newVal, err := cfg.exprEvalNode(node)
	if err != nil {
		entry.EvalState = CFG_EVAL_STATE_FAILED
		entry.EvalError = err
		cfg.Settings[entry.Name] = entry
		cfg.InvalidExpressions[entry.Name] = struct{}{}
		err = util.FmtNewtError("")
		return entry, err
	}

	switch val := newVal.(type) {
	case int:
		entry.EvalValue = val
		entry.Value = strconv.Itoa(val)
	case string:
		entry.EvalValue = val
		entry.Value = val
	default:
		panic("This should never happen :>")
	}

	entry.EvalState = CFG_EVAL_STATE_SUCCESS
	cfg.Settings[entry.Name] = entry

	return entry, nil
}

func (cfg *Cfg) Evaluate(name string) {
	entry := cfg.Settings[name]

	switch entry.EvalState {
	case CFG_EVAL_STATE_NONE:
		cfg.evalEntry(entry)
	case CFG_EVAL_STATE_RUNNING:
		panic("This should never happen :>")
	case CFG_EVAL_STATE_SUCCESS:
		// Already evaluated
	case CFG_EVAL_STATE_FAILED:
		// Already evaluated
	}
}
