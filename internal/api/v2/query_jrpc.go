// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

func (m *JrpcMethods) QueryTx(ctx context.Context, params json.RawMessage) interface{} {
	req := new(TxnQuery)
	err := m.parse(params, req)
	if err != nil {
		return accumulateError(err)
	}

	// Query directly
	if req.TxIdUrl != nil {
		return jrpcFormatResponse(waitFor(func() (*TransactionQueryResponse, error) {
			return queryTx(m.NetV3, ctx, req.TxIdUrl, req.Prove, req.IgnorePending, true)
		}, req.Wait, m.TxMaxWaitTime))
	}

	var hash [32]byte
	switch len(req.Txid) {
	case 0:
		return accumulateError(errors.BadRequest.WithFormat("no transaction ID present in request"))
	case 32:
		hash = *(*[32]byte)(req.Txid)
	default:
		return accumulateError(errors.BadRequest.WithFormat("invalid transaction hash length: want 32, got %d", len(req.Txid)))
	}

	q := &api.MessageHashSearchQuery{Hash: hash}
	return jrpcFormatResponse(waitFor(func() (*TransactionQueryResponse, error) {
		r, err := rangeOf[*api.TxIDRecord](m.NetV3.Query(ctx, protocol.UnknownUrl(), q))
		if err != nil {
			return nil, err
		}
		if r.Total == 0 {
			return nil, errors.NotFound.WithFormat("transaction %X not found", hash[:8])
		}

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		errg, ctx := errgroup.WithContext(ctx)
		ch := make(chan *TransactionQueryResponse)
		seen := map[[32]byte]bool{}
		for _, r := range r.Records {
			txid := r.Value
			if seen[txid.Hash()] {
				continue
			}
			seen[txid.Hash()] = true
			errg.Go(func() error {
				r, err := queryTx(m.NetV3, ctx, txid, req.Prove, req.IgnorePending, true)
				switch {
				case err == nil:
					ch <- r
					return nil
				case errors.Is(err, errors.NotFound):
					return nil
				default:
					return err
				}
			})
		}

		for r := range ch {
			if r.Status.Remote() {
				continue
			}
			return r, nil
		}

		err = errg.Wait()
		if err != nil {
			return nil, err
		}

		return nil, errors.NotFound.WithFormat("transaction %X not found", hash[:8])
	}, req.Wait, m.TxMaxWaitTime))
}
