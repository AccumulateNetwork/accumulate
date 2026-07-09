// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"
	"sync"
)

// HeaderDigest is a 32-byte SHA-256 hash uniquely identifying a header.
type HeaderDigest [32]byte

// IsZero returns true if the digest is all zeros.
func (d HeaderDigest) IsZero() bool {
	return d == HeaderDigest{}
}

// String returns a hex-encoded string representation of the digest.
func (d HeaderDigest) String() string {
	return hexEncode(d[:])
}

// Round represents a consensus round number.
type Round uint64

// WorkerID identifies a worker node (0-255).
type WorkerID uint8

// Header represents a block header in the DAG.
// Headers reference batches from workers and parent certificates from the previous round.
type Header struct {
	// Author is the public key of the validator that created this header.
	Author ed25519.PublicKey
	// Round is the consensus round number.
	Round Round
	// Epoch is the epoch number for committee changes.
	Epoch uint64
	// Timestamp is the author's wall clock at header creation, in Unix
	// nanoseconds. It is covered by the digest (and thus the signature) and
	// is used as the deterministic block time when the certificate is
	// executed — every validator MUST derive block time from the certificate
	// rather than its own clock, otherwise their state trees diverge and
	// cross-partition anchors can never gather a signature quorum (#4054).
	// Genesis headers use zero.
	Timestamp int64
	// Payload lists the batches this header commits, in canonical order
	// (strictly ascending by digest — NewHeader sorts, UnmarshalHeader
	// rejects anything else). Order is load-bearing: batches are EXECUTED in
	// this order, so it must be identical on every validator. This was a map
	// once, and iterating it made each validator execute batches in its own
	// random order, diverging the state trees (#4054).
	Payload []PayloadEntry
	// Parents contains digests of certificates from round-1 (must have 2f+1).
	Parents []CertificateDigest
	// Signature is the ed25519 signature over the header digest.
	Signature []byte

	// mu protects the cached digest.
	mu sync.RWMutex
	// digest is the cached header digest, computed lazily.
	digest HeaderDigest
	// digestComputed indicates whether the digest has been computed.
	digestComputed bool
}

// PayloadEntry pairs a batch digest with the worker that produced it.
type PayloadEntry struct {
	Digest BatchDigest
	Worker WorkerID
}

// NewHeader creates a new unsigned header. The payload is sorted into
// canonical order (strictly ascending by digest) and deduplicated. Workers
// can queue the same batch digest more than once — identical transaction
// content produces identical batches — and a duplicate must not survive
// here: UnmarshalHeader rejects non-strict order, so a header with a
// duplicate digest is valid to its author but unparseable to every peer,
// which silently wedges the round (#4057).
func NewHeader(
	author ed25519.PublicKey,
	round Round,
	epoch uint64,
	payload []PayloadEntry,
	parents []CertificateDigest,
) *Header {
	sort.Slice(payload, func(i, j int) bool {
		c := bytes.Compare(payload[i].Digest[:], payload[j].Digest[:])
		if c != 0 {
			return c < 0
		}
		return payload[i].Worker < payload[j].Worker
	})
	dedup := payload[:0]
	for i, entry := range payload {
		if i > 0 && entry.Digest == payload[i-1].Digest {
			continue
		}
		dedup = append(dedup, entry)
	}
	payload = dedup
	return &Header{
		Author:  author,
		Round:   round,
		Epoch:   epoch,
		Payload: payload,
		Parents: parents,
	}
}

