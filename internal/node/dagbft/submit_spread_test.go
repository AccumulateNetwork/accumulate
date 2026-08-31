// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The submitter API's traffic must reach more than one worker.
//
// pkg/consensus already tested that keyless submission round-robins, and it
// did — but nothing tested which consensus method the node actually calls.
// Service.SubmitTransaction called SubmitTransactionFor(""), routing on the
// hash of an empty key, which is a constant: every transaction on every node
// landed in worker 1. That worker then held all of the node's own uncommitted
// batches inside its 1/N share of the partition's byte budget (8 MB of 32 MB
// at the NumWorkers=4 we ship), could not evict them, and evicted peers'
// batches instead — the batches certificates need. Soak 20260831T060018Z
// collapsed 15 minutes in with 35,999 over-limit warnings, every one of them
// workerID=1 (#4179).
//
// This is the test that was missing: it exercises the path from the service
// down, not the routing function in isolation.
func TestServiceSubmitTransaction_SpreadsAcrossWorkers(t *testing.T) {
	for _, n := range []int{2, 4} {
		svc, _, _ := newCommitService(t, n)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = svc.node.Start(ctx) }()
		require.Eventually(t, func() bool {
			return svc.node.SubmitTransactionFor("probe", []byte("probe")) == nil
		}, 5*time.Second, 20*time.Millisecond, "node should accept submissions once started")

		before := make([]int, n)
		for i, w := range svc.node.Workers() {
			before[i] = w.PendingCount()
		}

		const perWorker = 5
		for i := 0; i < perWorker*n; i++ {
			require.NoError(t, svc.SubmitTransaction([]byte(fmt.Sprintf("tx-%d", i))))
		}

		for i, w := range svc.node.Workers() {
			assert.Equal(t, before[i]+perWorker, w.PendingCount(),
				"numWorkers=%d: worker %d must take its share — one worker holding all of it "+
					"is what collapsed the 500 tx/s soak", n, i)
		}
		cancel()
	}
}

// A not-started node must still refuse rather than panic on the nil node — the
// guard SubmitTransactionFor had and SubmitTransaction needed once it stopped
// delegating to it.
func TestServiceSubmitTransaction_RefusesBeforeStart(t *testing.T) {
	svc, _, _ := newCommitService(t, 2)
	svc.node = nil
	require.Error(t, svc.SubmitTransaction([]byte("tx")),
		"an unstarted node must refuse submissions, never absorb them silently")
}
