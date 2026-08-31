// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// startNode starts a node and stops it when the test ends. Submit requires a
// started node.
func startNode(t *testing.T, node *consensus.Node) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = node.Start(ctx) }()
	require.Eventually(t, func() bool {
		return node.SubmitTransactionFor("probe", []byte("probe")) == nil
	}, 5*time.Second, 20*time.Millisecond, "node should accept submissions once started")
	t.Cleanup(cancel)
	return ctx, cancel
}

// pending snapshots every worker's pending count, so a test can measure what
// IT submitted rather than whatever the readiness probe left behind.
func pending(node *consensus.Node) []int {
	out := make([]int, len(node.Workers()))
	for i, w := range node.Workers() {
		out[i] = w.PendingCount()
	}
	return out
}

func nodeWithWorkers(t *testing.T, n int) *consensus.Node {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	committee := types.NewCommittee([]types.ValidatorInfo{{PublicKey: pub, Stake: 100}}, 1)
	node, err := consensus.NewNode(consensus.NodeConfig{
		Partition:  "BVN1",
		KeyPair:    priv,
		NumWorkers: n,
	}, committee, nil, nil)
	require.NoError(t, err)
	return node
}

// The defect, stated as a property: everything from one signer must be handled
// by ONE worker.
//
// Round-robin routing spread a signer's transactions across workers, which
// batched them independently and committed them out of order; replay
// protection then rejected all but an increasing subsequence. 96 of 100
// transactions from the treasury were lost that way, silently (#4132).
func TestSubmitRouting_OneSignerLandsOnOneWorker(t *testing.T) {
	for _, n := range []int{2, 4, 64, 128} {
		node := nodeWithWorkers(t, n)
		require.Len(t, node.Workers(), n)

		const signer = "acc://f4a327b7cfbe971258b5a24c5ba3529bda09d8078ed35fac/ACME"
		want := node.WorkerFor(signer)
		for i := 0; i < 500; i++ {
			assert.Equal(t, want, node.WorkerFor(signer),
				"a signer must route to a stable worker (numWorkers=%d)", n)
		}
		require.GreaterOrEqual(t, want, 0)
		require.Less(t, want, n)
	}
}

// ...while distinct signers still spread, or the the worker fan-out buys nothing.
func TestSubmitRouting_DistinctSignersSpreadAcrossWorkers(t *testing.T) {
	node := nodeWithWorkers(t, 64)
	seen := map[int]bool{}
	for i := 0; i < 2000; i++ {
		seen[node.WorkerFor(fmt.Sprintf("acc://signer-%d.acme/book/1", i))] = true
	}
	assert.Len(t, seen, 64, "distinct signers should reach every worker")
}

// Submitting for a signer actually places the transaction in that signer's
// worker — routing that is computed but not used would be worse than none.
func TestSubmitRouting_TransactionLandsInTheRoutedWorker(t *testing.T) {
	node := nodeWithWorkers(t, 8)
	ctx, cancel := startNode(t, node)
	defer cancel()

	const signer = "acc://alice.acme/book/1"
	w := node.WorkerFor(signer)

	base := pending(node)
	require.NoError(t, node.SubmitTransactionFor(signer, []byte("tx-one")))
	after := pending(node)

	assert.Equal(t, base[w]+1, after[w], "the routed worker should hold the transaction")
	for i := range after {
		if i == w {
			continue
		}
		assert.Equal(t, base[i], after[i], "worker %d should not have taken it", i)
	}
	_ = ctx
}

// A burst from one signer must all land together — this is the exact shape
// that failed: 100 transactions from the treasury in one second.
func TestSubmitRouting_BurstFromOneSignerStaysTogether(t *testing.T) {
	node := nodeWithWorkers(t, 64)
	ctx, cancel := startNode(t, node)
	defer cancel()

	const signer = "acc://treasury.acme/ACME"
	w := node.WorkerFor(signer)

	base := pending(node)
	for i := 0; i < 100; i++ {
		require.NoError(t, node.SubmitTransactionFor(signer, []byte(fmt.Sprintf("tx-%d", i))))
	}
	after := pending(node)

	assert.Equal(t, base[w]+100, after[w], "all 100 must land in the signer's worker")
	for i := range after {
		if i != w {
			require.Equal(t, base[i], after[i], "worker %d must be untouched", i)
		}
	}
	_ = ctx
}