// Digest returns the SHA-256 hash of the header (excluding the signature).
// The digest is computed lazily and cached for subsequent calls.
func (h *Header) Digest() HeaderDigest {
	h.mu.RLock()
	if h.digestComputed {
		d := h.digest
		h.mu.RUnlock()
		return d
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Double-check after acquiring write lock
	if h.digestComputed {
		return h.digest
	}

	// Serialize and hash (excluding signature)
	data := h.marshalForDigest()
	h.digest = sha256.Sum256(data)
	h.digestComputed = true
	return h.digest
}

// marshalForDigest serializes the header for digest computation (excluding signature).
// Format:
//   - Author (32 bytes)
//   - Round (8 bytes)
//   - Epoch (8 bytes)
//   - Timestamp (8 bytes)
//   - NumPayload (4 bytes)
//   - Payload entries (sorted by digest): [digest (32 bytes)][worker (1 byte)] each
//   - NumParents (4 bytes)
//   - Parents (sorted): [digest (32 bytes)] each
func (h *Header) marshalForDigest() []byte {
	// Calculate size
	size := 32 + 8 + 8 + 8 + 4 + len(h.Payload)*33 + 4 + len(h.Parents)*32

	data := make([]byte, size)
	offset := 0

	// Author
	copy(data[offset:], h.Author)
	offset += 32

	// Round
	binary.BigEndian.PutUint64(data[offset:], uint64(h.Round))
	offset += 8

	// Epoch
	binary.BigEndian.PutUint64(data[offset:], h.Epoch)
	offset += 8

	// Timestamp
	binary.BigEndian.PutUint64(data[offset:], uint64(h.Timestamp))
	offset += 8

	// Payload (already in canonical order — see the Payload field docs)
	binary.BigEndian.PutUint32(data[offset:], uint32(len(h.Payload)))
	offset += 4

	for _, e := range h.Payload {
		e := e // Create local copy to avoid rangevarref lint warning
		copy(data[offset:], e.Digest[:])
		offset += 32
		data[offset] = byte(e.Worker)
		offset++
	}

	// Parents (sorted for deterministic hashing)
	binary.BigEndian.PutUint32(data[offset:], uint32(len(h.Parents)))
	offset += 4

	sortedParents := make([]CertificateDigest, len(h.Parents))
	copy(sortedParents, h.Parents)
	sort.Slice(sortedParents, func(i, j int) bool {
		return bytes.Compare(sortedParents[i][:], sortedParents[j][:]) < 0
	})

	for _, p := range sortedParents {
		p := p // Create local copy to avoid rangevarref lint warning
		copy(data[offset:], p[:])
		offset += 32
	}

	return data
}

// Sign signs the header with the given private key.
// The signature covers the header digest.
func (h *Header) Sign(key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return errors.New("invalid private key size")
	}

	// Verify the public key matches
	pub := key.Public().(ed25519.PublicKey)
	if !bytes.Equal(pub, h.Author) {
		return errors.New("private key does not match header author")
	}

	digest := h.Digest()
	h.Signature = ed25519.Sign(key, digest[:])
	return nil
}

// Verify verifies the header signature.
func (h *Header) Verify() error {
	if len(h.Author) != ed25519.PublicKeySize {
		return errors.New("invalid author public key size")
	}

	if len(h.Signature) != ed25519.SignatureSize {
		return errors.New("invalid signature size")
	}

	digest := h.Digest()
	if !ed25519.Verify(h.Author, digest[:], h.Signature) {
		return errors.New("invalid header signature")
	}

	return nil
}

// Marshal serializes the header to bytes.
func (h *Header) Marshal() ([]byte, error) {
	// Calculate size
	size := 32 + 8 + 8 + 8 + 4 + len(h.Payload)*33 + 4 + len(h.Parents)*32 + 4 + len(h.Signature)

	data := make([]byte, size)
	offset := 0

	// Author
	copy(data[offset:], h.Author)
	offset += 32

	// Round
	binary.BigEndian.PutUint64(data[offset:], uint64(h.Round))
	offset += 8

	// Epoch
	binary.BigEndian.PutUint64(data[offset:], h.Epoch)
	offset += 8

	// Timestamp
	binary.BigEndian.PutUint64(data[offset:], uint64(h.Timestamp))
	offset += 8

	// Payload (already in canonical order)
	binary.BigEndian.PutUint32(data[offset:], uint32(len(h.Payload)))
	offset += 4

	for _, e := range h.Payload {
		e := e // Create local copy to avoid rangevarref lint warning
		copy(data[offset:], e.Digest[:])
		offset += 32
		data[offset] = byte(e.Worker)
		offset++
	}

	// Parents (sorted)
	binary.BigEndian.PutUint32(data[offset:], uint32(len(h.Parents)))
	offset += 4

	sortedParents := make([]CertificateDigest, len(h.Parents))
	copy(sortedParents, h.Parents)
	sort.Slice(sortedParents, func(i, j int) bool {
		return bytes.Compare(sortedParents[i][:], sortedParents[j][:]) < 0
	})

	for _, p := range sortedParents {
		p := p // Create local copy to avoid rangevarref lint warning
		copy(data[offset:], p[:])
		offset += 32
	}

	// Signature
	binary.BigEndian.PutUint32(data[offset:], uint32(len(h.Signature)))
	offset += 4
	copy(data[offset:], h.Signature)

	return data, nil
}

