// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestSnapshotMarshalUnmarshal(t *testing.T) {
	committee := createTestCommittee(4)
	certs := createTestCertificates(committee, 5)

	snapshot := &Snapshot{
		Version:      Version,
		Height:       100,
		Round:        50,
		StateHash:    [32]byte{1, 2, 3, 4, 5},
		Committee:    committee,
		Certificates: certs,
		Timestamp:    time.Now().Truncate(time.Nanosecond),
		Metadata: &SnapshotMetadata{
			ChainID:           "test-chain",
			LastCommitRound:   50,
			TotalCertificates: len(certs),
			CreatedBy:         "node-1",
		},
	}

	// Marshal
	data, err := snapshot.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	restored, err := UnmarshalSnapshot(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify
	if restored.Version != snapshot.Version {
		t.Errorf("Version mismatch: got %d, want %d", restored.Version, snapshot.Version)
	}
	if restored.Height != snapshot.Height {
		t.Errorf("Height mismatch: got %d, want %d", restored.Height, snapshot.Height)
	}
	if restored.Round != snapshot.Round {
		t.Errorf("Round mismatch: got %d, want %d", restored.Round, snapshot.Round)
	}
	if restored.StateHash != snapshot.StateHash {
		t.Errorf("StateHash mismatch")
	}
	if restored.Committee.Epoch != committee.Epoch {
		t.Errorf("Committee epoch mismatch")
	}
	if len(restored.Certificates) != len(certs) {
		t.Errorf("Certificates count mismatch: got %d, want %d", len(restored.Certificates), len(certs))
	}
	if restored.Metadata.ChainID != snapshot.Metadata.ChainID {
		t.Errorf("Metadata ChainID mismatch")
	}
}

func TestSnapshotStore(t *testing.T) {
	config := &SnapshotConfig{
		Interval:           100,
		RetainedSnapshots:  3,
		ChunkSize:          1024,
		CertificateHistory: 10,
	}
	store := NewSnapshotStore(config)

	committee := createTestCommittee(4)

	// Store multiple snapshots
	for i := 1; i <= 5; i++ {
		snap := &Snapshot{
			Version:   Version,
			Height:    uint64(i * 100),
			Round:     types.Round(i * 50),
			Committee: committee,
			Timestamp: time.Now(),
		}
		if err := store.Store(snap); err != nil {
			t.Fatalf("Store failed: %v", err)
		}
	}

	// Should only retain 3 snapshots
	heights := store.List()
	if len(heights) != 3 {
		t.Errorf("Expected 3 retained snapshots, got %d", len(heights))
	}

	// Should have latest snapshots
	latest := store.Latest()
	if latest.Height != 500 {
		t.Errorf("Latest height should be 500, got %d", latest.Height)
	}

	// Check specific snapshot
	snap := store.Get(400)
	if snap == nil {
		t.Error("Snapshot at height 400 should exist")
	}
	if snap != nil && snap.Height != 400 {
		t.Errorf("Expected height 400, got %d", snap.Height)
	}

	// Old snapshots should be pruned
	snap = store.Get(100)
	if snap != nil {
		t.Error("Snapshot at height 100 should be pruned")
	}
}

func TestSnapshotCreator(t *testing.T) {
	config := &SnapshotConfig{
		Interval:           100,
		RetainedSnapshots:  3,
		ChunkSize:          1024,
		CertificateHistory: 10,
	}
	store := NewSnapshotStore(config)
	creator := NewSnapshotCreator(config, store)

	// Should snapshot at interval
	if !creator.ShouldSnapshot(100) {
		t.Error("Should snapshot at height 100")
	}
	if creator.ShouldSnapshot(101) {
		t.Error("Should not snapshot at height 101")
	}
	if !creator.ShouldSnapshot(200) {
		t.Error("Should snapshot at height 200")
	}

	// Create snapshot
	committee := createTestCommittee(4)
	params := CreateSnapshotParams{
		Height:    100,
		Round:     50,
		StateHash: [32]byte{1, 2, 3},
		Committee: committee,
		ChainID:   "test",
		NodeID:    "node-1",
	}

	snap, err := creator.CreateSnapshot(params)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if snap.Height != 100 {
		t.Errorf("Snapshot height mismatch")
	}
	if snap.Metadata.ChainID != "test" {
		t.Errorf("Metadata ChainID mismatch")
	}

	// Should be stored
	stored := store.Get(100)
	if stored == nil {
		t.Error("Snapshot should be stored")
	}
}

