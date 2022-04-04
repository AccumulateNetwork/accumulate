package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

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
	sig, err := new(signing.Builder).
		UseFaucet().
		UseSimpleHash().
		Initiate(env.Transaction)
	if err != nil {
		return accumulateError(err)
	}
	env.Signatures = append(env.Signatures, sig)

	txrq := new(TxRequest)
	txrq.Origin = env.Transaction.Header.Principal
	txrq.Signer.Timestamp = sig.GetTimestamp()
	txrq.Signer.PublicKey = sig.GetPublicKey()
	txrq.Signer.Url = protocol.FaucetUrl
	txrq.Signer.Version = sig.GetSignerVersion()
	txrq.Signer.UseSimpleHash = true
	txrq.Signature = sig.GetSignature()

	body, err := env.Transaction.Body.MarshalBinary()
	if err != nil {
		return accumulateError(err)
	}
	return m.execute(ctx, txrq, body)
}

type txRequestSigner struct {
	*TxRequest
}

func (r txRequestSigner) SetPublicKey(sig protocol.Signature) error {
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.PublicKey = r.Signer.PublicKey

	case *protocol.ED25519Signature:
		sig.PublicKey = r.Signer.PublicKey

	case *protocol.RCD1Signature:
		sig.PublicKey = r.Signer.PublicKey

	default:
		return fmt.Errorf("cannot set the public key on a %T", sig)
	}

	return nil
}

func (r txRequestSigner) Sign(sig protocol.Signature, message []byte) error {
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.Signature = r.Signature

	case *protocol.ED25519Signature:
		sig.Signature = r.Signature

	case *protocol.RCD1Signature:
		sig.Signature = r.Signature

	default:
		return fmt.Errorf("cannot sign %T with a key", sig)
	}
	return nil
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

		// Build the envelope
		env := new(protocol.Envelope)
		envs = append(envs, env)
		env.TxHash = req.TxHash
		env.Transaction = new(protocol.Transaction)
		env.Transaction.Body = body
		env.Transaction.Header.Principal = req.Origin
		env.Transaction.Header.Memo = req.Memo
		env.Transaction.Header.Metadata = req.Metadata

		// Sign and initiate the transaction
		sigBuilder := new(signing.Builder).
			SetType(req.Signer.SignatureType).
			SetTimestamp(req.Signer.Timestamp).
			SetUrl(req.Signer.Url).
			SetSigner(txRequestSigner{req})
		if req.Signer.UseSimpleHash {
			sigBuilder.UseSimpleHash()
		} else {
			sigBuilder.UseMerkleHash()
		}
		if req.Signer.Version != 0 {
			sigBuilder.SetVersion(req.Signer.Version)
		} else if req.KeyPage.Version != 0 {
			sigBuilder.SetVersion(req.KeyPage.Version)
		} else {
			return validatorError(errors.New("missing signer version"))
		}

		var sig protocol.Signature
		if env.Type() == protocol.TransactionTypeRemote {
			sig, err = sigBuilder.Sign(env.GetTxHash())
		} else {
			sig, err = sigBuilder.Initiate(env.Transaction)
		}
		if err != nil {
			return validatorError(err)
		}
		env.Signatures = append(env.Signatures, sig)

		if !env.VerifyTxHash() {
			return validatorError(errors.New("invalid transaction hash"))
		}

		if !sig.Verify(env.GetTxHash()) {
			return validatorError(errors.New("invalid signature"))
		}
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
	res.SignatureHashes = make([][]byte, len(envs[0].Signatures))
	res.SimpleHash = simpleHash[:]

	for i, sig := range envs[0].Signatures {
		res.SignatureHashes[i] = sig.Hash()
	}

	// Parse the results
	results := new(protocol.TransactionResultSet)
	err = results.UnmarshalBinary(resp.Data)
	if err != nil {
		m.logError("Failed to decode transaction results", "error", err)
	}

	if len(results.Results) == 1 {
		res.Result = results.Results[0]
	} else if len(results.Results) > 0 {
		res.Result = results.Results
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