// UnmarshalHeader deserializes a header from bytes.
func UnmarshalHeader(data []byte) (*Header, error) {
	if len(data) < 32+8+8+8+4 {
		return nil, errors.New("header data too short")
	}

	h := &Header{}
	offset := 0

	// Author
	h.Author = make([]byte, 32)
	copy(h.Author, data[offset:offset+32])
	offset += 32

	// Round
	h.Round = Round(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	// Epoch
	h.Epoch = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Timestamp
	h.Timestamp = int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	// Payload
	if offset+4 > len(data) {
		return nil, errors.New("header data truncated: missing payload count")
	}
	numPayload := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numPayload > 100_000 {
		return nil, errors.New("header has too many payload entries")
	}

	h.Payload = make([]PayloadEntry, numPayload)
	for i := uint32(0); i < numPayload; i++ {
		if offset+33 > len(data) {
			return nil, errors.New("header data truncated: missing payload entry")
		}
		copy(h.Payload[i].Digest[:], data[offset:offset+32])
		offset += 32
		h.Payload[i].Worker = WorkerID(data[offset])
		offset++

		// Enforce canonical order: strictly ascending digests. This also
		// rejects duplicates, which would otherwise execute a batch twice —
		// the map representation this replaced deduplicated silently.
		if i > 0 && bytes.Compare(h.Payload[i-1].Digest[:], h.Payload[i].Digest[:]) >= 0 {
			return nil, errors.New("header payload is not in canonical order")
		}
	}

	// Parents
	if offset+4 > len(data) {
		return nil, errors.New("header data truncated: missing parents count")
	}
	numParents := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numParents > 10_000 {
		return nil, errors.New("header has too many parents")
	}

	h.Parents = make([]CertificateDigest, numParents)
	for i := uint32(0); i < numParents; i++ {
		if offset+32 > len(data) {
			return nil, errors.New("header data truncated: missing parent digest")
		}
		copy(h.Parents[i][:], data[offset:offset+32])
		offset += 32
	}

	// Signature
	if offset+4 > len(data) {
		return nil, errors.New("header data truncated: missing signature length")
	}
	sigLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if sigLen > 1024 {
		return nil, errors.New("header signature too large")
	}

	if offset+int(sigLen) > len(data) {
		return nil, errors.New("header data truncated: missing signature")
	}
	h.Signature = make([]byte, sigLen)
	copy(h.Signature, data[offset:offset+int(sigLen)])
	offset += int(sigLen)

	if offset != len(data) {
		return nil, errors.New("header data has trailing bytes")
	}

	return h, nil
}

// Clone creates a deep copy of the header.
func (h *Header) Clone() *Header {
	cloned := h.copyFields()
	return &cloned
}

// copyFields returns a copy of the header's serializable fields as a value.
// This avoids copying the mutex by constructing a new Header with only the
// exported fields. The returned Header has fresh mutex/cache fields.
func (h *Header) copyFields() Header {
	payload := make([]PayloadEntry, len(h.Payload))
	copy(payload, h.Payload)

	parents := make([]CertificateDigest, len(h.Parents))
	copy(parents, h.Parents)

	author := make([]byte, len(h.Author))
	copy(author, h.Author)

	signature := make([]byte, len(h.Signature))
	copy(signature, h.Signature)

	return Header{
		Author:    author,
		Round:     h.Round,
		Epoch:     h.Epoch,
		Timestamp: h.Timestamp,
		Payload:   payload,
		Parents:   parents,
		Signature: signature,
	}
}
