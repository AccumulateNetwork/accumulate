// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Whitebox testing utilities

type Package struct{}

func (Package) ConstructFaucetTxn(req *protocol.AcmeFaucet) (*TxRequest, []byte, error) {
	return constructFaucetTxnV1(req)
}

func (Package) ProcessExecuteRequest(req *TxRequest, payload []byte) (*messaging.Envelope, error) {
	return processExecuteRequest(req, payload)
}

func constructFaucetTxnV1(req *protocol.AcmeFaucet) (*TxRequest, []byte, error) {
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
