// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"bytes"

	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
)

type Gossip struct {
	nodes []*Node
}

func (g *Gossip) adopt(n *Node) {
	ptr, new := sortutil.BinaryInsert(&g.nodes, func(m *Node) int {
		return bytes.Compare(m.pubKeyHash[:], n.pubKeyHash[:])
	})
	if !new {
		panic("attempted to adopt the same node twice")
	}
	*ptr = n
}
