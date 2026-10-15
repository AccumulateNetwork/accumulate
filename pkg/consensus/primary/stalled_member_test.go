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

// TestAStalledMemberWhoseHeaderIsCertifiedIsStillHeard pins #4581: a live
// member at a round that does not complete must keep sending something its
// peers count (RoundStatus.Authors, HeardWithin). Once its own header at the
// round is certified it has no uncertified header to rebroadcast, and when it
// cannot author the next round it sent nothing a peer counts, so a partition
// that lost two of four reported 1 of 4 producing, not 2. While the round is
// stalled it sends its certified header again; while the round is healthy it
// does not.
func TestAStalledMemberWhoseHeaderIsCertifiedIsStillHeard(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	newPrimary := func(v *testValidator) *Primary {
		d := newTestDAG()
		createGenesisCertificates(t, validators, committee, d)
		p := New(Config{Partition: "test", KeyPair: v.priv}, committee, nil, d, nil)
		p.SetRound(1)
		return p
	}
	a, b := newPrimary(validators[0]), newPrimary(validators[1])

	// a's header at round 1 is certified; the round cannot complete.
	header := types.NewHeader(validators[0].pub, 1, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))
	a.pendingMu.Lock()
	a.ourCerts[1] = types.NewCertificate(header, nil, nil)
	a.pendingMu.Unlock()

	stalledFor := func(d time.Duration) {
		a.roundMu.Lock()
		a.lastRoundAdvance = time.Now().Add(-d)
		a.roundMu.Unlock()
	}

	stalledFor(time.Second)
	require.Empty(t, a.headersToRebroadcast(), "a healthy round sends no certified header again")

	stalledFor(2 * roundStallAfter)
	sent := a.headersToRebroadcast()
	require.Len(t, sent, 1, "a stalled member sends its certified header again")
	require.Equal(t, header.Digest(), sent[0].Digest(), "the same header, not a second one for the round")

	b.OnHeaderReceived(sent[0])
	st := b.RoundStatus()
	require.Equal(t, 2, st.Authors, "the peer counts the stalled member: %+v", st)
}

// TestAResentCertifiedHeaderChangesNothing pins what #4581's re-send relies
// on. A peer that voted on the header answers it by re-sending the vote it
// already made, and the author, which dropped the header when it certified it,
// makes no second certificate from those votes. A peer with no record of the
// header votes on the same digest, which is no conflicting vote, and that vote
// changes nothing either. No peer's DAG changes.
func TestAResentCertifiedHeaderChangesNothing(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	var parents []types.CertificateDigest
	newPrimary := func(v *testValidator) *Primary {
		d := newTestDAG()
		parents = parents[:0]
		for _, c := range createGenesisCertificates(t, validators, committee, d) {
			parents = append(parents, c.Digest())
		}
		p := New(Config{Partition: "test", KeyPair: v.priv, NewCertsChannelSize: 10}, committee, nil, d, nil)
		p.SetRound(1)
		return p
	}
	a := newPrimary(validators[0])
	peers := []*Primary{newPrimary(validators[1]), newPrimary(validators[2]), newPrimary(validators[3])}

	// a authors round 1 and collects its own vote; b and c vote; it certifies.
	header := types.NewHeader(validators[0].pub, 1, 1, nil, parents)
	require.NoError(t, header.Sign(validators[0].priv))
	digest := header.Digest()
	own := types.NewVote(digest, 1, 1, validators[0].pub)
	require.NoError(t, own.Sign(validators[0].priv))
	a.pendingMu.Lock()
	a.ourHeaders[digest] = header
	a.pendingVotes[digest] = []*types.Vote{own}
	a.pendingMu.Unlock()
	for _, p := range peers[:2] {
		p.OnHeaderReceived(header)
		p.pendingMu.Lock()
		vote := p.sentVotes[digest]
		p.pendingMu.Unlock()
		require.NotNil(t, vote, "a peer votes on the header")
		a.OnVoteReceived(vote)
	}
	require.Eventually(t, func() bool { return a.HasCertificateForRound(1) }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, uint64(1), a.certificatesCreated.Load())
	a.pendingMu.Lock()
	certified := a.ourCerts[1].Digest()
	a.pendingMu.Unlock()

	type peerState struct {
		vote  *types.Vote
		voted int
		round int
	}
	before := make([]peerState, len(peers))
	for i, p := range peers {
		p.pendingMu.Lock()
		before[i] = peerState{p.sentVotes[digest], len(p.votedHeaders), p.dag.RoundCount(1)}
		p.pendingMu.Unlock()
	}

	// The round stalls; a sends its certified header again, three times, and
	// every vote a peer sends back reaches a.
	a.roundMu.Lock()
	a.lastRoundAdvance = time.Now().Add(-2 * roundStallAfter)
	a.roundMu.Unlock()
	sent := a.headersToRebroadcast()
	require.Len(t, sent, 1)
	for k := 0; k < 3; k++ {
		for _, p := range peers {
			p.OnHeaderReceived(sent[0])
			p.pendingMu.Lock()
			vote := p.sentVotes[digest]
			p.pendingMu.Unlock()
			if vote != nil {
				a.OnVoteReceived(vote)
			}
		}
	}

	// A peer restarted without its vote record votes on the same digest.
	fresh := newPrimary(validators[1])
	fresh.OnHeaderReceived(sent[0])
	fresh.pendingMu.Lock()
	freshVote := fresh.sentVotes[digest]
	fresh.pendingMu.Unlock()
	require.NotNil(t, freshVote)
	require.Equal(t, digest, freshVote.HeaderDigest, "the same digest: no conflicting vote")
	a.OnVoteReceived(freshVote)
	time.Sleep(100 * time.Millisecond)

	for i, p := range peers {
		p.pendingMu.Lock()
		after := peerState{p.sentVotes[digest], len(p.votedHeaders), p.dag.RoundCount(1)}
		p.pendingMu.Unlock()
		if before[i].vote != nil {
			require.Same(t, before[i].vote, after.vote, "peer %d re-sends the vote it made, not a new one", i)
			require.Equal(t, before[i].voted, after.voted, "peer %d records no new vote", i)
		}
		require.Equal(t, before[i].round, after.round, "peer %d's DAG is unchanged", i)
	}
	require.Equal(t, uint64(1), a.certificatesCreated.Load(), "no second certificate")
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	require.Equal(t, certified, a.ourCerts[1].Digest(), "the certificate is the same")
	require.Empty(t, a.pendingVotes[digest], "no votes are collected for the certified header")
	require.NotContains(t, a.ourHeaders, digest, "the header is not pending again")
}
