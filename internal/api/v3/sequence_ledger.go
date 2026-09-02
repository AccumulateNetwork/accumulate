// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// withSighted answers "how far has this stream been sighted" for a sequence
// ledger, computing it rather than reading it.
//
// `Received` is no longer stored (#4189). What a stream has sighted is
// staging's, which is durable but NOT hashed — and it must stay unhashed,
// because a value derived from staging written into an account would make a
// staging discrepancy a divergent block hash instead of a wrong number on a
// dashboard.
//
// But the question is still a real one, and it is the one every operator
// surface asks: `debug sequence` reports unreceived and unprocessed counts from
// it, and the soak dashboard derives a channel's backlog as received minus
// delivered — the quantity that pinned at exactly 4,096 while the network
// livelocked. Dropping the field silently turned every one of those readings
// into a zero, which reads as "nothing has ever arrived" and painted healthy
// streams as stalled.
//
// So it is filled in on the way out. Computed on read, never written: the
// account on disk carries no trace of it, and nothing about consensus depends
// on what this returns.
//
// The ledger is COPIED before being modified. The batch memoizes the record it
// loaded, so writing to it here would edit what every other reader of that
// batch sees — and this is a read.
func withSighted(batch *database.Batch, u *url.URL, account protocol.Account) protocol.Account {
	var seq []*protocol.PartitionSyntheticLedger
	switch l := account.(type) {
	case *protocol.SyntheticLedger:
		l = l.Copy()
		seq, account = l.Sequence, l
	case *protocol.AnchorLedger:
		l = l.Copy()
		seq, account = l.Sequence, l
	default:
		return account
	}

	for _, part := range seq {
		if part.Url == nil {
			continue
		}
		n, err := execute.Sighted(batch, execute.StreamID{Ledger: u, Source: part.Url})
		if err != nil {
			continue // a report, not a decision: a missing record reads as zero
		}
		if n > part.Delivered {
			part.Received = n
		} else {
			part.Received = part.Delivered
		}
	}
	return account
}
