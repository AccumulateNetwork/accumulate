// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package private

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// FetchStagingSnapshot assembles a peer's whole staging from the pages it
// serves. Every page must be as of the same block: the peer's staging moves
// on between calls, and pages from two blocks are not a state any node ever
// held, so a reader that assembled them would execute a block no one else
// executed (executor.md, "Sync" step 2). A page as of a different block is
// refused (errors.Conflict) and the caller starts over.
//
// The result is one snapshot, its streams in the order the pages carried
// them — a stream split across pages appears once per page, which is how
// staging takes it in.
func FetchStagingSnapshot(ctx context.Context, from StagingSnapshotter, partition string) (*StagingSnapshot, error) {
	req := &StagingSnapshotRequest{Partition: partition}
	whole := new(StagingSnapshot)
	for i := 0; ; i++ {
		page, err := from.StagingSnapshot(ctx, req)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		if i == 0 {
			whole.Block = page.Block
		} else if page.Block != whole.Block {
			return nil, errors.Conflict.WithFormat("staging moved from block %d to %d while it was being read", whole.Block, page.Block)
		}
		whole.Streams = append(whole.Streams, page.Streams...)
		if page.NextLedger == nil {
			return whole, nil
		}

		// A cursor that does not move is a server that will never finish.
		if req.Ledger != nil && req.Ledger.Equal(page.NextLedger) &&
			req.Source.Equal(page.NextSource) && req.Number == page.NextNumber {
			return nil, errors.InternalError.WithFormat("staging snapshot paging did not advance past %v %d", page.NextLedger, page.NextNumber)
		}
		req.Ledger, req.Source, req.Number = page.NextLedger, page.NextSource, page.NextNumber
	}
}
