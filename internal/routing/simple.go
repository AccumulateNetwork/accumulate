package routing

import (
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func BuildSimpleTable(net *config.Network) []protocol.Route {
	return buildSimpleTable(net.GetBvnNames(), 0, 0)
}

func buildSimpleTable(bvns []string, value, depth uint64) []protocol.Route {
	if len(bvns) > 1 {
		value <<= 1
		depth++
		i := len(bvns) / 2
		a := buildSimpleTable(bvns[:i], value|0, depth)
		b := buildSimpleTable(bvns[i:], value|1, depth)
		return append(a, b...)
	}

	return []protocol.Route{{
		Length:    depth,
		Value:     value,
		Partition: bvns[0],
	}}
}
