// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package partition

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"
)

// Anchor represents an anchor sent from a BVN to the DN.
// It contains a proof of the BVN's block state that gets included in the DN's ledger.
type Anchor struct {
	// SourcePartition is the name of the partition that created this anchor.
	SourcePartition string
	// BlockIndex is the block height being anchored.
	BlockIndex uint64
	// BlockHash is the state root hash of the anchored block.
	BlockHash [32]byte
	// RootChainAnchor is the root chain anchor (merkle root of chain updates).
	RootChainAnchor [32]byte
	// StateTreeAnchor is the BPT (Binary Patricia Tree) root hash.
	StateTreeAnchor [32]byte
	// Timestamp is when the anchor was created.
	Timestamp time.Time
	// Validator is the public key of the validator that signed this anchor.
	Validator ed25519.PublicKey
	// Signature is the ed25519 signature over the anchor digest.
	Signature []byte

	// cachedDigest caches the computed digest.
	cachedDigest *[32]byte
}

// NewAnchor creates a new unsigned anchor for a block.
func NewAnchor(
	sourcePartition string,
	blockIndex uint64,
	blockHash [32]byte,
	rootChainAnchor [32]byte,
	stateTreeAnchor [32]byte,
) *Anchor {
	return &Anchor{
		SourcePartition: sourcePartition,
		BlockIndex:      blockIndex,
		BlockHash:       blockHash,
		RootChainAnchor: rootChainAnchor,
		StateTreeAnchor: stateTreeAnchor,
		Timestamp:       time.Now(),
	}
}

// Digest computes the SHA-256 hash of the anchor data (excluding signature).
func (a *Anchor) Digest() [32]byte {
	if a.cachedDigest != nil {
		return *a.cachedDigest
	}

	// Serialize anchor data for hashing
	data := a.marshalForDigest()
	digest := sha256.Sum256(data)
	a.cachedDigest = &digest
	return digest
}

// marshalForDigest serializes the anchor fields for hashing (excluding signature).
func (a *Anchor) marshalForDigest() []byte {
	// Calculate size
	size := len(a.SourcePartition) + 8 + 32 + 32 + 32 + 8 + len(a.Validator)
	data := make([]byte, 0, size)

	// Source partition (length-prefixed)
	partBytes := []byte(a.SourcePartition)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(partBytes)))
	data = append(data, lenBuf...)
	data = append(data, partBytes...)

	// Block index
	idxBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(idxBuf, a.BlockIndex)
	data = append(data, idxBuf...)

	// Hashes
	data = append(data, a.BlockHash[:]...)
	data = append(data, a.RootChainAnchor[:]...)
	data = append(data, a.StateTreeAnchor[:]...)

	// Timestamp (Unix nano)
	tsBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBuf, uint64(a.Timestamp.UnixNano()))
	data = append(data, tsBuf...)

	// Validator public key
	data = append(data, a.Validator...)

	return data
}

// Sign signs the anchor with the given private key.
func (a *Anchor) Sign(key ed25519.PrivateKey) {
	// Clear cached digest since we're about to sign
	a.cachedDigest = nil

	digest := a.Digest()
	a.Signature = ed25519.Sign(key, digest[:])
}

// Verify verifies the anchor signature.
func (a *Anchor) Verify() error {
	if len(a.Validator) != ed25519.PublicKeySize {
		return errors.New("invalid validator public key size")
	}
	if len(a.Signature) != ed25519.SignatureSize {
		return errors.New("invalid signature size")
	}

	digest := a.Digest()
	if !ed25519.Verify(a.Validator, digest[:], a.Signature) {
		return errors.New("invalid anchor signature")
	}
	return nil
}

