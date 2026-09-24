// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// produceFailingAdapter fails every block it is asked to produce, as a
// freshly started executor does when its first block's cache seed reads a
// message the join never pulled (run 20260924T052134Z).
type produceFailingAdapter struct {
	collectingAdapter
	attempts int
}

func (a *produceFailingAdapter) ProduceBlock(_ context.Context, params adapter.BlockParams) ([32]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.attempts++
	return [32]byte{}, errors.NotFound.With("seed synthetic cache: load Directory receipts: Message.c06fcb7d….Main not found")
}

// A handoff whose first buffered group fails to produce must leave the node
// where a join can start again: collecting, with the blocks it collected. It
// leaves it neither: performHandoff (collect.go) takes the buffer, clears
// collecting and sets lastBlockIndex BEFORE producing, so after the failure
// the node is not joining, the 22 collected blocks are gone, a second
// Handoff is refused "not joining", and every group committed from then on
// goes to produce, fails, and is dropped by the production loop -- 457
// "Failed to process committed group" lines on acc-bvn3-val1 in 8 minutes.
func TestHandoff_AFailedHandoffLeavesTheNodeAbleToJoinAgain(t *testing.T) {
	svc, _, author := newJoiningService(t)
	fa := &produceFailingAdapter{}
	svc.adapter = fa
	w := svc.node.Workers()[0]

	svc.lastBlockIndex = 40
	svc.StartCollecting()
	for i := 0; i < 3; i++ {
		b := types.NewBatch([][]byte{{byte(i)}})
		require.NoError(t, w.StoreBatch(b))
		_, err := svc.processCommittedGroup(group(commitCert(author, types.Round(2*i+2), time.Unix(int64(100+i), 0),
			[]types.PayloadEntry{{Digest: b.Digest(), Worker: w.ID()}})))
		require.NoError(t, err)
	}
	require.Len(t, svc.Buffered(), 3)

	err := svc.performHandoff(40)
	require.Error(t, err, "precondition: the first buffered group cannot be produced")
	require.Equal(t, 1, fa.attempts)

	require.True(t, svc.Collecting(), "after a failed handoff the node must still be joining, or nothing can join it again")
	require.Len(t, svc.Buffered(), 3, "and must still hold what it collected, or a second handoff has nothing to produce")
	require.Equal(t, uint64(40), svc.lastBlockIndex, "and must still stand where it stood")
}
