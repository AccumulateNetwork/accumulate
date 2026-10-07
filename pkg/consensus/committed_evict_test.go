// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// TestCommittedBatchesSurviveEvictionUntilExecuted: a node whose executor is
// behind holds the batches of every certificate consensus committed and the
// executor has not reached, and its store can be over its limit with them. A
// follower casts no vote, so no vote pin covers them; the store evicted them,
// the validators had retired them once executed, and nothing could serve
// them again. Run 20261007T050444Z: bvn2-fol1 fell 300 blocks behind and
// waited on one such batch for good — 2,613 asks, no peer held it (consensus
// spec, invariant 3).
func TestCommittedBatchesSurviveEvictionUntilExecuted(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 100}}, 1)
	const batchBytes = 1024
	n, err := NewNode(NodeConfig{
		Partition:        "bvn0",
		KeyPair:          priv,
		NumWorkers:       1,
		CommitBufferSize: 1000,
		WorkerConfig: worker.Config{
			BatchSize:           5,
			MaxBatchBytes:       batchBytes,
			MaxStoredBatchBytes: 4 * batchBytes, // the store holds four peer batches
		},
	}, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := n.workers[0]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Start(ctx) }() // blocks until ctx ends
	n.ctx = ctx                       // order hands groups over under it; Start is not run

	// A chain of rounds, each certificate naming one peer's batch, run
	// through ordering. Nothing executes: the executor is behind.
	cert := func(round types.Round, parent *types.Certificate, b *types.Batch) *types.Certificate {
		var parents []types.CertificateDigest
		if parent != nil {
			parents = []types.CertificateDigest{parent.Digest()}
		}
		var payload []types.PayloadEntry
		if b != nil {
			payload = []types.PayloadEntry{{Digest: b.Digest()}}
		}
		h := types.NewHeader(pub, round, 1, payload, parents)
		if err := h.Sign(priv); err != nil {
			t.Fatal(err)
		}
		d := h.Digest()
		return &types.Certificate{Header: h, Signatures: [][]byte{ed25519.Sign(priv, d[:])}, SignedAuthorities: []uint16{0}}
	}
	prev := cert(0, nil, nil)
	if err := n.dag.InsertGenesis(prev); err != nil {
		t.Fatal(err)
	}
	var batches []*types.Batch
	for r := types.Round(1); r <= 20; r++ {
		tx := make([]byte, batchBytes-64)
		tx[0], tx[1] = byte(r), 'p'
		b := types.NewBatch([][]byte{tx})
		batches = append(batches, b)
		c := cert(r, prev, b)
		if err := n.dag.Insert(c); err != nil {
			t.Fatal(err)
		}
		if !n.order(c) {
			t.Fatal("ordering stopped")
		}
		prev = c
	}

	var groups [][]*types.Certificate
	for len(n.committed) > 0 {
		groups = append(groups, <-n.committed)
	}
	committed := map[types.BatchDigest]bool{}
	for _, g := range groups {
		for _, c := range g {
			for _, e := range c.Header.Payload {
				committed[e.Digest] = true
			}
		}
	}
	if len(committed) <= 4 {
		t.Fatalf("only %d batches committed; the test needs more than the store holds", len(committed))
	}

	// The batches arrive after the commit, as a lagging node fetches them,
	// and the eviction loop runs over the limit.
	for _, b := range batches {
		if err := w.StoreBatch(b); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(300 * time.Millisecond)
	for _, b := range batches {
		if committed[b.Digest()] && !w.HasBatch(b.Digest()) {
			t.Fatalf("eviction took the batch of a committed certificate the executor has not reached (%d committed, %d stored)",
				len(committed), w.BatchCount())
		}
	}

	// Executed and retired, they are the store's to evict again.
	var digests []types.BatchDigest
	for d := range committed {
		digests = append(digests, d)
	}
	w.PruneCommitted(digests, worker.CommitInfo{})
	if got := w.AwaitingExecution(); got != 0 {
		t.Fatalf("%d batches still marked awaiting execution after they were retired", got)
	}
}
