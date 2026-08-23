// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// #4141: a transaction larger than MaxBatchBytes can never fit in any batch.
// The Stage 0 characterization pinned the old behaviour — silently accepted,
// quietly distorting batching — so this refusal landed as a visible diff.

func TestWorker_SubmitRefusesTransactionLargerThanMaxBatchBytes(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)

	oversize := bytes.Repeat([]byte{1}, worker.DefaultMaxBatchBytes+1)
	err := w.Submit(oversize)
	require.Error(t, err, "an oversize transaction must be refused, not absorbed")
	assert.True(t, errors.Is(err, worker.ErrTransactionTooLarge),
		"a distinct error — not backpressure, which clears on retry; this never will")
	assert.False(t, errors.Is(err, worker.ErrBackpressure))
	assert.Zero(t, w.PendingCount(), "nothing may enter the pending buffer")
}

func TestWorker_SubmitStillAcceptsNormalSizeUnderBackpressureLimits(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)

	require.NoError(t, w.Submit(bytes.Repeat([]byte{1}, 1024)))
	require.NoError(t, w.Submit(bytes.Repeat([]byte{2}, worker.DefaultMaxBatchBytes)),
		"a transaction AT the limit still fits in one batch")
	assert.Equal(t, 2, w.PendingCount())
}

// The refusal is visible to the caller: the error names the size and the
// limit, so the operator learns WHY instead of watching a transaction vanish
// into batching (#4132's class of silence).
func TestWorker_RefusalIsVisibleToTheCaller(t *testing.T) {
	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)

	err := w.Submit(bytes.Repeat([]byte{1}, worker.DefaultMaxBatchBytes+100))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "512100 bytes")
	assert.Contains(t, err.Error(), "limit 512000")
}
