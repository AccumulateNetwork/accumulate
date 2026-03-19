// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"sort"
)

// CertificateDigest is a 32-byte hash uniquely identifying a certificate.
// It is the same as the header digest since certificates are uniquely
// identified by their headers.
type CertificateDigest [32]byte

// IsZero returns true if the digest is all zeros.
func (d CertificateDigest) IsZero() bool {
	return d == CertificateDigest{}
}

// String returns a hex-encoded string representation of the digest.
func (d CertificateDigest) String() string {
	return hexEncode(d[:])
}

// StateHash is a 32-byte hash representing the state after executing a certificate's transactions.
type StateHash [32]byte

// IsZero returns true if the state hash is all zeros.
func (h StateHash) IsZero() bool {
	return h == StateHash{}
}

// String returns a hex-encoded string representation of the state hash.
func (h StateHash) String() string {
	return hexEncode(h[:])
}

// Certificate represents a certified header with signatures from 2f+1 validators.
// A certificate proves that a quorum of validators have seen and validated the header.
type Certificate struct {
	// Header is the underlying header being certified.
	Header Header
	// Signatures contains the ed25519 signatures from validators who signed this header.
	// Each signature is over the header digest.
	Signatures [][]byte
	// SignedAuthorities contains the indices of validators who signed.
	// This allows efficient lookup in the committee.
	SignedAuthorities []uint16

	// StateHash is the state hash after executing this certificate's transactions.
	// This field is populated after execution and used for state consistency verification.
	// It is NOT part of the certificate digest since it's computed post-execution.
	StateHash StateHash
}

// NewCertificate creates a new certificate from a header and collected signatures.
// The header fields are copied to avoid copying the mutex.
func NewCertificate(header *Header, signatures [][]byte, signedAuthorities []uint16) *Certificate {
	return &Certificate{
		Header:            header.copyFields(),
		Signatures:        signatures,
		SignedAuthorities: signedAuthorities,
	}
}

// SetStateHash sets the state hash after execution.
// This should be called after the certificate's transactions have been executed.
func (c *Certificate) SetStateHash(hash StateHash) {
	c.StateHash = hash
}

// GetStateHash returns the state hash, or zero if not set.
func (c *Certificate) GetStateHash() StateHash {
	return c.StateHash
}

// HasStateHash returns true if a state hash has been set.
func (c *Certificate) HasStateHash() bool {
	return !c.StateHash.IsZero()
}

// Digest returns the certificate digest, which is the same as the header digest.
// Certificates are uniquely identified by their headers.
func (c *Certificate) Digest() CertificateDigest {
	hd := c.Header.Digest()
	return CertificateDigest(hd)
}

// Round returns the round number of this certificate.
func (c *Certificate) Round() Round {
	return c.Header.Round
}

// Author returns the public key of the validator who created this certificate's header.
func (c *Certificate) Author() ed25519.PublicKey {
	return c.Header.Author
}

// Epoch returns the epoch number of this certificate.
func (c *Certificate) Epoch() uint64 {
	return c.Header.Epoch
}

// Parents returns the parent certificate digests.
func (c *Certificate) Parents() []CertificateDigest {
	return c.Header.Parents
}

