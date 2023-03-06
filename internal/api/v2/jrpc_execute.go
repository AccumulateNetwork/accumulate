// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

	txrq, body, err := constructFaucetTxn(req)
	if err != nil {
		return err
	}
	return m.execute(ctx, txrq, body)
}

func constructFaucetTxn(req *protocol.AcmeFaucet) (*TxRequest, []byte, error) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.FaucetUrl
	txn.Body = req
	env := new(messaging.Envelope)
	env.Transaction = []*protocol.Transaction{txn}
	sig, err := new(signing.Builder).
		UseFaucet().
		UseSimpleHash().
		Initiate(txn)
	if err != nil {
		return nil, nil, accumulateError(err)
	}
	env.Signatures = append(env.Signatures, sig)

	keySig := sig.(protocol.KeySignature)

	txrq := new(TxRequest)
	txrq.Origin = txn.Header.Principal
	txrq.Signer.SignatureType = sig.Type()
	txrq.Signer.Timestamp = keySig.GetTimestamp()
	txrq.Signer.PublicKey = keySig.GetPublicKey()
	txrq.Signer.Url = protocol.FaucetUrl.RootIdentity()
	txrq.Signer.Version = keySig.GetSignerVersion()
	txrq.Signer.UseSimpleHash = true
	txrq.Signature = keySig.GetSignature()

	body, err := txn.Body.MarshalBinary()
	if err != nil {
		return nil, nil, accumulateError(err)
	}

	return txrq, body, nil
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

	case *protocol.BTCSignature:
		sig.PublicKey = r.Signer.PublicKey

	case *protocol.BTCLegacySignature:
		sig.PublicKey = r.Signer.PublicKey

	case *protocol.ETHSignature:
		sig.PublicKey = r.Signer.PublicKey

	default:
		return fmt.Errorf("cannot set the public key on a %T", sig)
	}

	return nil
}

func (r txRequestSigner) Sign(sig protocol.Signature, sigMdHash, message []byte) error {
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.Signature = r.Signature

	case *protocol.ED25519Signature:
		sig.Signature = r.Signature

	case *protocol.RCD1Signature:
		sig.Signature = r.Signature

	case *protocol.BTCSignature:
		sig.Signature = r.Signature

	case *protocol.BTCLegacySignature:
		sig.Signature = r.Signature

	case *protocol.ETHSignature:
		sig.Signature = r.Signature

	default:
		return fmt.Errorf("cannot sign %T with a key", sig)
	}
	return nil
}

// execute either executes the request locally, or dispatches it to another BVC
func (m *JrpcMethods) execute(ctx context.Context, req *TxRequest, payload []byte) interface{} {
	env, err := processExecuteRequest(req, payload)
	if err != nil {
		return err
	}

	return m.submit(m.NetV3, ctx, env, req.CheckOnly)
}

func (m *JrpcMethods) ExecuteDirect(ctx context.Context, params json.RawMessage) interface{} {
	req := new(ExecuteRequest)
	err := json.Unmarshal(params, req)
	if err != nil {
		return validatorError(err)
	}

	return m.submit(m.NetV3, ctx, req.Envelope, req.CheckOnly)
}

func (m *JrpcMethods) ExecuteLocal(ctx context.Context, params json.RawMessage) interface{} {
	req := new(ExecuteRequest)
	err := json.Unmarshal(params, req)
	if err != nil {
		return validatorError(err)
	}

	return m.submit(m.LocalV3, ctx, req.Envelope, req.CheckOnly)
}

func (m *JrpcMethods) submit(v3 V3, ctx context.Context, env *messaging.Envelope, checkOnly bool) interface{} {
	// Marshal the envelope
	txData, err := env.MarshalBinary()
	if err != nil {
		return accumulateError(err)
	}

	// Submit the envelope
	var resp []*api.Submission
	var yes, no = true, false
	if checkOnly {
		resp, err = v3.Validate(ctx, env, api.ValidateOptions{Full: &no})
	} else {
		resp, err = v3.Submit(ctx, env, api.SubmitOptions{Verify: &no, Wait: &yes})
	}
	if err != nil {
		return accumulateError(err)
	}

	// Build the response
	simpleHash := sha256.Sum256(txData)
	res := new(TxResponse)
	for _, r := range resp {
		if !r.Success {
			res.Code = 1
			res.Message = r.Message + "; "
		}
	}
	res.Txid = env.Transaction[0].ID()
	res.TransactionHash = env.Transaction[0].GetHash()
	res.SignatureHashes = make([][]byte, len(env.Signatures))
	res.SimpleHash = simpleHash[:]

	for i, sig := range env.Signatures {
		res.SignatureHashes[i] = sig.Hash()
	}

	if len(resp) == 1 {
		res.Result = resp[0].Status
	} else if len(resp) > 0 {
		var st []*protocol.TransactionStatus
		for _, r := range resp {
			st = append(st, r.Status)
		}
		res.Result = st
	}

	return res
}

func processExecuteRequest(req *TxRequest, payload []byte) (*messaging.Envelope, error) {
	if req.IsEnvelope {
		env := new(messaging.Envelope)
		err := env.UnmarshalBinary(payload)
		return env, err
	}

	body, err := protocol.UnmarshalTransactionBody(payload)
	if err != nil {
		return nil, accumulateError(err)
	}

	// Build the envelope
	txn := new(protocol.Transaction)
	txn.Body = body
	txn.Header.Principal = req.Origin
	txn.Header.Memo = req.Memo
	txn.Header.Metadata = req.Metadata
	env := new(messaging.Envelope)
	env.TxHash = req.TxHash
	env.Transaction = append(env.Transaction, txn)
	if remote, ok := body.(*protocol.RemoteTransaction); ok && len(remote.Hash) == 0 {
		remote.Hash = *(*[32]byte)(env.TxHash)
	}

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
		return nil, validatorError(errors.New("missing signer version"))
	}

	var sig protocol.Signature
	if txn.Body.Type() == protocol.TransactionTypeRemote {
		sig, err = sigBuilder.Sign(txn.GetHash())
	} else {
		sig, err = sigBuilder.Initiate(txn)
	}
	if err != nil {
		return nil, validatorError(err)
	}
	env.Signatures = append(env.Signatures, sig)

	return env, nil
}
