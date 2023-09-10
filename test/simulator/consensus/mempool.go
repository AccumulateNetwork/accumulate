// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"crypto/sha256"
	"sort"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// mempool tracks submitted envelopes.
type mempool struct {
	count      int
	logger     logging.OptionalLogger
	byHash     map[[32]byte]*messaging.Envelope
	arrival    map[*messaging.Envelope]int
	candidates []*messaging.Envelope
}

func newMempool(logger log.Logger) *mempool {
	m := new(mempool)
	m.logger.Set(logger)
	m.byHash = map[[32]byte]*messaging.Envelope{}
	m.arrival = map[*messaging.Envelope]int{}
	return m
}

// Add adds an envelope to the mempool, tracking its arrival order.
func (m *mempool) Add(block uint64, envelope *messaging.Envelope) {
	b, err := envelope.MarshalBinary()
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)

	m.byHash[h] = envelope
	m.arrival[envelope] = m.count
	m.count++
}

// Propose proposes a list of envelopes to execute in the next block.
func (m *mempool) Propose(block uint64) []*messaging.Envelope {
	return m.candidates
}

// CheckPropose checks a block proposal received from another node.
func (m *mempool) CheckProposed(block uint64, envelope *messaging.Envelope) error {
	b, err := envelope.MarshalBinary()
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)

	if _, ok := m.byHash[h]; !ok {
		return errors.Conflict.With("not present in mempool")
	}
	return nil
}

// AcceptProposed removes the block proposal envelopes from the mempool and
// prepares the node's candidates to propose for the next block (if the node is
// selected as the leader).
func (m *mempool) AcceptProposed(block uint64, envelopes []*messaging.Envelope) {
	// Remove proposed envelopes
	for _, env := range envelopes {
		b, err := env.MarshalBinary()
		if err != nil {
			panic(err)
		}
		h := sha256.Sum256(b)

		delete(m.arrival, m.byHash[h])
		delete(m.byHash, h)
	}

	// Use remaining envelopes as candidates and sort by arrival order
	m.candidates = m.candidates[:0]
	for env := range m.arrival {
		m.candidates = append(m.candidates, env)
	}
	sort.Slice(m.candidates, func(i, j int) bool {
		a, b := m.candidates[i], m.candidates[j]
		return m.arrival[a] < m.arrival[b]
	})
}
