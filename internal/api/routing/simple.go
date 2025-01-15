// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func BuildSimpleTable(bvns []string) []protocol.Route {
	return buildSimpleTable(bvns, 0, 0)
}

func buildSimpleTable(bvns []string, value, depth uint64) []protocol.Route {
	if len(bvns) > 1 {
		value <<= 1
		depth++
		i := len(bvns) / 2
		a := buildSimpleTable(bvns[:i], value|0, depth) //nolint clarity
		b := buildSimpleTable(bvns[i:], value|1, depth)
		return append(a, b...)
	}

	return []protocol.Route{{
		Length:    depth,
		Value:     value,
		Partition: bvns[0],
	}}
}
