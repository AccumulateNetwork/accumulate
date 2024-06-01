// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Signer signs messages and has an address.
type Signer interface {
	Address() address.Address
	Sign(message []byte) ([]byte, error)
}

// ED25519PrivateKey is an ED25519 private key used as a [Signer].
type ED25519PrivateKey []byte

func (sk ED25519PrivateKey) Address() address.Address {
	return &address.PrivateKey{
		Key: sk,
		PublicKey: address.PublicKey{
			Type: protocol.SignatureTypeED25519,
			Key:  sk[32:],
		},
	}
}

func (sk ED25519PrivateKey) Sign(message []byte) ([]byte, error) {
	return ed25519.Sign(ed25519.PrivateKey(sk), message), nil
}

// signerShim is a shim so [Signer] can be used as a [signing.Signer].
type signerShim struct{ Signer }

func (k signerShim) SetPublicKey(sig protocol.Signature) error {
	pk, ok := k.Address().GetPublicKey()
	if !ok {
		return errors.BadRequest.With("signer cannot provide public key")
	}

	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.PublicKey = pk

	case *protocol.ED25519Signature:
		sig.PublicKey = pk

	case *protocol.RCD1Signature:
		sig.PublicKey = pk

	// case *protocol.BTCSignature:
	// case *protocol.BTCLegacySignature:
	// case *protocol.ETHSignature:

	default:
		return fmt.Errorf("unsupported key type %v", sig.Type())
	}

	return nil
}

func (k signerShim) Sign(sig protocol.Signature, sigMdHash, message []byte) error {
	if k.Address().GetType() != sig.Type() {
		return errors.BadRequest.WithFormat("cannot create a %v signature with a %v key", sig.Type(), k.Address().GetType())
	}
	if sigMdHash == nil {
		sigMdHash = sig.Metadata().Hash()
	}

	var err error
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		data := sigMdHash
		data = append(data, common.Uint64Bytes(sig.Timestamp)...)
		data = append(data, message...)
		hash := sha256.Sum256(data)
		sig.Signature, err = k.Signer.Sign(hash[:])

	case *protocol.ED25519Signature:
		data := sigMdHash
		data = append(data, message...)
		hash := sha256.Sum256(data)
		sig.Signature, err = k.Signer.Sign(hash[:])

	case *protocol.RCD1Signature:
		data := sigMdHash
		data = append(data, message...)
		hash := sha256.Sum256(data)
		sig.Signature, err = k.Signer.Sign(hash[:])

	default:
		return fmt.Errorf("unsupported key type %v", sig.Type())
	}
	return errors.UnknownError.Wrap(err)
}
