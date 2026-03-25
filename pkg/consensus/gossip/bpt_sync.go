// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
)

// BPTSyncRequest is a request to fetch missing BPT entries by key hash.
type BPTSyncRequest struct {
	// Keys are the BPT key hashes we need (32-byte hashes of record.Key)
	Keys [][32]byte
	// Requester is the public key of the requesting node
	Requester ed25519.PublicKey
	// RequestID is used for response correlation
	RequestID uint64
}

// BPTSyncResponse is a response containing requested BPT entries.
type BPTSyncResponse struct {
	// Entries contains the requested BPT entries (key hash -> value)
	Entries []BPTEntry
	// RequestID is the correlation ID from the request
	RequestID uint64
	// Missing contains key hashes we don't have
	Missing [][32]byte
}

// BPTEntry represents a single BPT key-value entry.
type BPTEntry struct {
	// KeyHash is the 32-byte hash of the record key
	KeyHash [32]byte
	// Value is the BPT value (typically a 32-byte hash)
	Value []byte
}

// Marshal serializes a BPTSyncRequest.
// Format:
//   - RequestID (8 bytes)
//   - RequesterLen (4 bytes)
//   - Requester (variable, typically 32 bytes)
//   - NumKeys (4 bytes)
//   - Keys (32 bytes each)
func (r *BPTSyncRequest) Marshal() ([]byte, error) {
	if len(r.Keys) > MaxBPTSyncKeys {
		return nil, fmt.Errorf("too many keys: %d > %d", len(r.Keys), MaxBPTSyncKeys)
	}

	size := 8 + 4 + len(r.Requester) + 4 + len(r.Keys)*32
	data := make([]byte, size)
	offset := 0

	// RequestID
	binary.BigEndian.PutUint64(data[offset:], r.RequestID)
	offset += 8

	// Requester
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.Requester)))
	offset += 4
	copy(data[offset:], r.Requester)
	offset += len(r.Requester)

	// Keys
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.Keys)))
	offset += 4

	for _, key := range r.Keys {
		copy(data[offset:], key[:])
		offset += 32
	}

	return data, nil
}

// UnmarshalBPTSyncRequest deserializes a BPTSyncRequest.
func UnmarshalBPTSyncRequest(data []byte) (*BPTSyncRequest, error) {
	if len(data) < 16 { // Minimum: 8 (requestID) + 4 (requesterLen) + 4 (numKeys)
		return nil, errors.New("BPT sync request data too short")
	}

	offset := 0
	r := &BPTSyncRequest{}

	// RequestID
	r.RequestID = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Requester
	requesterLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if requesterLen > 64 {
		return nil, errors.New("requester too large")
	}

	if offset+int(requesterLen) > len(data) {
		return nil, errors.New("BPT sync request truncated: missing requester")
	}

	r.Requester = make([]byte, requesterLen)
	copy(r.Requester, data[offset:offset+int(requesterLen)])
	offset += int(requesterLen)

	// Keys
	if offset+4 > len(data) {
		return nil, errors.New("BPT sync request truncated: missing key count")
	}

	numKeys := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numKeys > MaxBPTSyncKeys {
		return nil, fmt.Errorf("too many keys: %d > %d", numKeys, MaxBPTSyncKeys)
	}

	r.Keys = make([][32]byte, numKeys)
	for i := uint32(0); i < numKeys; i++ {
		if offset+32 > len(data) {
			return nil, errors.New("BPT sync request truncated: missing key")
		}
		copy(r.Keys[i][:], data[offset:offset+32])
		offset += 32
	}

	return r, nil
}

// Marshal serializes a BPTSyncResponse.
// Format:
//   - RequestID (8 bytes)
//   - NumEntries (4 bytes)
//   - For each entry: [keyHash (32 bytes)][valueLen (4 bytes)][value]
//   - NumMissing (4 bytes)
//   - Missing key hashes (32 bytes each)
func (r *BPTSyncResponse) Marshal() ([]byte, error) {
	if len(r.Entries) > MaxBPTSyncEntries {
		return nil, fmt.Errorf("too many entries: %d > %d", len(r.Entries), MaxBPTSyncEntries)
	}

	// Calculate size
	size := 8 + 4 // RequestID + NumEntries

	for _, entry := range r.Entries {
		size += 32 + 4 + len(entry.Value) // keyHash + valueLen + value
	}

	size += 4 + len(r.Missing)*32 // NumMissing + missing keys

	data := make([]byte, size)
	offset := 0

	// RequestID
	binary.BigEndian.PutUint64(data[offset:], r.RequestID)
	offset += 8

	// Entries
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.Entries)))
	offset += 4

	for _, entry := range r.Entries {
		copy(data[offset:], entry.KeyHash[:])
		offset += 32
		binary.BigEndian.PutUint32(data[offset:], uint32(len(entry.Value)))
		offset += 4
		copy(data[offset:], entry.Value)
		offset += len(entry.Value)
	}

	// Missing
	binary.BigEndian.PutUint32(data[offset:], uint32(len(r.Missing)))
	offset += 4

	for _, key := range r.Missing {
		copy(data[offset:], key[:])
		offset += 32
	}

	return data, nil
}

// UnmarshalBPTSyncResponse deserializes a BPTSyncResponse.
func UnmarshalBPTSyncResponse(data []byte) (*BPTSyncResponse, error) {
	if len(data) < 16 { // Minimum: 8 (requestID) + 4 (numEntries) + 4 (numMissing)
		return nil, errors.New("BPT sync response data too short")
	}

	offset := 0
	r := &BPTSyncResponse{}

	// RequestID
	r.RequestID = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Entries
	numEntries := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numEntries > MaxBPTSyncEntries {
		return nil, fmt.Errorf("too many entries: %d > %d", numEntries, MaxBPTSyncEntries)
	}

	r.Entries = make([]BPTEntry, numEntries)
	for i := uint32(0); i < numEntries; i++ {
		if offset+36 > len(data) { // 32 (keyHash) + 4 (valueLen)
			return nil, errors.New("BPT sync response truncated: missing entry header")
		}

		copy(r.Entries[i].KeyHash[:], data[offset:offset+32])
		offset += 32

		valueLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4

		if valueLen > MaxBPTValueSize {
			return nil, errors.New("BPT value too large")
		}

		if offset+int(valueLen) > len(data) {
			return nil, errors.New("BPT sync response truncated: missing entry value")
		}

		r.Entries[i].Value = make([]byte, valueLen)
		copy(r.Entries[i].Value, data[offset:offset+int(valueLen)])
		offset += int(valueLen)
	}

	// Missing
	if offset+4 > len(data) {
		return nil, errors.New("BPT sync response truncated: missing count")
	}

	numMissing := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numMissing > MaxBPTSyncKeys {
		return nil, fmt.Errorf("too many missing keys: %d > %d", numMissing, MaxBPTSyncKeys)
	}

	r.Missing = make([][32]byte, numMissing)
	for i := uint32(0); i < numMissing; i++ {
		if offset+32 > len(data) {
			return nil, errors.New("BPT sync response truncated: missing key hash")
		}
		copy(r.Missing[i][:], data[offset:offset+32])
		offset += 32
	}

	return r, nil
}

// Limits for BPT sync messages.
const (
	// MaxBPTSyncKeys is the maximum number of keys in a BPT sync request.
	MaxBPTSyncKeys = 1000
	// MaxBPTSyncEntries is the maximum number of entries in a BPT sync response.
	MaxBPTSyncEntries = 100
	// MaxBPTValueSize is the maximum size of a BPT value.
	MaxBPTValueSize = 1024 // BPT values are typically 32-byte hashes
)
