package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"crypto/subtle"
	"errors"

	"github.com/FactomProject/ed25519/edwards25519"
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

// GetPublicKey returns PublicKey.
func (e *RCD1Signature) GetPublicKey() []byte {
	return e.PublicKey
}

// GetSignature returns Signature.
func (e *RCD1Signature) GetSignature() []byte {
	return e.Signature
}

// Sign
// Returns the signature for the given message.  What happens is the message
// is hashed with sha256, then the hash is signed.  The signature of the hash
// is returned.
func (e *RCD1Signature) Sign(nonce uint64, privateKey []byte, hash []byte) error {
	if len(privateKey) != 64 {
		return errors.New("invalid private key")
	}

	e.PublicKey = privateKey[32:]
	e.Signature = ed25519.Sign(privateKey, hash)
	return nil
}

func (e *RCD1Signature) Verify(hash []byte) bool {
	if e.Signature[63]&224 != 0 {
		return false
	}

	var A edwards25519.ExtendedGroupElement
	if !A.FromBytes((*[32]byte)(e.PublicKey)) {
		return false
	}

	h := sha512.New()
	h.Write(e.Signature[:32])
	h.Write(e.PublicKey[:])
	h.Write(hash)
	var digest [64]byte
	h.Sum(digest[:0])

	var hReduced [32]byte
	edwards25519.ScReduce(&hReduced, &digest)

	var R edwards25519.ProjectiveGroupElement
	var b [32]byte
	copy(b[:], e.Signature[32:])
	edwards25519.GeDoubleScalarMultVartime(&R, &hReduced, &A, &b)

	var checkR [32]byte
	R.ToBytes(&checkR)
	return subtle.ConstantTimeCompare(e.Signature[:32], checkR[:]) == 1
}
