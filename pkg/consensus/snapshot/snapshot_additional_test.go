// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestSyncStatus_String(t *testing.T) {
	testCases := []struct {
		status   SyncStatus
		expected string
	}{
		{SyncStatusIdle, "idle"},
		{SyncStatusDiscovering, "discovering"},
		{SyncStatusDownloading, "downloading"},
		{SyncStatusVerifying, "verifying"},
		{SyncStatusApplying, "applying"},
		{SyncStatusComplete, "complete"},
		{SyncStatusFailed, "failed"},
		{SyncStatus(999), "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.status.String())
		})
	}
}

func TestSyncProgress_PercentComplete(t *testing.T) {
	// Zero chunks
	progress := &SyncProgress{
		TotalChunks:    0,
		ReceivedChunks: 0,
	}
	assert.Equal(t, 0.0, progress.PercentComplete())

	// Partial progress
	progress = &SyncProgress{
		TotalChunks:    10,
		ReceivedChunks: 5,
	}
	assert.Equal(t, 50.0, progress.PercentComplete())

	// Complete
	progress = &SyncProgress{
		TotalChunks:    10,
		ReceivedChunks: 10,
	}
	assert.Equal(t, 100.0, progress.PercentComplete())
}

func TestSnapshotDigest_IsZero(t *testing.T) {
	var zero SnapshotDigest
	assert.True(t, zero.IsZero())

	nonZero := SnapshotDigest{1, 2, 3}
	assert.False(t, nonZero.IsZero())
}

func TestSnapshot_Validate(t *testing.T) {
	committee := createTestCommittee(4)

	// Valid snapshot
	valid := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}
	assert.NoError(t, valid.Validate())

	// Version zero
	badVersion := &Snapshot{
		Version:   0,
		Height:    100,
		Committee: committee,
	}
	assert.Error(t, badVersion.Validate())

	// Height zero
	badHeight := &Snapshot{
		Version:   Version,
		Height:    0,
		Committee: committee,
	}
	assert.Error(t, badHeight.Validate())

	// Nil committee
	badCommittee := &Snapshot{
		Version: Version,
		Height:  100,
	}
	assert.Error(t, badCommittee.Validate())
}

func TestSnapshotStore_NilConfig(t *testing.T) {
	store := NewSnapshotStore(nil)
	assert.NotNil(t, store)

	// Should use default config
	committee := createTestCommittee(4)
	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}
	err := store.Store(snap)
	assert.NoError(t, err)
}

func TestSnapshotStore_NilSnapshot(t *testing.T) {
	store := NewSnapshotStore(nil)
	err := store.Store(nil)
	assert.Error(t, err)
}

func TestSnapshotStore_InvalidSnapshot(t *testing.T) {
	store := NewSnapshotStore(nil)
	snap := &Snapshot{
		Version: 0, // Invalid
		Height:  100,
	}
	err := store.Store(snap)
	assert.Error(t, err)
}

func TestSnapshotStore_List_Descending(t *testing.T) {
	config := &SnapshotConfig{
		RetainedSnapshots: 10,
	}
	store := NewSnapshotStore(config)
	committee := createTestCommittee(4)

	// Store in non-sequential order
	for _, h := range []uint64{300, 100, 200, 500, 400} {
		snap := &Snapshot{
			Version:   Version,
			Height:    h,
			Committee: committee,
		}
		require.NoError(t, store.Store(snap))
	}

	// List should be descending
	heights := store.List()
	assert.Equal(t, []uint64{500, 400, 300, 200, 100}, heights)
}

func TestSnapshotStore_Latest_Empty(t *testing.T) {
	store := NewSnapshotStore(nil)
	assert.Nil(t, store.Latest())
}

func TestSnapshotStore_Get_NotFound(t *testing.T) {
	store := NewSnapshotStore(nil)
	assert.Nil(t, store.Get(999))
}

func TestSnapshotCreator_NilConfig(t *testing.T) {
	store := NewSnapshotStore(nil)
	creator := NewSnapshotCreator(nil, store)
	assert.NotNil(t, creator)

	// ShouldSnapshot with default interval
	assert.True(t, creator.ShouldSnapshot(1000))
	assert.False(t, creator.ShouldSnapshot(999))
}

