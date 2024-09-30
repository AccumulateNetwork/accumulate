// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
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

	if query == nil {
		query = new(DefaultQuery)
	}

	switch query := query.(type) {
	case *DefaultQuery:
		r, err := c.queryMessage(ctx, scope, query)
		if r != nil || err != nil {
			return r, errors.UnknownError.Wrap(err)
		}

	case *ChainQuery:
		r, err := c.queryChain(ctx, scope, query)
		if r != nil || err != nil {
			return r, errors.UnknownError.Wrap(err)
		}

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
	// Scope must be unknown
	if scope == nil || !protocol.IsUnknown(scope) {
		return nil, nil
	}

	// List partitions
	ns, err := c.Network.NetworkStatus(ctx, NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query network status: %w", err)
	}

	// For each partition
	all := new(RecordRange[Record])
	for _, part := range ns.Network.Partitions {
		// Don't query the summary network
		if part.Type == protocol.PartitionTypeBlockSummary {
			continue
		}

		// Query the partition
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

		// Collate the responses
		rr, ok := r.(*RecordRange[Record])
		if !ok {
			return nil, errors.InternalError.WithFormat("unexpected response: want %v, got %v", RecordTypeRange, r.RecordType())
		}
		all.Records = append(all.Records, rr.Records...)
		all.Total += rr.Total
		setLastBlockTime(&all.LastBlockTime, rr.LastBlockTime)
	}

	return all, nil
}

func (c *Collator) queryMessage(ctx context.Context, scope *url.URL, query *DefaultQuery) (Record, error) {
	// Query each partition
	var all *MessageRecord[messaging.Message]
	txid, err := c.collateTxn(ctx, scope, func(scope *url.TxID) error {
		r, err := Querier2{c.Querier}.QueryMessage(ctx, scope, query)
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			return nil
		default:
			return errors.UnknownError.Wrap(err)
		}

		// Collate the response
		if all == nil {
			all = r
			return nil
		}

		if r.Error != nil {
			all.Error = r.Error
		}
		if r.Status.Delivered() {
			all.Status = r.Status
		} else if r.Status != 0 && (all.Status == 0 || all.Status == errors.Remote) {
			all.Status = r.Status
		}

		all.Produced.Append(r.Produced)
		all.Cause.Append(r.Cause)
		all.Signatures.Append(r.Signatures)
		setLastBlockTime(&all.LastBlockTime, r.LastBlockTime)
		return nil
	})
	if err != nil || txid == nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if all == nil {
		return nil, errors.NotFound.WithFormat("message %x not found", txid.Hash())
	}

	// This field is not meaningful for collated responses
	all.Received = 0

	// Remove duplicates
	uniqueTxid(all.Produced)
	uniqueTxid(all.Cause)
	mergeSignatureSets(all.Signatures)

	return all, nil
}

func (c *Collator) queryChain(ctx context.Context, scope *url.URL, query *ChainQuery) (Record, error) {
	// Query each partition
	all := new(RecordRange[*ChainEntryRecord[Record]])
	txid, err := c.collateTxn(ctx, scope, func(scope *url.TxID) error {
		r, err := Querier2{c.Querier}.QueryTransactionChains(ctx, scope, query)
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errors.NotFound):
			return nil
		default:
			return errors.UnknownError.Wrap(err)
		}

		// Collate the response
		all.Records = append(all.Records, r.Records...)
		all.Total += r.Total
		setLastBlockTime(&all.LastBlockTime, r.LastBlockTime)
		return nil
	})
	if err != nil || txid == nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return all, nil
}

func (c *Collator) collateTxn(ctx context.Context, scope *url.URL, fn func(*url.TxID) error) (*url.TxID, error) {
	// Sanity check
	if scope == nil {
		return nil, nil
	}

	// Scope must have a transaction ID
	id, err := scope.AsTxID()
	if err != nil {
		return nil, nil
	}

	// List partitions
	ns, err := c.Network.NetworkStatus(ctx, NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query network status: %w", err)
	}

	// For each partition
	for _, part := range ns.Network.Partitions {
		// Don't query the summary network
		if part.Type == protocol.PartitionTypeBlockSummary {
			continue
		}

		// Query the partition
		err := fn(protocol.PartitionUrl(part.ID).WithTxID(id.Hash()))
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}
	return id, nil
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

func uniqueTxid(r *RecordRange[*TxIDRecord]) {
	unique := make([]*TxIDRecord, 0, len(r.Records))
	seen := map[[32]byte]bool{}
	for _, r := range r.Records {
		if seen[r.Value.Hash()] {
			continue
		}
		unique = append(unique, r)
		seen[r.Value.Hash()] = true
	}
	r.Records = unique
}

func mergeSignatureSets(r *RecordRange[*SignatureSetRecord]) {
	merged := make([]*SignatureSetRecord, 0, len(r.Records))
	lut := map[[32]byte]int{}

	// Merge sets
	for _, r := range r.Records {
		i, ok := lut[r.Account.GetUrl().AccountID32()]
		if !ok {
			lut[r.Account.GetUrl().AccountID32()] = len(merged)
			merged = append(merged, r)
			continue
		}

		m := merged[i]
		switch m.Account.Type() {
		case protocol.AccountTypeUnknown,
			protocol.AccountTypeUnknownSigner:
			m.Account = r.Account
		}
		m.Signatures.Append(r.Signatures)
	}
	r.Records = merged

	// Deduplicate
	for _, r := range r.Records {
		unique := make([]*MessageRecord[messaging.Message], 0, len(r.Signatures.Records))
		seen := map[[32]byte]bool{}
		for _, r := range r.Signatures.Records {
			h := r.Message.Hash()
			if seen[h] {
				// Duplicate signatures should not happen, but even if they do
				// there's not a lot of point to merging the records. Very few
				// fields are populated, other than the message.
				continue
			}
			unique = append(unique, r)
			seen[h] = true
		}
		r.Signatures.Records = unique
	}
}

// setLastBlockTime is used to set LastBlockTime to the earliest value from all
// collated responses.
func setLastBlockTime(dst **time.Time, src *time.Time) {
	if src == nil {
		return
	}
	if *dst == nil || (**dst).After(*src) {
		*dst = src
	}
}
