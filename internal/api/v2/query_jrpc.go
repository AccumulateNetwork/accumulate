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

	resCh := make(chan interface{})                    // Result channel
	errCh := make(chan error)                          // Error channel
	doneCh := make(chan struct{})                      // Completion channel
	wg := new(sync.WaitGroup)                          // Wait for completion
	wg.Add(len(m.Options.Describe.Network.Partitions)) //

	// Mark complete on return
	defer close(doneCh)

	go func() {
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
	for _, subnet := range m.Options.Describe.Network.Partitions {
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
		}(subnet.Id)
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
	var txid []byte
	if req.Txid != nil {
		txid = req.Txid
	} else if req.TxUrl != nil {
		txId, err2 := req.TxUrl.AsTxID()
		if err2 != nil {
			return nil, errors.Unknown("transaction ID could not be parsed from the txID URL")
		}
		hash := txId.Hash()
		txid = hash[:]
	} else {
		return nil, errors.Unknown("no transaction ID present in request")
	}
	return txid, nil
}

func formatTxIdError(req *TxnQuery) error {
	if req.Txid != nil {
		return errors.NotFound("transaction %X not found", req.Txid[:8])
	} else if req.TxUrl != nil {
		txID, err := req.TxUrl.AsTxID()
		if err != nil {
			return errors.Unknown("transaction ID could not be parsed from the txID URL")
		}
		return errors.NotFound("transaction %s not found", txID.ShortString())
	}
	return errors.Unknown("no transaction ID present in request")
}
