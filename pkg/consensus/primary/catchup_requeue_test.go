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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// buildCert creates a quorum-signed certificate for the given validator/round
// referencing the given parents, mirroring the bullshark test helper.
func buildCert(t *testing.T, validators []*testValidator, idx int, round types.Round, parents []*types.Certificate) *types.Certificate {
	var parentDigests []types.CertificateDigest
	for _, p := range parents {
		parentDigests = append(parentDigests, p.Digest())
	}
	header := types.NewHeader(validators[idx].pub, round, 1, nil, parentDigests)
	require.NoError(t, header.Sign(validators[idx].priv))

	var sigs [][]byte
	var authors []uint16
	digest := header.Digest()
	for i, v := range validators {
		sigs = append(sigs, ed25519.Sign(v.priv, digest[:]))
		authors = append(authors, uint16(i))
	}
	return types.NewCertificate(header, sigs, authors)
}

// TestTryAdvanceRound_FreeRunsBehindFrontier pins the fix for the absorbing
// state found in run 20260820T060039Z: a validator behind the round frontier
// used to advance one round per MinRoundInterval — the same rate as the
// frontier itself — so it stayed N rounds behind forever, every header it
// authored was stale on arrival, and every transaction its workers batched
// was lost. Catch-up through rounds whose certificates already exist must be
// free-running; only the frontier advance is paced.
func TestTryAdvanceRound_FreeRunsBehindFrontier(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	p := New(Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
		// A rate limit far longer than the test: if catch-up were paced by
		// it, the assertions below could never pass.
		MinRoundInterval: time.Hour,
	}, committee, nil, d, nil)

	// Fill the DAG with full rounds 0..10 — the network has moved on.
	var prev []*types.Certificate
	for i := range validators {
		cert := buildCert(t, validators, i, 0, nil)
		require.NoError(t, d.InsertGenesis(cert))
		prev = append(prev, cert)
	}
	for r := types.Round(1); r <= 10; r++ {
		var certs []*types.Certificate
		for i := range validators {
			cert := buildCert(t, validators, i, r, prev)
			require.NoError(t, d.Insert(cert))
			certs = append(certs, cert)
		}
		prev = certs
	}

	// The node is at round 1, ten rounds behind.
	p.SetRound(1)

	// One call must free-run to the frontier despite the hour-long rate
	// limit. (The frontier advance itself — past round 10 — is paced, so the
	// node stops AT the highest full round, not beyond it.)
	p.tryAdvanceRound()
	require.Equal(t, types.Round(10), p.CurrentRound(),
		"a node behind the frontier must catch up in one pass, not one round per MinRoundInterval")

	// A second call must not blow past the frontier: quorum exists for round
	// 10, but advancing to 11 is a frontier advance and the rate limit holds.
	p.tryAdvanceRound()
	require.Equal(t, types.Round(10), p.CurrentRound(),
		"the frontier advance must still be paced by MinRoundInterval")

	p.wg.Wait()
}

