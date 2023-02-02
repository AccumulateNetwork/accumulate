// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type Querier2 struct {
	Querier
}

func (q Querier2) QueryAccount(ctx context.Context, account *url.URL, query *DefaultQuery) (*AccountRecord, error) {
	return recordIs[*AccountRecord](doQuery(q, ctx, account, query))
}

func (q Querier2) QueryAccountAs(ctx context.Context, account *url.URL, query *DefaultQuery, target any) (*AccountRecord, error) {
	r, err := q.QueryAccount(ctx, account, query)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	err = encoding.SetPtr(r.Account, target)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return r, nil
}

func (q Querier2) QuerySignature(ctx context.Context, txid *url.TxID, query *DefaultQuery) (*SignatureRecord, error) {
	return recordIs[*SignatureRecord](doQuery(q, ctx, txid.AsUrl(), query))
}

func (q Querier2) QueryTransaction(ctx context.Context, txid *url.TxID, query *DefaultQuery) (*TransactionRecord, error) {
	return recordIs[*TransactionRecord](doQuery(q, ctx, txid.AsUrl(), query))
}

func (q Querier2) QueryChain(ctx context.Context, scope *url.URL, query *ChainQuery) (*ChainRecord, error) {
	return recordIs[*ChainRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryAccountChains(ctx context.Context, scope *url.URL, query *ChainQuery) (*RecordRange[*ChainRecord], error) {
	return rangeOf[*ChainRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryTransactionChains(ctx context.Context, scope *url.TxID, query *ChainQuery) (*RecordRange[*ChainEntryRecord[Record]], error) {
	return rangeOf[*ChainEntryRecord[Record]](doQuery(q, ctx, scope.AsUrl(), query))
}

func (q Querier2) QueryChainEntry(ctx context.Context, scope *url.URL, query *ChainQuery) (*ChainEntryRecord[Record], error) {
	return chainEntryOf[Record](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryChainEntries(ctx context.Context, scope *url.URL, query *ChainQuery) (*RecordRange[*ChainEntryRecord[Record]], error) {
	return chainRangeOf[Record](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryTxnChainEntry(ctx context.Context, scope *url.URL, query *ChainQuery) (*ChainEntryRecord[*TransactionRecord], error) {
	return chainEntryOf[*TransactionRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryTxnChainEntries(ctx context.Context, scope *url.URL, query *ChainQuery) (*RecordRange[*ChainEntryRecord[*TransactionRecord]], error) {
	return chainRangeOf[*TransactionRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QuerySigChainEntry(ctx context.Context, scope *url.URL, query *ChainQuery) (*ChainEntryRecord[*SignatureRecord], error) {
	return chainEntryOf[*SignatureRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QuerySigChainEntries(ctx context.Context, scope *url.URL, query *ChainQuery) (*RecordRange[*ChainEntryRecord[*SignatureRecord]], error) {
	return chainRangeOf[*SignatureRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryIdxChainEntry(ctx context.Context, scope *url.URL, query *ChainQuery) (*ChainEntryRecord[*IndexEntryRecord], error) {
	return chainEntryOf[*IndexEntryRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryIdxChainEntries(ctx context.Context, scope *url.URL, query *ChainQuery) (*RecordRange[*ChainEntryRecord[*IndexEntryRecord]], error) {
	return chainRangeOf[*IndexEntryRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryDataEntry(ctx context.Context, scope *url.URL, query *DataQuery) (*ChainEntryRecord[*TransactionRecord], error) {
	return chainEntryOf[*TransactionRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryDataEntries(ctx context.Context, scope *url.URL, query *DataQuery) (*RecordRange[*ChainEntryRecord[*TransactionRecord]], error) {
	return chainRangeOf[*TransactionRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryDirectoryUrls(ctx context.Context, scope *url.URL, query *DirectoryQuery) (*RecordRange[*UrlRecord], error) {
	if query == nil {
		query = new(DirectoryQuery)
	}
	if query.Range == nil {
		query.Range = new(RangeOptions)
	}
	query.Range.Expand = nil
	return rangeOf[*UrlRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryDirectory(ctx context.Context, scope *url.URL, query *DirectoryQuery) (*RecordRange[*AccountRecord], error) {
	if query == nil {
		query = new(DirectoryQuery)
	}
	if query.Range == nil {
		query.Range = new(RangeOptions)
	}
	expand := true
	query.Range.Expand = &expand
	return rangeOf[*AccountRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryPendingIds(ctx context.Context, scope *url.URL, query *PendingQuery) (*RecordRange[*TxIDRecord], error) {
	if query == nil {
		query = new(PendingQuery)
	}
	if query.Range == nil {
		query.Range = new(RangeOptions)
	}
	query.Range.Expand = nil
	return rangeOf[*TxIDRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryPending(ctx context.Context, scope *url.URL, query *PendingQuery) (*RecordRange[*TransactionRecord], error) {
	if query == nil {
		query = new(PendingQuery)
	}
	if query.Range == nil {
		query.Range = new(RangeOptions)
	}
	expand := true
	query.Range.Expand = &expand
	return rangeOf[*TransactionRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryMinorBlock(ctx context.Context, scope *url.URL, query *BlockQuery) (*MinorBlockRecord, error) {
	return recordIs[*MinorBlockRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryMinorBlocks(ctx context.Context, scope *url.URL, query *BlockQuery) (*RecordRange[*MinorBlockRecord], error) {
	return rangeOf[*MinorBlockRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryMajorBlock(ctx context.Context, scope *url.URL, query *BlockQuery) (*MajorBlockRecord, error) {
	return recordIs[*MajorBlockRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) QueryMajorBlocks(ctx context.Context, scope *url.URL, query *BlockQuery) (*RecordRange[*MajorBlockRecord], error) {
	return rangeOf[*MajorBlockRecord](doQuery(q, ctx, scope, query))
}

func (q Querier2) SearchForAnchor(ctx context.Context, scope *url.URL, search *AnchorSearchQuery) (*RecordRange[*ChainEntryRecord[Record]], error) {
	return chainRangeOf[Record](doQuery(q, ctx, scope, search))
}

func (q Querier2) SearchForPublicKey(ctx context.Context, scope *url.URL, search *PublicKeySearchQuery) (*RecordRange[*KeyRecord], error) {
	return rangeOf[*KeyRecord](doQuery(q, ctx, scope, search))
}

func (q Querier2) SearchForPublicKeyHash(ctx context.Context, scope *url.URL, search *PublicKeyHashSearchQuery) (*RecordRange[*KeyRecord], error) {
	return rangeOf[*KeyRecord](doQuery(q, ctx, scope, search))
}

func (q Querier2) SearchForDelegate(ctx context.Context, scope *url.URL, search *DelegateSearchQuery) (*RecordRange[*KeyRecord], error) {
	return rangeOf[*KeyRecord](doQuery(q, ctx, scope, search))
}

func (q Querier2) SearchForTransactionHash(ctx context.Context, scope *url.URL, search *MessageHashSearchQuery) (*RecordRange[*TxIDRecord], error) {
	return rangeOf[*TxIDRecord](doQuery(q, ctx, scope, search))
}

type queryPtr[T any] interface {
	Query
	*T
}

func doQuery[T any, PT queryPtr[T]](q Querier, ctx context.Context, scope *url.URL, query PT) (Record, error) {
	if query == nil {
		query = PT(new(T))
	}
	r, err := q.Query(ctx, scope, query)
	if err != nil {
		return nil, err
	}

	// Remarshal to erase type info. TODO: add a method that does the same thing.
	b, err := r.MarshalBinary()
	if err != nil {
		return nil, errors.EncodingError.Wrap(err)
	}
	r, err = UnmarshalRecord(b)
	if err != nil {
		return nil, errors.EncodingError.Wrap(err)
	}

	return r, nil
}

func recordIs[T Record](r Record, err error) (T, error) {
	var z T
	if err != nil {
		return z, err
	}
	if v, ok := r.(T); ok {
		return v, nil
	}
	return z, fmt.Errorf("rpc returned unexpected type: want %T, got %T", z, r)
}

func chainEntryOf[T Record](r Record, err error) (*ChainEntryRecord[T], error) {
	cr, err := recordIs[*ChainEntryRecord[Record]](r, err)
	if err != nil {
		return nil, err
	}
	return ChainEntryAs[T](cr)
}

func rangeOf[T Record](r Record, err error) (*RecordRange[T], error) {
	rr, err := recordIs[*RecordRange[Record]](r, err)
	if err != nil {
		return nil, err
	}
	return MapRange(rr, func(r Record) (T, error) {
		if v, ok := r.(T); ok {
			return v, nil
		}
		var z T
		return z, fmt.Errorf("rpc returned unexpected type: want %T, got %T", z, r)
	})
}

func chainRangeOf[T Record](r Record, err error) (*RecordRange[*ChainEntryRecord[T]], error) {
	rr, err := recordIs[*RecordRange[Record]](r, err)
	if err != nil {
		return nil, err
	}
	return MapRange(rr, func(r Record) (*ChainEntryRecord[T], error) {
		return chainEntryOf[T](r, nil)
	})
}
