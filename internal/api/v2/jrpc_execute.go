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

// This is effectively, and technically, the entry point of execution on the
// server side within the accumulate network for transactions redirected from
// another node (with action "execute", defined in ./api_gen.go).
//
// The JSON message is passed verbatim to this function. The protocol resolved
// from the action sent with the original request was packaged into this RPC
// so we can just unpackage it here, redirect to execute(), and carry on as if
// this node were the original recipient.
//
// TODO: Update the above reference to execute() if that method's name changes.
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

// This is effectively the entry point of execution on the server side within
// the accumulate network for transactions sent from a client. The technical
// entry point is any one of the methods in ./api_gen.go, selected according
// to an action string sent with the RPC.
//
// The JSON message is passed verbatim to this function. The action is
// resolved to a protocol type which is passed, brand-new and without any
// data, as the third parameter for this function.
func (m *JrpcMethods) executeWith(ctx context.Context, params json.RawMessage, payload protocol.TransactionPayload, validateFields ...string) interface{} {

	// Create a transaction request and unpack the raw JSON into its fields.
	var raw json.RawMessage
	trxRequest := new(TxRequest)
	trxRequest.Payload = &raw
	err := m.parse(params, trxRequest)
	if err != nil {
		return err
	}

	// The transaction request also contains a JSON-ized payload which needs
	// to be unpacked. Until now payload does not have any data, only a type.
	err = m.parse(raw, payload, validateFields...)
	if err != nil {
		return err
	}

	binaryData, err := payload.MarshalBinary()
	if err != nil {
		return accumulateError(err)
	}

	return m.execute(ctx, trxRequest, binaryData)
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

// This method determines where a transaction request should be executed:
//
// If this node is within the correct BVN for processing this transaction, then
// the request is handed down to this node's general executor.
//
// If this node is NOT within the correct BVN for processing this transaction
// then the request is kicked back out to JSON-RPC-2 and sent to a node
// within the appropriate BVN with the "execute" action. Processing resumes
// on the remote node at Execute().
//
// SUGGEST: Having two methods with basically-identical names is confusing.
// This one should be renamed to avoid conflict with Execute. Perhaps something
// like route() ?
func (m *JrpcMethods) execute(ctx context.Context, req *TxRequest, payload []byte) interface{} {

	// This node may not be within the correct BVN for processing this
	// transaction. First we have to determine where it is actually
	// supposed to go.
	destinationIndex := int(req.Origin.Routing() % uint64(len(m.opts.Remote)))
	if destinationIndex == m.localIndex {
		// This node is within the correct BVN, so execute this
		// transaction here.
		return m.executeLocal(ctx, req, payload)
	}

	// At this point we know that this request needs to be sent to a
	// different BVN, so we'll package it up and send it there.

	// Prepare the request for dispatch to a remote BVN
	var err error
	req.Payload = hex.EncodeToString(payload)
	ex := new(executeRequest)
	ex.remote = destinationIndex
	ex.params, err = req.MarshalJSON()
	if err != nil {
		return accumulateError(err)
	}
	ex.done = make(chan struct{})

	// TODO: All this batching stuff is apparently going bye-bye.

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
		//
		//Send this trx back into the JRPC black box for transmission to another
		//node. Lands at Execute() in this file.
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

// SUGGEST: This method's name is very similar to Execute() and execute(), and
// this method doesn't actually execute the transaction: the executors do. To
// avoid confusion this method should be renamed. Perhaps "broadcastLocally" ?
func (m *JrpcMethods) executeLocal(ctx context.Context, req *TxRequest, payload []byte) interface{} {

	// Reconstruct the transaction from the request.
	trxEnvelope := new(transactions.Envelope)
	trxEnvelope.Transaction = new(transactions.Transaction)
	trxEnvelope.Transaction.Body = payload
	trxEnvelope.Transaction.Origin = req.Origin
	trxEnvelope.Transaction.Nonce = req.Signer.Nonce
	trxEnvelope.Transaction.KeyPageHeight = req.KeyPage.Height
	trxEnvelope.Transaction.KeyPageIndex = req.KeyPage.Index

	// We create a new signing mechanism here not because we want it to sign
	// anything, but so we can use it as a vehicle for this transaction's
	// signature data.
	signingMechanism := new(transactions.ED25519Sig)
	signingMechanism.Nonce = req.Signer.Nonce
	signingMechanism.PublicKey = req.Signer.PublicKey
	signingMechanism.Signature = req.Signature
	trxEnvelope.Signatures = append(trxEnvelope.Signatures, signingMechanism)

	evelopeBinary, err := trxEnvelope.MarshalBinary()
	if err != nil {
		return accumulateError(err)
	}

	// The two cases below look virtually identical. Unfortunately, CheckTx and
	// BroadcastTxSync return different types, and the latter does not have any
	// methods, so they have to be handled separately.

	// QUERY: Why did we use a switch here and not a simple if/else? What other
	// case might there be?

	var code uint64
	var mempool, log string
	var data []byte
	switch {
	case req.CheckOnly:
		// Check the transaction to see if it would succeed, but don't actually
		// execute it. CheckOnly is an option sent from the client with the
		// original transaction request.
		tMintResult, err := m.opts.Local.CheckTx(ctx, evelopeBinary)
		if err != nil {
			return accumulateError(err)
		}

		code = uint64(tMintResult.Code)
		mempool = tMintResult.MempoolError
		log = tMintResult.Log
		data = tMintResult.Data

	default:
		// Hand off the transaction to the local Tendermint client, which will
		// then broadcast the transaction to all nodes within the local BVN
		// (including this node) for validation, consensus, and execution.
		// Execution resumes, on each node in the local BVN, at
		// /internal/abci/accumulator.go. For details on how this takes place
		// see step 11 in /docs/developer/Transaction_Trace.md.
		tMintResult, err := m.opts.Local.BroadcastTxSync(ctx, evelopeBinary)
		if err != nil {
			return accumulateError(err)
		}

		code = uint64(tMintResult.Code)
		mempool = tMintResult.MempoolError
		log = tMintResult.Log
		data = tMintResult.Data
	}

	response := new(TxResponse)
	response.Code = code
	response.Txid = trxEnvelope.Transaction.Hash()
	response.Hash = sha256.Sum256(evelopeBinary)

	var results []protocol.TransactionResult
	for len(data) > 0 {
		result, err := protocol.UnmarshalTransactionResult(data)
		if err != nil {
			m.logError("Failed to decode transaction results", "error", err)
			break
		}
		data = data[result.BinarySize():]
		if _, ok := result.(*protocol.EmptyResult); ok {
			result = nil
		}
		results = append(results, result)
	}

	if len(results) == 1 {
		response.Result = results[0]
	} else if len(results) > 0 {
		response.Result = results
	}

	// Check for errors
	switch {
	case len(mempool) > 0:
		response.Message = mempool
		return response
	case len(log) > 0:
		response.Message = log
		return response
	case code != 0:
		response.Message = "An unknown error occured"
		return response
	default:
		return response
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
