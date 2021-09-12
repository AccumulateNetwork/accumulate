package proto

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/AccumulateNetwork/SMT/common"
)

// ED25519Sig
// Implements signing and validating ED25519 signatures
type ED25519Sig struct {
	Nonce     uint64 // We increment the Nonce on the signature to eliminate duplicates
	PublicKey []byte // 32 byte public key
	Signature []byte // a set of 64 byte signatures
}

// Sign
// Returns the signature for the given message.  What happens is the message
// is hashed with sha256, then the hash is signed.  The signature of the hash
// is returned.
func Sign(privateKey []byte, message []byte) *ED25519Sig {
	h := sha256.Sum256(message)
	return SignHash(privateKey, h[:])
}

// SignHash
// Signs the hash of a message.  This is needed to separate the signatures from
// the messages we sign.  In theory we could avoid the extra hash, but the
// libraries we use don't provide a function to avoid the extra hash.
func SignHash(privateKey []byte, hash []byte) *ED25519Sig {
	ed25519sig := new(ED25519Sig)                               // Create a new ed25519sig
	s := ed25519.Sign(privateKey, hash[:])                      // Sign the hash
	ed25519sig.PublicKey = append([]byte{}, privateKey[32:]...) // Note that the last 32 bytes of a private key is the
	ed25519sig.Signature = s                                    // public key.  Save the signature.
	return ed25519sig
}

// Verify
// Returns true if the signature matches the message.  Involves a hash of the
// message.
func (e *ED25519Sig) Verify(message []byte) bool {
	h := sha256.Sum256(message)
	return e.VerifyHash(h[:])
}

// VerifyHash
// Verifies the hash of a message.
func (e *ED25519Sig) VerifyHash(hash []byte) bool {
	return ed25519.Verify(e.PublicKey, hash, e.Signature)
}

// Marshal
// Marshal a signature
func (e *ED25519Sig) Marshal() (data []byte) {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return nil
	}
	data = common.Uint64Bytes(e.Nonce)
	data = append(data, e.PublicKey...)
	data = append(data, e.Signature...)
	return data
}

// Unmarshal
// UnMarshal a signature
func (e *ED25519Sig) Unmarshal(data []byte) []byte {
	e.Nonce, data = common.BytesUint64(data)
	e.PublicKey = append([]byte{}, data[:32]...)
	data = data[32:]
	e.Signature = append([]byte{}, data[:64]...)
	data = data[64:]
	return data
}
