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

package parse

import (
	"fmt"
	"sort"

	"mynewt.apache.org/newt/util"
)

// expr     ::= <unary><expr> | "("<expr>")" |
//              <expr><binary><expr> | <ident> | <literal>
// ident    ::= <printable-char> { <printable-char> }
// literal  ::= """ <printable-char> { <printable-char> } """
// unary    ::= "!"
// binary   ::= "&&" | "^^" | "||" | "==" | "!=" | "<" | "<=" | ">" | ">="

type ParseCode int

const (
	PARSE_NOT_EQUALS ParseCode = iota
	PARSE_NOT
	PARSE_EQUALS
	PARSE_LT
	PARSE_LTE
	PARSE_GT
	PARSE_GTE
	PARSE_AND
	PARSE_OR
	PARSE_XOR
	PARSE_NUMBER
	PARSE_STRING
	PARSE_IDENT
)

type Node struct {
	Code ParseCode
	Data string

	Left  *Node
	Right *Node
}

func (n *Node) String() string {
	if n == nil {
		return ""
	}

	s := ""

	if n.Left != nil {
		s += n.Left.String() + " "
	}

	s += n.Data

	if n.Right != nil {
		s += " " + n.Right.String()
	}

	return s
}

func (n *Node) RpnString() string {
	if n == nil {
		return ""
	}

	s := fmt.Sprintf("<%s>", n.Data)
	if n.Left != nil {
		s += " " + n.Left.RpnString()
	}
	if n.Right != nil {
		s += " " + n.Right.RpnString()
	}

	return s
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
	return s.nodes[i].String() < s.nodes[j].String()
}

func SortNodes(nodes []*Node) {
	sort.Sort(nodeSorter{nodes})
}

// Searches a tokenized expression.  The location of the first token that
// matches a member of the supplied token set is returned.  This function does
// not descend into parenthesized expressions.
func findAnyToken(tokens []Token, any []TokenCode) (int, error) {
	pcount := 0

	for _, a := range any {
		for i, t := range tokens {
			if t.Code == TOKEN_LPAREN {
				pcount++
			} else if t.Code == TOKEN_RPAREN {
				pcount--
				if pcount < 0 {
					return -1, fmt.Errorf("imbalanced parenthesis")
				}
			} else if pcount == 0 && t.Code == a {
				return i, nil
			}
		}

	}
	return -1, nil
}

func binTokenToParse(t TokenCode) ParseCode {
	return map[TokenCode]ParseCode{
		TOKEN_NOT_EQUALS: PARSE_NOT_EQUALS,
		TOKEN_EQUALS:     PARSE_EQUALS,
		TOKEN_LT:         PARSE_LT,
		TOKEN_LTE:        PARSE_LTE,
		TOKEN_GT:         PARSE_GT,
		TOKEN_GTE:        PARSE_GTE,
		TOKEN_AND:        PARSE_AND,
		TOKEN_OR:         PARSE_OR,
		TOKEN_XOR:        PARSE_XOR,
	}[t]
}

// Removes the outer layer of parentheses from a tokenized expression.
func stripParens(tokens []Token) ([]Token, error) {
	if tokens[0].Code != TOKEN_LPAREN {
		panic("internal error: stripParens() received unparenthesized string")
	}

	pcount := 1
	for i := 1; i < len(tokens); i++ {
		switch tokens[i].Code {
		case TOKEN_LPAREN:
			pcount++

		case TOKEN_RPAREN:
			pcount--
			if pcount == 0 {
				return tokens[1:i], nil
			}

		default:
		}
	}

	return nil, fmt.Errorf("unterminated parenthesis")
}

var binaryTokens = []TokenCode{
	// Lowest precedence.
	TOKEN_AND,
	TOKEN_XOR,
	TOKEN_OR,
	TOKEN_EQUALS,
	TOKEN_NOT_EQUALS,
	TOKEN_LT,
	TOKEN_LTE,
	TOKEN_GT,
	TOKEN_GTE,
	// Highest precedence.
}

