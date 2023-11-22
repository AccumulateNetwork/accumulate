// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package apiutil

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type TimestampService struct {
	Querier api.Querier
	Cache   keyvalue.Beginner
}

type ptrVal[T any] interface {
	~*T
	encoding.BinaryValue
}

func (s *TimestampService) GetTimestamp(ctx context.Context, id *url.TxID) (_ *MessageData, err error) {
	cache := s.Cache.Begin(nil, true)
	defer func() {
		if err == nil {
			err = cache.Commit()
		}
	}()

	cached := cv[*MessageData](cache, "Message", id.Hash(), id.Account())
	v, err := cached.Get()
	if err != nil {
		return nil, err
	}
	if v != nil && time.Since(v.LastUpdated) < time.Minute {
		return v, nil
	}

	v = new(MessageData)
	v.LastUpdated = time.Now()

	Q := api.Querier2{Querier: s.Querier}
	r, err := Q.QueryTransactionChains(ctx, id, &api.ChainQuery{IncludeReceipt: true})
	if err != nil {
		return nil, err
	}

	for _, c := range r.Records {
		entry, err := s.getChainData(ctx, cache, c)
		if err != nil {
			return nil, err
		}
		v.Chains = append(v.Chains, entry)
	}

	err = cached.Put(v)
	return v, err
}

func (s *TimestampService) getChainData(ctx context.Context, cache keyvalue.Store, r *api.ChainEntryRecord[api.Record]) (*ChainData, error) {
	cached := cv[*ChainData](cache, "Chain", r.Account, r.Name, r.Entry)
	v, err := cached.Get()
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}

	anchor, err := s.getAnchorData(ctx, cache, *(*[32]byte)(r.Receipt.Anchor))
	if err != nil {
		return nil, err
	}

	v = new(ChainData)
	v.Account = r.Account
	v.Chain = r.Name
	v.Block = anchor.Block
	v.Time = anchor.Time
	err = cached.Put(v)
	return v, err
}

func (s *TimestampService) getAnchorData(ctx context.Context, cache keyvalue.Store, anchor [32]byte) (*AnchorData, error) {
	cached := cv[*AnchorData](cache, "Anchor", anchor)
	v, err := cached.Get()
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}

	Q := api.Querier2{Querier: s.Querier}
	r, err := Q.SearchForAnchor(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool), &api.AnchorSearchQuery{
		Anchor:         anchor[:],
		IncludeReceipt: true,
	})
	if err != nil {
		return nil, err
	}
	if len(r.Records) == 0 {
		return nil, errors.NotFound.WithFormat("anchor %x not found", anchor)
	}
	rr := r.Records[0]

	v = new(AnchorData)
	v.Block = rr.Receipt.LocalBlock
	v.Time = rr.Receipt.LocalBlockTime
	err = cached.Put(v)
	return v, err
}

func cv[PT ptrVal[T], T any](cache keyvalue.Store, key ...any) cachedValue[PT, T] {
	return cachedValue[PT, T]{cache, record.NewKey(key...)}
}

type cachedValue[PT ptrVal[T], T any] struct {
	cache keyvalue.Store
	key   *record.Key
}

func (c cachedValue[PT, T]) Get() (*T, error) {
	b, err := c.cache.Get(c.key)
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil, nil
	default:
		return nil, err
	}

	v := new(T)
	err = PT(v).UnmarshalBinary(b)
	return v, err
}

func (c cachedValue[PT, T]) Put(v *T) error {
	b, err := PT(v).MarshalBinary()
	if err != nil {
		return err
	}
	return c.cache.Put(c.key, b)
}