// Marshal serializes the anchor to bytes.
func (a *Anchor) Marshal() ([]byte, error) {
	// Calculate size
	size := 4 + len(a.SourcePartition) + // source partition (length-prefixed)
		8 + // block index
		32 + 32 + 32 + // hashes
		8 + // timestamp
		4 + len(a.Validator) + // validator (length-prefixed)
		4 + len(a.Signature) // signature (length-prefixed)

	data := make([]byte, size)
	offset := 0

	// Source partition
	binary.BigEndian.PutUint32(data[offset:], uint32(len(a.SourcePartition)))
	offset += 4
	copy(data[offset:], a.SourcePartition)
	offset += len(a.SourcePartition)

	// Block index
	binary.BigEndian.PutUint64(data[offset:], a.BlockIndex)
	offset += 8

	// Hashes
	copy(data[offset:], a.BlockHash[:])
	offset += 32
	copy(data[offset:], a.RootChainAnchor[:])
	offset += 32
	copy(data[offset:], a.StateTreeAnchor[:])
	offset += 32

	// Timestamp
	binary.BigEndian.PutUint64(data[offset:], uint64(a.Timestamp.UnixNano()))
	offset += 8

	// Validator
	binary.BigEndian.PutUint32(data[offset:], uint32(len(a.Validator)))
	offset += 4
	copy(data[offset:], a.Validator)
	offset += len(a.Validator)

	// Signature
	binary.BigEndian.PutUint32(data[offset:], uint32(len(a.Signature)))
	offset += 4
	copy(data[offset:], a.Signature)

	return data, nil
}

// UnmarshalAnchor deserializes an anchor from bytes.
func UnmarshalAnchor(data []byte) (*Anchor, error) {
	if len(data) < 4 {
		return nil, errors.New("anchor data too short")
	}

	offset := 0
	a := &Anchor{}

	// Source partition
	partLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if offset+int(partLen) > len(data) {
		return nil, errors.New("anchor data truncated: source partition")
	}
	a.SourcePartition = string(data[offset : offset+int(partLen)])
	offset += int(partLen)

	// Block index
	if offset+8 > len(data) {
		return nil, errors.New("anchor data truncated: block index")
	}
	a.BlockIndex = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Hashes
	if offset+96 > len(data) {
		return nil, errors.New("anchor data truncated: hashes")
	}
	copy(a.BlockHash[:], data[offset:offset+32])
	offset += 32
	copy(a.RootChainAnchor[:], data[offset:offset+32])
	offset += 32
	copy(a.StateTreeAnchor[:], data[offset:offset+32])
	offset += 32

	// Timestamp
	if offset+8 > len(data) {
		return nil, errors.New("anchor data truncated: timestamp")
	}
	tsNano := binary.BigEndian.Uint64(data[offset:])
	a.Timestamp = time.Unix(0, int64(tsNano))
	offset += 8

	// Validator
	if offset+4 > len(data) {
		return nil, errors.New("anchor data truncated: validator length")
	}
	valLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if offset+int(valLen) > len(data) {
		return nil, errors.New("anchor data truncated: validator")
	}
	a.Validator = make([]byte, valLen)
	copy(a.Validator, data[offset:offset+int(valLen)])
	offset += int(valLen)

	// Signature
	if offset+4 > len(data) {
		return nil, errors.New("anchor data truncated: signature length")
	}
	sigLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if offset+int(sigLen) > len(data) {
		return nil, errors.New("anchor data truncated: signature")
	}
	a.Signature = make([]byte, sigLen)
	copy(a.Signature, data[offset:offset+int(sigLen)])

	return a, nil
}

// String returns a human-readable representation of the anchor.
func (a *Anchor) String() string {
	return "Anchor{" + a.SourcePartition + " block=" + string(rune(a.BlockIndex)) +
		" hash=" + hex.EncodeToString(a.BlockHash[:8]) + "...}"
}

// AnchorReceipt is a receipt proving that an anchor was included in a DN block.
// BVNs use receipts to verify cross-partition transaction finality.
type AnchorReceipt struct {
	// SourcePartition is the partition that sent the anchor.
	SourcePartition string
	// AnchorBlockIdx is the BVN block index that was anchored.
	AnchorBlockIdx uint64
	// AnchorBlockHash is the BVN block hash that was anchored.
	AnchorBlockHash [32]byte
	// DNBlockIndex is the DN block that included this anchor.
	DNBlockIndex uint64
	// DNBlockHash is the DN block hash.
	DNBlockHash [32]byte
	// Timestamp is when the receipt was created.
	Timestamp time.Time
	// Proof is the merkle proof of anchor inclusion (optional).
	Proof []byte
}

// NewAnchorReceipt creates a new anchor receipt.
func NewAnchorReceipt(
	sourcePartition string,
	anchorBlockIdx uint64,
	anchorBlockHash [32]byte,
	dnBlockIndex uint64,
	dnBlockHash [32]byte,
) *AnchorReceipt {
	return &AnchorReceipt{
		SourcePartition: sourcePartition,
		AnchorBlockIdx:  anchorBlockIdx,
		AnchorBlockHash: anchorBlockHash,
		DNBlockIndex:    dnBlockIndex,
		DNBlockHash:     dnBlockHash,
		Timestamp:       time.Now(),
	}
}

