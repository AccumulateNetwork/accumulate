// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consim

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// TestEveryValidatorHearsEveryOther pins #4586. A GossipSub created while its
// host already has connections dials those peers at once, and a peer with no
// GossipSub yet fails the negotiation and is never sent to on that connection:
// the validator whose GossipSub was created last could hear nobody. Here that
// validator's GossipSub is created late on purpose, and every validator must
// still hear every other one's subscription.
func TestEveryValidatorHearsEveryOther(t *testing.T) {
	const vals = 4
	defer func(old func(int)) { beforeGossipSub = old }(beforeGossipSub)
	beforeGossipSub = func(i int) {
		if i == vals-1 {
			time.Sleep(300 * time.Millisecond)
		}
	}

	sim, err := New(Config{BVNs: 1, ValidatorsPerBVN: vals})
	require.NoError(t, err)
	defer sim.Close()

	const topic = "consim-test-hears-every-other"
	var dn []*simNode
	for _, sn := range sim.byPart["Directory"] {
		dn = append(dn, sn)
		tp, err := sn.ps.Join(topic)
		require.NoError(t, err)
		sub, err := tp.Subscribe()
		require.NoError(t, err)
		defer sub.Cancel()
	}
	require.Len(t, dn, vals)

	for _, sn := range dn {
		want := map[peer.ID]bool{}
		for _, other := range dn {
			if other.val != sn.val {
				want[sim.hosts[other.val].ID()] = true
			}
		}
		require.Eventually(t, func() bool {
			heard := map[peer.ID]bool{}
			for _, p := range sn.ps.ListPeers(topic) {
				heard[p] = true
			}
			for p := range want {
				if !heard[p] {
					return false
				}
			}
			return true
		}, 5*time.Second, 50*time.Millisecond, "validator %d does not hear every other validator", sn.val)
	}
}
