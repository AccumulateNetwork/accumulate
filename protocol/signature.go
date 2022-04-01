package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

var ErrCannotInitiate = errors.New("signature cannot initiate a transaction: values are missing")

func signatureHash(sig Signature) []byte {
	// This should never fail unless the signature uses bigints
	data, _ := sig.MarshalBinary()
	return doSha256(data)
}

func doSha256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

/*
 * Legacy ED25519 Signature
 */

func SignLegacyED25519(sig *LegacyED25519Signature, privateKey, txnHash []byte) {
	data := sig.MetadataHash()
	data = append(data, common.Uint64Bytes(sig.Timestamp)...)
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig.Signature = ed25519.Sign(privateKey, hash[:])
}

// GetSigner returns Signer.
func (s *LegacyED25519Signature) GetSigner() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *LegacyED25519Signature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *LegacyED25519Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKey returns PublicKey.
func (s *LegacyED25519Signature) GetPublicKeyHash() []byte { return doSha256(s.PublicKey) }

// GetSignature returns Signature.
func (s *LegacyED25519Signature) GetSignature() []byte { return s.Signature }

// MetadataHash hashes the signature metadata.
func (s *LegacyED25519Signature) MetadataHash() []byte {
	r := *s                  // Copy the struct
	r.Signature = nil        // Clear the signature
	return signatureHash(&r) // Hash it
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *LegacyED25519Signature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// Verify returns true if this signature is a valid legacy ED25519 signature of
// the hash.
func (e *LegacyED25519Signature) Verify(txnHash []byte) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	data := e.MetadataHash()
	data = append(data, common.Uint64Bytes(e.Timestamp)...)
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	return ed25519.Verify(e.PublicKey, hash[:], e.Signature)
}

/*
 * ED25519 Signature
 */

func SignED25519(sig *ED25519Signature, privateKey, txnHash []byte) {
	data := sig.MetadataHash()
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig.Signature = ed25519.Sign(privateKey, hash[:])
}

// GetSigner returns Signer.
func (s *ED25519Signature) GetSigner() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *ED25519Signature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *ED25519Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKey returns PublicKey.
func (s *ED25519Signature) GetPublicKeyHash() []byte { return doSha256(s.PublicKey) }

// GetSignature returns Signature.
func (s *ED25519Signature) GetSignature() []byte { return s.Signature }

// MetadataHash hashes the signature metadata.
func (s *ED25519Signature) MetadataHash() []byte {
	r := *s                  // Copy the struct
	r.Signature = nil        // Clear the signature
	return signatureHash(&r) // Hash it
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *ED25519Signature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

// Verify returns true if this signature is a valid ED25519 signature of the
// hash.
func (e *ED25519Signature) Verify(txnHash []byte) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	data := e.MetadataHash()
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	return ed25519.Verify(e.PublicKey, hash[:], e.Signature)
}

/*
 * RCD1 Signature
 */

func SignRCD1(sig *RCD1Signature, privateKey, txnHash []byte) {
	data := sig.MetadataHash()
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	sig.Signature = ed25519.Sign(privateKey, hash[:])
}

// GetSigner returns Signer.
func (s *RCD1Signature) GetSigner() *url.URL { return s.Signer }

// GetSignerVersion returns SignerVersion.
func (s *RCD1Signature) GetSignerVersion() uint64 { return s.SignerVersion }

// GetTimestamp returns Timestamp.
func (s *RCD1Signature) GetTimestamp() uint64 { return s.Timestamp }

// GetPublicKey returns PublicKey prefixed with the RCD version number.
func (s *RCD1Signature) GetPublicKeyHash() []byte {
	return GetRCDHashFromPublicKey(s.PublicKey, 1)
}

// Verify returns true if this signature is a valid RCD1 signature of the hash.
func (e *RCD1Signature) Verify(txnHash []byte) bool {
	if len(e.PublicKey) != 32 || len(e.Signature) != 64 {
		return false
	}
	data := e.MetadataHash()
	data = append(data, txnHash...)
	hash := sha256.Sum256(data)
	return ed25519.Verify(e.PublicKey, hash[:], e.Signature)
}

// GetSignature returns Signature.
func (s *RCD1Signature) GetSignature() []byte {
	return s.Signature
}

