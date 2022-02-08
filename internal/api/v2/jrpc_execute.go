package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

func (m *JrpcMethods) Execute(ctx context.Context, params json.RawMessage) interface{} {
	var payload string
	req := new(TxRequest)
	req.Payload = &payload
	err := json.Unmarshal(params, req)
	if err != nil {
		return validatorError(err)
	}

	if req.IsEnvelope {
		err = m.validate.StructPartial(req, "Origin", "Payload")
	} else {
		err = m.validate.Struct(req)
	}
	if err != nil {
		return validatorError(err)
	}

	data, err := hex.DecodeString(payload)
	if err != nil {
		return validatorError(err)
	}

	return m.execute(ctx, req, data)
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

	faucet := protocol.Faucet.Signer()
	tx := new(transactions.Envelope)
	tx.Transaction = new(transactions.Transaction)
	tx.Transaction.Origin = protocol.FaucetUrl
	tx.Transaction.Nonce = faucet.Nonce()
	tx.Transaction.KeyPageHeight = 1
	tx.Transaction.Body, err = req.MarshalBinary()
	if err != nil {
		return accumulateError(err)
	}

	ed, err := faucet.Sign(tx.GetTxHash())
	if err != nil {
		return accumulateError(err)
	}
	tx.Signatures = append(tx.Signatures, ed)

	txrq := new(TxRequest)
	txrq.Origin = tx.Transaction.Origin
	txrq.Signer.Nonce = tx.Transaction.Nonce
	txrq.Signer.PublicKey = tx.Signatures[0].PublicKey
	txrq.KeyPage.Height = tx.Transaction.KeyPageHeight
	txrq.Signature = tx.Signatures[0].Signature
	return m.execute(ctx, txrq, tx.Transaction.Body)
}

// execute either executes the request locally, or dispatches it to another BVC
func (m *JrpcMethods) execute(ctx context.Context, req *TxRequest, payload []byte) interface{} {
	// Route the request
	subnet, err := m.Router.Route(req.Origin)
	if err != nil {
		return validatorError(err)
	}

	var envs []*transactions.Envelope
	if req.IsEnvelope {
		// Unmarshal all the envelopes
		envs, err = transactions.UnmarshalAll(payload)
		if err != nil {
			return accumulateError(err)
		}
	} else {
		// Build the envelope
		env := new(transactions.Envelope)
		env.TxHash = req.TxHash
		env.Transaction = new(transactions.Transaction)
		env.Transaction.Body = payload
		env.Transaction.Origin = req.Origin
		env.Transaction.Nonce = req.Signer.Nonce
		env.Transaction.KeyPageHeight = req.KeyPage.Height
		env.Transaction.KeyPageIndex = req.KeyPage.Index
		envs = append(envs, env)

		ed := new(transactions.ED25519Sig)
		ed.Nonce = req.Signer.Nonce
		ed.PublicKey = req.Signer.PublicKey
		ed.Signature = req.Signature
		env.Signatures = append(env.Signatures, ed)
	}

	// Marshal the envelope(s)
	var txData []byte
	for _, env := range envs {
		b, err := env.MarshalBinary()
		if err != nil {
			return accumulateError(err)
		}
		txData = append(txData, b...)
	}

	// Submit the envelope(s)
	resp, err := m.Router.Submit(ctx, subnet, txData, req.CheckOnly, false)
	if err != nil {
		return accumulateError(err)
	}

	// Build the response
	simpleHash := sha256.Sum256(txData)
	res := new(TxResponse)
	res.Code = uint64(resp.Code)
	res.TransactionHash = envs[0].GetTxHash()
	res.EnvelopeHash = envs[0].EnvHash()
	res.SimpleHash = simpleHash[:]

	// Parse the results
	var results []protocol.TransactionResult
	rd := bytes.NewReader(resp.Data)
	for rd.Len() > 0 {
		result, err := protocol.UnmarshalTransactionResultFrom(rd)
		if err != nil {
			m.logError("Failed to decode transaction results", "error", err)
			break
		}
		if _, ok := result.(*protocol.EmptyResult); ok {
			result = nil
		}
		results = append(results, result)
	}

	if len(results) == 1 {
		res.Result = results[0]
	} else if len(results) > 0 {
		res.Result = results
	}

	// Check for errors
	switch {
	case len(resp.MempoolError) > 0:
		res.Message = resp.MempoolError
		return res
	case len(resp.Log) > 0:
		res.Message = resp.Log
		return res
	case len(resp.Info) > 0:
		res.Message = resp.Info
		return res
	case resp.Code != 0:
		res.Message = "An unknown error occured"
		return res
	default:
		return res
	}
}
