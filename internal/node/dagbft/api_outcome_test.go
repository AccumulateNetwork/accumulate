// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// TestSubmitter_EverySubmitIsAcceptedOrRejectedNeverBoth — #4404 item 4.
//
// Run 20260924T052134Z's submissions.csv read accepted 4,086 and rejected
// 4,086 for acc-bvn1-val1/BVN1 at 05:46:10, and the run-analyst asked whether
// a refused submission is also counted accepted — if it were, the stranded
// figure (accepted - certified - relayed{taken} - relayed{refused}) would
// overstate the loss. The harness can only trust the figure if the two
// outcomes partition Submit calls: every call moves exactly one of them by
// exactly one. This drives every road out of Submit that a unit test can
// reach and checks that sum, not each counter alone.
func TestSubmitter_EverySubmitIsAcceptedOrRejectedNeverBoth(t *testing.T) {
	const part = "bvn1"
	no := false
	hash := []byte("0123456789abcdef0123456789abcdef")

	cases := []struct {
		name string
		sub  func(t *testing.T) *SubmitterService
		env  *messaging.Envelope
		opts api.SubmitOptions
	}{
		{"relayed and taken", func(t *testing.T) *SubmitterService {
			svc, _, mine := newJoiningService(t)
			val, target := otherKey(t), peer.ID("a-validator")
			m := NewMembership(part, mine)
			m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{part: {val}}))
			return NewSubmitterService(SubmitterServiceParams{Service: svc, Membership: m,
				Relay: NewRelay(RelayParams{Partition: part, Membership: m,
					Peers: &fakePeers{self: "self", peers: []peer.ID{target}},
					RPC:   &fakeRPC{partition: part, keys: map[peer.ID]ed25519.PublicKey{target: val}}})})
		}, new(messaging.Envelope), api.SubmitOptions{}},
		{"relayed, no target (not-ready)", func(t *testing.T) *SubmitterService {
			svc, _, mine := newJoiningService(t)
			m := NewMembership(part, mine) // no globals: no committee known
			return NewSubmitterService(SubmitterServiceParams{Service: svc, Membership: m,
				Relay: NewRelay(RelayParams{Partition: part, Membership: m,
					Peers: &fakePeers{self: "self", peers: []peer.ID{"somebody"}},
					RPC:   &fakeRPC{partition: part}})})
		}, new(messaging.Envelope), api.SubmitOptions{Verify: &no}},
		{"cannot propose, no relay", func(t *testing.T) *SubmitterService {
			svc, _, mine := newJoiningService(t)
			m := NewMembership(part, mine)
			m.SetGlobals(globalsWith(t, map[string][]ed25519.PublicKey{part: {otherKey(t)}}))
			return NewSubmitterService(SubmitterServiceParams{Service: svc, Membership: m})
		}, new(messaging.Envelope), api.SubmitOptions{}},
		{"undecodable envelope", func(t *testing.T) *SubmitterService {
			svc, _, _ := newJoiningService(t)
			return NewSubmitterService(SubmitterServiceParams{Service: svc})
		}, &messaging.Envelope{TxHash: []byte("short")}, api.SubmitOptions{}},
		{"own worker refuses", func(t *testing.T) *SubmitterService {
			svc, _, _ := newJoiningService(t)
			return NewSubmitterService(SubmitterServiceParams{Service: svc})
		}, &messaging.Envelope{TxHash: hash}, api.SubmitOptions{Verify: &no}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sub := c.sub(t)
			a0, r0, _, _, _ := counted(part)
			_, _ = sub.Submit(context.Background(), c.env, c.opts)
			a1, r1, _, _, _ := counted(part)
			require.Equal(t, float64(1), (a1-a0)+(r1-r0),
				"one Submit call moves accepted OR rejected, by one: accepted +%v, rejected +%v", a1-a0, r1-r0)
		})
	}
}
