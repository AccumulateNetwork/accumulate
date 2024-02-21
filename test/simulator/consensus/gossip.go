// Copyright 2024 The Accumulate Authors
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
		return bytes.Compare(m.self.PubKeyHash[:], n.self.PubKeyHash[:])
	})
	if !new {
		panic("attempted to adopt the same node twice")
	}
	*ptr = n
}
