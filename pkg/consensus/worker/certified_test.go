// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// certifiedSet stands in for dag.HasCertifiedBatch: the batches a certified
// header in the DAG names, until the DAG collects that header's round.
type certifiedSet struct {
	mu sync.Mutex
	m  map[types.BatchDigest]bool
}

func (c *certifiedSet) has(d types.BatchDigest) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[d]
}

func (c *certifiedSet) set(d types.BatchDigest, on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[d] = on
}

// storeUnnamed stores n batches of about 1 KiB that no header names, each
// through StoreBatch as gossip does, running eviction after each one.
func storeUnnamed(t *testing.T, w *Worker, tag string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		b := types.NewBatch([][]byte{[]byte(fmt.Sprintf("%s-%04d-%01000d", tag, i, 0))})
		require.NoError(t, w.StoreBatch(b))
		w.performEviction()
	}
}

// A certificate is a proof that 2f+1 validators hold the batches its header
// names (Narwhal sec. 4), so a voter's copy of a certified batch is not cache
// until the certificate is executed (consensus spec, invariant 3). A batch
// carries no author, so any peer can send batches nobody names; a stream of
// them evicted the voter's copy, leaving the author's as the only one.
func TestCertified_SurvivesUnnamedPeerBatches(t *testing.T) {
	certified := types.NewBatch([][]byte{[]byte("a batch a certificate names")})
	cd := certified.Digest()
	cs := &certifiedSet{m: map[types.BatchDigest]bool{cd: true}}
	w := New(Config{
		Partition:           "t",
		MaxBatchBytes:       1 << 10,
		MaxStoredBatchBytes: 64 << 10,
		Certified:           cs.has,
	}, nil)

	require.NoError(t, w.StoreBatch(certified))
	storeUnnamed(t, w, "unnamed", 200)

	require.True(t, w.HasBatch(cd), "a batch an uncommitted certificate names was evicted")
	require.LessOrEqual(t, w.peerBytes(), 64<<10, "unnamed batches are still evicted to the budget")

	// Committed and executed: the executor retires it to retention, so the
	// active store no longer holds it on the certificate's account.
	w.AwaitExecution([]types.BatchDigest{cd})
	w.PruneCommitted([]types.BatchDigest{cd}, CommitInfo{Detail: "executed"})
	require.False(t, w.HasBatch(cd))
	require.True(t, w.CanServeBatch(cd), "retained for peers that fell behind")
}

// A certificate never committed stops protecting its batches once the DAG
// collects its round: the store's bound does not depend on every certificate
// committing.
func TestCertified_CollectedRoundRestoresEvictability(t *testing.T) {
	certified := types.NewBatch([][]byte{[]byte("a batch an orphaned certificate names")})
	cd := certified.Digest()
	cs := &certifiedSet{m: map[types.BatchDigest]bool{cd: true}}
	w := New(Config{
		Partition:           "t",
		MaxBatchBytes:       1 << 10,
		MaxStoredBatchBytes: 64 << 10,
		Certified:           cs.has,
	}, nil)

	require.NoError(t, w.StoreBatch(certified))
	storeUnnamed(t, w, "before", 100)
	require.True(t, w.HasBatch(cd))

	cs.set(cd, false) // the DAG collected the round
	storeUnnamed(t, w, "after", 100)
	require.False(t, w.HasBatch(cd), "nothing names it any more, so it is cache again")
}

// Between this node's vote and the certificate reaching its DAG, a header it
// voted on names the batch. Batches no header names are evicted before it,
// so a stream of them cannot take it; batches headers do name still bound
// the store, oldest first.
func TestNamed_VotedBatchSurvivesUnnamedPeerBatches(t *testing.T) {
	w := New(Config{
		Partition:           "t",
		MaxBatchBytes:       1 << 10,
		MaxStoredBatchBytes: 64 << 10,
	}, nil)

	voted := types.NewBatch([][]byte{[]byte("a batch this node voted for")})
	vd := voted.Digest()
	require.NoError(t, w.StoreBatch(voted))
	w.NameBatches([]types.BatchDigest{vd})

	storeUnnamed(t, w, "unnamed", 200)
	require.True(t, w.HasBatch(vd), "unnamed batches evicted a batch a voted header names")

	for i := 0; i < 100; i++ {
		b := types.NewBatch([][]byte{[]byte(fmt.Sprintf("named-%04d-%01000d", i, 0))})
		require.NoError(t, w.StoreBatch(b))
		w.NameBatches([]types.BatchDigest{b.Digest()})
		w.performEviction()
	}
	require.False(t, w.HasBatch(vd), "named batches beyond the budget are evicted, oldest first")
	require.LessOrEqual(t, w.peerBytes(), 64<<10)
}
