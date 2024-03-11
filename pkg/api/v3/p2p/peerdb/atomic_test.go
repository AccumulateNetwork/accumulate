// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerdb

import (
	"fmt"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestRemove(t *testing.T) {
	p := make([]*PeerStatus, 5)
	for i := range p {
		p[i] = &PeerStatus{ID: peer.ID(fmt.Sprint(i))}
	}

	cases := []struct {
		Remove []int
		Expect []int
	}{
		{[]int{2}, []int{0, 1, 3, 4}},
		{[]int{1, 3}, []int{0, 2, 4}},
		{[]int{1, 2, 3}, []int{0, 4}},
		{[]int{0}, []int{1, 2, 3, 4}},
		{[]int{4}, []int{0, 1, 2, 3}},
		{[]int{0, 1, 2, 3, 4}, []int{}},
	}

	mk := func(i []int) []*PeerStatus {
		q := make([]*PeerStatus, len(i))
		for j, i := range i {
			q[j] = p[i]
		}
		return q
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Case %d", i), func(t *testing.T) {
			l := new(AtomicSlice[*PeerStatus, PeerStatus])
			for _, p := range p {
				l.Insert(p)
			}

			l.Remove(mk(c.Remove)...)
			require.Equal(t, mk(c.Expect), l.Load())
		})
	}
}