func FindBinaryToken(tokens []Token) int {
	binIdx, err := findAnyToken(tokens, binaryTokens)
	if err != nil {
		return -1
	}
	return binIdx
}

// Recursively parses a tokenized expression.
//
// @param tokens                The sequence of tokens representing the
//                                  expression to parse.  This is acquired by a
//                                  call to `Lex()`.
//
// @return *Node                The expression parse tree.
func Parse(tokens []Token) (*Node, error) {
	if len(tokens) == 0 {
		return nil, nil
	}

	////// Terminal symbols.

	if len(tokens) == 1 {
		switch tokens[0].Code {
		case TOKEN_NUMBER:
			return &Node{
				Code: PARSE_NUMBER,
				Data: tokens[0].Text,
			}, nil

		case TOKEN_STRING:
			return &Node{
				Code: PARSE_STRING,
				Data: tokens[0].Text,
			}, nil

		case TOKEN_IDENT:
			return &Node{
				Code: PARSE_IDENT,
				Data: tokens[0].Text,
			}, nil

		default:
			return nil, fmt.Errorf("invalid expression: %s", tokens[0].Text)
		}
	}

	////// Nonterminal symbols.

	// <expr><binary><expr>
	binIdx, err := findAnyToken(tokens, binaryTokens)
	if err != nil {
		return nil, err
	}
	if binIdx == 0 || binIdx == len(tokens)-1 {
		return nil, fmt.Errorf("binary operator %s at start or end",
			tokens[binIdx].Text)
	}
	if binIdx != -1 {
		n := &Node{
			Code: binTokenToParse(tokens[binIdx].Code),
			Data: tokens[binIdx].Text,
		}

		l, err := Parse(tokens[0:binIdx])
		if err != nil {
			return nil, err
		}

		r, err := Parse(tokens[binIdx+1 : len(tokens)])
		if err != nil {
			return nil, err
		}

		n.Left = l
		n.Right = r

		return n, nil
	}

	// <unary><expr>
	if tokens[0].Code == TOKEN_NOT {
		n := &Node{
			Code: PARSE_NOT,
			Data: tokens[0].Text,
		}
		r, err := Parse(tokens[1:])
		if err != nil {
			return nil, err
		}
		n.Right = r
		return n, nil
	}

	// "("<expr>")"
	if tokens[0].Code == TOKEN_LPAREN {
		stripped, err := stripParens(tokens)
		if err != nil {
			return nil, err
		}

		return Parse(stripped)
	}

	return nil, fmt.Errorf("invalid expression")
}

// Evaluates two expressions into boolean values.
func evalTwo(expr1 *Node, expr2 *Node,
	settings map[string]string) (bool, bool, error) {

	v1, err := Eval(expr1, settings)
	if err != nil {
		return false, false, err
	}
	v2, err := Eval(expr2, settings)
	if err != nil {
		return false, false, err
	}

	return v1, v2, nil
}

func coerceToInt(n *Node, settings map[string]string) (int, error) {
	switch n.Code {
	case PARSE_NUMBER:
		num, ok := util.AtoiNoOctTry(n.Data)
		if !ok {
			return 0,
				util.FmtNewtError("expression contains invalid number: `%s`",
					n.Data)
		}
		return num, nil

	case PARSE_IDENT:
		val := settings[n.Data]
		num, ok := util.AtoiNoOctTry(val)
		if !ok {
			return 0,
				util.FmtNewtError("setting %s has value `%s`, "+
					"which is not a number", n.Data, val)
		}
		return num, nil

	default:
		return 0,
			util.FmtNewtError("expression `%s` is not a valid number",
				n.String())
	}
}

