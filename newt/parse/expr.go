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

// ExprSet is a set of expressions.  The map value is the expression itself;
// the key is the string representation of the expression, obtained via
// `Node#String()`.  This is implemented as a map in this way to prevent
// duplicate expressions.
type ExprSet map[string]*Node

// ExprMap is a collection of named expression sets.
type ExprMap map[string]ExprSet

func NewExprSet(exprs []*Node) ExprSet {
	if len(exprs) == 0 {
		return nil
	}

	es := make(ExprSet, len(exprs))
	es.Add(exprs)
	return es
}

// Exprs returns a slice of all expressions contained in the expression set.
// The resulting slice is sorted.
func (es ExprSet) Exprs() []*Node {
	if len(es) == 0 {
		return nil
	}

	nodes := make([]*Node, 0, len(es))
	for _, expr := range es {
		nodes = append(nodes, expr)
	}
	SortNodes(nodes)
	return nodes
}

// Add adds a set of expressions to an expression set.
func (es ExprSet) Add(exprs []*Node) {
	for _, e := range exprs {
		es[e.String()] = e
	}
}

// Disjunction ORs together all the elements in an expression set, producing a
// single expression.
func (es ExprSet) Disjunction() *Node {
	exprs := es.Exprs()

	if len(exprs) == 0 {
		return nil
	}

	// Recursively OR the first expression with the rest.
	var iter func(nodes []*Node) *Node
	iter = func(nodes []*Node) *Node {
		if len(nodes) == 1 {
			return nodes[0]
		}

		return &Node{
			Code:  PARSE_OR,
			Data:  "||",
			Left:  nodes[0],
			Right: iter(nodes[1:]),
		}
	}

	return iter(exprs)
}

// Add adds a set of expressions to an expression map.
func (m ExprMap) Add(key string, exprs []*Node) {
	if len(exprs) == 0 {
		return
	}

	// Create a new expression set if this is a new key.
	es := m[key]
	if es == nil {
		es = ExprSet{}
		m[key] = es
	}

	es.Add(exprs)
}
