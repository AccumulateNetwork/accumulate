package protocol

import (
	"bytes"
	"crypto/ed25519"
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

var ErrCannotInitiate = errors.New("signature cannot initiate a transaction: values are missing")

// GetSigner returns Signer.
func (s *LegacyED25519Signature) GetSigner() *url.URL { return s.Signer }

// GetSigner returns Signer.
func (s *ED25519Signature) GetSigner() *url.URL { return s.Signer }

// GetSigner returns Signer.
func (s *RCD1Signature) GetSigner() *url.URL { return s.Signer }

// GetSigner panics.
func (s *ReceiptSignature) GetSigner() *url.URL { panic("a receipt does not have a signer") }

// GetSigner returns SourceNetwork.
func (s *SyntheticSignature) GetSigner() *url.URL { return s.SourceNetwork }

// GetSignerHeight returns SignerHeight.
func (s *LegacyED25519Signature) GetSignerHeight() uint64 { return s.SignerHeight }

// GetSignerHeight returns SignerHeight.
func (s *ED25519Signature) GetSignerHeight() uint64 { return s.SignerHeight }

// GetSignerHeight returns SignerHeight.
func (s *RCD1Signature) GetSignerHeight() uint64 { return s.SignerHeight }

// GetSignerHeight panics.
func (s *ReceiptSignature) GetSignerHeight() uint64 { panic("a receipt does not have a signer") }

// GetSignerHeight panics.
func (s *SyntheticSignature) GetSignerHeight() uint64 {
	panic("a synthetic signature does not have a signer height")
}

// GetTimestamp returns Timestamp.
func (s *LegacyED25519Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetTimestamp returns Timestamp.
func (s *ED25519Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetTimestamp returns Timestamp.
func (s *RCD1Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetTimestamp panics.
func (s *ReceiptSignature) GetTimestamp() uint64 { panic("a receipt does not have a signer") }

// GetTimestamp panics.
func (s *SyntheticSignature) GetTimestamp() uint64 {
	panic("a synthetic signature does not have a signer height")
}

// GetPublicKey returns PublicKey.
func (s *LegacyED25519Signature) GetPublicKey() []byte { return s.PublicKey }

// GetPublicKey returns PublicKey.
func (s *ED25519Signature) GetPublicKey() []byte { return s.PublicKey }

// GetPublicKey returns PublicKey prefixed with the RCD version number.
func (s *RCD1Signature) GetPublicKey() []byte {
	b := make([]byte, len(s.PublicKey)+1)
	b[0] = 1
	copy(b[1:], s.PublicKey)
	return b
}

// GetPublicKey returns nil
func (s *ReceiptSignature) GetPublicKey() []byte { return nil }

// GetPublicKey returns nil
func (s *SyntheticSignature) GetPublicKey() []byte { return nil }

// GetSignature returns Signature.
func (s *LegacyED25519Signature) GetSignature() []byte { return s.Signature }

// GetSignature returns Signature.
func (s *ED25519Signature) GetSignature() []byte { return s.Signature }

// GetSignature returns Signature.
func (s *RCD1Signature) GetSignature() []byte {
	return s.Signature
}

// GetSignature returns the marshalled receipt.
func (s *ReceiptSignature) GetSignature() []byte {
	b, _ := s.Receipt.MarshalBinary()
	return b
}

// GetSignature returns nil.
func (s *SyntheticSignature) GetSignature() []byte {
	return nil
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *LegacyED25519Signature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerHeight == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerHeight)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *ED25519Signature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerHeight == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerHeight)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *RCD1Signature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerHeight == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerHeight)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// InitiatorHash panics
func (s *ReceiptSignature) InitiatorHash() ([]byte, error) {
	panic("a receipt signature cannot initiate a transaction")
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *SyntheticSignature) InitiatorHash() ([]byte, error) {
	if s.SourceNetwork == nil || s.DestinationNetwork == nil || s.SequenceNumber == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 3)
	hasher.AddUrl(s.SourceNetwork)
	hasher.AddUrl(s.DestinationNetwork)
	hasher.AddUint(s.SequenceNumber)
	return hasher.MerkleHash(), nil
}

// Verify returns true if this signature is a valid legacy ED25519 signature of
// the hash.
func (e *LegacyED25519Signature) Verify(hash []byte) bool {
	nonceHash := append(common.Uint64Bytes(e.Timestamp), hash...)
	return len(e.PublicKey) == 32 && len(e.Signature) == 64 && ed25519.Verify(e.PublicKey, nonceHash, e.Signature)
}

// Verify returns true if this signature is a valid ED25519 signature of the
// hash.
func (e *ED25519Signature) Verify(hash []byte) bool {
	return len(e.PublicKey) == 32 && len(e.Signature) == 64 && ed25519.Verify(e.PublicKey, hash, e.Signature)
}

// Verify returns true if this signature is a valid RCD1 signature of the hash.
func (e *RCD1Signature) Verify(hash []byte) bool {
	return len(e.PublicKey) == 32 && len(e.Signature) == 64 && ed25519.Verify(e.PublicKey, hash, e.Signature)
}

// Verify returns true if this receipt is a valid receipt of the hash.
func (s *ReceiptSignature) Verify(hash []byte) bool {
	return bytes.Equal(s.Start, hash) && s.Receipt.Convert().Validate()
}

// Verify returns true.
func (s *SyntheticSignature) Verify(hash []byte) bool {
	return true
}
