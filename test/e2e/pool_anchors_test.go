// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// readPoolAnchors reads every entry of pool through querier, a page at a
// time with each entry's message, and judges each anchor producer produced by
// the rule the join's collector takes an anchor by (anchorsrc.VerifyQuorum):
// verified names the blocks whose anchor a quorum signed, and refused says
// why each other one was not taken -- block 0 for an entry served without its
// body. A page the querier refuses with NotReady is asked again, up to tries
// times, so a querier that rotates among peers asks the next; a page every
// try refuses is the read's error.
func readPoolAnchors(ctx context.Context, querier api.Querier, pool, producer *url.URL, authority *anchorsrc.Authority, tries int) (verified map[uint64]bool, refused map[uint64]string, err error) {
	verified, refused = map[uint64]bool{}, map[uint64]string{}
	id, ok := protocol.ParsePartitionUrl(producer)
	if !ok {
		return nil, nil, errors.BadRequest.WithFormat("%v is not a partition", producer)
	}
	q := api.Querier2{Querier: querier}
	chain, err := q.QueryChain(ctx, pool, &api.ChainQuery{Name: "main"})
	switch {
	case errors.Is(err, errors.NotFound):
		return verified, refused, nil
	case err != nil:
		return verified, refused, err
	}
	const size = 64
	for start := uint64(0); start < chain.Count; start += size {
		count, expand := uint64(size), true
		var page *api.RecordRange[*api.ChainEntryRecord[api.Record]]
		for i := 0; i < tries; i++ {
			page, err = q.QueryChainEntries(ctx, pool, &api.ChainQuery{
				Name:  "main",
				Range: &api.RangeOptions{Start: start, Count: &count, Expand: &expand},
			})
			if !errors.Is(err, errors.NotReady) {
				break
			}
		}
		if err != nil {
			return verified, refused, err
		}
		for _, entry := range page.Records {
			v, ok := entry.Value.(*api.MessageRecord[messaging.Message])
			if !ok || v.Message == nil {
				refused[0] = "the entry was served without its body"
				continue
			}
			rec, err := api.MessageRecordAs[*messaging.TransactionMessage](v)
			if err != nil || rec.Message.Transaction == nil {
				continue // Not a transaction
			}
			body, ok := rec.Message.Transaction.Body.(protocol.AnchorBody)
			if !ok {
				continue
			}
			pa := body.GetPartitionAnchor()
			if pa == nil || pa.Source == nil || !pa.Source.Equal(producer) {
				continue
			}
			if err := anchorsrc.VerifyQuorum(authority, id, rec); err != nil {
				refused[pa.MinorBlockIndex] = err.Error()
				continue
			}
			verified[pa.MinorBlockIndex] = true
		}
	}
	return verified, refused, nil
}
