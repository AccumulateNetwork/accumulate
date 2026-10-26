// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"
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

// An author whose clock runs ahead stamps its headers ahead, and a voter
// whose clock is behind it by more than the bound does not vote on them
// until its own clock has caught up to within the bound: the author is
// delayed by its skew, and the time it stamps never certifies more than the
// bound ahead of the voter's clock.
func TestVote_AnAuthorWhoseClockRunsAhead(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)

	now := time.Unix(1_000_000, 0)
	skew := 10 * time.Second
	authorDAG, voterDAG := newTestDAG(), newTestDAG()
	createGenesisCertificates(t, validators, committee, authorDAG)
	createGenesisCertificates(t, validators, committee, voterDAG)
	author := New(Config{Partition: "test", KeyPair: validators[1].priv}, committee, nil, authorDAG, nil)
	author.now = func() time.Time { return now.Add(skew) }
	voterClock := now
	voter := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, voterDAG, nil)
	voter.now = func() time.Time { return voterClock }

	header, err := author.CreateHeader()
	require.NoError(t, err)
	require.Equal(t, now.Add(skew).UnixNano(), header.Timestamp, "the author stamps its own clock")

	voted := func() bool {
		voter.pendingMu.Lock()
		defer voter.pendingMu.Unlock()
		_, ok := voter.sentVotes[header.Digest()]
		return ok
	}

	voter.OnHeaderReceived(header)
	require.False(t, voted(), "a header stamped 10 s ahead must not be voted on")

	voterClock = now.Add(skew - MaxHeaderTimeAhead - time.Millisecond)
	voter.OnHeaderReceived(header)
	require.False(t, voted(), "still more than the bound ahead")

	voterClock = now.Add(skew - MaxHeaderTimeAhead)
	voter.OnHeaderReceived(header)
	require.True(t, voted(), "the rebroadcast is voted on once the voter's clock is within the bound")
}

// The time check runs before the vote-once record (#4590): a header refused
// for its time leaves no record, in memory or through the persistence hook,
// so neither its own rebroadcast nor another header from the author for the
// round is refused for it once the voter's clock has caught up.
func TestVote_HeaderRefusedForTimeLeavesNoVoteRecord(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)

	clock := time.Unix(1_000_000, 0)
	newVoter := func() (*Primary, *[]types.HeaderDigest) {
		p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, newTestDAG(), nil)
		p.now = func() time.Time { return clock }
		var persisted []types.HeaderDigest
		p.SetOnVoted(func(_ ed25519.PublicKey, _ types.Round, digest types.HeaderDigest) error {
			persisted = append(persisted, digest)
			return nil
		})
		return p, &persisted
	}
	recorded := func(p *Primary) (types.HeaderDigest, bool) {
		p.pendingMu.Lock()
		defer p.pendingMu.Unlock()
		d, ok := p.votedRounds[authorRound{string(validators[1].pub), 0}]
		return d, ok
	}

	ahead := signedHeader(t, validators[1], 0, clock.Add(MaxHeaderTimeAhead+time.Second).UnixNano())
	other := signedHeader(t, validators[1], 0, clock.UnixNano())
	require.NotEqual(t, ahead.Digest(), other.Digest())

	t.Run("another header for the round", func(t *testing.T) {
		p, persisted := newVoter()
		p.OnHeaderReceived(ahead)
		require.False(t, votedFor(p, ahead), "a header stamped past the bound is not voted on")
		_, ok := recorded(p)
		require.False(t, ok, "a header refused for its time leaves no vote record")
		require.Empty(t, *persisted, "and persists none")

		p.OnHeaderReceived(other)
		require.True(t, votedFor(p, other), "the author's header within the bound is voted on")
		d, ok := recorded(p)
		require.True(t, ok)
		require.Equal(t, other.Digest(), d)
		require.Equal(t, []types.HeaderDigest{other.Digest()}, *persisted)
	})

	t.Run("its rebroadcast once the clock has caught up", func(t *testing.T) {
		clock = time.Unix(1_000_000, 0)
		p, persisted := newVoter()
		p.OnHeaderReceived(ahead)
		require.False(t, votedFor(p, ahead))
		require.Empty(t, *persisted)

		clock = clock.Add(time.Second)
		p.OnHeaderReceived(ahead)
		require.True(t, votedFor(p, ahead), "the rebroadcast is voted on once the voter's clock is within the bound")
		require.Equal(t, []types.HeaderDigest{ahead.Digest()}, *persisted, "the record is written once, for the vote")

		p.OnHeaderReceived(other)
		require.False(t, votedFor(p, other), "the vote-once rule then holds for the round")
	})
}