// TestCleanupOldHeaders_RequeuesUncertifiedBatches pins the fix for the batch
// loss found in run 20260820T063739Z: a header that aged out of retention
// without ever becoming a certificate took its consumed batches with it —
// every transaction inside silently vanished. Cleanup must return those
// batches to their workers; batches of headers that DID certify must not be
// requeued (their commit prunes them).
func TestCleanupOldHeaders_RequeuesUncertifiedBatches(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)
	p := New(Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}, committee, nil, d, []*worker.Worker{w})

	// Two batches stored in the worker, as if consumed into headers.
	uncertified := types.NewBatch([][]byte{[]byte("lost-without-requeue")})
	certified := types.NewBatch([][]byte{[]byte("committed-elsewhere")})
	require.NoError(t, w.StoreBatch(uncertified))
	require.NoError(t, w.StoreBatch(certified))

	makeHeader := func(round types.Round, digest types.BatchDigest) *types.Header {
		h := types.NewHeader(validators[0].pub, round, 1,
			[]types.PayloadEntry{{Digest: digest, Worker: w.ID()}}, nil)
		require.NoError(t, h.Sign(validators[0].priv))
		return h
	}
	hUncert := makeHeader(1, uncertified.Digest())
	hCert := makeHeader(2, certified.Digest())

	p.pendingMu.Lock()
	p.ourHeaders[hUncert.Digest()] = hUncert
	p.ourHeaders[hCert.Digest()] = hCert
	// Round 2's header certified; round 1's never did.
	p.ourCerts[2] = &types.Certificate{Header: hCert}
	p.pendingMu.Unlock()

	// Advance far enough that both headers age out of retention.
	p.SetRound(10)
	p.cleanupOldHeaders()

	requeued := w.ConsumeAvailableBatches()
	require.Equal(t, []types.BatchDigest{uncertified.Digest()}, requeued,
		"the uncertified header's batch must be requeued, and only that one")

	p.pendingMu.Lock()
	require.Empty(t, p.ourHeaders, "both headers should be cleaned up")
	p.pendingMu.Unlock()
}

// TestCreateHeader_DedupsPayloadDigests: the requeue (never-certified headers)
// and re-proposal (never-committed batches) paths can both re-enqueue the
// same digest; a header must not list the same batch twice.
func TestCreateHeader_DedupsPayloadDigests(t *testing.T) {
	validators := []*testValidator{newTestValidator(t)}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)
	p := New(Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}, committee, nil, d, []*worker.Worker{w})

	batch := types.NewBatch([][]byte{[]byte("tx")})
	require.NoError(t, w.StoreBatch(batch))
	// Enqueue the same digest twice — requeue and re-proposal overlapping.
	w.RequeueBatches([]types.BatchDigest{batch.Digest()})
	w.RequeueBatches([]types.BatchDigest{batch.Digest()})

	// Round 1 with an empty round 0 is allowed to have no parents, which
	// keeps this test focused on the payload.
	header, err := p.createHeaderLockedWithRound(1, 1)
	require.NoError(t, err)
	require.Len(t, header.Payload, 1, "duplicate queue entries must collapse to one payload entry")
	require.Equal(t, batch.Digest(), header.Payload[0].Digest)
}

// TestGetParentCerts_IncludesWeakLinks: headers must reference certificates
// from recent older rounds, not just round-1. A certificate no header ever
// references can never enter a committed leader's causal history — 46% of
// all certificates in run 20260820T090939Z were orphaned this way (#4111).
func TestGetParentCerts_IncludesWeakLinks(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	p := New(Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}, committee, nil, d, nil)

	var prev []*types.Certificate
	for i := range validators {
		cert := buildCert(t, validators, i, 0, nil)
		require.NoError(t, d.InsertGenesis(cert))
		prev = append(prev, cert)
	}
	byRound := map[types.Round][]*types.Certificate{}
	for r := types.Round(1); r <= 5; r++ {
		var certs []*types.Certificate
		for i := range validators {
			cert := buildCert(t, validators, i, r, prev)
			require.NoError(t, d.Insert(cert))
			certs = append(certs, cert)
		}
		byRound[r] = certs
		prev = certs
	}

	parents, err := p.getParentCertsForRound(6)
	require.NoError(t, err)

	have := map[types.CertificateDigest]bool{}
	for _, d := range parents {
		require.False(t, have[d], "parent digests must be unique")
		have[d] = true
	}
	// Round-1 parents (round 5) are all present...
	for _, c := range byRound[5] {
		require.True(t, have[c.Digest()], "round-5 (round-1) parent missing")
	}
	// ...and so are weak links from older rounds within the window.
	for _, c := range byRound[2] {
		require.True(t, have[c.Digest()],
			"round-2 certificate must be weak-linked so stragglers stay reachable")
	}
}
