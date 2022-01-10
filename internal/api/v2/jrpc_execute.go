package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/ybbus/jsonrpc/v2"
)

func (m *JrpcMethods) Execute(ctx context.Context, params json.RawMessage) interface{} {
	var payload string
	req := new(TxRequest)
	req.Payload = &payload
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	data, err := hex.DecodeString(payload)
	if err != nil {
		return validatorError(err)
	}

	return m.execute(ctx, req, data)
}

func (m *JrpcMethods) ExecuteCreateIdentity(ctx context.Context, params json.RawMessage) interface{} {
	return m.executeWith(ctx, params, new(protocol.CreateIdentity))
}

func (m *JrpcMethods) executeWith(ctx context.Context, params json.RawMessage, payload protocol.TransactionPayload, validateFields ...string) interface{} {
	var raw json.RawMessage
	req := new(TxRequest)
	req.Payload = &raw
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	err = m.parse(raw, payload, validateFields...)
	if err != nil {
		return err
	}

	b, err := payload.MarshalBinary()
	if err != nil {
		return accumulateError(err)
	}

	return m.execute(ctx, req, b)
}

func (m *JrpcMethods) Faucet(ctx context.Context, params json.RawMessage) interface{} {
	req := new(protocol.AcmeFaucet)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	protocol.FaucetWallet.Nonce = uint64(time.Now().UnixNano())
	tx := new(transactions.Envelope)
	tx.Transaction = new(transactions.Transaction)
	tx.Transaction.Origin = protocol.FaucetUrl
	tx.Transaction.Nonce = protocol.FaucetWallet.Nonce
	tx.Transaction.KeyPageHeight = 1
	tx.Transaction.Body, err = req.MarshalBinary()
	if err != nil {
		return accumulateError(err)
	}

	ed := new(transactions.ED25519Sig)
	tx.Signatures = append(tx.Signatures, ed)
	err = ed.Sign(protocol.FaucetWallet.Nonce, protocol.FaucetWallet.PrivateKey, tx.Transaction.Hash())
	if err != nil {
		return accumulateError(err)
	}

	txrq := new(TxRequest)
	txrq.Origin = tx.Transaction.Origin
	txrq.Signer.Nonce = tx.Transaction.Nonce
	txrq.Signer.PublicKey = tx.Signatures[0].PublicKey
	txrq.KeyPage.Height = tx.Transaction.KeyPageHeight
	txrq.Signature = tx.Signatures[0].Signature
	return m.execute(ctx, txrq, tx.Transaction.Body)
}

// executeQueue manages queues for batching and dispatch of execute requests.
type executeQueue struct {
	leader  chan struct{}
	enqueue chan *executeRequest
}

// executeRequest captures the state of an execute requests.
type executeRequest struct {
	remote int
	params json.RawMessage
	result interface{}
	done   chan struct{}
}

// execute either executes the request locally, or dispatches it to another BVC
func (m *JrpcMethods) execute(ctx context.Context, req *TxRequest, payload []byte) interface{} {
	// Route the request
	i := int(req.Origin.Routing() % uint64(len(m.opts.Remote)))
	if i == m.localIndex {
		// We have a local node and the routing number is local, so process the
		// request and broadcast it locally
		return m.executeLocal(ctx, req, payload)
	}

	// Prepare the request for dispatch to a remote BVC
	var err error
	req.Payload = hex.EncodeToString(payload)
	ex := new(executeRequest)
	ex.remote = i
	ex.params, err = req.MarshalJSON()
	if err != nil {
		return accumulateError(err)
	}
	ex.done = make(chan struct{})

	// Either send the request to the active dispatcher, or start a new
	// dispatcher
	select {
	case <-ctx.Done():
		// Request was canceled
		return ErrCanceled

	case m.queue.enqueue <- ex:
		// Request was accepted by the leader

	case <-m.queue.leader:
		// We are the leader, start a new dispatcher
		go m.executeBatch(ex)
	}

	// Wait for dispatch to complete
	select {
	case <-ctx.Done():
		// Canceled
		return ErrCanceled

	case <-ex.done:
		// Completed
		return ex.result
	}
}

