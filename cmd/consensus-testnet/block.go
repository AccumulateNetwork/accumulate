// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// Block represents a block of ordered transactions.
type Block struct {
	Height    uint64    // Block number
	PrevHash  [32]byte  // Hash of previous block
	Timestamp time.Time // When block was created
	TxnCount  uint32    // Number of transactions in this block
	TxnsHash  [32]byte  // Merkle root of transaction hashes
	StateHash [32]byte  // Cumulative state hash (all txns ever)
}

// Hash computes the block hash.
func (b *Block) Hash() [32]byte {
	data := b.Marshal()
	return sha256.Sum256(data)
}

// Marshal serializes the block header.
func (b *Block) Marshal() []byte {
	// height(8) + prevHash(32) + timestamp(8) + txnCount(4) + txnsHash(32) + stateHash(32) = 116
	buf := make([]byte, 116)
	offset := 0

	binary.BigEndian.PutUint64(buf[offset:], b.Height)
	offset += 8

	copy(buf[offset:], b.PrevHash[:])
	offset += 32

	binary.BigEndian.PutUint64(buf[offset:], uint64(b.Timestamp.UnixNano()))
	offset += 8

	binary.BigEndian.PutUint32(buf[offset:], b.TxnCount)
	offset += 4

	copy(buf[offset:], b.TxnsHash[:])
	offset += 32

	copy(buf[offset:], b.StateHash[:])
	return buf
}

// UnmarshalBlock deserializes a block.
func UnmarshalBlock(data []byte) (*Block, error) {
	if len(data) < 116 {
		return nil, fmt.Errorf("block data too short: %d < 116", len(data))
	}

	b := &Block{}
	offset := 0

	b.Height = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	copy(b.PrevHash[:], data[offset:offset+32])
	offset += 32

	b.Timestamp = time.Unix(0, int64(binary.BigEndian.Uint64(data[offset:])))
	offset += 8

	b.TxnCount = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	copy(b.TxnsHash[:], data[offset:offset+32])
	offset += 32

	copy(b.StateHash[:], data[offset:offset+32])

	return b, nil
}

// String returns a human-readable representation of the block.
func (b *Block) String() string {
	hash := b.Hash()
	return fmt.Sprintf("Block{height=%d, prev=%s, txns=%d, hash=%s}",
		b.Height,
		hex.EncodeToString(b.PrevHash[:8]),
		b.TxnCount,
		hex.EncodeToString(hash[:8]))
}

// GenesisBlock creates the genesis block.
func GenesisBlock() *Block {
	return &Block{
		Height:    0,
		PrevHash:  [32]byte{},
		Timestamp: time.Unix(0, 0),
		TxnCount:  0,
		TxnsHash:  sha256.Sum256(nil),
		StateHash: sha256.Sum256(nil),
	}
}

// ComputeTxnsHash computes the merkle root of transaction hashes.
func ComputeTxnsHash(txHashes [][32]byte) [32]byte {
	if len(txHashes) == 0 {
		return sha256.Sum256(nil)
	}

	// Simple approach: hash all tx hashes together
	// A proper merkle tree would be more efficient for proofs
	hasher := sha256.New()
	for _, h := range txHashes {
		h := h // Create local copy to avoid rangevarref lint warning
		hasher.Write(h[:])
	}
	var result [32]byte
	copy(result[:], hasher.Sum(nil))
	return result
}

// UpdateStateHash updates the cumulative state hash with new transactions.
func UpdateStateHash(prevState [32]byte, txHashes [][32]byte) [32]byte {
	hasher := sha256.New()
	hasher.Write(prevState[:])
	for _, h := range txHashes {
		h := h // Create local copy to avoid rangevarref lint warning
		hasher.Write(h[:])
	}
	var result [32]byte
	copy(result[:], hasher.Sum(nil))
	return result
}
