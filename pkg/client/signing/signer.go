package signing

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Signer interface {
	SetPublicKey(protocol.Signature) error
	Sign(protocol.Signature, []byte) error
}

type PrivateKey []byte

func (k PrivateKey) SetPublicKey(sig protocol.Signature) error {
	if len(k) != ed25519.PrivateKeySize {
		return errors.New("invalid private key")
	}

	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.PublicKey = k[32:]

	case *protocol.ED25519Signature:
		sig.PublicKey = k[32:]

	case *protocol.RCD1Signature:
		sig.PublicKey = k[32:]

	default:
		return fmt.Errorf("cannot set the public key on a %T", sig)
	}

	return nil
}

func (k PrivateKey) Sign(sig protocol.Signature, message []byte) error {
	if len(k) != ed25519.PrivateKeySize {
		return errors.New("invalid private key")
	}

	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		protocol.SignLegacyED25519(sig, k, message)

	case *protocol.ED25519Signature:
		protocol.SignED25519(sig, k, message)

	case *protocol.RCD1Signature:
		protocol.SignRCD1(sig, k, message)

	default:
		return fmt.Errorf("cannot sign %T with a key", sig)
	}
	return nil
}
