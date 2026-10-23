// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package types defines the core data structures for DAG-based consensus.
// These types are used by the Bullshark-style consensus algorithm to
// organize transactions into batches, headers, and certificates.
package types

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
)

// BatchDigest is a 32-byte SHA-256 hash uniquely identifying a batch.
type BatchDigest [32]byte

// IsZero returns true if the digest is all zeros.
func (d BatchDigest) IsZero() bool {
	return d == BatchDigest{}
}

// String returns a hex-encoded string representation of the digest.
func (d BatchDigest) String() string {
	return hexEncode(d[:])
}

// Batch represents a collection of transactions created by a worker.
// Batches are the atomic units of transaction grouping in the DAG.
type Batch struct {
	// Transactions is the ordered list of raw transaction bytes.
	Transactions [][]byte

	// mu protects the cached digest.
	mu sync.RWMutex
	// digest is the cached batch digest, computed lazily.
	digest BatchDigest
	// digestComputed indicates whether the digest has been computed.
	digestComputed bool
}

// NewBatch creates a new Batch from a slice of transactions.
func NewBatch(transactions [][]byte) *Batch {
	return &Batch{
		Transactions: transactions,
	}
}

// Digest returns the SHA-256 hash of the serialized batch.
// The digest is computed lazily and cached for subsequent calls.
func (b *Batch) Digest() BatchDigest {
	b.mu.RLock()
	if b.digestComputed {
		d := b.digest
		b.mu.RUnlock()
		return d
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	// Double-check after acquiring write lock
	if b.digestComputed {
		return b.digest
	}

	// Serialize and hash
	data, _ := b.marshalForDigest()
	b.digest = sha256.Sum256(data)
	b.digestComputed = true
	return b.digest
}

// marshalForDigest serializes the batch for digest computation.
// Format: [num_txs (4 bytes)] + for each tx: [len (4 bytes)] + [data]
func (b *Batch) marshalForDigest() ([]byte, error) {
	// Calculate total size
	size := 4 // num transactions
	for _, tx := range b.Transactions {
		size += 4 + len(tx) // length prefix + data
	}

	data := make([]byte, size)
	offset := 0

	// Write number of transactions
	binary.BigEndian.PutUint32(data[offset:], uint32(len(b.Transactions)))
	offset += 4

	// Write each transaction
	for _, tx := range b.Transactions {
		binary.BigEndian.PutUint32(data[offset:], uint32(len(tx)))
		offset += 4
		copy(data[offset:], tx)
		offset += len(tx)
	}

	return data, nil
}

// Marshal serializes the batch to bytes.
// Format: [num_txs (4 bytes)] + for each tx: [len (4 bytes)] + [data]
func (b *Batch) Marshal() ([]byte, error) {
	return b.marshalForDigest()
}

// UnmarshalBatch decodes a batch from its wire encoding. It takes OWNERSHIP
// of data: the returned batch's transactions alias the buffer, so the caller
// must not reuse or mutate it afterward.
func UnmarshalBatch(data []byte) (*Batch, error) {
	if len(data) < 4 {
		return nil, errors.New("batch data too short")
	}

	numTxs := binary.BigEndian.Uint32(data[0:4])
	offset := 4

	// Sanity check to prevent excessive allocation
	if numTxs > 1_000_000 {
		return nil, errors.New("batch contains too many transactions")
	}

	transactions := make([][]byte, 0, numTxs)

	for i := uint32(0); i < numTxs; i++ {
		if offset+4 > len(data) {
			return nil, errors.New("batch data truncated: missing transaction length")
		}

		txLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4

		if offset+int(txLen) > len(data) {
			return nil, errors.New("batch data truncated: missing transaction data")
		}

		// Zero-copy: sub-slice the wire buffer instead of copying every
		// transaction out of it. UnmarshalBatch takes OWNERSHIP of data —
		// both callers (pubsub batch messages, fetch-batch responses) hand
		// over per-message buffers that are never reused — so aliasing is
		// safe, and it removes one allocation + copy per transaction from
		// the hottest decode path in the system (#4164: batch decode was the
		// top allocator at 400-700 tx/s). The full-slice expression pins
		// capacity so appends by a caller cannot bleed into the next
		// transaction.
		tx := data[offset : offset+int(txLen) : offset+int(txLen)]
		offset += int(txLen)

		transactions = append(transactions, tx)
	}

	if offset != len(data) {
		return nil, errors.New("batch data has trailing bytes")
	}

	return &Batch{
		Transactions: transactions,
	}, nil
}

// Size returns the total size of all transactions in the batch.
func (b *Batch) Size() int {
	total := 0
	for _, tx := range b.Transactions {
		total += len(tx)
	}
	return total
}

// Len returns the number of transactions in the batch.
func (b *Batch) Len() int {
	return len(b.Transactions)
}

// Clone creates a deep copy of the batch.
func (b *Batch) Clone() *Batch {
	transactions := make([][]byte, len(b.Transactions))
	for i, tx := range b.Transactions {
		transactions[i] = make([]byte, len(tx))
		copy(transactions[i], tx)
	}
	return &Batch{
		Transactions: transactions,
	}
}
