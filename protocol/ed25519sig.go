package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

var _ Signature = (*ED25519Sig)(nil) // Verify at compile time that ED25519Sig implements the Signature interface

// GetNonce
// Returns the nonce for this signature.  All signatures use a nonce, and this
// is done to avoid replay attacks.
func (e *ED25519Sig) GetNonce() uint64 {
	return e.Nonce
}

// GetPublicKey
// Return the Public Key used by this signature
func (e *ED25519Sig) GetPublicKey() []byte {
	return e.PublicKey
}

// GetSignature
// Returns the signature used to sign the hash of some transaction
func (e *ED25519Sig) GetSignature() []byte {
	return e.Signature
}

// Sign
// Returns the signature for the given message.  What happens is the message
// is hashed with sha256, then the hash is signed.  The signature of the hash
// is returned.
func (e *ED25519Sig) Sign(nonce uint64, privateKey []byte, hash []byte) error {
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

// CanVerify
// will check to see if the keyHash of the public key, matches the key
// hash of the key that is part of this transaction. This is useful for
// situations where we need to see if the sender has permission to sign
// the transaction.
func (e *ED25519Sig) CanVerify(keyHash []byte) bool {
	if keyHash == nil { //if no key hash is provided assume the caller has no specification to compare
		return true
	}
	pubKeyHash := sha256.Sum256(e.PublicKey)
	return bytes.Compare(keyHash, pubKeyHash[:]) == 0
}

func (e *ED25519Sig) WellFormed() bool {
	return len(e.PublicKey) == 32 && len(e.Signature) == 64
}

// Verify
// Returns true if the signature matches the message.  Involves a hash of the
// message.
func (e *ED25519Sig) Verify(hash []byte) bool {
	return e.WellFormed() &&
		ed25519.Verify( //                               ed25519 verify signature
			e.PublicKey, //                                     with the public key of the signature
			append(common.Uint64Bytes(e.Nonce), hash...), //    Of the nonce+transaction hash
			e.Signature) //                                     with the signature
}
