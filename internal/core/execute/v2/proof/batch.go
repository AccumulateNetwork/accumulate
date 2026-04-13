// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package proof

import (
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Batcher handles batching of proof requests for optimization
type Batcher struct {
	mu       sync.RWMutex
	batches  map[string]*ProofBatch
	maxBatch int
}

// NewBatcher creates a new proof batcher
func NewBatcher(maxBatchSize int) *Batcher {
	if maxBatchSize <= 0 {
		maxBatchSize = 100
	}
	return &Batcher{
		batches:  make(map[string]*ProofBatch),
		maxBatch: maxBatchSize,
	}
}

// AddRequest adds a proof request to the appropriate batch
func (b *Batcher) AddRequest(req *ProofRequest) (*ProofBatch, bool) {
	if req == nil || req.Anchor == nil {
		return nil, false
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	key := req.Anchor.String()
	batch, exists := b.batches[key]

	if !exists {
		batch = &ProofBatch{
			Requests:    make([]*ProofRequest, 0, b.maxBatch),
			Destination: req.Anchor,
			CreatedAt:   time.Now(),
		}
		b.batches[key] = batch
	}

	batch.Requests = append(batch.Requests, req)

	// Return batch if full
	if len(batch.Requests) >= b.maxBatch {
		delete(b.batches, key)
		return batch, true
	}

	return nil, false
}

// Flush returns any pending batches
func (b *Batcher) Flush() []*ProofBatch {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]*ProofBatch, 0, len(b.batches))
	for _, batch := range b.batches {
		result = append(result, batch)
	}
	b.batches = make(map[string]*ProofBatch)

	return result
}

// GroupByDestination groups requests by their anchor destination
func GroupByDestination(requests []*ProofRequest) map[string][]*ProofRequest {
	groups := make(map[string][]*ProofRequest)
	for _, req := range requests {
		if req == nil || req.Anchor == nil {
			continue
		}
		key := req.Anchor.String()
		groups[key] = append(groups[key], req)
	}
	return groups
}

// MergeSequences merges consecutive sequence numbers for the same destination
func MergeSequences(batch *ProofBatch) map[*url.URL][]uint64 {
	result := make(map[*url.URL][]uint64)
	sequences := make([]uint64, 0)

	for _, req := range batch.Requests {
		sequences = append(sequences, req.Sequence)
	}

	// For now, just return all sequences grouped by destination
	if len(sequences) > 0 {
		result[batch.Destination] = sequences
	}

	return result
}