// Marshal serializes the receipt to bytes.
func (r *AnchorReceipt) Marshal() ([]byte, error) {
	size := 4 + len(r.SourcePartition) + // source partition
		8 + 32 + // anchor block
		8 + 32 + // DN block
		8 + // timestamp
		4 + len(r.Proof) // proof

	data := make([]byte, size)
	offset := 0

	// Source partition
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.SourcePartition)))
	offset += 4
	copy(data[offset:], r.SourcePartition)
	offset += len(r.SourcePartition)

	// Anchor block
	binary.BigEndian.PutUint64(data[offset:], r.AnchorBlockIdx)
	offset += 8
	copy(data[offset:], r.AnchorBlockHash[:])
	offset += 32

	// DN block
	binary.BigEndian.PutUint64(data[offset:], r.DNBlockIndex)
	offset += 8
	copy(data[offset:], r.DNBlockHash[:])
	offset += 32

	// Timestamp
	binary.BigEndian.PutUint64(data[offset:], uint64(r.Timestamp.UnixNano()))
	offset += 8

	// Proof
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.Proof)))
	offset += 4
	copy(data[offset:], r.Proof)

	return data, nil
}

// UnmarshalAnchorReceipt deserializes a receipt from bytes.
func UnmarshalAnchorReceipt(data []byte) (*AnchorReceipt, error) {
	if len(data) < 4 {
		return nil, errors.New("receipt data too short")
	}

	offset := 0
	r := &AnchorReceipt{}

	// Source partition
	partLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if offset+int(partLen) > len(data) {
		return nil, errors.New("receipt data truncated: source partition")
	}
	r.SourcePartition = string(data[offset : offset+int(partLen)])
	offset += int(partLen)

	// Anchor block
	if offset+40 > len(data) {
		return nil, errors.New("receipt data truncated: anchor block")
	}
	r.AnchorBlockIdx = binary.BigEndian.Uint64(data[offset:])
	offset += 8
	copy(r.AnchorBlockHash[:], data[offset:offset+32])
	offset += 32

	// DN block
	if offset+40 > len(data) {
		return nil, errors.New("receipt data truncated: DN block")
	}
	r.DNBlockIndex = binary.BigEndian.Uint64(data[offset:])
	offset += 8
	copy(r.DNBlockHash[:], data[offset:offset+32])
	offset += 32

	// Timestamp
	if offset+8 > len(data) {
		return nil, errors.New("receipt data truncated: timestamp")
	}
	tsNano := binary.BigEndian.Uint64(data[offset:])
	r.Timestamp = time.Unix(0, int64(tsNano))
	offset += 8

	// Proof
	if offset+4 > len(data) {
		return nil, errors.New("receipt data truncated: proof length")
	}
	proofLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if offset+int(proofLen) > len(data) {
		return nil, errors.New("receipt data truncated: proof")
	}
	r.Proof = make([]byte, proofLen)
	copy(r.Proof, data[offset:offset+int(proofLen)])

	return r, nil
}

// DefaultAnchorer is a simple implementation of the Anchorer interface.
type DefaultAnchorer struct {
	partition string
}

// NewDefaultAnchorer creates a new default anchorer.
func NewDefaultAnchorer(partition string) *DefaultAnchorer {
	return &DefaultAnchorer{partition: partition}
}

// CreateAnchor creates an anchor for the given block.
func (a *DefaultAnchorer) CreateAnchor(block *Block) (*Anchor, error) {
	if block == nil {
		return nil, errors.New("block is nil")
	}

	return &Anchor{
		SourcePartition: a.partition,
		BlockIndex:      block.Index,
		BlockHash:       block.Hash,
		// RootChainAnchor and StateTreeAnchor would come from the execution layer
		// For now, we use the block hash as a placeholder
		RootChainAnchor: block.Hash,
		StateTreeAnchor: block.Hash,
		Timestamp:       block.Time,
	}, nil
}

// VerifyAnchor verifies an anchor's signature.
func (a *DefaultAnchorer) VerifyAnchor(anchor *Anchor) error {
	if anchor == nil {
		return errors.New("anchor is nil")
	}
	return anchor.Verify()
}
