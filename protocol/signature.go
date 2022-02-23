package protocol

import (
	"bytes"
	"crypto/ed25519"
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

// GetPublicKey returns PublicKey.
func (e *LegacyED25519Signature) GetPublicKey() []byte {
	return e.PublicKey
}

// GetSignature returns Signature.
func (e *LegacyED25519Signature) GetSignature() []byte {
	return e.Signature
}

// Sign
// Returns the signature for the given message.  What happens is the message
// is hashed with sha256, then the hash is signed.  The signature of the hash
// is returned.
func (e *LegacyED25519Signature) Sign(nonce uint64, privateKey []byte, hash []byte) error {
	e.Nonce = nonce
	nHash := append(common.Uint64Bytes(nonce), hash...) //       Add nonce to hash
	s := ed25519.Sign(privateKey, nHash)                //       Sign the nonce+hash
	e.PublicKey = append([]byte{}, privateKey[32:]...)  //       Note that the last 32 bytes of a private key is the
	if !bytes.Equal(e.PublicKey, privateKey[32:]) {     //       Check that we have the proper keys to sign
		return errors.New("privateKey cannot sign this struct") // Return error that the doesn't match
	}
	e.Signature = s // public key.  Save the signature. // Save away the signature.
	return nil
}

// Verify
// Returns true if the signature matches the message.  Involves a hash of the
// message.
func (e *LegacyED25519Signature) Verify(hash []byte) bool {
	return len(e.PublicKey) == 32 &&
		len(e.Signature) == 64 &&
		ed25519.Verify( //                               ed25519 verify signature
			e.PublicKey, //                                     with the public key of the signature
			append(common.Uint64Bytes(e.Nonce), hash...), //    Of the nonce+transaction hash
			e.Signature) //                                     with the signature
}

// GetPublicKey returns PublicKey.
func (e *ED25519Signature) GetPublicKey() []byte {
	return e.PublicKey
}

// GetSignature returns Signature.
func (e *ED25519Signature) GetSignature() []byte {
	return e.Signature
}

// Sign
// Returns the signature for the given message.  What happens is the message
// is hashed with sha256, then the hash is signed.  The signature of the hash
// is returned.
func (e *ED25519Signature) Sign(nonce uint64, privateKey []byte, hash []byte) error {
	if len(privateKey) != 64 {
		return errors.New("invalid private key")
	}

	e.PublicKey = privateKey[32:]
	e.Signature = ed25519.Sign(privateKey, hash)
	return nil
}

func (e *ED25519Signature) Verify(hash []byte) bool {
	return len(e.PublicKey) == 32 && len(e.Signature) == 64 && ed25519.Verify(e.PublicKey, hash, e.Signature)
}