func TestSnapshotCreator_IntervalZero(t *testing.T) {
	config := &SnapshotConfig{
		Interval: 0,
	}
	creator := NewSnapshotCreator(config, nil)

	// Should never snapshot when interval is 0
	assert.False(t, creator.ShouldSnapshot(100))
	assert.False(t, creator.ShouldSnapshot(1000))
}

func TestSnapshotCreator_HeightZero(t *testing.T) {
	creator := NewSnapshotCreator(nil, nil)
	// Height 0 should not trigger snapshot
	assert.False(t, creator.ShouldSnapshot(0))
}

func TestSnapshotCreator_CreateSnapshot_HeightRegression(t *testing.T) {
	creator := NewSnapshotCreator(nil, nil)
	committee := createTestCommittee(4)

	// Create first snapshot
	_, err := creator.CreateSnapshot(CreateSnapshotParams{
		Height:    200,
		Committee: committee,
	})
	require.NoError(t, err)

	// Try to create snapshot at lower height
	_, err = creator.CreateSnapshot(CreateSnapshotParams{
		Height:    100,
		Committee: committee,
	})
	assert.Error(t, err)
}

func TestSnapshotCreator_CreateSnapshot_NilCommittee(t *testing.T) {
	creator := NewSnapshotCreator(nil, nil)

	_, err := creator.CreateSnapshot(CreateSnapshotParams{
		Height:    100,
		Committee: nil,
	})
	assert.Error(t, err)
}

func TestSnapshotCreator_CreateSnapshot_WithCerts(t *testing.T) {
	config := &SnapshotConfig{
		CertificateHistory: 10,
	}
	creator := NewSnapshotCreator(config, nil)
	committee := createTestCommittee(4)
	certs := createTestCertificates(committee, 5)

	getCerts := func(from, to types.Round) []*types.Certificate {
		return certs
	}

	snap, err := creator.CreateSnapshot(CreateSnapshotParams{
		Height:    100,
		Round:     50,
		Committee: committee,
		GetCerts:  getCerts,
	})
	require.NoError(t, err)
	assert.NotNil(t, snap.Certificates)
}

func TestSnapshotCreator_CreateSnapshot_NilStore(t *testing.T) {
	creator := NewSnapshotCreator(nil, nil)
	committee := createTestCommittee(4)

	// Should succeed even with nil store
	snap, err := creator.CreateSnapshot(CreateSnapshotParams{
		Height:    100,
		Committee: committee,
	})
	require.NoError(t, err)
	assert.NotNil(t, snap)
}

func TestDefaultSnapshotConfig(t *testing.T) {
	config := DefaultSnapshotConfig()
	assert.Equal(t, uint64(DefaultSnapshotInterval), config.Interval)
	assert.Equal(t, DefaultRetainedSnapshots, config.RetainedSnapshots)
	assert.Equal(t, DefaultChunkSize, config.ChunkSize)
	assert.Equal(t, DefaultCertificateHistory, config.CertificateHistory)
}

func TestUnmarshalSnapshot_TooShort(t *testing.T) {
	data := make([]byte, 10)
	_, err := UnmarshalSnapshot(data)
	assert.Error(t, err)
}

func TestUnmarshalSnapshot_TruncatedCommittee(t *testing.T) {
	// Create valid header but truncate at committee
	data := make([]byte, 64) // Minimum header size
	// This should fail when trying to read committee
	_, err := UnmarshalSnapshot(data)
	assert.Error(t, err)
}

func TestMarshalCertificates_Empty(t *testing.T) {
	data, err := marshalCertificates(nil)
	require.NoError(t, err)
	assert.Equal(t, 4, len(data)) // Just the count

	certs, err := unmarshalCertificates(data)
	require.NoError(t, err)
	assert.Nil(t, certs)
}

func TestUnmarshalCertificates_TooShort(t *testing.T) {
	data := make([]byte, 2)
	_, err := unmarshalCertificates(data)
	assert.Error(t, err)
}

