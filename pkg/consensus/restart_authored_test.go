// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// TestCheckpointKeepsTheAuthoredHeadersBatches: a header this node authored
// and no certificate has named yet keeps its batches across the block
// checkpoints written while it waits for votes, and a restart reloads them
// into the batch store. Before the fix the checkpoint's Retain removed every
// batch no certificate named, the restored header came back without its
// payload, no peer could vote for it, and its author could not author the
// round again: run 20261002T090546Z, BVN3 held at round 25839 with two of
// the three certificates it needed, the two batches absent from every
// validator's disk.
func TestCheckpointKeepsTheAuthoredHeadersBatches(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 100}}, 1)
	dir := t.TempDir()
	cfg := NodeConfig{
		Partition:    "bvn0",
		KeyPair:      priv,
		NumWorkers:   1,
		WorkerConfig: worker.Config{BatchSize: 5},
		StateDir:     dir,
	}
	n, err := NewNode(cfg, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The worker holds a batch; the primary authors a header naming it,
	// which persists the header and the batch (persistAuthored).
	b := &types.Batch{Transactions: [][]byte{[]byte("a transaction")}}
	if err := n.workers[0].StoreBatch(b); err != nil {
		t.Fatal(err)
	}
	h := types.NewHeader(pub, 7, 1, []types.PayloadEntry{{Digest: b.Digest()}}, nil)
	if err := h.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := n.persistAuthored(h); err != nil {
		t.Fatal(err)
	}
	if !n.dagStore.HasBatch(b.Digest()) {
		t.Fatal("authoring did not persist the header's batch")
	}

	// A block checkpoint follows, as one does after every block the
	// executor produces: nothing certified names the batch.
	cp := n.CheckpointAt(0)
	if !n.dagStore.HasBatch(b.Digest()) {
		t.Fatal("the checkpoint removed the batch of the header this node authored and no certificate names yet")
	}

	// Restarted from that checkpoint, the node serves the batch again.
	n2, err := NewNode(cfg, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := n2.Restore(cp); err != nil {
		t.Fatal(err)
	}
	if !n2.workers[0].CanServeBatch(b.Digest()) {
		t.Fatal("the restarted node does not hold the batch its restored header names")
	}
}
