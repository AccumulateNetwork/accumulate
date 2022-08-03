package api

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

func (m *JrpcMethods) QueryTxLocal(ctx context.Context, params json.RawMessage) interface{} {
	req := new(TxnQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	return jrpcFormatResponse(m.querier.QueryTxLocal(req.Txid, req.Wait, req.IgnorePending, req.QueryOptions))
}

func (m *JrpcMethods) QueryTx(ctx context.Context, params json.RawMessage) interface{} {
	if m.globals == nil {
		return accumulateError(errors.Format(errors.StatusUninitialized, "globals have not been initialized"))
	}

	req := new(TxnQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	resCh := make(chan interface{})        // Result channel
	errCh := make(chan error)              // Error channel
	doneCh := make(chan struct{})          // Completion channel
	wg := new(sync.WaitGroup)              // Wait for completion
	wg.Add(len(m.globals.GetPartitions())) //

	// Mark complete on return
	defer close(doneCh)

	go func() {
		// Wait for all queries to complete
		wg.Wait()

		// If all queries are done and no error or result has been produced, the
		// record must not exist
		select {
		case errCh <- errors.NotFound("transaction %X not found", req.Txid[:8]):
		case <-doneCh:
		}
	}()

	// Create a request for each client in a separate goroutine
	for _, subnet := range m.globals.GetPartitions() {
		go func(subnetId string) {
			// Mark complete on return
			defer wg.Done()

			var result *TransactionQueryResponse
			var rpcErr jsonrpc2.Error
			err := m.Router.RequestAPIv2(ctx, subnetId, "query-tx-local", params, &result)
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
		}(subnet)
	}

	// Wait for an error or a result
	select {
	case res := <-resCh:
		return res
	case err := <-errCh:
		return accumulateError(err)
	}
}