func TestUnmarshalCertificates_TooMany(t *testing.T) {
	data := make([]byte, 4)
	// Set count to very large number
	data[0] = 0xFF
	data[1] = 0xFF
	data[2] = 0xFF
	data[3] = 0xFF
	_, err := unmarshalCertificates(data)
	assert.Error(t, err)
}

func TestMarshalMetadata_Nil(t *testing.T) {
	data, err := marshalMetadata(nil)
	require.NoError(t, err)
	assert.Nil(t, data)
}

func TestUnmarshalMetadata_TooShort(t *testing.T) {
	data := make([]byte, 10)
	_, err := unmarshalMetadata(data)
	assert.Error(t, err)
}

func TestUnmarshalCommittee_TooShort(t *testing.T) {
	data := make([]byte, 5)
	_, err := unmarshalCommittee(data)
	assert.Error(t, err)
}

func TestUnmarshalCommittee_TooManyValidators(t *testing.T) {
	data := make([]byte, 12)
	// Set validator count to very large number
	data[8] = 0xFF
	data[9] = 0xFF
	data[10] = 0xFF
	data[11] = 0xFF
	_, err := unmarshalCommittee(data)
	assert.Error(t, err)
}

func TestUnmarshalSnapshotInfo_TooShort(t *testing.T) {
	data := make([]byte, 50)
	_, err := UnmarshalSnapshotInfo(data)
	assert.Error(t, err)
}

func TestUnmarshalSnapshotChunk_TooShort(t *testing.T) {
	data := make([]byte, 5)
	_, err := UnmarshalSnapshotChunk(data)
	assert.Error(t, err)
}

func TestUnmarshalSnapshotChunk_Truncated(t *testing.T) {
	data := make([]byte, 12)
	// Set data length to 100 but don't provide the data
	data[8] = 0
	data[9] = 0
	data[10] = 0
	data[11] = 100
	_, err := UnmarshalSnapshotChunk(data)
	assert.Error(t, err)
}

func TestUnmarshalSnapshotInfoList_TooShort(t *testing.T) {
	data := make([]byte, 2)
	_, err := unmarshalSnapshotInfoList(data)
	assert.Error(t, err)
}

func TestUnmarshalSnapshotInfoList_Empty(t *testing.T) {
	data := make([]byte, 4)
	// Count = 0
	infos, err := unmarshalSnapshotInfoList(data)
	require.NoError(t, err)
	assert.Nil(t, infos)
}

func TestUnmarshalSnapshotInfoList_TooMany(t *testing.T) {
	data := make([]byte, 4)
	// Set count to very large number
	data[0] = 0xFF
	data[1] = 0xFF
	data[2] = 0xFF
	data[3] = 0xFF
	_, err := unmarshalSnapshotInfoList(data)
	assert.Error(t, err)
}

func TestUnmarshalSnapshotInfoList_Truncated(t *testing.T) {
	data := make([]byte, 8)
	// Count = 2 but only space for partial data
	data[3] = 2
	_, err := unmarshalSnapshotInfoList(data)
	assert.Error(t, err)
}

func TestMarshalSnapshotInfoList_Empty(t *testing.T) {
	data := marshalSnapshotInfoList(nil)
	assert.Equal(t, 4, len(data))

	infos, err := unmarshalSnapshotInfoList(data)
	require.NoError(t, err)
	assert.Nil(t, infos)
}

func TestMarshalSnapshotInfoList_Multiple(t *testing.T) {
	infos := []*SnapshotInfo{
		{Height: 100, Round: 50},
		{Height: 200, Round: 100},
	}

	data := marshalSnapshotInfoList(infos)

	restored, err := unmarshalSnapshotInfoList(data)
	require.NoError(t, err)
	assert.Len(t, restored, 2)
	assert.Equal(t, uint64(100), restored[0].Height)
	assert.Equal(t, uint64(200), restored[1].Height)
}

