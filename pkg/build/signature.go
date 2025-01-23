// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SignatureBuilder builds a signature. SignatureBuilder should not be
// constructed directly.
type SignatureBuilder struct {
	parser
	transaction *protocol.Transaction
	message     messaging.Message
	signatures  []protocol.Signature

	signer signing.Builder
	noInit bool

	// Ignore64Byte (when set) stops the signature builder from automatically
	// correcting a transaction header or body that marshals to 64 bytes.
	Ignore64Byte bool

	// Use a Merkle hash to initiate the transaction. Only intended for testing
	// purposes.
	//
	// Deprecated: Use plain hashes for initiation instead.
	InitMerkle bool
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

func (b SignatureBuilder) Delegators(delegators ...*url.URL) SignatureBuilder {
	b.signer.Delegators = append(b.signer.Delegators, delegators...)
	return b
}

func (b SignatureBuilder) Accept() SignatureBuilder {
	b.signer.Vote = protocol.VoteTypeAccept
	return b
}

func (b SignatureBuilder) Reject() SignatureBuilder {
	b.signer.Vote = protocol.VoteTypeReject
	return b
}

func (b SignatureBuilder) Abstain() SignatureBuilder {
	b.signer.Vote = protocol.VoteTypeAbstain
	return b
}

func (b SignatureBuilder) Suggest() SignatureBuilder {
	b.signer.Vote = protocol.VoteTypeSuggest
	return b
}

func (b SignatureBuilder) Vote(vote protocol.VoteType) SignatureBuilder {
	b.signer.Vote = vote
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

// NoInitiator tells the signature builder not to initiate the transaction, even
// if the initiator hash is not set.
func (b SignatureBuilder) NoInitiator() SignatureBuilder {
	b.noInit = true
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

func (b SignatureBuilder) Memo(s string) SignatureBuilder {
	b.signer.Memo = s
	return b
}

func (b SignatureBuilder) Metadata(v any) SignatureBuilder {
	b.signer.Data = b.parseBytes(v)
	return b
}

func (b SignatureBuilder) PrivateKey(key any) SignatureBuilder {
	addr := b.parseKey(key, b.signer.Type, true)
	sk, ok := addr.GetPrivateKey()
	if !ok {
		b.errorf(errors.BadRequest, "%v is not a private key", addr)
		return b
	}

	b.signer.Signer = signing.PrivateKey(sk)
	if b.signer.Type == 0 {
		b.signer.Type = addr.GetType()
	}
	return b
}

func (b SignatureBuilder) Done() (*messaging.Envelope, error) {
	b = b.sign()
	if !b.ok() {
		return nil, b.err()
	}

	env := new(messaging.Envelope)
	if b.transaction != nil {
		env.Transaction = []*protocol.Transaction{b.transaction}
	}
	if b.message != nil {
		env.Messages = []messaging.Message{b.message}
	}
	env.Signatures = b.signatures
	return env, nil
}

func (b SignatureBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.sign().Url(signer, path...)
}

func (b SignatureBuilder) sign() SignatureBuilder {
	if b.transaction == nil && b.message == nil {
		panic("transaction is missing")
	}

	// Sign will almost certainly fail if any construction call failed, and
	// there's no point to trying if the transaction couldn't be built, so don't
	// even try
	if !b.ok() {
		return b
	}

	// Adjust the body length (header is handled by the signature builder)
	if !b.adjustBody() {
		return b
	}

	if b.InitMerkle {
		b.signer.InitMode = signing.InitWithMerkleHash //nolint:staticcheck // Yes, this is deprecated. So is InitMerkle.
	} else {
		b.signer.InitMode = signing.InitWithSimpleHash
	}

	var signature protocol.Signature
	var err error
	b.signer.Ignore64Byte = b.Ignore64Byte
	switch {
	case b.message != nil:
		h := b.message.Hash()
		signature, err = b.signer.Sign(h[:])
	case b.noInit,
		b.transaction.Body.Type() == protocol.TransactionTypeRemote,
		b.transaction.Header.Initiator != [32]byte{}:
		signature, err = b.signer.Sign(b.transaction.GetHash())
	default:
		signature, err = b.signer.Initiate(b.transaction)
	}
	if err != nil {
		b.errorf(errors.UnknownError, "sign: %w", err)
	} else {
		b.signatures = append(b.signatures, signature)
	}

	// Reset the signer fields
	b.signer = signing.Builder{}
	b.noInit = false
	return b
}

func (b *SignatureBuilder) adjustBody() bool {
	if b.Ignore64Byte || b.transaction == nil {
		return b.ok()
	}

	// Is the body exactly 64 bytes?
	body, err := b.transaction.Body.MarshalBinary()
	if err != nil {
		b.errorf(errors.EncodingError, "marshal body: %w", err)
		return b.ok()
	}
	if len(body) != 64 {
		return b.ok()
	}

	// Copy to reset the cached hash if there is one
	b.transaction = b.transaction.Copy()

	// Pad
	body = append(body, 0)
	b.transaction.Body, err = protocol.UnmarshalTransactionBody(body)
	if err != nil {
		b.errorf(errors.EncodingError, "unmarshal body: %w", err)
		return b.ok()
	}

	return b.ok()
}
