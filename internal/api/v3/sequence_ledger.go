// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// stagingFor is the staging the querier reports from: the one it was given,
// or the one registered for its partition.
func (s *Querier) stagingFor() *execute.Staging {
	if s.staging != nil {
		return s.staging
	}
	id, _ := protocol.ParsePartitionUrl(s.partition.URL)
	return execute.StagingFor(id)
}

// sighted answers "how far has this stream been sighted" for a sequence
// ledger, computing it rather than reading it.
//
// `Received` is no longer stored (#4189). What a stream has sighted is
// staging's, which is durable but NOT hashed — and it must stay unhashed,
// because a value derived from staging written into an account would make a
// staging discrepancy a divergent block hash instead of a wrong number on a
// dashboard.
//
// But the question is still a real one, and it is the one every operator
// surface asks: `debug sequence` reports unreceived and unprocessed counts
// from it, and the soak dashboard derives a channel's backlog as received
// minus delivered — the quantity that pinned at exactly 4,096 while the
// network livelocked. Dropping the field silently turned every one of those
// readings into a zero, which reads as "nothing has ever arrived" and painted
// healthy streams as stalled.
//
// So it is answered BESIDE the body and never inside it. It used to be filled
// into the ledger on the way out, and that is what stopped every restarted
// node rejoining (#4295): the same call serves a receipt built from the
// STORED account, so an account whose body had been rewritten no longer
// hashed to the leaf its own receipt proved, pull.Verify's third check
// refused it, and the node pulled the same account forever. Nothing that
// hashes may be synthesised on the way out — not here, and not by whatever
// wants to fill something in next.
//
// Nothing is copied and nothing is written: the ledger the batch memoized is
// only read.
func sighted(staging *execute.Staging, u *url.URL, account protocol.Account) []*api.SightedStream {
	if staging == nil {
		return nil
	}
	var seq []*protocol.PartitionSyntheticLedger
	switch l := account.(type) {
	case *protocol.SyntheticLedger:
		seq = l.Sequence
	case *protocol.AnchorLedger:
		seq = l.Sequence
	default:
		return nil
	}

	var out []*api.SightedStream
	for _, part := range seq {
		if part.Url == nil {
			continue
		}
		n := staging.SightedOn(execute.StreamID{Ledger: u, Source: part.Url})
		if n < part.Delivered {
			n = part.Delivered
		}
		out = append(out, &api.SightedStream{Source: part.Url, Received: n})
	}
	return out
}