func TestCanRestoreFrom(t *testing.T) {
	committee := createTestCommittee(4)

	// Nil snapshot
	assert.False(t, CanRestoreFrom(nil, 0))

	// Snapshot behind current height
	snap := &Snapshot{Height: 100, Committee: committee}
	assert.False(t, CanRestoreFrom(snap, 200))

	// Snapshot at current height
	assert.False(t, CanRestoreFrom(snap, 100))

	// Snapshot ahead of current height
	assert.True(t, CanRestoreFrom(snap, 50))
}

func TestSelectBestSnapshot(t *testing.T) {
	committee := createTestCommittee(4)

	// Empty list
	assert.Nil(t, SelectBestSnapshot(nil, committee))
	assert.Nil(t, SelectBestSnapshot([]*Snapshot{}, committee))

	// Valid snapshots - should return highest height that verifies
	snapshots := []*Snapshot{
		{Version: Version, Height: 100, Committee: committee},
		{Version: Version, Height: 300, Committee: committee},
		{Version: Version, Height: 200, Committee: committee},
	}

	best := SelectBestSnapshot(snapshots, committee)
	require.NotNil(t, best)
	assert.Equal(t, uint64(300), best.Height)

	// All invalid
	invalidSnapshots := []*Snapshot{
		{Version: 0, Height: 100, Committee: committee},
		{Version: 0, Height: 200, Committee: committee},
	}
	assert.Nil(t, SelectBestSnapshot(invalidSnapshots, committee))
}

func TestDefaultRestoreOptions(t *testing.T) {
	opts := DefaultRestoreOptions()
	assert.True(t, opts.VerifyState)
	assert.True(t, opts.VerifyCertificates)
	assert.False(t, opts.SkipAppState)
}

func TestRestorer_Restore_NilSnapshot(t *testing.T) {
	restorer := NewRestorer(nil, nil)
	_, err := restorer.Restore(context.Background(), nil)
	assert.Error(t, err)
}

func TestRestorer_Restore_InvalidSnapshot(t *testing.T) {
	restorer := NewRestorer(nil, nil)
	snap := &Snapshot{Version: 0, Height: 100}
	_, err := restorer.Restore(context.Background(), snap)
	assert.Error(t, err)
}

func TestRestorer_RestoreWithOptions_NilSnapshot(t *testing.T) {
	restorer := NewRestorer(nil, nil)
	_, err := restorer.RestoreWithOptions(context.Background(), nil, nil)
	assert.Error(t, err)
}

func TestRestorer_RestoreWithOptions_InvalidSnapshot(t *testing.T) {
	restorer := NewRestorer(nil, nil)
	snap := &Snapshot{Version: 0, Height: 100}
	_, err := restorer.RestoreWithOptions(context.Background(), snap, nil)
	assert.Error(t, err)
}

func TestRestorer_RestoreWithOptions_NilOpts(t *testing.T) {
	committee := createTestCommittee(4)
	restorer := NewRestorer(nil, nil)
	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}

	// Should use default options when nil
	result, err := restorer.RestoreWithOptions(context.Background(), snap, nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), result.Height)
}

func TestRestorer_RestoreWithOptions_SkipAppState(t *testing.T) {
	committee := createTestCommittee(4)
	restorer := NewRestorer(nil, nil)
	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}

	opts := &RestoreOptions{
		SkipAppState:       true,
		VerifyCertificates: false,
	}

	result, err := restorer.RestoreWithOptions(context.Background(), snap, opts)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), result.Height)
}

func TestNewStateSync(t *testing.T) {
	store := NewSnapshotStore(nil)
	restorer := NewRestorer(nil, nil)

	// With nil config
	ss := NewStateSync(nil, store, restorer, nil)
	assert.NotNil(t, ss)
	assert.Equal(t, SyncStatusIdle, ss.Progress().Status)
}

func TestStateSync_Progress(t *testing.T) {
	ss := NewStateSync(nil, nil, nil, nil)
	progress := ss.Progress()
	assert.NotNil(t, progress)
	assert.Equal(t, SyncStatusIdle, progress.Status)
}

func TestStateSync_Cancel_WhenNil(t *testing.T) {
	ss := NewStateSync(nil, nil, nil, nil)
	// Should not panic
	ss.Cancel()
}

