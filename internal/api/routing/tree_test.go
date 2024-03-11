// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestNewRouteTree(t *testing.T) {
	routes := buildSimpleTable([]string{"A", "B", "C", "D", "E", "F", "G"}, 0, 0)
	tree, err := NewRouteTree(&protocol.RoutingTable{Routes: routes})
	require.NoError(t, err)
	require.Equal(t, &RouteTree{
		overrides: map[[32]uint8]string{},
		root: prefixTreeBranch{
			bits: 2,
			children: []prefixTreeNode{
				prefixTreeLeaf("A"),
				prefixTreeBranch{
					bits: 1,
					children: []prefixTreeNode{
						prefixTreeLeaf("B"),
						prefixTreeLeaf("C"),
					},
				},
				prefixTreeBranch{
					bits: 1,
					children: []prefixTreeNode{
						prefixTreeLeaf("D"),
						prefixTreeLeaf("E"),
					},
				},
				prefixTreeBranch{
					bits: 1,
					children: []prefixTreeNode{
						prefixTreeLeaf("F"),
						prefixTreeLeaf("G"),
					},
				},
			},
		},
	}, tree)
}

func FuzzRouteTree_Route(f *testing.F) {
	routes := buildSimpleTable([]string{"A", "B", "C", "D", "E", "F", "G"}, 0, 0)
	tree, err := NewRouteTree(&protocol.RoutingTable{Routes: routes})
	require.NoError(f, err)

	f.Add("foo")
	f.Add("bar/baz")
	f.Fuzz(func(t *testing.T, s string) {
		t.Parallel()
		u, err := url.Parse(s)
		if err != nil {
			t.Skip()
		}

		_, err = tree.Route(u)
		require.NoError(t, err)
	})
}
