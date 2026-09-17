// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
)

// A bundle folds its messages' states into the block in the order the
// messages ran, whatever their hashes sort to. A map sorted by hash threw
// that order away and the chain segment bookkeeping downstream lost an
// anchor's span (#4279).
func TestBundleStates_FoldInExecutionOrder(t *testing.T) {
	var s bundleStates
	ran := [][32]byte{{9}, {1}, {5}} // not in byte order
	states := make([]*chain.ProcessTransactionState, len(ran))
	for i, h := range ran {
		states[i] = new(chain.ProcessTransactionState)
		s.Set(h, states[i])
	}

	var got [][32]byte
	var gotStates []*chain.ProcessTransactionState
	require.NoError(t, s.For(func(h [32]byte, st *chain.ProcessTransactionState) error {
		got = append(got, h)
		gotStates = append(gotStates, st)
		return nil
	}))
	require.Equal(t, ran, got, "states fold in the order they were set")
	require.Equal(t, states, gotStates)

	// A hash set again keeps its place and takes the new state
	again := new(chain.ProcessTransactionState)
	s.Set(ran[1], again)
	got, gotStates = nil, nil
	_ = s.For(func(h [32]byte, st *chain.ProcessTransactionState) error {
		got = append(got, h)
		gotStates = append(gotStates, st)
		return nil
	})
	require.Equal(t, ran, got)
	require.Same(t, again, gotStates[1])
	require.Len(t, gotStates, 3)
}