// executeLocal constructs a TX, broadcasts it to the local node, and waits for
// results.
func (m *JrpcMethods) executeLocal(ctx context.Context, req *TxRequest, payload []byte) interface{} {

	// Build the TX
	tx := new(transactions.Envelope)
	tx.Transaction = new(transactions.Transaction)
	tx.Transaction.Body = payload
	tx.Transaction.Origin = req.Origin
	tx.Transaction.Nonce = req.Signer.Nonce
	tx.Transaction.KeyPageHeight = req.KeyPage.Height
	tx.Transaction.KeyPageIndex = req.KeyPage.Index

	ed := new(transactions.ED25519Sig)
	ed.Nonce = req.Signer.Nonce
	ed.PublicKey = req.Signer.PublicKey
	ed.Signature = req.Signature
	tx.Signatures = append(tx.Signatures, ed)

	txb, err := tx.MarshalBinary()
	if err != nil {
		return accumulateError(err)
	}

	// The two cases below look virtually identical. Unfortunately, CheckTx and
	// BroadcastTxSync return different types, and the latter does not have any
	// methods, so they have to be handled separately.

	switch {
	case req.CheckOnly:
		// Check the TX
		r, err := m.opts.Local.CheckTx(ctx, txb)
		if err != nil {
			return accumulateError(err)
		}

		res := new(TxResponse)
		res.Code = uint64(r.Code)
		res.Txid = tx.Transaction.Hash()
		res.Hash = sha256.Sum256(txb)

		// Check for errors
		switch {
		case len(r.MempoolError) > 0:
			res.Message = r.MempoolError
			return res
		case len(r.Log) > 0:
			res.Message = r.Log
			return res
		case r.Code != 0:
			res.Message = "An unknown error occured"
			return res
		default:
			return res
		}

	default:
		// Broadcast the TX
		r, err := m.opts.Local.BroadcastTxSync(ctx, txb)
		if err != nil {
			return accumulateError(err)
		}

		res := new(TxResponse)
		res.Code = uint64(r.Code)
		res.Txid = tx.Transaction.Hash()
		res.Hash = sha256.Sum256(txb)

		// Check for errors
		switch {
		case len(r.MempoolError) > 0:
			res.Message = r.MempoolError
			return res
		case len(r.Log) > 0:
			res.Message = r.Log
			return res
		case r.Code != 0:
			res.Message = "An unknown error occured"
			return res
		default:
			return res
		}
	}
}

// executeBatch accepts execute requests for dispatch, then dispatches requests
// in batches to the appropriate remote BVCs.
func (m *JrpcMethods) executeBatch(queue ...*executeRequest) {
	// Free the leader semaphore once we're done
	defer func() { m.queue.leader <- struct{}{} }()

	timer := time.NewTimer(m.opts.QueueDuration)
	defer timer.Stop()

	// Accept requests until we reach the target depth or the timer fires
loop:
	for {
		select {
		case <-timer.C:
			break loop
		case ex := <-m.queue.enqueue:
			queue = append(queue, ex)
			if len(queue) >= m.opts.QueueDepth {
				break loop
			}
		}
	}

	// Construct batches
	lup := map[*jsonrpc.RPCRequest]*executeRequest{}
	batches := make([]jsonrpc.RPCRequests, len(m.remote))
	for _, ex := range queue {
		rq := &jsonrpc.RPCRequest{
			Method: "execute",
			Params: ex.params,
		}
		lup[rq] = ex
		batches[ex.remote] = append(batches[ex.remote], rq)
	}

	for i, rq := range batches {
		var res jsonrpc.RPCResponses
		var err error
		switch len(rq) {
		case 0:
			// Nothing to do
		case 1:
			// Send single (Tendermint JSON-RPC behaves badly)
			m.logDebug("Sending call", "remote", m.opts.Remote[i])
			r, e := m.remote[i].Call(rq[0].Method, rq[0].Params)
			res, err = jsonrpc.RPCResponses{r}, e
		default:
			// Send batch
			m.logDebug("Sending call batch", "remote", m.opts.Remote[i])
			res, err = m.remote[i].CallBatch(rq)
		}

		// Forward results
		for j := range rq {
			ex := lup[rq[j]]
			switch {
			case err != nil:
				m.logError("Execute batch failed", "error", err, "remote", m.opts.Remote[i])
				ex.result = internalError(err)
			case res[j].Error != nil:
				err := res[j].Error
				code := jsonrpc2.ErrorCode(err.Code)
				if code.IsReserved() {
					ex.result = jsonrpc2.NewError(ErrCodeDispatch, err.Message, err)
				} else {
					ex.result = jsonrpc2.NewError(code, err.Message, err.Data)
				}
			default:
				ex.result = res[j].Result
			}
			close(ex.done)
		}
	}
}
