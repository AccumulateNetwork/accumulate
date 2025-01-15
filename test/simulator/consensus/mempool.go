// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"crypto/sha256"
	"sort"
	"sync"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// mempool tracks submitted envelopes.
type mempool struct {
	count      int
	mu         sync.Mutex
	logger     logging.OptionalLogger
	pool       map[[32]byte]*mpEntry
	candidates []*mpEntry
}

type mpEntry struct {
	order int
	hash  [32]byte
	env   *messaging.Envelope
}

func newMempool(logger log.Logger) *mempool {
	m := new(mempool)
	m.logger.Set(logger)
	m.pool = map[[32]byte]*mpEntry{}
	return m
}

// Add adds an envelope to the mempool, tracking its arrival order.
func (m *mempool) Add(envelope *messaging.Envelope) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, err := envelope.MarshalBinary()
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)

	m.pool[h] = &mpEntry{m.count, h, envelope.Copy()}
	m.count++
}

// Propose proposes a list of envelopes to execute in the next block.
func (m *mempool) Propose(block uint64) []*messaging.Envelope {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Sort by arrival order
	sort.Slice(m.candidates, func(i, j int) bool {
		a, b := m.candidates[i], m.candidates[j]
		return a.order < b.order
	})

	env := make([]*messaging.Envelope, len(m.candidates))
	for i, c := range m.candidates {
		env[i] = c.env
	}
	return env
}

// CheckPropose checks a block proposal received from another node.
func (m *mempool) CheckProposed(block uint64, envelope *messaging.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, err := envelope.MarshalBinary()
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)

	if _, ok := m.pool[h]; !ok {
		return errors.Conflict.With("not present in mempool")
	}
	return nil
}

// AcceptProposed removes the block proposal envelopes from the mempool and
// prepares the node's candidates to propose for the next block (if the node is
// selected as the leader).
func (m *mempool) AcceptProposed(block uint64, envelopes []*messaging.Envelope) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove proposed envelopes
	for _, env := range envelopes {
		b, err := env.MarshalBinary()
		if err != nil {
			panic(err)
		}
		h := sha256.Sum256(b)

		delete(m.pool, h)
	}

	// Use remaining envelopes as candidates
	m.candidates = m.candidates[:0]
	for _, c := range m.pool {
		m.candidates = append(m.candidates, c)
	}
}
