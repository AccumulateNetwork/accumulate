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

// TestRestoredAuthoredBatchesSurviveEviction: the persisted tail is a whole
// rescue window of batches and can be larger than the active store. The
// restart loads it all, the store evicts down to its limit — and the batch
// this node's own restored header names must not be among what goes: its
// author is the only node that holds it, every voter defers until it can
// fetch it from the author, and an author that cannot serve it can neither
// certify the round nor author it again. Run 20261006T220257Z: all three
// BVNs restored round 269626 one certificate short and stayed there; val2
// evicted 126 of its 176 restored batches three seconds after restoring
// them, its own header's batch among them (#4545).
func TestRestoredAuthoredBatchesSurviveEviction(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 100}}, 1)
	dir := t.TempDir()
	const batchBytes = 1024
	cfg := NodeConfig{
		Partition:  "bvn0",
		KeyPair:    priv,
		NumWorkers: 1,
		WorkerConfig: worker.Config{
			BatchSize:           5,
			MaxBatchBytes:       batchBytes,
			MaxStoredBatchBytes: 4 * batchBytes, // the store holds four peer batches
		},
		StateDir: dir,
	}
	n, err := NewNode(cfg, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// The tail on disk: twenty peer certificates, each naming a batch this
	// node holds, and the batch the authored header names.
	var certs []*types.Certificate
	for i := 0; i < 20; i++ {
		tx := make([]byte, batchBytes-64)
		tx[0], tx[1] = byte(i), 'p'
		b := types.NewBatch([][]byte{tx})
		if err := n.workers[0].StoreBatch(b); err != nil {
			t.Fatal(err)
		}
		peer, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		payload := []types.PayloadEntry{{Digest: b.Digest()}}
		certs = append(certs, &types.Certificate{Header: types.NewHeader(peer, types.Round(i+1), 0, payload, nil)})
	}
	if err := n.dag.Restore(certs); err != nil {
		t.Fatal(err)
	}
	own := types.NewBatch([][]byte{[]byte("the batch this node's header names")})
	if err := n.workers[0].StoreBatch(own); err != nil {
		t.Fatal(err)
	}
	h := types.NewHeader(pub, 7, 1, []types.PayloadEntry{{Digest: own.Digest()}}, nil)
	if err := h.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := n.persistAuthored(h); err != nil {
		t.Fatal(err)
	}
	cp := n.CheckpointAt(0)
	if got := len(cp.Tail); got != 41 {
		t.Fatalf("the checkpoint's tail names %d files, not 20 certificates and 21 batches", got)
	}

	// Restarted, with the store's eviction loop running.
	n2, err := NewNode(cfg, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	n2.Restore(cp)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = n2.workers[0].Start(ctx) }() // blocks until ctx ends
	deadline := time.Now().Add(2 * time.Second)
	for n2.workers[0].BatchCount() > 4 {
		if time.Now().After(deadline) {
			t.Fatalf("the store did not evict down to its limit: %d batches", n2.workers[0].BatchCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !n2.workers[0].CanServeBatch(own.Digest()) {
		t.Fatal("eviction took the batch of the header this node authored: no peer can vote for it now")
	}
}
