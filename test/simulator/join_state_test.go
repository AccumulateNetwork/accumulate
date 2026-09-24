// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

import (
	"testing"

	"github.com/stretchr/testify/require"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// The simulator's join follows the DAG service's rule for a handoff that
// fails (executor spec, "Sync", step 5; #4401): the blocks produced before
// the failure stand, and the node goes back to collecting, holding the block
// that failed and the ones after it, so the join can hand off again and a
// block arriving meanwhile is kept rather than executed.
func TestJoinState_AFailedHandoffKeepsCollecting(t *testing.T) {
	var produced []uint64
	failed := false
	j := &joinState{produce: func(p coreexec.BlockParams, _ []*messaging.Envelope) error {
		if p.Index == 42 && !failed {
			failed = true
			return errors.NotFound.With("the first attempt at 42 fails")
		}
		produced = append(produced, p.Index)
		return nil
	}}
	j.leave()
	for i := uint64(41); i <= 43; i++ {
		require.NoError(t, j.Collect(coreexec.BlockParams{Index: i}, nil))
	}

	require.Error(t, j.Handoff(40))
	require.Equal(t, []uint64{41}, produced, "41 was produced before the failure")
	require.True(t, j.Joining(), "the node is collecting again")
	require.NoError(t, j.Collect(coreexec.BlockParams{Index: 44}, nil))
	require.Equal(t, []uint64{41}, produced, "and a block arriving now is kept, not executed")

	require.NoError(t, j.Handoff(41))
	require.Equal(t, []uint64{41, 42, 43, 44}, produced, "the second handoff produces what was kept")
	require.False(t, j.Joining())
}
