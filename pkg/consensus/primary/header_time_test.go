// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A leader's header time is its block's time (consensus spec, invariant 13),
// so a validator votes only on a header stamped no further ahead of its own
// clock than MaxHeaderTimeAhead. A header stamped further ahead is not voted
// on; the author's rebroadcast is, once the voter's clock has reached it.
func TestVote_HeaderTimeAheadOfTheVotersClock(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	genesis := createGenesisCertificates(t, validators, committee, d)
	parents := make([]types.CertificateDigest, len(genesis))
	for i, c := range genesis {
		parents[i] = c.Digest()
	}
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)

	clock := time.Unix(1_000_000, 0)
	p.now = func() time.Time { return clock }

	voted := func(h *types.Header) bool {
		p.pendingMu.Lock()
		defer p.pendingMu.Unlock()
		_, ok := p.sentVotes[h.Digest()]
		return ok
	}
	header := func(author int, ts time.Time) *types.Header {
		h := types.NewHeader(validators[author].pub, 1, 1, nil, parents)
		h.Timestamp = ts.UnixNano()
		require.NoError(t, h.Sign(validators[author].priv))
		return h
	}

	// At the bound: voted on.
	atBound := header(1, clock.Add(MaxHeaderTimeAhead))
	p.OnHeaderReceived(atBound)
	require.True(t, voted(atBound), "a header stamped at the bound must be voted on")

	// Behind the voter's clock: voted on. The executor's clamp keeps block
	// time increasing; a header stamped late is not refused.
	behind := header(2, clock.Add(-time.Hour))
	p.OnHeaderReceived(behind)
	require.True(t, voted(behind), "a header stamped behind the voter's clock must be voted on")

	// Past the bound: not voted on.
	ahead := header(3, clock.Add(MaxHeaderTimeAhead+time.Millisecond))
	p.OnHeaderReceived(ahead)
	require.False(t, voted(ahead), "a header stamped past the bound must not be voted on")

	// Its rebroadcast, once the voter's clock has reached it: voted on.
	clock = clock.Add(time.Second)
	p.OnHeaderReceived(ahead)
	require.True(t, voted(ahead), "the rebroadcast must be voted on once the voter's clock is within the bound")
}
