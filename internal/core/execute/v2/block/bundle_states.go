// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"

// bundleStates holds each message's execution state in the order the
// messages ran, which is the order staging released them and the order the
// chains were appended to. It is folded into the block in that order.
//
// This was a map sorted by message hash. Every node folded in the same
// order, which is all a map can promise, but it was the wrong order: the
// chain appends of three anchors from one partition reached the block's
// segment bookkeeping as 4041, 4039, 4040, the span for 4039 was adjacent
// to nothing and was dropped, and the Directory never closed another block
// (#4279, run 20260915T042428Z). Execution order is deterministic on every
// node -- messages run in envelope order and bundles fold serially -- so
// there is nothing to sort, and nothing downstream has to put the order
// back.
type bundleStates struct {
	order []bundleState
	// at is the position in order of each hash, so a state set again --
	// the same transaction reached twice in one bundle -- replaces the
	// first in place instead of folding twice.
	at map[[32]byte]int
}

type bundleState struct {
	hash  [32]byte
	state *chain.ProcessTransactionState
}

// Set records the state of the message with the given hash. A hash set
// again keeps its place and takes the new state.
func (s *bundleStates) Set(h [32]byte, state *chain.ProcessTransactionState) {
	if i, ok := s.at[h]; ok {
		s.order[i].state = state
		return
	}
	if s.at == nil {
		s.at = map[[32]byte]int{}
	}
	s.at[h] = len(s.order)
	s.order = append(s.order, bundleState{hash: h, state: state})
}

// For visits every state in execution order.
func (s *bundleStates) For(fn func(h [32]byte, state *chain.ProcessTransactionState) error) error {
	for _, e := range s.order {
		if err := fn(e.hash, e.state); err != nil {
			return err
		}
	}
	return nil
}
