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
	"strings"

	"mynewt.apache.org/newt/util"
)

type TokenCode int

const (
	TOKEN_NOT_EQUALS TokenCode = iota
	TOKEN_NOT
	TOKEN_EQUALS
	TOKEN_LT
	TOKEN_LTE
	TOKEN_GT
	TOKEN_GTE
	TOKEN_AND
	TOKEN_OR
	TOKEN_XOR
	TOKEN_LPAREN
	TOKEN_RPAREN
	TOKEN_STRING
	TOKEN_NUMBER
	TOKEN_IDENT
)

type Token struct {
	Code   TokenCode
	Text   string
	Offset int
}

// Returns length of token on success; 0 if no match.
type LexFn func(s string) (string, int, error)

const delimChars = "!='\"&|^() \t\n"

func lexString(s string, sought string) (string, int, error) {
	if strings.HasPrefix(s, sought) {
		return sought, len(sought), nil
	}

	return "", 0, nil
}

func lexStringFn(sought string) LexFn {
	return func(s string) (string, int, error) { return lexString(s, sought) }
}

func lexLitNumber(s string) (string, int, error) {
	var sub string
	idx := strings.IndexAny(s, delimChars)
	if idx == -1 {
		sub = s
	} else {
		sub = s[:idx]
	}
	if _, ok := util.AtoiNoOctTry(sub); ok {
		return sub, len(sub), nil
	}

	return "", 0, nil
}

func lexLitString(s string) (string, int, error) {
	if s[0] != '"' {
		return "", 0, nil
	}

	quote2 := strings.IndexByte(s[1:], '"')
	if quote2 == -1 {
		return "", 0, fmt.Errorf("unterminated quote: %s", s)
	}

	return s[1 : quote2+1], quote2 + 2, nil
}

func lexIdent(s string) (string, int, error) {
	idx := strings.IndexAny(s, delimChars)
	if idx == -1 {
		return s, len(s), nil
	} else {
		return s[:idx], idx, nil
	}
}

type lexEntry struct {
	code TokenCode
	fn   LexFn
}

var lexEntries = []lexEntry{
	{TOKEN_NOT_EQUALS, lexStringFn("!=")},
	{TOKEN_EQUALS, lexStringFn("==")},
	{TOKEN_AND, lexStringFn("&&")},
	{TOKEN_OR, lexStringFn("||")},
	{TOKEN_XOR, lexStringFn("^^")},
	{TOKEN_LTE, lexStringFn("<=")},
	{TOKEN_GTE, lexStringFn(">=")},
	{TOKEN_NOT, lexStringFn("!")},
	{TOKEN_LT, lexStringFn("<")},
	{TOKEN_GT, lexStringFn(">")},
	{TOKEN_LPAREN, lexStringFn("(")},
	{TOKEN_RPAREN, lexStringFn(")")},
	{TOKEN_STRING, lexLitString},
	{TOKEN_NUMBER, lexLitNumber},
	{TOKEN_IDENT, lexIdent},
}

func lexOneToken(expr string, offset int) (Token, int, error) {
	var t Token

	subexpr := expr[offset:]
	for _, e := range lexEntries {
		text, sz, err := e.fn(subexpr)
		if err != nil {
			return t, 0, err
		}

		if sz != 0 {
			t.Code = e.code
			t.Text = text
			t.Offset = offset
			return t, sz, nil
		}
	}

	return t, 0, nil
}

func skipSpaceLeft(s string, offset int) int {
	sub := s[offset:]
	newSub := strings.TrimLeft(sub, " \t\n'")
	return len(sub) - len(newSub)
}

// Tokenizes a string.
func Lex(expr string) ([]Token, error) {
	tokens := []Token{}
	off := 0

	off += skipSpaceLeft(expr, off)
	for off < len(expr) {
		t, skip, err := lexOneToken(expr, off)
		if err != nil {
			return nil, err
		}

		if skip == 0 {
			return nil, fmt.Errorf("Invalid token starting with: %s", expr)
		}

		tokens = append(tokens, t)

		off += skip
		off += skipSpaceLeft(expr, off)
	}

	return tokens, nil
}

// Produces a string representation of a token sequence.
func SprintfTokens(tokens []Token) string {
	s := ""

	for _, t := range tokens {
		switch t.Code {
		case TOKEN_NUMBER:
			s += fmt.Sprintf("#[%s] ", t.Text)
		case TOKEN_IDENT:
			s += fmt.Sprintf("i[%s] ", t.Text)
		case TOKEN_STRING:
			s += fmt.Sprintf("\"%s\" ", t.Text)
		default:
			s += fmt.Sprintf("%s ", t.Text)
		}
	}

	return s
}