// Verify verifies the certificate against the given committee.
// It checks that:
//  1. The header signature is valid
//  2. All aggregated signatures are valid
//  3. The signers have sufficient stake (quorum)
//  4. The header author is in the committee
func (c *Certificate) Verify(committee *Committee) error {
	if committee == nil {
		return errors.New("committee is nil")
	}

	// Check epoch matches
	if c.Header.Epoch != committee.Epoch {
		return errors.New("certificate epoch does not match committee epoch")
	}

	// Verify header signature
	if err := c.Header.Verify(); err != nil {
		return err
	}

	// Check author is in committee
	if !committee.ContainsValidator(c.Header.Author) {
		return errors.New("header author is not in committee")
	}

	// Check we have matching signatures and authorities
	if len(c.Signatures) != len(c.SignedAuthorities) {
		return errors.New("signatures and signed authorities count mismatch")
	}

	if len(c.Signatures) == 0 {
		return errors.New("certificate has no signatures")
	}

	// Verify each signature and compute total stake
	headerDigest := c.Header.Digest()

	// Build vote content for verification (headerDigest + round + epoch)
	// This matches what Vote.voteContent() produces
	voteContent := make([]byte, 32+8+8)
	copy(voteContent[0:32], headerDigest[:])
	binary.BigEndian.PutUint64(voteContent[32:40], uint64(c.Header.Round))
	binary.BigEndian.PutUint64(voteContent[40:48], c.Header.Epoch)

	var totalStake uint64
	seen := make(map[uint16]bool)

	for i, sig := range c.Signatures {
		authorityIdx := c.SignedAuthorities[i]

		// Check for duplicate signers
		if seen[authorityIdx] {
			return errors.New("duplicate signer in certificate")
		}
		seen[authorityIdx] = true

		// Get validator
		validator := committee.GetValidator(authorityIdx)
		if validator == nil {
			return errors.New("invalid authority index in certificate")
		}

		// Verify signature
		if len(sig) != ed25519.SignatureSize {
			return errors.New("invalid signature size")
		}

		// Try to verify against voteContent first (from votes)
		// Fall back to headerDigest for backward compatibility
		if !ed25519.Verify(validator.PublicKey, voteContent, sig) {
			if !ed25519.Verify(validator.PublicKey, headerDigest[:], sig) {
				return errors.New("invalid signature in certificate")
			}
		}

		totalStake += validator.Stake
	}

	// Check quorum
	if !committee.HasQuorum(totalStake) {
		return errors.New("certificate does not have quorum")
	}

	return nil
}

// TotalStake returns the total stake of signers.
func (c *Certificate) TotalStake(committee *Committee) uint64 {
	var total uint64
	for _, idx := range c.SignedAuthorities {
		v := committee.GetValidator(idx)
		if v != nil {
			total += v.Stake
		}
	}
	return total
}

// Marshal serializes the certificate to bytes.
func (c *Certificate) Marshal() ([]byte, error) {
	headerData, err := c.Header.Marshal()
	if err != nil {
		return nil, err
	}

	// Calculate size
	size := 4 + len(headerData) + 4 + len(c.Signatures)*68 // 68 = 64 (sig) + 4 (len)

	// Add space for authorities
	size += 4 + len(c.SignedAuthorities)*2

	// Add space for state hash (32 bytes)
	size += 32

	data := make([]byte, size)
	offset := 0

	// Header length and data
	binary.BigEndian.PutUint32(data[offset:], uint32(len(headerData)))
	offset += 4
	copy(data[offset:], headerData)
	offset += len(headerData)

	// Signatures
	binary.BigEndian.PutUint32(data[offset:], uint32(len(c.Signatures)))
	offset += 4

	for _, sig := range c.Signatures {
		binary.BigEndian.PutUint32(data[offset:], uint32(len(sig)))
		offset += 4
		copy(data[offset:], sig)
		offset += len(sig)
	}

	// Signed authorities
	binary.BigEndian.PutUint32(data[offset:], uint32(len(c.SignedAuthorities)))
	offset += 4

	for _, auth := range c.SignedAuthorities {
		binary.BigEndian.PutUint16(data[offset:], auth)
		offset += 2
	}

	// State hash (always write, even if zero)
	copy(data[offset:], c.StateHash[:])
	offset += 32

	return data[:offset], nil
}

