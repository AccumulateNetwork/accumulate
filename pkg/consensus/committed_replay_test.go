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
	"fmt"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// TestCommittedBatchStoredAgainAfterExecutionIsCache: a certificate protects
// its batches in the active store only until this node executes them
// (consensus spec, invariant 3). Retention is capped by time and bytes and
// can be off, while the DAG keeps the certificate for 2,000 rounds; a copy
// that arrives after retention dropped it was stored as certified and never
// pruned again, so a peer could replay executed batches into a store that
// never let them go (#4598). The node marks what it executed in the DAG when
// it prunes, and the late copy is cache the flood evicts.
func TestCommittedBatchStoredAgainAfterExecutionIsCache(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 100}}, 1)
	n, err := NewNode(NodeConfig{
		Partition:  "bvn0",
		KeyPair:    priv,
		NumWorkers: 1,
		WorkerConfig: worker.Config{
			BatchSize:           5,
			MaxBatchBytes:       1 << 10,
			MaxStoredBatchBytes: 64 << 10,
			MaxRetainedBatches:  -1, // retention off: the copy has nowhere else to go
		},
	}, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := n.workers[0]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Start(ctx) }() // the eviction loop; blocks until ctx ends

	cert := func(round types.Round, payload []types.PayloadEntry, parents []types.CertificateDigest) *types.Certificate {
		h := types.NewHeader(pub, round, 1, payload, parents)
		if err := h.Sign(priv); err != nil {
			t.Fatal(err)
		}
		d := h.Digest()
		return &types.Certificate{Header: h, Signatures: [][]byte{ed25519.Sign(priv, d[:])}, SignedAuthorities: []uint16{0}}
	}
	genesis := cert(0, nil, nil)
	if err := n.dag.InsertGenesis(genesis); err != nil {
		t.Fatal(err)
	}

	// A peer's batch this node holds, certified, committed and executed.
	b := types.NewBatch([][]byte{[]byte("a peer batch this node executed")})
	if err := w.StoreBatch(b); err != nil {
		t.Fatal(err)
	}
	c := cert(1, []types.PayloadEntry{{Digest: b.Digest()}}, []types.CertificateDigest{genesis.Digest()})
	if err := n.dag.Insert(c); err != nil {
		t.Fatal(err)
	}
	n.PruneCommitted(c, worker.CommitInfo{Detail: "block 1", Cert: c.Digest().String()})
	if w.HasBatch(b.Digest()) {
		t.Fatal("pruned, the batch left the active store")
	}

	// The same batch arrives again, then a flood of batches nothing names.
	if err := w.StoreBatch(b); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		u := types.NewBatch([][]byte{[]byte(fmt.Sprintf("unnamed-%04d-%01000d", i, 0))})
		if err := w.StoreBatch(u); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for w.HasBatch(b.Digest()) || w.BatchCount() > 64 {
		if time.Now().After(deadline) {
			t.Fatalf("an executed batch stored again is still protected: held=%v, %d batches over a budget of 64",
				w.HasBatch(b.Digest()), w.BatchCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
