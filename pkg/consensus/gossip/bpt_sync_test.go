// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip_test

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
)

func TestBPTSyncRequest_MarshalUnmarshal(t *testing.T) {
	// Generate test data
	requester := make([]byte, 32)
	_, err := rand.Read(requester)
	require.NoError(t, err)

	keys := make([][32]byte, 5)
	for i := range keys {
		_, err := rand.Read(keys[i][:])
		require.NoError(t, err)
	}

	req := &gossip.BPTSyncRequest{
		Keys:      keys,
		Requester: requester,
		RequestID: 12345,
	}

	// Marshal
	data, err := req.Marshal()
	require.NoError(t, err)

	// Unmarshal
	req2, err := gossip.UnmarshalBPTSyncRequest(data)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, req.RequestID, req2.RequestID)
	assert.Equal(t, req.Requester, req2.Requester)
	assert.Equal(t, req.Keys, req2.Keys)
}

func TestBPTSyncResponse_MarshalUnmarshal(t *testing.T) {
	// Generate test data
	entries := make([]gossip.BPTEntry, 3)
	for i := range entries {
		_, err := rand.Read(entries[i].KeyHash[:])
		require.NoError(t, err)
		entries[i].Value = make([]byte, 32)
		_, err = rand.Read(entries[i].Value)
		require.NoError(t, err)
	}

	missing := make([][32]byte, 2)
	for i := range missing {
		_, err := rand.Read(missing[i][:])
		require.NoError(t, err)
	}

	resp := &gossip.BPTSyncResponse{
		Entries:   entries,
		RequestID: 12345,
		Missing:   missing,
	}

	// Marshal
	data, err := resp.Marshal()
	require.NoError(t, err)

	// Unmarshal
	resp2, err := gossip.UnmarshalBPTSyncResponse(data)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, resp.RequestID, resp2.RequestID)
	require.Len(t, resp2.Entries, len(resp.Entries))
	for i := range resp.Entries {
		assert.Equal(t, resp.Entries[i].KeyHash, resp2.Entries[i].KeyHash)
		assert.Equal(t, resp.Entries[i].Value, resp2.Entries[i].Value)
	}
	assert.Equal(t, resp.Missing, resp2.Missing)
}

func TestBPTSyncRequest_TooManyKeys(t *testing.T) {
	keys := make([][32]byte, gossip.MaxBPTSyncKeys+1)
	req := &gossip.BPTSyncRequest{
		Keys:      keys,
		Requester: []byte("test"),
		RequestID: 1,
	}
	_, err := req.Marshal()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many keys")
}

func TestBPTSyncResponse_TooManyEntries(t *testing.T) {
	entries := make([]gossip.BPTEntry, gossip.MaxBPTSyncEntries+1)
	resp := &gossip.BPTSyncResponse{
		Entries:   entries,
		RequestID: 1,
	}
	_, err := resp.Marshal()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many entries")
}

func TestBPTSyncRequest_EmptyKeys(t *testing.T) {
	req := &gossip.BPTSyncRequest{
		Keys:      nil,
		Requester: []byte("test"),
		RequestID: 1,
	}

	data, err := req.Marshal()
	require.NoError(t, err)

	req2, err := gossip.UnmarshalBPTSyncRequest(data)
	require.NoError(t, err)
	assert.Empty(t, req2.Keys)
}

func TestBPTSyncResponse_EmptyEntries(t *testing.T) {
	resp := &gossip.BPTSyncResponse{
		Entries:   nil,
		RequestID: 1,
		Missing:   nil,
	}

	data, err := resp.Marshal()
	require.NoError(t, err)

	resp2, err := gossip.UnmarshalBPTSyncResponse(data)
	require.NoError(t, err)
	assert.Empty(t, resp2.Entries)
	assert.Empty(t, resp2.Missing)
}

func TestBPTSyncRequest_InvalidData(t *testing.T) {
	// Too short
	_, err := gossip.UnmarshalBPTSyncRequest([]byte{1, 2, 3})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")

	// Invalid requester length
	data := make([]byte, 16)
	// Set requesterLen to 100 (too large)
	data[8] = 0
	data[9] = 0
	data[10] = 0
	data[11] = 100
	_, err = gossip.UnmarshalBPTSyncRequest(data)
	assert.Error(t, err)
}

func TestBPTSyncResponse_InvalidData(t *testing.T) {
	// Too short
	_, err := gossip.UnmarshalBPTSyncResponse([]byte{1, 2, 3})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestBPTSyncResponse_VariableLengthValues(t *testing.T) {
	// Test with different value sizes
	entries := []gossip.BPTEntry{
		{Value: make([]byte, 32)},  // Normal hash
		{Value: make([]byte, 64)},  // Larger value
		{Value: make([]byte, 100)}, // Even larger
		{Value: make([]byte, 1)},   // Small value
	}

	for i := range entries {
		_, err := rand.Read(entries[i].KeyHash[:])
		require.NoError(t, err)
		_, err = rand.Read(entries[i].Value)
		require.NoError(t, err)
	}

	resp := &gossip.BPTSyncResponse{
		Entries:   entries,
		RequestID: 42,
	}

	data, err := resp.Marshal()
	require.NoError(t, err)

	resp2, err := gossip.UnmarshalBPTSyncResponse(data)
	require.NoError(t, err)

	require.Len(t, resp2.Entries, len(entries))
	for i := range entries {
		assert.Equal(t, entries[i].KeyHash, resp2.Entries[i].KeyHash)
		assert.Equal(t, entries[i].Value, resp2.Entries[i].Value)
	}
}