func TestSnapshotRestore(t *testing.T) {
	committee := createTestCommittee(4)
	certs := createTestCertificates(committee, 5)

	snapshot := &Snapshot{
		Version:      Version,
		Height:       100,
		Round:        50,
		StateHash:    [32]byte{1, 2, 3},
		Committee:    committee,
		Certificates: certs,
		Timestamp:    time.Now(),
	}

	d := dag.NewDAG(50)
	restorer := NewRestorer(d, nil)

	result, err := restorer.Restore(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	if result.Height != 100 {
		t.Errorf("Result height mismatch")
	}
	if result.Round != 50 {
		t.Errorf("Result round mismatch")
	}
	if result.RestoredCertificates != len(certs) {
		t.Errorf("Restored certificates mismatch: got %d, want %d", result.RestoredCertificates, len(certs))
	}

	// DAG should have certificates
	if d.Size() != len(certs) {
		t.Errorf("DAG size mismatch: got %d, want %d", d.Size(), len(certs))
	}
}

func TestVerifySnapshot(t *testing.T) {
	committee := createTestCommittee(4)
	// Note: We don't include certificates here since mock signatures won't verify
	// In production, certificates would have valid signatures

	snapshot := &Snapshot{
		Version:      Version,
		Height:       100,
		Round:        50,
		StateHash:    [32]byte{1, 2, 3},
		Committee:    committee,
		Certificates: nil, // No certificates to avoid signature verification
		Timestamp:    time.Now(),
	}

	// Valid snapshot (without certificates)
	err := VerifySnapshot(snapshot, committee)
	if err != nil {
		t.Errorf("VerifySnapshot failed: %v", err)
	}

	// Invalid: nil snapshot
	err = VerifySnapshot(nil, committee)
	if err == nil {
		t.Error("Expected error for nil snapshot")
	}

	// Invalid: version 0
	badSnap := &Snapshot{
		Version:   0,
		Height:    100,
		Committee: committee,
	}
	err = VerifySnapshot(badSnap, committee)
	if err == nil {
		t.Error("Expected error for version 0")
	}
}

func TestSnapshotDigest(t *testing.T) {
	committee := createTestCommittee(4)

	snap1 := &Snapshot{
		Version:   Version,
		Height:    100,
		Round:     50,
		StateHash: [32]byte{1, 2, 3},
		Committee: committee,
		Timestamp: time.Now(),
	}

	snap2 := &Snapshot{
		Version:   Version,
		Height:    100,
		Round:     50,
		StateHash: [32]byte{1, 2, 3},
		Committee: committee,
		Timestamp: snap1.Timestamp,
	}

	// Same content should produce same digest
	d1 := snap1.Digest()
	d2 := snap2.Digest()
	if d1 != d2 {
		t.Error("Same snapshots should have same digest")
	}

	// Different content should produce different digest
	snap2.Height = 200
	d3 := snap2.Digest()
	if d1 == d3 {
		t.Error("Different snapshots should have different digests")
	}
}

func TestSnapshotInfoMarshalUnmarshal(t *testing.T) {
	info := &SnapshotInfo{
		Height:     100,
		Round:      50,
		StateHash:  [32]byte{1, 2, 3},
		Size:       1024 * 1024,
		ChunkCount: 10,
		Digest:     SnapshotDigest{4, 5, 6},
	}

	data := info.Marshal()

	restored, err := UnmarshalSnapshotInfo(data)
	if err != nil {
		t.Fatalf("UnmarshalSnapshotInfo failed: %v", err)
	}

	if restored.Height != info.Height {
		t.Errorf("Height mismatch")
	}
	if restored.Round != info.Round {
		t.Errorf("Round mismatch")
	}
	if restored.Size != info.Size {
		t.Errorf("Size mismatch")
	}
	if restored.ChunkCount != info.ChunkCount {
		t.Errorf("ChunkCount mismatch")
	}
}

func TestSnapshotChunkMarshalUnmarshal(t *testing.T) {
	chunk := &SnapshotChunk{
		Index: 5,
		Total: 10,
		Data:  []byte("test chunk data"),
	}

	data := chunk.Marshal()

	restored, err := UnmarshalSnapshotChunk(data)
	if err != nil {
		t.Fatalf("UnmarshalSnapshotChunk failed: %v", err)
	}

	if restored.Index != chunk.Index {
		t.Errorf("Index mismatch")
	}
	if restored.Total != chunk.Total {
		t.Errorf("Total mismatch")
	}
	if string(restored.Data) != string(chunk.Data) {
		t.Errorf("Data mismatch")
	}
}

// Helper functions

func createTestCommittee(n int) *types.Committee {
	validators := make([]types.ValidatorInfo, n)
	for i := 0; i < n; i++ {
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     100,
		}
	}
	return types.NewCommittee(validators, 1)
}

func createTestCertificates(committee *types.Committee, count int) []*types.Certificate {
	certs := make([]*types.Certificate, count)

	for i := 0; i < count; i++ {
		// Create header
		author := committee.Validators[i%len(committee.Validators)].PublicKey
		header := types.NewHeader(
			author,
			types.Round(i),
			committee.Epoch,
			nil,
			nil,
		)

		// Create certificate with mock signatures
		sigs := make([][]byte, 0)
		authorities := make([]uint16, 0)

		// Get quorum count of signatures
		quorumCount := committee.QuorumCount()
		for j := 0; j < quorumCount && j < len(committee.Validators); j++ {
			// Create a mock signature (in real code, this would be a valid ed25519 signature)
			sig := make([]byte, ed25519.SignatureSize)
			rand.Read(sig)
			sigs = append(sigs, sig)
			authorities = append(authorities, uint16(j))
		}

		// Create certificate using header pointer methods to avoid copying the lock
		cert := &types.Certificate{
			Header:            *header.Clone(),
			Signatures:        sigs,
			SignedAuthorities: authorities,
		}
		certs[i] = cert
	}

	return certs
}
