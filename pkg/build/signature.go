// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SignatureBuilder builds a signature. SignatureBuilder should not be
// constructed directly.
type SignatureBuilder struct {
	parser
	transaction *protocol.Transaction
	signatures  []protocol.Signature
	signer      signing.Builder
}

func (b SignatureBuilder) Type(typ protocol.SignatureType) SignatureBuilder {
	b.signer.Type = typ
	return b
}

func (b SignatureBuilder) Url(signer any, path ...string) SignatureBuilder {
	b.signer.Url = b.parseUrl(signer, path...)
	return b
}

func (b SignatureBuilder) Delegator(delegator any, path ...string) SignatureBuilder {
	b.signer.Delegators = append(b.signer.Delegators, b.parseUrl(delegator, path...))
	return b
}

func (b SignatureBuilder) Version(version any) SignatureBuilder {
	b.signer.Version = b.parseUint(version)
	return b
}

func (b SignatureBuilder) Timestamp(timestamp any) SignatureBuilder {
	b.signer.Timestamp = b.parseTimestamp(timestamp)
	return b
}

func (b SignatureBuilder) Signer(signer any) SignatureBuilder {
	switch signer := signer.(type) {
	case signing.Signer:
		b.signer.Signer = signer
	case Signer:
		b.signer.Signer = signerShim{signer}
		b.signer.Type = signer.Address().GetType()
	default:
		b.errorf(errors.BadRequest, "%T is not a supported signer", signer)
	}
	return b
}

func (b SignatureBuilder) PrivateKey(key []byte) SignatureBuilder {
	b.signer.Signer = signing.PrivateKey(key)
	return b
}

func (b SignatureBuilder) Done() (*messaging.Envelope, error) {
	b = b.sign()
	if !b.ok() {
		return nil, b.err()
	}

	env := new(messaging.Envelope)
	env.Transaction = []*protocol.Transaction{b.transaction}
	env.Signatures = b.signatures
	return env, nil
}

func (b SignatureBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.sign().Url(signer, path...)
}

func (b SignatureBuilder) sign() SignatureBuilder {
	if b.transaction == nil {
		panic("transaction is missing")
	}

	// Sign will almost certainly fail if any construction call failed, and
	// there's no point to trying if the transaction couldn't be built, so don't
	// even try
	if !b.ok() {
		return b
	}

	// Always use a simple hash
	b.signer.InitMode = signing.InitWithSimpleHash

	var signature protocol.Signature
	var err error
	if b.transaction.Body.Type() != protocol.TransactionTypeRemote && b.transaction.Header.Initiator == ([32]byte{}) {
		signature, err = b.signer.Initiate(b.transaction)
	} else {
		signature, err = b.signer.Sign(b.transaction.GetHash())
	}
	if err != nil {
		b.errorf(errors.UnknownError, "sign: %w", err)
	} else {
		b.signatures = append(b.signatures, signature)
	}
	return b
}
