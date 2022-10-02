// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

func (m *JrpcMethods) QueryTxLocal(ctx context.Context, params json.RawMessage) interface{} {
	req := new(TxnQuery)
	err := m.parse(params, req)
	if err != nil {
		return accumulateError(err)
	}

	txid, err := getTxId(req)
	if err != nil {
		return accumulateError(err)
	}
	return jrpcFormatResponse(m.querier.QueryTxLocal(txid, req.Wait, req.IgnorePending, req.QueryOptions))
}

func (m *JrpcMethods) QueryTx(ctx context.Context, params json.RawMessage) interface{} {
	req := new(TxnQuery)
	err := m.parse(params, req)
	if err != nil {
		return accumulateError(err)
	}

	// When querying with a full transaction ID URL, route the request
	if req.TxIdUrl != nil {
		subnet, err := m.Router.RouteAccount(req.TxIdUrl.Account())
		if err != nil {
			return validatorError(err)
		}

		if subnet == m.Options.Describe.PartitionId {
			return m.QueryTxLocal(ctx, params)
		}

		var result interface{}
		err = m.Router.RequestAPIv2(ctx, subnet, "query-tx-local", params, &result)
		if err != nil {
			return accumulateError(err)
		}
		return result
	}

	resCh := make(chan interface{})                    // Result channel
	errCh := make(chan error)                          // Error channel
	doneCh := make(chan struct{})                      // Completion channel
	wg := new(sync.WaitGroup)                          // Wait for completion
	wg.Add(len(m.Options.Describe.Network.Partitions)) //

	// Mark complete on return
	defer close(doneCh)

	go func() {
		defer logging.Recover(m.logger, "Panicked in QueryTx wait routine", "request", req)

		// Wait for all queries to complete
		wg.Wait()

		// If all queries are done and no error or result has been produced, the
		// record must not exist
		select {
		case errCh <- formatTxIdError(req):
		case <-doneCh:
		}
	}()

	// Create a request for each client in a separate goroutine
	for _, partition := range m.Options.Describe.Network.Partitions {
		go func(partition string) {
			defer logging.Recover(m.logger, "Panicked in QueryTx query routine", "request", req, "partition", partition)

			// Mark complete on return
			defer wg.Done()

			var result *TransactionQueryResponse
			var rpcErr jsonrpc2.Error
			err := m.Router.RequestAPIv2(ctx, partition, "query-tx-local", params, &result)
			switch {
			case err == nil:
				select {
				case resCh <- result:
					// Send the result
				case <-doneCh:
					// A result or error has already been sent
				}
			case !errors.As(err, &rpcErr) || rpcErr.Code != ErrCodeNotFound:
				select {
				case errCh <- err:
					// Send the error
				case <-doneCh:
					// A result or error has already been sent
				}
			}
		}(partition.Id)
	}

	// Wait for an error or a result
	select {
	case res := <-resCh:
		return res
	case err := <-errCh:
		return accumulateError(err)
	}
}

func getTxId(req *TxnQuery) ([]byte, error) {
	switch {
	case len(req.Txid) == 32:
		return req.Txid, nil
	case req.TxIdUrl != nil:
		hash := req.TxIdUrl.Hash()
		return hash[:], nil
	case len(req.Txid) != 0:
		return nil, errors.Format(errors.StatusBadRequest, "invalid transaction hash length: want 32, got %d", len(req.Txid))
	default:
		return nil, errors.Format(errors.StatusBadRequest, "no transaction ID present in request")
	}
}

func formatTxIdError(req *TxnQuery) error {
	hash, err := getTxId(req)
	if err != nil {
		return err
	}
	return errors.NotFound("transaction %X not found", hash[:8])
}
