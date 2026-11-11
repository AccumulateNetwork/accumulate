// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// signedHeader is v's header for round, with the given parents, differing
// from v's other headers for the round only in its timestamp.
func signedHeader(t *testing.T, v *testValidator, round types.Round, timestamp int64, parents ...types.CertificateDigest) *types.Header {
	h := types.NewHeader(v.pub, round, 1, nil, parents)
	h.Timestamp = timestamp
	require.NoError(t, h.Sign(v.priv))
	return h
}

// TestVoteOncePerAuthorRound: a validator signs one header per author and
// round, the first it votes for (Narwhal 4.1, #4590). Two headers from one
// author for one round, differing only in timestamp, otherwise both reached
// a quorum of honest votes; each node then kept whichever certificate
// arrived first and refused the other for good, and the partition split on
// which one was real.
func TestVoteOncePerAuthorRound(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	var genesis []types.CertificateDigest
	for _, c := range createGenesisCertificates(t, validators, committee, d) {
		genesis = append(genesis, c.Digest())
	}
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)

	first := signedHeader(t, validators[1], 0, 1)
	second := signedHeader(t, validators[1], 0, 2)
	require.NotEqual(t, first.Digest(), second.Digest())

	p.OnHeaderReceived(first)
	p.OnHeaderReceived(second)
	require.True(t, votedFor(p, first), "the first header from the author gets the vote")
	require.False(t, votedFor(p, second), "a second header from the same author for the same round must not be voted")

	// The rule is per author and round: the author's next round and another
	// author's header for this round are voted as before.
	p.SetRound(1)
	nextRound := signedHeader(t, validators[1], 1, 3, genesis...)
	p.OnHeaderReceived(nextRound)
	require.True(t, votedFor(p, nextRound))
	other := signedHeader(t, validators[2], 0, 4)
	p.OnHeaderReceived(other)
	require.True(t, votedFor(p, other))
}

// TestVoteRecordIsPersistedBeforeSigning: the (author, round) -> header
// record is handed to the persistence hook before the vote exists, and a
// record that cannot be persisted is not voted: a restart would otherwise
// have no memory of the vote and could sign another header for the pair.
func TestVoteRecordIsPersistedBeforeSigning(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, newTestDAG(), nil)
	header := signedHeader(t, validators[1], 0, 1)

	var calls int
	fail := errors.New("disk full")
	p.SetOnVoted(func(author ed25519.PublicKey, round types.Round, digest types.HeaderDigest) error {
		calls++
		require.Equal(t, validators[1].pub, author)
		require.Equal(t, types.Round(0), round)
		require.Equal(t, header.Digest(), digest)
		require.False(t, votedFor(p, header), "the record is persisted before the vote is signed")
		if calls == 1 {
			return fail
		}
		return nil
	})

	p.OnHeaderReceived(header)
	require.Equal(t, 1, calls)
	require.False(t, votedFor(p, header), "a vote whose record could not be persisted is not sent")

	// The author rebroadcasts; the record persists this time and the vote
	// goes out.
	p.OnHeaderReceived(header)
	require.Equal(t, 2, calls)
	require.True(t, votedFor(p, header))

	// A resend of a vote already sent persists nothing again.
	p.OnHeaderReceived(header)
	require.Equal(t, 2, calls)
}

// TestRestoredVoteRecordRefusesTheOtherHeader: a primary given the record of
// a vote it signed before a restart refuses a different header from that
// author for that round, and still votes for the recorded one when the
// author rebroadcasts it.
func TestRestoredVoteRecordRefusesTheOtherHeader(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	recorded := signedHeader(t, validators[1], 0, 1)
	other := signedHeader(t, validators[1], 0, 2)

	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, newTestDAG(), nil)
	p.RestoreVoted(validators[1].pub, 0, recorded.Digest())

	p.OnHeaderReceived(other)
	require.False(t, votedFor(p, other), "the restarted primary must not vote for a header other than the one it recorded")
	p.OnHeaderReceived(recorded)
	require.True(t, votedFor(p, recorded), "the recorded header is voted for again")
}

// TestPrunedRoundIsNeverVotedAgain: a round whose vote records were pruned
// from memory is never voted in again, even when a rejoin lowers the round
// below it (#4590). Before the fix the pruned round came back with no record
// of what had been voted there, and a second header from an author for it
// was voted.
func TestPrunedRoundIsNeverVotedAgain(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	var parents []types.CertificateDigest
	for _, c := range createGenesisCertificates(t, validators, committee, d) {
		parents = append(parents, c.Digest())
	}
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)

	p.SetRound(2)
	first := signedHeader(t, validators[1], 1, 1, parents...)
	second := signedHeader(t, validators[1], 1, 2, parents...)
	p.OnHeaderReceived(first)
	require.True(t, votedFor(p, first))

	// The round moves on until the record of round 1 is pruned.
	p.SetRound(12)
	p.cleanupOldHeaders()
	p.pendingMu.Lock()
	_, kept := p.votedRounds[authorRound{string(validators[1].pub), 1}]
	p.pendingMu.Unlock()
	require.False(t, kept, "round 1 should have been pruned from memory")

	// A rejoin lowers the round, and round 1 is votable again.
	p.SetRound(1)
	p.OnHeaderReceived(second)
	require.False(t, votedFor(p, second), "a header for a round whose record was pruned must not be voted")
	p.OnHeaderReceived(first)
	_, _, _, sent := p.Metrics()
	require.Equal(t, uint64(1), sent, "the header voted before the prune is not voted a second time")
}