// The old behaviour, kept and named. SubmitTransaction has no key, so it still
// round-robins — callers that know the sender must use SubmitTransactionFor.
func TestSubmitRouting_KeylessSubmitStillRoundRobins(t *testing.T) {
	node := nodeWithWorkers(t, 4)
	ctx, cancel := startNode(t, node)
	defer cancel()

	base := pending(node)
	for i := 0; i < 8; i++ {
		require.NoError(t, node.SubmitTransaction([]byte(fmt.Sprintf("tx-%d", i))))
	}
	after := pending(node)
	for i := range after {
		assert.Equal(t, base[i]+2, after[i],
			"round-robin should have given worker %d an even share", i)
	}
	_ = ctx
}

// An EMPTY key is not a key. It used to be hashed like any other, and the hash
// of a constant is a constant — so a caller passing "" put every transaction in
// the network into one worker, holding all of that node's own uncommitted
// batches inside 1/N of the partition's byte budget (#4179).
func TestSubmitRouting_EmptyKeyRoundRobinsInsteadOfPinning(t *testing.T) {
	for _, n := range []int{2, 4, 8} {
		node := nodeWithWorkers(t, n)
		ctx, cancel := startNode(t, node)

		base := pending(node)
		for i := 0; i < 4*n; i++ {
			require.NoError(t, node.SubmitTransactionFor("", []byte(fmt.Sprintf("tx-%d", i))))
		}
		after := pending(node)

		for i := range after {
			assert.Equal(t, base[i]+4, after[i],
				"numWorkers=%d: worker %d should have taken an even share of keyless traffic", n, i)
		}
		cancel()
		_ = ctx
	}
}

// The constant that did the damage, pinned so it cannot come back by accident:
// hashing an empty routing key picks worker 1, not worker 0, so the defect was
// invisible to anyone checking whether traffic all landed in the first worker.
func TestSubmitRouting_HashingAnEmptyKeyIsAConstant(t *testing.T) {
	for _, n := range []int{2, 4} {
		node := nodeWithWorkers(t, n)
		assert.Equal(t, 1, node.WorkerFor(""),
			"numWorkers=%d: the empty key hashes to worker 1 — this is why it must not be hashed", n)
	}
}

// Received batches must spread rather than all landing in worker 0.
//
// Both intake paths used to store every batch this node did not create into
// worker 0, while MaxStoredBatches is enforced per worker — so worker 0 filled
// and evicted far sooner than the rest, dropping exactly the batches peers ask
// for (#4133, and the failure #4128 is about).
func TestBatchStore_ReceivedBatchesSpreadAcrossWorkers(t *testing.T) {
	node := nodeWithWorkers(t, 16)
	store := node.BatchStore()
	require.NotNil(t, store)

	for i := 0; i < 800; i++ {
		require.NoError(t, store.StoreBatch(
			types.NewBatch([][]byte{[]byte(fmt.Sprintf("remote-%d", i))})))
	}

	used, worst := 0, 0
	for _, wk := range node.Workers() {
		c := wk.BatchCount()
		if c > 0 {
			used++
		}
		if c > worst {
			worst = c
		}
	}
	assert.Greater(t, used, 1, "received batches must not all land in one worker")
	assert.Less(t, worst, 800, "no single worker should hold every received batch")
}

// A batch is retrievable no matter which worker stored it, or spreading the
// writes would break every read.
func TestBatchStore_SpreadBatchesAreStillFindable(t *testing.T) {
	node := nodeWithWorkers(t, 16)
	store := node.BatchStore()

	var digests []types.BatchDigest
	for i := 0; i < 200; i++ {
		b := types.NewBatch([][]byte{[]byte(fmt.Sprintf("remote-%d", i))})
		digests = append(digests, b.Digest())
		require.NoError(t, store.StoreBatch(b))
	}
	for _, d := range digests {
		found := false
		for _, wk := range node.Workers() {
			if b, err := wk.GetBatch(d); err == nil && b != nil {
				found = true
				break
			}
		}
		assert.True(t, found, "a stored batch must be findable: %v", d)
	}
}

// Worker counts that are not powers of two still route in range — a running
// deployment must not be refused service over a configuration opinion.
func TestSubmitRouting_OddWorkerCountsStillWork(t *testing.T) {
	for _, n := range []int{1, 3, 100} {
		node := nodeWithWorkers(t, n)
		for i := 0; i < 100; i++ {
			w := node.WorkerFor(fmt.Sprintf("acc://signer-%d", i))
			require.GreaterOrEqual(t, w, 0)
			require.Less(t, w, n, "numWorkers=%d must route in range", n)
		}
	}
}

var _ = worker.GonePruned // keep the worker import meaningful across edits
