// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A certificate held before a seed is taken after it. A joining node's DAG
// has a floor of zero until it is seeded, so every certificate gossip brings
// it meanwhile is held for want of parents. The seed makes the ones at its
// round and the round after it acceptable without parents -- and while they
// stayed held, every later copy was refused as already held, the node's DAG
// stayed empty, and it ordered nothing: three runs in four of the libp2p join
// test once the join seeded below its state instead of at it (#4530).
func TestASeedForgetsWhatWasHeldBeforeIt(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)
	p.requestParents = func([]types.CertificateDigest) {}

	cert := func(author int, round types.Round) *types.Certificate {
		h := types.NewHeader(validators[author].pub, round, 1, nil, []types.CertificateDigest{{byte(author), byte(round)}})
		require.NoError(t, h.Sign(validators[author].priv))
		sigs := make([][]byte, len(validators))
		idx := make([]uint16, len(validators))
		for i, v := range validators {
			vote := types.NewVote(h.Digest(), round, 1, v.pub)
			require.NoError(t, vote.Sign(v.priv))
			sigs[i], idx[i] = vote.Signature, uint16(i)
		}
		return types.NewCertificate(h, sigs, idx)
	}

	// Before the seed: the frontier's certificates arrive and are held.
	at, after, above := cert(1, 40), cert(2, 41), cert(3, 42)
	for _, c := range []*types.Certificate{at, after, above} {
		require.Equal(t, CertBuffered, p.receiveCertificate(c, false))
	}
	require.Equal(t, 3, p.PendingCertsCount())

	// The seed, as Node.Rejoin makes it.
	p.SetRound(40)
	d.SetLastCommitRound(40)
	require.Equal(t, 3, p.ForgetHeld())
	require.Zero(t, p.PendingCertsCount())

	// Delivered again, the ones at the seed and the round after it are
	// taken; the one above still wants its parents and is held again.
	require.Equal(t, CertInserted, p.receiveCertificate(at, false), "a certificate at the seed is still held from before it")
	require.Equal(t, CertInserted, p.receiveCertificate(after, false))
	require.Equal(t, CertBuffered, p.receiveCertificate(above, false))
	require.True(t, d.Contains(at.Digest()))
	require.True(t, d.Contains(after.Digest()))
}

// A certificate asked for before a seed is asked for again at once after it.
// A joining node asks for the weak-link parents of the frontier certificates
// it holds; before the seed the answer is held for want of parents in turn,
// and a certificate held does not check, so its digest stays in flight
// (#4597). The seed forgot the held certificate but not the request, and the
// node's next ask for it went unsent for the five-second deduplication
// interval and then until the pending rescan, fifteen seconds after: the
// round two above the seed waited on it, and the libp2p join test pulled past
// its first proven state on every CI run once #4596 made a held certificate
// not check (#4610).
func TestASeedForgetsWhatItAskedForBeforeIt(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)
	p.requestParents = func([]types.CertificateDigest) {}
	config := CertSyncerConfig{PublicKey: validators[0].pub, Partition: "test"}
	config.applyDefaults()
	s := NewCertSyncer(config, d, nil, p.pendingCerts)
	s.SetCertReceivedCallback(func(c *types.Certificate) CertOutcome { return p.receiveCertificate(c, false) })
	p.certSyncer = s

	h := types.NewHeader(validators[1].pub, 38, 1, nil, []types.CertificateDigest{{1, 37}})
	require.NoError(t, h.Sign(validators[1].priv))
	sigs := make([][]byte, len(validators))
	idx := make([]uint16, len(validators))
	for i, v := range validators {
		vote := types.NewVote(h.Digest(), h.Round, h.Epoch, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i], idx[i] = vote.Signature, uint16(i)
	}
	parent := types.NewCertificate(h, sigs, idx)

	// Before the seed: asked for, answered, and held.
	s.RequestMissing([]types.CertificateDigest{parent.Digest()})
	s.sendBatchRequest()
	s.handleSyncResponse(&gossip.CertSyncResponse{Certificates: []*types.Certificate{parent}})
	require.Equal(t, 1, p.PendingCertsCount())

	// The seed, as Node.Rejoin makes it.
	p.SetRound(40)
	d.SetLastCommitRound(40)
	p.ForgetHeld()

	// Asked for again, it is sent with the next batch.
	s.RequestMissing([]types.CertificateDigest{parent.Digest()})
	s.batchQueueMu.Lock()
	queued := append([]types.CertificateDigest(nil), s.batchQueue...)
	s.batchQueueMu.Unlock()
	require.Contains(t, queued, parent.Digest(), "a certificate asked for before the seed is not asked for again after it")
}

