// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// #4141 Stage 0 characterization: there is NO per-transaction size ceiling on
// the live submission path. A transaction larger than MaxBatchBytes is
// accepted — only aggregate backpressure (pending count, pending size,
// stored batches) can refuse it — and it becomes a batch bigger than the
// batch size limit was supposed to allow. The port of main's packaging adds
// the refusal (`SubmitRefusesTransactionLargerThanMaxBatchBytes`); this pins
// the today-side so the new behaviour is a visible diff.
func TestCharacterize_WorkerAcceptsTransactionLargerThanMaxBatchBytes(t *testing.T) {
	w := worker.New(worker.Config{
		ID:        0,
		Partition: "test",
	}, nil)

	oversize := bytes.Repeat([]byte{1}, worker.DefaultMaxBatchBytes+worker.DefaultMaxBatchBytes/5)
	err := w.Submit(oversize)
	require.NoError(t, err,
		"today an oversize transaction is ACCEPTED — the refusal does not exist yet")
	assert.Equal(t, 1, w.PendingCount())
	assert.Greater(t, w.PendingSize(), worker.DefaultMaxBatchBytes,
		"and the pending buffer now exceeds the batch byte limit by construction")
}
