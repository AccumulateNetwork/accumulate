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
// restart loads it all and the store evicts what it may — not the batches
// the restored certificates name (consensus spec, invariant 3), and not the
// batch this node's own restored header names, which no certificate names
// yet and so is evicted first unless it is held as this node's own: its
// author is the only node that holds it, every voter defers until it can
// fetch it from the author, and an author that cannot serve it can neither
// certify the round nor author it again. Run 20261006T220257Z: all three
// BVNs restored round 269626 one certificate short and stayed there; val2
// evicted 126 of its 176 restored batches three seconds after restoring
// them, its own header's batch among them (#4545).
//
// The checkpoint is at round 0: none of the twenty certificates has
// committed, so each still proves this node holds its batch and the store
// keeps all twenty over a budget of four. That is what invariant 3 asks for:
// a certified, unexecuted batch is held until it executes or the DAG
// collects its round, and it is the certificates, not the budget, that bound
// the store then.
func TestRestoredAuthoredBatchesSurviveEviction(t *testing.T) {
	n2, certs, own := restoredTailFixture(t, 0, 0)

	deadline := time.Now().Add(2 * time.Second)
	for n2.workers[0].BatchCount() > 21 {
		if time.Now().After(deadline) {
			t.Fatalf("the store did not evict the batches nothing names: %d batches", n2.workers[0].BatchCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, c := range certs {
		if !n2.workers[0].HasBatch(c.Header.Payload[0].Digest) {
			t.Fatal("eviction took a batch a restored certificate names")
		}
	}
	if !n2.workers[0].CanServeBatch(own.Digest()) {
		t.Fatal("eviction took the batch of the header this node authored: no peer can vote for it now")
	}
}

// TestRestoredExecutedBatchesAreRetained: every certificate in the tail is
// in the checkpoint's committed set, so its batches executed before the
// checkpoint. Loaded into the active store, they would sit there, certified
// and never pruned, for the 2,000 rounds the DAG keeps the certificate —
// twenty batches over a budget of four. They are what retention is for:
// servable to a peer that fell behind, owed to no vote (#4598). The first
// certificate is this node's own: its batch executed like the rest and is
// retained, not held as own, or the stale-own re-proposal would propose an
// executed batch again (invariant 7).
func TestRestoredExecutedBatchesAreRetained(t *testing.T) {
	n2, certs, own := restoredTailFixture(t, 20, 20)

	deadline := time.Now().Add(2 * time.Second)
	for n2.workers[0].BatchCount() > 4 {
		if time.Now().After(deadline) {
			t.Fatalf("the store did not evict down to its budget: %d batches", n2.workers[0].BatchCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, c := range certs {
		d := c.Header.Payload[0].Digest
		if n2.workers[0].HasBatch(d) {
			t.Fatal("an executed batch was restored into the active store")
		}
		if !n2.workers[0].CanServeBatch(d) {
			t.Fatal("an executed batch was not restored into retention")
		}
	}
	if !n2.workers[0].CanServeBatch(own.Digest()) {
		t.Fatal("eviction took the batch of the header this node authored: no peer can vote for it now")
	}
}

// TestRestoredUncommittedCertificateAtTheFloorKeepsItsBatch: the commit
// floor is round 20, but a certificate at the floor that no committed
// leader's history reaches has not committed. Whether a batch executed is
// the checkpoint's committed set, not the round: certificates 1 to 19 are in
// it and their batches go to retention; certificate 20 is not, and its batch
// stays in the active store on the certificate's account (invariant 3).
func TestRestoredUncommittedCertificateAtTheFloorKeepsItsBatch(t *testing.T) {
	n2, certs, own := restoredTailFixture(t, 20, 19)

	// The budget's worth plus the one batch the certificate protects.
	deadline := time.Now().Add(2 * time.Second)
	for n2.workers[0].BatchCount() > 5 {
		if time.Now().After(deadline) {
			t.Fatalf("the store did not evict down to its budget: %d batches", n2.workers[0].BatchCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i, c := range certs {
		d := c.Header.Payload[0].Digest
		if i == 19 {
			if !n2.workers[0].HasBatch(d) {
				t.Fatal("the batch of a certificate not in the committed set left the active store")
			}
			continue
		}
		if n2.workers[0].HasBatch(d) {
			t.Fatalf("certificate %d is committed, but its batch was restored into the active store", i+1)
		}
		if !n2.workers[0].CanServeBatch(d) {
			t.Fatalf("certificate %d is committed, but its batch was not restored into retention", i+1)
		}
	}
	if !n2.workers[0].CanServeBatch(own.Digest()) {
		t.Fatal("eviction took the batch of the header this node authored: no peer can vote for it now")
	}
}

// restoredTailFixture writes a checkpoint whose tail is twenty certificates
// at rounds 1 to 20 — the first this node's own, the rest peers' — each
// naming one batch of about 1 KiB this node holds, and the header this node
// authored with its batch; restores it into a node whose store holds four
// peer batches, with the eviction loop running; and sends eight batches no
// header names. lastCommit is the checkpoint's commit floor, and the first
// committed certificates are in its committed set: 0 leaves every
// certificate uncommitted.
func restoredTailFixture(t *testing.T, lastCommit types.Round, committed int) (*Node, []*types.Certificate, *types.Batch) {
	t.Helper()
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
		author := pub
		if i > 0 {
			if author, _, err = ed25519.GenerateKey(rand.Reader); err != nil {
				t.Fatal(err)
			}
		}
		payload := []types.PayloadEntry{{Digest: b.Digest()}}
		certs = append(certs, &types.Certificate{Header: types.NewHeader(author, types.Round(i+1), 0, payload, nil)})
	}
	if err := n.dag.Restore(certs); err != nil {
		t.Fatal(err)
	}
	own := types.NewBatch([][]byte{[]byte("the batch this node's header names")})
	if err := n.workers[0].StoreBatch(own); err != nil {
		t.Fatal(err)
	}
	h := types.NewHeader(pub, lastCommit+7, 1, []types.PayloadEntry{{Digest: own.Digest()}}, nil)
	if err := h.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := n.persistAuthored(h); err != nil {
		t.Fatal(err)
	}
	if lastCommit > 0 {
		n.bullshark.SetLastCommitRound(lastCommit)
		n.primary.SetRound(lastCommit + 1)
	}
	for _, c := range certs[:committed] {
		n.bullshark.MarkCommitted(c)
	}
	cp := n.CheckpointAt(0)
	if cp.LastCommitRound != lastCommit {
		t.Fatalf("the checkpoint's commit floor is %d, not %d", cp.LastCommitRound, lastCommit)
	}
	if len(cp.Committed) != committed {
		t.Fatalf("the checkpoint's committed set holds %d certificates, not %d", len(cp.Committed), committed)
	}
	if got := len(cp.Tail); got != 41 {
		t.Fatalf("the checkpoint's tail names %d files, not 20 certificates and 21 batches", got)
	}

	// Restarted, with the store's eviction loop running.
	n2, err := NewNode(cfg, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := n2.Restore(cp); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = n2.workers[0].Start(ctx) }() // blocks until ctx ends

	// Peer batches no header names arrive; they are what the store evicts.
	for i := 0; i < 8; i++ {
		tx := make([]byte, batchBytes-64)
		tx[0], tx[1] = byte(i), 'u'
		if err := n2.workers[0].StoreBatch(types.NewBatch([][]byte{tx})); err != nil {
			t.Fatal(err)
		}
	}
	return n2, certs, own
}
