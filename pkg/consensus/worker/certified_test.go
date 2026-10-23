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

// A batch that arrives again after it executed is already in retention and
// servable from there. Storing it in the active store again would hold it
// there for as long as the DAG keeps the certificate that named it, which is
// thousands of rounds after it executed, and the active store's bound would
// no longer be what the spec says.
func TestCertified_ExecutedBatchStoredAgainStaysRetained(t *testing.T) {
	executed := types.NewBatch([][]byte{[]byte("a batch that executed, then arrived again")})
	ed := executed.Digest()
	cs := &certifiedSet{m: map[types.BatchDigest]bool{ed: true}}
	w := New(Config{
		Partition:           "t",
		MaxBatchBytes:       1 << 10,
		MaxStoredBatchBytes: 64 << 10,
		Certified:           cs.has,
	}, nil)

	require.NoError(t, w.StoreBatch(executed))
	w.AwaitExecution([]types.BatchDigest{ed})
	w.PruneCommitted([]types.BatchDigest{ed}, CommitInfo{Detail: "executed"})

	// Gossip delivers it late, while the certificate is still in the DAG.
	require.NoError(t, w.StoreBatch(executed))
	require.False(t, w.HasBatch(ed), "an executed batch is retained, not active, however often it arrives")
	require.True(t, w.CanServeBatch(ed))

	storeUnnamed(t, w, "unnamed", 200)
	require.LessOrEqual(t, w.peerBytes(), 64<<10, "the active store stays within its budget")
}

// NameBatches is the vote gate's one look at the store: it marks what the
// header names and reports what this worker holds in neither store. Checking
// and marking in two steps let eviction take a batch between them, and the
// vote went out for a batch the voter no longer held.
func TestNameBatches_ReportsMissingAndMarksActive(t *testing.T) {
	w := New(Config{
		Partition:           "t",
		MaxBatchBytes:       1 << 10,
		MaxStoredBatchBytes: 64 << 10,
	}, nil)

	active := types.NewBatch([][]byte{[]byte("held in the active store")})
	retained := types.NewBatch([][]byte{[]byte("executed and retained")})
	unstored := types.NewBatch([][]byte{[]byte("never seen")})
	ad, rd, ud := active.Digest(), retained.Digest(), unstored.Digest()

	require.NoError(t, w.StoreBatch(active))
	require.NoError(t, w.StoreBatch(retained))
	w.PruneCommitted([]types.BatchDigest{rd}, CommitInfo{Detail: "executed"})
	require.True(t, w.CanServeBatch(rd))

	missing := w.NameBatches([]types.BatchDigest{ad, rd, ud})
	require.Equal(t, []types.BatchDigest{ud}, missing, "only the batch held nowhere is missing")

	w.batchMu.RLock()
	defer w.batchMu.RUnlock()
	require.True(t, w.batches[ad].named, "the active batch is marked as named")
	_, inActive := w.batches[rd]
	require.False(t, inActive, "naming a retained batch does not move it to the active store")
}