func TestStateSync_SelectBestSnapshot_Empty(t *testing.T) {
	ss := NewStateSync(nil, nil, nil, nil)
	result := ss.selectBestSnapshot(nil)
	assert.Nil(t, result)
}

func TestStateSync_SelectBestSnapshot(t *testing.T) {
	ss := NewStateSync(nil, nil, nil, nil)

	snapshots := []peerSnapshot{
		{info: &SnapshotInfo{Height: 100}},
		{info: &SnapshotInfo{Height: 300}},
		{info: &SnapshotInfo{Height: 200}},
	}

	best := ss.selectBestSnapshot(snapshots)
	require.NotNil(t, best)
	assert.Equal(t, uint64(300), best.info.Height)
}

func TestVerifySnapshot_MismatchedCommittee(t *testing.T) {
	committee1 := createTestCommittee(4)
	committee2 := createTestCommittee(3)

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee1,
	}

	// Should fail with mismatched committee size
	err := VerifySnapshot(snap, committee2)
	assert.Error(t, err)
}

func TestVerifySnapshot_MismatchedEpoch(t *testing.T) {
	committee1 := createTestCommittee(4)
	committee2 := types.NewCommittee(committee1.Validators, 2) // Different epoch

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee1,
	}

	err := VerifySnapshot(snap, committee2)
	assert.Error(t, err)
}

func TestSnapshotStore_Prune(t *testing.T) {
	config := &SnapshotConfig{
		RetainedSnapshots: 2,
	}
	store := NewSnapshotStore(config)
	committee := createTestCommittee(4)

	// Store 5 snapshots
	for i := 1; i <= 5; i++ {
		snap := &Snapshot{
			Version:   Version,
			Height:    uint64(i * 100),
			Committee: committee,
		}
		require.NoError(t, store.Store(snap))
	}

	// Should only keep 2
	heights := store.List()
	assert.Len(t, heights, 2)

	// Should keep the latest ones
	assert.Contains(t, heights, uint64(500))
	assert.Contains(t, heights, uint64(400))
}

func TestSnapshotRoundTrip(t *testing.T) {
	committee := createTestCommittee(4)

	original := &Snapshot{
		Version:   Version,
		Height:    12345,
		Round:     678,
		StateHash: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Committee: committee,
		Timestamp: time.Now().Truncate(time.Nanosecond),
		Metadata: &SnapshotMetadata{
			ChainID:           "test-chain-id",
			LastCommitRound:   678,
			TotalCertificates: 0,
			CreatedBy:         "test-node",
		},
	}

	// Marshal
	data, err := original.Marshal()
	require.NoError(t, err)

	// Unmarshal
	restored, err := UnmarshalSnapshot(data)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, original.Version, restored.Version)
	assert.Equal(t, original.Height, restored.Height)
	assert.Equal(t, original.Round, restored.Round)
	assert.Equal(t, original.StateHash, restored.StateHash)
	assert.Equal(t, original.Committee.Epoch, restored.Committee.Epoch)
	assert.Equal(t, len(original.Committee.Validators), len(restored.Committee.Validators))
	assert.Equal(t, original.Timestamp.UnixNano(), restored.Timestamp.UnixNano())
	assert.Equal(t, original.Metadata.ChainID, restored.Metadata.ChainID)
	assert.Equal(t, original.Metadata.LastCommitRound, restored.Metadata.LastCommitRound)
	assert.Equal(t, original.Metadata.CreatedBy, restored.Metadata.CreatedBy)

	// Verify digests match
	assert.Equal(t, original.Digest(), restored.Digest())
}

func TestSnapshotInfo_RoundTrip(t *testing.T) {
	original := &SnapshotInfo{
		Height:     999,
		Round:      555,
		StateHash:  [32]byte{10, 20, 30},
		Size:       1024 * 1024 * 100,
		ChunkCount: 100,
		Digest:     SnapshotDigest{40, 50, 60},
	}

	data := original.Marshal()

	restored, err := UnmarshalSnapshotInfo(data)
	require.NoError(t, err)

	assert.Equal(t, original.Height, restored.Height)
	assert.Equal(t, original.Round, restored.Round)
	assert.Equal(t, original.StateHash, restored.StateHash)
	assert.Equal(t, original.Size, restored.Size)
	assert.Equal(t, original.ChunkCount, restored.ChunkCount)
	assert.Equal(t, original.Digest, restored.Digest)
}