// MetadataHash hashes the signature metadata.
func (s *RCD1Signature) MetadataHash() []byte {
	r := *s                  // Copy the struct
	r.Signature = nil        // Clear the signature
	return signatureHash(&r) // Hash it
}

// InitiatorHash calculates the Merkle hash of the signature.
func (s *RCD1Signature) InitiatorHash() ([]byte, error) {
	if len(s.PublicKey) == 0 || s.Signer == nil || s.SignerVersion == 0 || s.Timestamp == 0 {
		return nil, ErrCannotInitiate
	}

	hasher := make(hash.Hasher, 0, 4)
	hasher.AddBytes(s.PublicKey)
	hasher.AddUrl(s.Signer)
	hasher.AddUint(s.SignerVersion)
	hasher.AddUint(s.Timestamp)
	return hasher.MerkleHash(), nil
}

/*
 * Receipt Signature
 */

// GetSigner panics.
func (s *ReceiptSignature) GetSigner() *url.URL {
	panic("a receipt does not have a signer")
}

// GetSignerVersion panics.
func (s *ReceiptSignature) GetSignerVersion() uint64 {
	panic("a receipt does not have a signer")
}

// GetTimestamp panics.
func (s *ReceiptSignature) GetTimestamp() uint64 {
	panic("a receipt does not have a signer")
}

// GetPublicKey returns nil.
func (s *ReceiptSignature) GetPublicKeyHash() []byte { return nil }

// GetSignature returns the marshalled receipt.
func (s *ReceiptSignature) GetSignature() []byte {
	b, _ := s.Receipt.MarshalBinary()
	return b
}

// MetadataHash hashes the signature metadata.
func (s *ReceiptSignature) MetadataHash() []byte {
	return signatureHash(s) // Hash it
}

// InitiatorHash panics
func (s *ReceiptSignature) InitiatorHash() ([]byte, error) {
	panic("a receipt signature cannot initiate a transaction")
}

// Verify returns true if this receipt is a valid receipt of the hash.
func (s *ReceiptSignature) Verify(hash []byte) bool {
	return bytes.Equal(s.Start, hash) && s.Receipt.Convert().Validate()
}

/*
 * Synthetic Signature
 */

// GetSigner returns the URL of the destination network's validator key page.
func (s *SyntheticSignature) GetSigner() *url.URL {
	// This is kind of a hack, but it makes things work.
	return FormatKeyPageUrl(s.DestinationNetwork.JoinPath(ValidatorBook), 0)
}

// GetSignerVersion panics.
func (s *SyntheticSignature) GetSignerVersion() uint64 {
	panic("a synthetic signature does not have a signer height")
}

// GetTimestamp panics.
func (s *SyntheticSignature) GetTimestamp() uint64 {
	panic("a synthetic signature does not have a signer height")
}

// GetPublicKey returns nil.
func (s *SyntheticSignature) GetPublicKeyHash() []byte { return nil }

// GetSignature returns nil.
func (s *SyntheticSignature) GetSignature() []byte { return nil }

// MetadataHash hashes the signature metadata.
func (s *SyntheticSignature) MetadataHash() []byte {
	return signatureHash(s) // Hash it
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

// Verify returns true.
func (s *SyntheticSignature) Verify(hash []byte) bool {
	return true
}

/*
 * Internal Signature
 */

// GetSigner returns SourceNetwork.
func (s *InternalSignature) GetSigner() *url.URL { return s.Network }

// GetSignerVersion panics.
func (s *InternalSignature) GetSignerVersion() uint64 {
	panic("an internal signature does not have a signer height")
}

// GetTimestamp panics.
func (s *InternalSignature) GetTimestamp() uint64 {
	panic("an internal signature does not have a signer height")
}

// GetPublicKey returns nil
func (s *InternalSignature) GetPublicKeyHash() []byte { return nil }

// GetSignature returns nil.
func (s *InternalSignature) GetSignature() []byte { return nil }

// MetadataHash hashes the signature metadata.
func (s *InternalSignature) MetadataHash() []byte {
	return signatureHash(s) // Hash it
}

// InitiatorHash panics
func (s *InternalSignature) InitiatorHash() ([]byte, error) {
	if s.Network == nil {
		return nil, ErrCannotInitiate
	}

	return s.Network.AccountID(), nil
}

// Verify returns true.
func (s *InternalSignature) Verify(hash []byte) bool {
	return true
}
