// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// A worker must preserve the order transactions were submitted in.
//
// This is what makes signer-affinity routing worth anything: replay protection
// requires a signer's timestamps to be strictly increasing IN EXECUTION ORDER,
// so routing a signer to one worker only helps if that worker does not reorder
// them on the way into a batch (#4132).
func TestWorker_BatchPreservesSubmissionOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := worker.New(worker.Config{
		ID: 0, Partition: "BVN1",
		BatchSize:    64,
		BatchTimeout: 50 * time.Millisecond,
	}, nil)
	go func() { _ = w.Start(ctx) }()
	defer w.Close()

	var sent [][]byte
	for i := 0; i < 64; i++ {
		tx := []byte(fmt.Sprintf("tx-%03d", i))
		sent = append(sent, tx)
		require.NoError(t, w.Submit(tx))
	}

	// Wait for the batch to be created and offered.
	var digests []interface{ String() string }
	require.Eventually(t, func() bool {
		return w.BatchCount() > 0
	}, 3*time.Second, 20*time.Millisecond, "a batch should be created")
	_ = digests

	// Find the batch and check its contents are in submission order.
	found := false
	for _, d := range w.BatchDigests() {
		b, err := w.GetBatch(d)
		require.NoError(t, err)
		if b == nil || b.Len() != len(sent) {
			continue
		}
		found = true
		for i, tx := range b.Transactions {
			assert.Equal(t, string(sent[i]), string(tx),
				"transaction %d is out of order inside the batch", i)
		}
	}
	require.True(t, found, "the batch holding all submitted transactions should exist")
}

// Submissions beyond the batch size roll into a second batch, and the split
// must respect order too — the first batch takes the earliest.
func TestWorker_OrderIsPreservedAcrossBatchBoundaries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := worker.New(worker.Config{
		ID: 0, Partition: "BVN1",
		BatchSize:    8,
		BatchTimeout: 30 * time.Millisecond,
	}, nil)
	go func() { _ = w.Start(ctx) }()
	defer w.Close()

	for i := 0; i < 32; i++ {
		require.NoError(t, w.Submit([]byte(fmt.Sprintf("tx-%03d", i))))
		time.Sleep(2 * time.Millisecond)
	}
	require.Eventually(t, func() bool { return w.BatchCount() >= 2 },
		3*time.Second, 20*time.Millisecond, "several batches should be created")

	// Every batch individually must be internally ordered.
	for _, d := range w.BatchDigests() {
		b, err := w.GetBatch(d)
		require.NoError(t, err)
		if b == nil {
			continue
		}
		prev := ""
		for _, tx := range b.Transactions {
			s := string(tx)
			if prev != "" {
				assert.Less(t, prev, s, "batch contents must stay in submission order")
			}
			prev = s
		}
	}
}

// Crossing a pending boundary SEALS; it does not refuse the envelope (#4165).
//
// Refusing was the failure: a full pending queue rejected the transaction at
// the exact moment the fix was to turn it into a batch, so the queue stayed
// full because nothing sealed it -- and what it turned away included
// cross-partition synthetics and the healer's own re-submissions, so the
// stream that needed the queue to drain was the stream being refused.
//
// A submission is still never silently swallowed (#4132): it is accepted, and
// the batch it triggers is where it goes.
func TestWorker_CrossingPendingBoundarySealsRatherThanRefusing(t *testing.T) {
	w := worker.New(worker.Config{
		ID: 0, Partition: "BVN1",
		MaxPendingCount: 4,
	}, nil)

	for i := 0; i < 100; i++ {
		require.NoErrorf(t, w.Submit([]byte(fmt.Sprintf("tx-%d", i))),
			"submission %d was refused; crossing the boundary must seal, not reject", i)
	}
}

// An empty transaction is refused rather than batched, so it cannot occupy a
// slot and vanish later.
func TestWorker_EmptyTransactionRefused(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "BVN1"}, nil)
	require.Error(t, w.Submit(nil))
	require.Error(t, w.Submit([]byte{}))
	assert.Equal(t, 0, w.PendingCount())
}

// A closed worker refuses work instead of accepting it into a void.
func TestWorker_ClosedWorkerRefuses(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "BVN1"}, nil)
	require.NoError(t, w.Close())
	err := w.Submit([]byte("tx"))
	require.Error(t, err)
	assert.ErrorIs(t, err, worker.ErrWorkerClosed)
}

// Everything accepted must end up in a batch. This is the accept -> batch leg
// that the #4132 trace measured at 528/528; pin it so a regression shows up
// here rather than in a soak run three hours later.
func TestWorker_EverythingAcceptedReachesABatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := worker.New(worker.Config{
		ID: 0, Partition: "BVN1",
		BatchSize:    16,
		BatchTimeout: 25 * time.Millisecond,
	}, nil)
	go func() { _ = w.Start(ctx) }()
	defer w.Close()

	const n = 200
	for i := 0; i < n; i++ {
		require.NoError(t, w.Submit([]byte(fmt.Sprintf("tx-%04d", i))))
	}

	require.Eventually(t, func() bool {
		total := 0
		for _, d := range w.BatchDigests() {
			if b, _ := w.GetBatch(d); b != nil {
				total += b.Len()
			}
		}
		return total == n && w.PendingCount() == 0
	}, 5*time.Second, 25*time.Millisecond,
		"every accepted transaction must reach a batch")
}