func TestSnapshotChunk_RoundTrip(t *testing.T) {
	original := &SnapshotChunk{
		Index: 42,
		Total: 100,
		Data:  []byte("this is a chunk of snapshot data with some content"),
	}

	data := original.Marshal()

	restored, err := UnmarshalSnapshotChunk(data)
	require.NoError(t, err)

	assert.Equal(t, original.Index, restored.Index)
	assert.Equal(t, original.Total, restored.Total)
	assert.Equal(t, original.Data, restored.Data)
}

func TestStateSync_SetStatus(t *testing.T) {
	ss := NewStateSync(nil, nil, nil, nil)

	// Test setStatus
	ss.setStatus(SyncStatusDownloading, nil)
	progress := ss.Progress()
	assert.Equal(t, SyncStatusDownloading, progress.Status)
	assert.Nil(t, progress.Error)

	// With error
	testErr := errors.New("test error")
	ss.setStatus(SyncStatusFailed, testErr)
	progress = ss.Progress()
	assert.Equal(t, SyncStatusFailed, progress.Status)
	assert.Equal(t, testErr, progress.Error)
}

func TestStateSync_Cancel_WithFunc(t *testing.T) {
	ss := NewStateSync(nil, nil, nil, nil)

	// Set a cancel function
	called := false
	ss.cancel = func() {
		called = true
	}

	ss.Cancel()
	assert.True(t, called)
	assert.Nil(t, ss.cancel)
}

func TestStateSync_DiscoverSnapshots_Empty(t *testing.T) {
	ss := NewStateSync(nil, nil, nil, nil)

	ctx := context.Background()
	snapshots, err := ss.discoverSnapshots(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, snapshots, 0)
}

func TestRestorer_NilDAG(t *testing.T) {
	// Test restoring with nil DAG
	committee := createTestCommittee(4)
	restorer := NewRestorer(nil, nil)

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}

	result, err := restorer.Restore(context.Background(), snap)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), result.Height)
	assert.Equal(t, 0, result.RestoredCertificates)
}

func TestRestorer_WithStateRestorer(t *testing.T) {
	committee := createTestCommittee(4)

	stateRestored := false
	mockRestorer := &mockStateRestorerFunc{
		restoreFunc: func(ctx context.Context, height uint64, stateHash [32]byte) error {
			stateRestored = true
			return nil
		},
	}

	restorer := NewRestorer(nil, mockRestorer)

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}

	_, err := restorer.Restore(context.Background(), snap)
	require.NoError(t, err)
	assert.True(t, stateRestored)
}

func TestRestorer_StateRestorerError(t *testing.T) {
	committee := createTestCommittee(4)

	mockRestorer := &mockStateRestorerFunc{
		restoreFunc: func(ctx context.Context, height uint64, stateHash [32]byte) error {
			return errors.New("state restore failed")
		},
	}

	restorer := NewRestorer(nil, mockRestorer)

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}

	_, err := restorer.Restore(context.Background(), snap)
	assert.Error(t, err)
}

type mockStateRestorerFunc struct {
	restoreFunc func(ctx context.Context, height uint64, stateHash [32]byte) error
}

func (m *mockStateRestorerFunc) RestoreState(ctx context.Context, height uint64, stateHash [32]byte) error {
	if m.restoreFunc != nil {
		return m.restoreFunc(ctx, height, stateHash)
	}
	return nil
}

func TestSnapshotMarshalWithNoCertificates(t *testing.T) {
	committee := createTestCommittee(4)

	snap := &Snapshot{
		Version:      Version,
		Height:       100,
		Round:        50,
		StateHash:    [32]byte{1, 2, 3},
		Committee:    committee,
		Certificates: nil,
		Timestamp:    time.Now().Truncate(time.Nanosecond),
	}

	data, err := snap.Marshal()
	require.NoError(t, err)

	restored, err := UnmarshalSnapshot(data)
	require.NoError(t, err)

	assert.Equal(t, snap.Height, restored.Height)
	assert.Nil(t, restored.Certificates)
}

