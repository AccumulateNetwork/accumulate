// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker

import "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"

// AddTestBatch adds a batch to the worker and makes it available.
// This is intended for testing purposes only.
func (w *Worker) AddTestBatch(batch *types.Batch) {
	digest := batch.Digest()

	w.batchMu.Lock()
	element := w.lruList.PushFront(digest)
	w.batches[digest] = &lruEntry{
		batch:   batch,
		element: element,
	}
	w.batchMu.Unlock()

	// Add to available batch queue
	w.queueDepth.Add(1)
	w.availableBatchQueue <- digest
}