func coerceTwoInts(left *Node, right *Node,
	settings map[string]string, opStr string) (int, int, error) {

	lnum, err := coerceToInt(left, settings)
	if err != nil {
		return 0, 0, util.FmtNewtError("cannot apply %s to `%s`; "+
			"operand not a number", opStr, left.String())
	}

	rnum, err := coerceToInt(right, settings)
	if err != nil {
		return 0, 0, util.FmtNewtError("cannot apply %s to `%s`; "+
			"operand not a number", opStr, right.String())
	}

	return lnum, rnum, nil
}

type equalsFn func(left *Node, right *Node, settings map[string]string) bool
type equalsEntry struct {
	LeftCode  ParseCode
	RightCode ParseCode
	Fn        equalsFn
}

var equalsDispatch = []equalsEntry{
	// <ident1> == <ident2>
	// True if both syscfg settings have identical text values.
	equalsEntry{
		LeftCode:  PARSE_IDENT,
		RightCode: PARSE_IDENT,
		Fn: func(left *Node, right *Node, settings map[string]string) bool {
			return settings[left.Data] == settings[right.Data]
		},
	},

	// <ident> == <number>
	// True if the syscfg setting's value is a valid representation of the
	// number on the right.
	equalsEntry{
		LeftCode:  PARSE_IDENT,
		RightCode: PARSE_NUMBER,
		Fn: func(left *Node, right *Node, settings map[string]string) bool {
			lnum, ok := util.AtoiNoOctTry(settings[left.Data])
			if !ok {
				return false
			}
			rnum, ok := util.AtoiNoOctTry(right.Data)
			if !ok {
				return false
			}
			return lnum == rnum
		},
	},

	// <ident> == <string>
	// True if the syscfg setting's text value is identical to the string on
	// the right.
	equalsEntry{
		LeftCode:  PARSE_IDENT,
		RightCode: PARSE_STRING,
		Fn: func(left *Node, right *Node, settings map[string]string) bool {
			return settings[left.Data] == right.Data
		},
	},

	// <number1> == <number2>
	// True if both numbers have the same value.
	equalsEntry{
		LeftCode:  PARSE_NUMBER,
		RightCode: PARSE_NUMBER,
		Fn: func(left *Node, right *Node, settings map[string]string) bool {
			lnum, ok := util.AtoiNoOctTry(left.Data)
			if !ok {
				return false
			}
			rnum, ok := util.AtoiNoOctTry(right.Data)
			if !ok {
				return false
			}
			return lnum == rnum
		},
	},

	// <number> == <string>
	// True if the string is a valid representation of the number.
	equalsEntry{
		LeftCode:  PARSE_NUMBER,
		RightCode: PARSE_STRING,
		Fn: func(left *Node, right *Node, settings map[string]string) bool {
			return left.Data == right.Data
		},
	},

	// <string1> == <string2>
	// True if both strings are identical (case-sensitive).
	equalsEntry{
		LeftCode:  PARSE_STRING,
		RightCode: PARSE_STRING,
		Fn: func(left *Node, right *Node, settings map[string]string) bool {
			return left.Data == right.Data
		},
	},
}

func evalEqualsOnce(
	left *Node, right *Node, settings map[string]string) (bool, bool) {

	for _, entry := range equalsDispatch {
		if entry.LeftCode == left.Code && entry.RightCode == right.Code {
			return entry.Fn(left, right, settings), true
		}
	}

	return false, false
}

// Evaluates an equals expression (`x == y`)
//
// @param left                  The fully-parsed left operand.
// @param left                  The fully-parsed right operand.
// @param settings              The map of syscfg settings.
//
// @return bool                 Whether the expression evaluates to true.
func evalEquals(
	left *Node, right *Node, settings map[string]string) (bool, error) {

	// The equals operator has special semantics.  Rather than evaluating both
	// operands as booleans and then comparing, the behavior of this operator
	// varies based on the types of operands.  Perform a table lookup using the
	// operand types, and call the appropriate comparison function if a match
	// is found.
	val, ok := evalEqualsOnce(left, right, settings)
	if ok {
		return val, nil
	}
	val, ok = evalEqualsOnce(right, left, settings)
	if ok {
		return val, nil
	}

	// No special procedure identified.  Fallback to evaluating both operands
	// as booleans and comparing the results.
	booll, boolr, err := evalTwo(left, right, settings)
	if err != nil {
		return false, err
	}
	return booll == boolr, nil
}

