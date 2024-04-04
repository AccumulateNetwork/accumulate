// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type LedgerSignature struct {
}

type EmptySigner struct {
	signing.Signer
	PubKey       []byte
	SignThis     []byte
	preSignature protocol.Signature
}

var (
	UseRealSigner = errors.New("no key to sign")
)

func (l *EmptySigner) SetPublicKey(sig protocol.Signature) error {
	l.preSignature = sig
	if l.PubKey == nil {
		return fmt.Errorf("public key not set")
	}
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.PublicKey = l.PubKey
	case *protocol.ED25519Signature:
		sig.PublicKey = l.PubKey
	case *protocol.RCD1Signature:
		sig.PublicKey = l.PubKey
	case *protocol.BTCSignature:
		sig.PublicKey = l.PubKey
	case *protocol.BTCLegacySignature:
		sig.PublicKey = l.PubKey
	case *protocol.ETHSignature:
		sig.PublicKey = l.PubKey
	case *protocol.DelegatedSignature:
		//do nothing
	default:
		return fmt.Errorf("unsupported signature type %v", sig.Type())
	}
	return nil
}

func (l *EmptySigner) Sign(sig protocol.Signature, sigMdHash []byte, txnHash []byte) error {
	if sigMdHash == nil {
		sigMdHash = sig.Metadata().Hash()
	}
	data := sigMdHash
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	l.SignThis = hash[:]
	return UseRealSigner
}

func (l *EmptySigner) SignTransaction(sig protocol.Signature, txn *protocol.Transaction) error {
	return l.Sign(sig, nil, txn.GetHash())
}

func (l *EmptySigner) PreSignature() protocol.Signature {
	return l.preSignature
}