// A node seeded at S asks, with each round of catch-up, for the rounds below S
// that the certificates above it name as weak links. A header names every
// certificate of the eight rounds below it but the last, so those at S + 2
// name S - 6 to S; the DAG takes S and S + 1 without parents, and the
// certificates above wait for the rounds below S, which round catch-up --
// one round below the node's own, upward -- never asked for. They were asked
// for by digest only, and an answer lost was asked for again only by the
// pending rescan fifteen seconds after: the node sat at S + 2, the join's
// five-second wait ran out, and it pulled past its first proven state (the
// libp2p join test in CI, 77 s against 56 s for a pass; #4610). Asked for by
// round, they come with the next second's request.
func TestASeedAsksForTheRoundsItsCertificatesNameBelowIt(t *testing.T) {
	validators := []*testValidator{newTestValidator(t)}
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, newTestCommittee(validators, 1), nil, newTestDAG(), nil)

	// Not seeded: one request, from one round below the node's own.
	requests := p.catchUpRequests(40, 100)
	require.Len(t, requests, 1)
	require.Equal(t, types.Round(39), requests[0][0])

	p.SetRoundAndSeed(40)
	requests = p.catchUpRequests(40, 100)
	require.Len(t, requests, 2, "the rounds S+2 names below the seed are not asked for")
	require.Equal(t, types.Round(39), requests[0][0], "the node's own rounds come first")
	require.Len(t, requests[0], 62)
	require.Equal(t, []types.Round{34, 35, 36, 37, 38}, requests[1], "S-1 is in the node's own request")
	require.Equal(t, []types.Round{34, 35, 36, 37, 38, 39}, p.catchUpRequests(48, 100)[1], "S+7 names S-1")

	// Past S + 8 no certificate still to come names a round below the seed.
	require.Len(t, p.catchUpRequests(49, 100), 1)
}

// What a seeded node asks for below its seed must not crowd out of the answer
// the rounds it needs to advance. An answer carries at most
// MaxSyncCertificates, the rounds in the order asked; asked for ahead of the
// node's own rounds, the eight rounds S-6..S+1 of 13 validators were 104
// certificates, and a node at S+2..S+8 got none of its own round's (the
// review of !1278). A round below the seed the node holds whole is not asked
// for again.
func TestASeedsCatchUpAnswersCarryItsOwnRound(t *testing.T) {
	for _, n := range []int{4, 12, 13, 20} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			const seed = types.Round(40)
			validators := make([]*testValidator, n)
			for i := range validators {
				validators[i] = newTestValidator(t)
			}

			// The peer answering holds every validator's certificate of
			// every round up to S + 12.
			var certs []*types.Certificate
			for r := types.Round(1); r <= seed+12; r++ {
				for _, v := range validators {
					certs = append(certs, types.NewCertificate(types.NewHeader(v.pub, r, 1, nil, nil), nil, nil))
				}
			}
			held := dag.NewDAG(100)
			require.NoError(t, held.Restore(certs))
			peer := NewCertSyncer(CertSyncerConfig{Partition: "test"}, held, nil, nil)

			d := newTestDAG()
			p := New(Config{Partition: "test", KeyPair: validators[0].priv}, newTestCommittee(validators, 1), nil, d, nil)
			p.SetRoundAndSeed(seed)

			for current := seed + 2; current <= seed+weakLinkWindow; current++ {
				got := map[types.Round]int{}
				for i, rounds := range p.catchUpRequests(current, current+4) {
					resp := peer.answer(&gossip.CertSyncRequest{Rounds: rounds, Requester: validators[0].pub, RequestID: uint64(current)*10 + uint64(i)})
					require.NotNil(t, resp)
					for _, c := range resp.Certificates {
						got[c.Round()]++
					}
				}
				require.Equal(t, n, got[current], "certificates of round %d, the node's own", current)
				t.Logf("n=%d current=S+%d: %d of the node's round", n, current-seed, got[current])
			}

			// The rounds below the seed come too, as many answers as they
			// take, and once held whole are not asked for again.
			for i := 0; ; i++ {
				requests := p.catchUpRequests(seed+2, seed+6)
				if len(requests) == 1 {
					break
				}
				require.Less(t, i, 3, "rounds held whole are asked for again")
				resp := peer.answer(&gossip.CertSyncRequest{Rounds: requests[1], Requester: validators[0].pub, RequestID: uint64(i)})
				require.NoError(t, d.Restore(resp.Certificates))
			}
			for r := seed + 2 - weakLinkWindow; r < seed; r++ {
				require.Len(t, d.GetRound(r), n, "round %d below the seed", r)
			}
		})
	}
}