// Evaluates a fully-parsed expression.
//
// @param node                  The root of the expression to evaluate.
// @param settings              The map of syscfg settings.
//
// @return bool                 Whether the expression evaluates to true.
func Eval(expr *Node, settings map[string]string) (bool, error) {
	switch expr.Code {
	case PARSE_NOT:
		r, err := Eval(expr.Right, settings)
		if err != nil {
			return false, err
		}
		return !r, nil

	case PARSE_EQUALS:
		return evalEquals(expr.Left, expr.Right, settings)

	case PARSE_NOT_EQUALS:
		v, err := evalEquals(expr.Left, expr.Right, settings)
		if err != nil {
			return false, err
		}
		return !v, nil

	case PARSE_LT:
		l, r, err := coerceTwoInts(expr.Left, expr.Right, settings, "<")
		if err != nil {
			return false, err
		}
		return l < r, nil

	case PARSE_LTE:
		l, r, err := coerceTwoInts(expr.Left, expr.Right, settings, "<=")
		if err != nil {
			return false, err
		}
		return l <= r, nil

	case PARSE_GT:
		l, r, err := coerceTwoInts(expr.Left, expr.Right, settings, ">")
		if err != nil {
			return false, err
		}
		return l > r, nil

	case PARSE_GTE:
		l, r, err := coerceTwoInts(expr.Left, expr.Right, settings, ">=")
		if err != nil {
			return false, err
		}
		return l >= r, nil

	case PARSE_AND:
		l, r, err := evalTwo(expr.Left, expr.Right, settings)
		if err != nil {
			return false, err
		}
		return l && r, nil

	case PARSE_OR:
		l, r, err := evalTwo(expr.Left, expr.Right, settings)
		if err != nil {
			return false, err
		}
		return l || r, nil

	case PARSE_XOR:
		l, r, err := evalTwo(expr.Left, expr.Right, settings)
		if err != nil {
			return false, err
		}
		return (l && !r) || (!l && r), nil

	case PARSE_NUMBER:
		num, ok := util.AtoiNoOctTry(expr.Data)
		return ok && num != 0, nil

	case PARSE_STRING:
		return ValueIsTrue(expr.Data), nil

	case PARSE_IDENT:
		val := settings[expr.Data]
		return ValueIsTrue(val), nil

	default:
		return false, fmt.Errorf("invalid parse code: %d", expr.Code)
	}
}

func LexAndParse(expr string) (*Node, error) {
	tokens, err := Lex(expr)
	if err != nil {
		return nil, err
	}

	n, err := Parse(tokens)
	if err != nil {
		return nil, util.FmtNewtError("error parsing [%s]: %s",
			expr, err.Error())
	}

	return n, nil
}

// Parses and evaluates string containing a syscfg expression.
//
// @param expr                  The expression to parse.
// @param settings              The map of syscfg settings.
//
// @return bool                 Whether the expression evaluates to true.
func ParseAndEval(expr string, settings map[string]string) (bool, error) {
	n, err := LexAndParse(expr)
	if err != nil {
		return false, err
	}

	v, err := Eval(n, settings)
	return v, err
}

// Parses an expression and converts it to its normalized text form.
func NormalizeExpr(expr string) (string, error) {
	n, err := LexAndParse(expr)
	if err != nil {
		return "", err
	}

	if n == nil {
		return "", nil
	}

	return n.String(), nil
}

// Evaluates the truthfulness of a text expression.
func ValueIsTrue(val string) bool {
	if val == "" {
		return false
	}

	i, ok := util.AtoiNoOctTry(val)
	if ok && i == 0 {
		return false
	}

	return true
}
