package parse

type Operator struct {
	Code ParseCode
	Text string
}

func Combine(nodes []*Node, op Operator) *Node {
	if len(nodes) == 0 {
		return nil
	}

	// Sort all the subexpressions and apply the operator to adjacent pair.
	sorted := make([]*Node, len(nodes))
	copy(sorted, nodes)
	SortNodes(sorted)

	var iter func(nodes []*Node) *Node
	iter = func(nodes []*Node) *Node {
		if len(nodes) == 1 {
			return nodes[0]
		}

		return &Node{
			Code:  op.Code,
			Data:  op.Text,
			Left:  nodes[0],
			Right: iter(nodes[1:]),
		}
	}

	return iter(sorted)

}

func Disjunction(nodes []*Node) *Node {
	for _, n := range nodes {
		if n == nil {
			return nil
		}
	}

	return Combine(nodes, Operator{PARSE_OR, "||"})
}

func Conjunction(nodes []*Node) *Node {
	for _, n := range nodes {
		if n == nil {
			return nil
		}
	}
	return Combine(nodes, Operator{PARSE_AND, "&&"})
}