func TestSnapshotMarshalWithNoMetadata(t *testing.T) {
	committee := createTestCommittee(4)

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
		Metadata:  nil,
	}

	data, err := snap.Marshal()
	require.NoError(t, err)

	restored, err := UnmarshalSnapshot(data)
	require.NoError(t, err)

	assert.Nil(t, restored.Metadata)
}

func TestRestoreWithOptions_VerifyCertificates(t *testing.T) {
	committee := createTestCommittee(4)
	// Create snapshot without certificates to pass verification
	snap := &Snapshot{
		Version:      Version,
		Height:       100,
		Committee:    committee,
		Certificates: nil,
	}

	restorer := NewRestorer(nil, nil)

	opts := &RestoreOptions{
		VerifyCertificates: true,
		SkipAppState:       true,
	}

	result, err := restorer.RestoreWithOptions(context.Background(), snap, opts)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), result.Height)
}

func TestUnmarshalSnapshot_VariousTruncations(t *testing.T) {
	committee := createTestCommittee(4)

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
		Timestamp: time.Now(),
		Metadata: &SnapshotMetadata{
			ChainID:   "test",
			CreatedBy: "node",
		},
	}

	data, err := snap.Marshal()
	require.NoError(t, err)

	// Test various truncation points
	truncationTests := []struct {
		name   string
		length int
	}{
		{"too short for header", 50},
		{"truncated at committee length", 64},
	}

	for _, tc := range truncationTests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.length < len(data) {
				_, err := UnmarshalSnapshot(data[:tc.length])
				assert.Error(t, err)
			}
		})
	}
}

func TestCommitteeMarshalRoundTrip(t *testing.T) {
	committee := createTestCommittee(10)

	data, err := marshalCommittee(committee)
	require.NoError(t, err)

	restored, err := unmarshalCommittee(data)
	require.NoError(t, err)

	assert.Equal(t, committee.Epoch, restored.Epoch)
	assert.Equal(t, len(committee.Validators), len(restored.Validators))

	for i := range committee.Validators {
		assert.Equal(t, committee.Validators[i].Stake, restored.Validators[i].Stake)
	}
}

func TestMarshalCommittee_Nil(t *testing.T) {
	_, err := marshalCommittee(nil)
	assert.Error(t, err)
}

func TestUnmarshalCommittee_Truncated(t *testing.T) {
	committee := createTestCommittee(4)
	data, err := marshalCommittee(committee)
	require.NoError(t, err)

	// Truncate data
	_, err = unmarshalCommittee(data[:len(data)-10])
	assert.Error(t, err)
}

func TestUnmarshalCertificates_Truncated(t *testing.T) {
	// Data has count > 0 but missing actual cert data
	data := make([]byte, 8)
	data[3] = 1 // count = 1
	data[7] = 100 // cert length = 100, but data not provided

	_, err := unmarshalCertificates(data)
	assert.Error(t, err)
}

func TestUnmarshalMetadata_VariousTruncations(t *testing.T) {
	// Create metadata
	m := &SnapshotMetadata{
		ChainID:           "test-chain",
		LastCommitRound:   100,
		TotalCertificates: 5,
		CreatedBy:         "node-1",
	}

	data, err := marshalMetadata(m)
	require.NoError(t, err)

	// Test various truncation points
	truncationPoints := []int{10, 15, len(data) - 5}
	for _, point := range truncationPoints {
		if point < len(data) {
			_, err := unmarshalMetadata(data[:point])
			assert.Error(t, err)
		}
	}
}

func TestSnapshotCreator_GetCerts_LargeRound(t *testing.T) {
	config := &SnapshotConfig{
		CertificateHistory: 10,
	}
	creator := NewSnapshotCreator(config, nil)
	committee := createTestCommittee(4)

	// Test with round greater than CertificateHistory
	getCerts := func(from, to types.Round) []*types.Certificate {
		// Just verify from is computed correctly
		assert.Equal(t, types.Round(40), from)
		assert.Equal(t, types.Round(50), to)
		return nil
	}

	_, err := creator.CreateSnapshot(CreateSnapshotParams{
		Height:    100,
		Round:     50,
		Committee: committee,
		GetCerts:  getCerts,
	})
	require.NoError(t, err)
}

