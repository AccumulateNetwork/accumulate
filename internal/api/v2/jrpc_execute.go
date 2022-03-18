package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
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

func (m *JrpcMethods) executeWith(ctx context.Context, params json.RawMessage, payload protocol.TransactionBody, validateFields ...string) interface{} {
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

	env := new(protocol.Envelope)
	env.Transaction = new(protocol.Transaction)
	env.Transaction.Header.Principal = protocol.FaucetUrl
	env.Transaction.Body = req

	sig, err := signing.Faucet(env.Transaction)
	if err != nil {
		return accumulateError(err)
	}
	env.Signatures = append(env.Signatures, sig)

	txrq := new(TxRequest)
	txrq.Origin = env.Transaction.Header.Principal
	txrq.Signer.Nonce = env.Signatures[0].GetTimestamp()
	txrq.Signer.PublicKey = env.Signatures[0].GetPublicKey()
	txrq.Signer.Url = protocol.FaucetUrl
	txrq.KeyPage.Height = env.Signatures[0].GetSignerHeight()
	txrq.Signature = env.Signatures[0].GetSignature()

	body, err := env.Transaction.Body.MarshalBinary()
	if err != nil {
		return accumulateError(err)
	}
	return m.execute(ctx, txrq, body)
}

// execute either executes the request locally, or dispatches it to another BVC
func (m *JrpcMethods) execute(ctx context.Context, req *TxRequest, payload []byte) interface{} {
	var envs []*protocol.Envelope
	var err error
	if req.IsEnvelope {
		// Unmarshal all the envelopes
		envs, err = transactions.UnmarshalAll(payload)
		if err != nil {
			return accumulateError(err)
		}
	} else {
		body, err := protocol.UnmarshalTransaction(payload)
		if err != nil {
			return accumulateError(err)
		}

		// Build the initiator
		initSig := new(protocol.LegacyED25519Signature)
		initSig.Timestamp = req.Signer.Nonce
		initSig.PublicKey = req.Signer.PublicKey
		initSig.Signer = req.Signer.Url
		initSig.SignerHeight = req.KeyPage.Height
		initSig.Signature = req.Signature
		initHash, err := initSig.InitiatorHash()
		if err != nil {
			return validatorError(err)
		}

		// Build the envelope
		env := new(protocol.Envelope)
		env.TxHash = req.TxHash
		env.Transaction = new(protocol.Transaction)
		env.Transaction.Body = body
		env.Transaction.Header.Principal = req.Origin
		env.Transaction.Header.Initiator = *(*[32]byte)(initHash)
		env.Transaction.Header.Memo = req.Memo
		env.Transaction.Header.Metadata = req.Metadata
		env.Signatures = append(env.Signatures, initSig)
		envs = append(envs, env)
	}

	// Route the request
	subnet, err := m.Router.Route(envs...)
	if err != nil {
		return validatorError(err)
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
	var results []*protocol.TransactionStatus
	rd := bytes.NewReader(resp.Data)
	for rd.Len() > 0 {
		status := new(protocol.TransactionStatus)
		err := status.UnmarshalBinaryFrom(rd)
		if err != nil {
			m.logError("Failed to decode transaction results", "error", err)
			break
		}
		results = append(results, status)
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
