package build

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SignatureBuilder struct {
	parser
	env    EnvelopeBuilder
	signer signing.Builder
}

func (b SignatureBuilder) Type(typ protocol.SignatureType) SignatureBuilder {
	b.signer.Type = typ
	return b
}

func (b SignatureBuilder) Url(signer any, path ...string) SignatureBuilder {
	b.signer.Url = b.parseUrl(signer, path...)
	return b
}

func (b SignatureBuilder) Delegator(delegator any) SignatureBuilder {
	b.signer.Delegators = append(b.signer.Delegators, b.parseUrl(delegator))
	return b
}

func (b SignatureBuilder) Version(version any) SignatureBuilder {
	b.signer.Version = b.parseUint(version)
	return b
}

func (b SignatureBuilder) Timestamp(timestamp any) SignatureBuilder {
	b.signer.Timestamp = signing.TimestampFromValue(b.parseUint(timestamp))
	return b
}

func (b SignatureBuilder) Signer(signer signing.Signer) SignatureBuilder {
	b.signer.Signer = signer
	return b
}

func (b SignatureBuilder) PrivateKey(key []byte) SignatureBuilder {
	b.signer.Signer = signing.PrivateKey(key)
	return b
}

// FinishSignature completes the signature and returns an EnvelopeBuilder.
// FinishSignature will panic if called on a SignatureBuilder that was not
// created with Transaction().Sign().
func (b SignatureBuilder) FinishSignature() EnvelopeBuilder {
	// Sign will almost certainly fail if one of the With methods failed so
	// don't try
	if !b.ok() {
		b.env.record(b.errs...)
		return b.env
	}

	if b.env.transaction == nil {
		panic("transaction is missing")
	}

	signature, err := b.sign(b.env.transaction)
	if err != nil {
		b.env.errorf(errors.StatusUnknownError, "sign: %w", err)
		return b.env
	}

	b.env.signatures = append(b.env.signatures, signature)
	return b.env
}

func (b SignatureBuilder) Build() (*protocol.Envelope, error) {
	return b.FinishSignature().Build()
}

func (b SignatureBuilder) SignWith(signer any, path ...string) SignatureBuilder {
	return b.FinishSignature().SignWith(signer, path...)
}

func (b SignatureBuilder) Sign(txn *protocol.Transaction) (protocol.Signature, error) {
	if !b.ok() {
		return nil, b.err()
	}

	return b.sign(txn)
}

func (b SignatureBuilder) sign(txn *protocol.Transaction) (protocol.Signature, error) {
	// Always use a simple hash
	b.signer.InitMode = signing.InitWithSimpleHash

	if txn.Header.Initiator == ([32]byte{}) {
		return b.signer.Initiate(txn)
	} else {
		return b.signer.Sign(txn.GetHash())
	}
}