func TestSnapshotCreator_GetCerts_SmallRound(t *testing.T) {
	config := &SnapshotConfig{
		CertificateHistory: 100,
	}
	creator := NewSnapshotCreator(config, nil)
	committee := createTestCommittee(4)

	// Test with round less than CertificateHistory
	getCerts := func(from, to types.Round) []*types.Certificate {
		assert.Equal(t, types.Round(0), from)
		assert.Equal(t, types.Round(10), to)
		return nil
	}

	_, err := creator.CreateSnapshot(CreateSnapshotParams{
		Height:    100,
		Round:     10,
		Committee: committee,
		GetCerts:  getCerts,
	})
	require.NoError(t, err)
}

func TestVerifySnapshot_WithCertificateVerificationError(t *testing.T) {
	committee := createTestCommittee(4)

	// Create a certificate with invalid signatures for testing
	header := types.NewHeader(
		committee.Validators[0].PublicKey,
		types.Round(1),
		committee.Epoch,
		nil,
		nil,
	)
	cert := types.NewCertificate(header, [][]byte{make([]byte, 64)}, []uint16{0}) // Invalid signature

	snap := &Snapshot{
		Version:      Version,
		Height:       100,
		Committee:    committee,
		Certificates: []*types.Certificate{cert},
	}

	err := VerifySnapshot(snap, committee)
	assert.Error(t, err)
}

func TestRestoreWithOptions_CertificateVerificationFails(t *testing.T) {
	committee := createTestCommittee(4)

	// Create a certificate with invalid signatures
	header := types.NewHeader(
		committee.Validators[0].PublicKey,
		types.Round(1),
		committee.Epoch,
		nil,
		nil,
	)
	cert := types.NewCertificate(header, [][]byte{make([]byte, 64)}, []uint16{0})

	snap := &Snapshot{
		Version:      Version,
		Height:       100,
		Committee:    committee,
		Certificates: []*types.Certificate{cert},
	}

	restorer := NewRestorer(nil, nil)

	opts := &RestoreOptions{
		VerifyCertificates: true,
		SkipAppState:       true,
	}

	_, err := restorer.RestoreWithOptions(context.Background(), snap, opts)
	assert.Error(t, err)
}

func TestSnapshotValidate_InvalidCommittee(t *testing.T) {
	// Create an invalid committee (empty validators)
	committee := types.NewCommittee(nil, 0)

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}

	err := snap.Validate()
	assert.Error(t, err)
}

func TestRestoreWithOptions_StateRestorerWithOptions(t *testing.T) {
	committee := createTestCommittee(4)

	stateRestored := false
	mockRestorer := &mockStateRestorerFunc{
		restoreFunc: func(ctx context.Context, height uint64, stateHash [32]byte) error {
			stateRestored = true
			return nil
		},
	}

	restorer := NewRestorer(nil, mockRestorer)

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}

	// When SkipAppState is false, state should be restored
	opts := &RestoreOptions{
		SkipAppState:       false,
		VerifyCertificates: false,
	}

	_, err := restorer.RestoreWithOptions(context.Background(), snap, opts)
	require.NoError(t, err)
	assert.True(t, stateRestored)
}

func TestRestoreWithOptions_StateRestorerError(t *testing.T) {
	committee := createTestCommittee(4)

	mockRestorer := &mockStateRestorerFunc{
		restoreFunc: func(ctx context.Context, height uint64, stateHash [32]byte) error {
			return errors.New("restore failed")
		},
	}

	restorer := NewRestorer(nil, mockRestorer)

	snap := &Snapshot{
		Version:   Version,
		Height:    100,
		Committee: committee,
	}

	opts := &RestoreOptions{
		SkipAppState:       false,
		VerifyCertificates: false,
	}

	_, err := restorer.RestoreWithOptions(context.Background(), snap, opts)
	assert.Error(t, err)
}
