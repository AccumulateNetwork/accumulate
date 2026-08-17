// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// devnetNodes builds the node layout devnet.go produces for a given shape,
// using the same IsVal rule: the first Validators nodes of each BVN validate,
// the rest are followers.
func devnetNodes(t *testing.T, bvns, validators, followers int) [][]*nodeOpts {
	t.Helper()
	perPart := validators + followers
	nodes := make([][]*nodeOpts, bvns)
	for bvn := range nodes {
		nodes[bvn] = make([]*nodeOpts, perPart)
		for node := range nodes[bvn] {
			// AddValidator takes PrivVal[32:], i.e. the public half.
			pub, _, err := ed25519.GenerateKey(nil)
			require.NoError(t, err)
			privVal := make([]byte, 64)
			copy(privVal[32:], pub)

			nodes[bvn][node] = &nodeOpts{
				BVN:     bvn + 1,
				Node:    node + 1,
				IsVal:   node < validators,
				PrivVal: privVal,
			}
		}
	}
	return nodes
}

// active counts the validators marked Active for a partition.
func active(n *protocol.NetworkDefinition, partition string) (activeN, total int) {
	for _, v := range n.Validators {
		for _, p := range v.Partitions {
			if p.ID == partition {
				total++
				if p.Active {
					activeN++
				}
			}
		}
	}
	return
}

// A devnet with followers must produce a network where the followers are NOT
// voting validators. Before the fix every node was added active regardless of
// IsVal, so there was no way to create a non-validating node at all.
func TestDevnetValidators_FollowersAreNotActive(t *testing.T) {
	cases := []struct {
		name                           string
		bvns, validators, followers    int
		wantActiveDN, wantRegisteredDN int
	}{
		{"2 validators 1 follower", 1, 2, 1, 2, 3},
		{"1 validator 2 followers", 1, 1, 2, 1, 3},
		{"no followers", 1, 3, 0, 3, 3},
		{"two bvns", 2, 2, 1, 4, 6},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nodes := devnetNodes(t, c.bvns, c.validators, c.followers)

			n := new(protocol.NetworkDefinition)
			n.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)
			for bvn := 0; bvn < c.bvns; bvn++ {
				n.AddPartition(fmt.Sprintf("BVN%d", bvn+1), protocol.PartitionTypeBlockValidator)
			}
			addDevnetValidators(n, nodes)

			gotActive, gotTotal := active(n, protocol.Directory)

			// Followers stay registered — they are part of the network and
			// gossip; they simply do not vote. Dropping them entirely would
			// be a different and wrong fix.
			require.Equal(t, c.wantRegisteredDN, gotTotal, "every node should be registered")
			require.Equal(t, c.wantActiveDN, gotActive, "only validators should be active")

			// Each BVN sees only its own nodes.
			for bvn := 0; bvn < c.bvns; bvn++ {
				a, tot := active(n, fmt.Sprintf("BVN%d", bvn+1))
				require.Equal(t, c.validators, a)
				require.Equal(t, c.validators+c.followers, tot)
			}
		})
	}
}

// The reproduction from the issue: Bvns 1, Validators 1, Followers 2 yielded
// 3 active validators.
func TestDevnetValidators_IssueReproduction(t *testing.T) {
	nodes := devnetNodes(t, 1, 1, 2)

	n := new(protocol.NetworkDefinition)
	n.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)
	n.AddPartition("BVN1", protocol.PartitionTypeBlockValidator)
	addDevnetValidators(n, nodes)

	a, total := active(n, protocol.Directory)
	require.Equal(t, 3, total, "all three nodes are part of the network")
	require.Equal(t, 1, a, "issue #4078: this used to be 3")
}
