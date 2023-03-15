// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Collator is a [Querier] implementation that can collate query responses from
// multiple networks.
type Collator struct {
	Querier Querier
	Network NetworkService
}

func (c *Collator) Query(ctx context.Context, scope *url.URL, query Query) (Record, error) {
	ctx, cancel, _ := ContextWithBatchData(ctx)
	defer cancel()

	switch query := query.(type) {
	case *MessageHashSearchQuery:
		r, err := c.messageHashSearch(ctx, scope, query)
		if r != nil || err != nil {
			return r, errors.UnknownError.Wrap(err)
		}

	case *BlockQuery:
		r, err := c.Querier.Query(ctx, scope, query)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		err = c.visitBlockResponse(ctx, r)
		return r, errors.UnknownError.Wrap(err)
	}

	return c.Querier.Query(ctx, scope, query)
}

func (c *Collator) messageHashSearch(ctx context.Context, scope *url.URL, query *MessageHashSearchQuery) (Record, error) {
	if scope == nil || !protocol.IsUnknown(scope) {
		return nil, nil
	}

	ns, err := c.Network.NetworkStatus(ctx, NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query network status: %w", err)
	}

	var values []Record
	var total uint64
	for _, part := range ns.Network.Partitions {
		scope := protocol.PartitionUrl(part.ID).WithTxID(query.Hash).AsUrl()
		r, err := c.Querier.Query(ctx, scope, query)
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			continue
		default:
			return nil, errors.UnknownError.Wrap(err)
		}

		rr, ok := r.(*RecordRange[Record])
		if !ok {
			return nil, errors.InternalError.WithFormat("unexpected response: want %v, got %v", RecordTypeRange, r.RecordType())
		}
		values = append(values, rr.Records...)
		total += rr.Total
	}

	rr := new(RecordRange[Record])
	rr.Records = values
	rr.Total = total
	return rr, nil
}

func (c *Collator) visitBlockResponse(ctx context.Context, r Record) error {
	switch r := r.(type) {
	case *RecordRange[Record]:
		for _, r := range r.Records {
			err := c.visitBlockResponse(ctx, r)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}

	case *MajorBlockRecord:
		if r.MinorBlocks != nil {
			for _, r := range r.MinorBlocks.Records {
				err := c.visitBlockResponse(ctx, r)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
			}
		}

	case *MinorBlockRecord:
		if r.Anchored == nil {
			break
		}
		for i, a := range r.Anchored.Records {
			b, err := c.Querier.Query(ctx, a.Source, &BlockQuery{Minor: &a.Index})
			switch {
			case err == nil:
				break
			case errors.Code(err).IsClientError():
				continue // Not found or something, ignore
			default:
				return errors.UnknownError.Wrap(err)
			}
			mbr, ok := b.(*MinorBlockRecord)
			if !ok {
				continue // Invalid response, ignore
			}
			r.Anchored.Records[i] = mbr
		}
	}

	return nil
}
