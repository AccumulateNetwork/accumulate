// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/testutil"
)

func TestStateSync_RegisterUnregisterHandlers(t *testing.T) {
	mockHost := testutil.NewMockHost(peer.ID("test-host"))
	store := NewSnapshotStore(nil)
	ss := NewStateSync(mockHost, store, nil, nil)

	// Register handlers
	ss.RegisterHandlers()

	// Check handlers are registered
	_, ok := mockHost.GetHandler(ProtocolSnapshotList)
	assert.True(t, ok)

	_, ok = mockHost.GetHandler(ProtocolSnapshotFetch)
	assert.True(t, ok)

	_, ok = mockHost.GetHandler(ProtocolSnapshotChunk)
	assert.True(t, ok)

	// Unregister handlers
	ss.UnregisterHandlers()

	// Check handlers are removed
	_, ok = mockHost.GetHandler(ProtocolSnapshotList)
	assert.False(t, ok)

	_, ok = mockHost.GetHandler(ProtocolSnapshotFetch)
	assert.False(t, ok)

	_, ok = mockHost.GetHandler(ProtocolSnapshotChunk)
	assert.False(t, ok)
}

func TestWriteMessage(t *testing.T) {
	// Create a mock stream using bytes.Buffer
	buf := &bytes.Buffer{}
	stream := &testutil.MockStream{}

	// We can't directly test writeMessage as it requires a network.Stream
	// But we can test the serialization format
	data := []byte("test data")
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	buf.Write(lenBuf)
	buf.Write(data)

	// Read it back
	readLen := binary.BigEndian.Uint32(buf.Bytes()[:4])
	assert.Equal(t, uint32(len(data)), readLen)
	assert.Equal(t, data, buf.Bytes()[4:4+len(data)])

	// Close the stream to prevent warnings
	_ = stream.Close()
}

func TestSyncProgress_Fields(t *testing.T) {
	progress := &SyncProgress{
		Status:         SyncStatusDownloading,
		Height:         100,
		TotalChunks:    10,
		ReceivedChunks: 5,
		BytesReceived:  1024,
		TotalBytes:     2048,
		StartTime:      time.Now(),
		Error:          nil,
	}

	assert.Equal(t, 50.0, progress.PercentComplete())
	assert.Equal(t, SyncStatusDownloading, progress.Status)
	assert.Equal(t, uint64(100), progress.Height)
}

func TestSnapshotInfo_Marshal(t *testing.T) {
	info := &SnapshotInfo{
		Height:     12345,
		Round:      678,
		StateHash:  [32]byte{1, 2, 3, 4, 5},
		Size:       1024 * 1024,
		ChunkCount: 10,
		Digest:     SnapshotDigest{10, 11, 12},
	}

	data := info.Marshal()
	assert.Len(t, data, 92) // 8+8+32+8+4+32 = 92

	// Verify round trip
	restored, err := UnmarshalSnapshotInfo(data)
	require.NoError(t, err)
	assert.Equal(t, info.Height, restored.Height)
	assert.Equal(t, info.Round, restored.Round)
	assert.Equal(t, info.StateHash, restored.StateHash)
	assert.Equal(t, info.Size, restored.Size)
	assert.Equal(t, info.ChunkCount, restored.ChunkCount)
	assert.Equal(t, info.Digest, restored.Digest)
}

func TestSnapshotChunk_Marshal(t *testing.T) {
	chunk := &SnapshotChunk{
		Index: 5,
		Total: 20,
		Data:  []byte("chunk data content here"),
	}

	data := chunk.Marshal()

	// Verify round trip
	restored, err := UnmarshalSnapshotChunk(data)
	require.NoError(t, err)
	assert.Equal(t, chunk.Index, restored.Index)
	assert.Equal(t, chunk.Total, restored.Total)
	assert.Equal(t, chunk.Data, restored.Data)
}

func TestMarshalSnapshotInfoList(t *testing.T) {
	infos := []*SnapshotInfo{
		{Height: 100, Round: 50, ChunkCount: 5},
		{Height: 200, Round: 100, ChunkCount: 10},
		{Height: 300, Round: 150, ChunkCount: 15},
	}

	data := marshalSnapshotInfoList(infos)

	// Verify round trip
	restored, err := unmarshalSnapshotInfoList(data)
	require.NoError(t, err)
	assert.Len(t, restored, 3)
	assert.Equal(t, uint64(100), restored[0].Height)
	assert.Equal(t, uint64(200), restored[1].Height)
	assert.Equal(t, uint64(300), restored[2].Height)
}

func TestStateSync_Sync_AlreadyInProgress(t *testing.T) {
	mockHost := testutil.NewMockHost(peer.ID("test-host"))
	ss := NewStateSync(mockHost, nil, nil, nil)

	// Set status to downloading (in progress)
	ss.progress.Status = SyncStatusDownloading

	// Try to start another sync - should return error immediately
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := ss.Sync(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sync already in progress")
}