// UnmarshalCertificate deserializes a certificate from bytes.
func UnmarshalCertificate(data []byte) (*Certificate, error) {
	if len(data) < 8 {
		return nil, errors.New("certificate data too short")
	}

	offset := 0

	// Header
	headerLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if headerLen > 10_000_000 {
		return nil, errors.New("header too large")
	}

	if offset+int(headerLen) > len(data) {
		return nil, errors.New("certificate data truncated: missing header")
	}

	header, err := UnmarshalHeader(data[offset : offset+int(headerLen)])
	if err != nil {
		return nil, err
	}
	offset += int(headerLen)

	// Signatures
	if offset+4 > len(data) {
		return nil, errors.New("certificate data truncated: missing signature count")
	}

	numSigs := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numSigs > 10_000 {
		return nil, errors.New("too many signatures")
	}

	signatures := make([][]byte, numSigs)
	for i := uint32(0); i < numSigs; i++ {
		if offset+4 > len(data) {
			return nil, errors.New("certificate data truncated: missing signature length")
		}

		sigLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4

		if sigLen > 1024 {
			return nil, errors.New("signature too large")
		}

		if offset+int(sigLen) > len(data) {
			return nil, errors.New("certificate data truncated: missing signature")
		}

		signatures[i] = make([]byte, sigLen)
		copy(signatures[i], data[offset:offset+int(sigLen)])
		offset += int(sigLen)
	}

	// Signed authorities
	if offset+4 > len(data) {
		return nil, errors.New("certificate data truncated: missing authority count")
	}

	numAuth := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numAuth > 10_000 {
		return nil, errors.New("too many authorities")
	}

	if int(numAuth) != len(signatures) {
		return nil, errors.New("authority count does not match signature count")
	}

	authorities := make([]uint16, numAuth)
	for i := uint32(0); i < numAuth; i++ {
		if offset+2 > len(data) {
			return nil, errors.New("certificate data truncated: missing authority index")
		}

		authorities[i] = binary.BigEndian.Uint16(data[offset:])
		offset += 2
	}

	// State hash (optional - only present in newer format)
	var stateHash StateHash
	if offset+32 <= len(data) {
		copy(stateHash[:], data[offset:offset+32])
		offset += 32
	}

	if offset != len(data) {
		return nil, errors.New("certificate data has trailing bytes")
	}

	return &Certificate{
		Header:            header.copyFields(),
		Signatures:        signatures,
		SignedAuthorities: authorities,
		StateHash:         stateHash,
	}, nil
}

// Clone creates a deep copy of the certificate.
func (c *Certificate) Clone() *Certificate {
	signatures := make([][]byte, len(c.Signatures))
	for i, sig := range c.Signatures {
		signatures[i] = make([]byte, len(sig))
		copy(signatures[i], sig)
	}

	authorities := make([]uint16, len(c.SignedAuthorities))
	copy(authorities, c.SignedAuthorities)

	return &Certificate{
		Header:            c.Header.copyFields(),
		Signatures:        signatures,
		SignedAuthorities: authorities,
		StateHash:         c.StateHash,
	}
}

// AddSignature adds a signature to the certificate.
// This is used during certificate aggregation.
func (c *Certificate) AddSignature(authorityIdx uint16, signature []byte) {
	// Insert in sorted order by authority index
	i := sort.Search(len(c.SignedAuthorities), func(i int) bool {
		return c.SignedAuthorities[i] >= authorityIdx
	})

	// Check if already present
	if i < len(c.SignedAuthorities) && c.SignedAuthorities[i] == authorityIdx {
		return // Already have this signature
	}

	// Insert at position i
	c.SignedAuthorities = append(c.SignedAuthorities, 0)
	copy(c.SignedAuthorities[i+1:], c.SignedAuthorities[i:])
	c.SignedAuthorities[i] = authorityIdx

	c.Signatures = append(c.Signatures, nil)
	copy(c.Signatures[i+1:], c.Signatures[i:])
	sigCopy := make([]byte, len(signature))
	copy(sigCopy, signature)
	c.Signatures[i] = sigCopy
}

// HasSignatureFrom returns true if the certificate has a signature from the given validator.
func (c *Certificate) HasSignatureFrom(authorityIdx uint16) bool {
	i := sort.Search(len(c.SignedAuthorities), func(i int) bool {
		return c.SignedAuthorities[i] >= authorityIdx
	})
	return i < len(c.SignedAuthorities) && c.SignedAuthorities[i] == authorityIdx
}
