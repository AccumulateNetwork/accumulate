// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Protocol IDs for snapshot sync.
const (
	ProtocolSnapshotList   = "/acc/consensus/snapshot/list/1.0.0"
	ProtocolSnapshotFetch  = "/acc/consensus/snapshot/fetch/1.0.0"
	ProtocolSnapshotChunk  = "/acc/consensus/snapshot/chunk/1.0.0"
	ProtocolStateSync      = "/acc/consensus/state-sync/1.0.0"
)

// Default timeouts and limits.
const (
	DefaultSyncTimeout     = 5 * time.Minute
	DefaultChunkTimeout    = 30 * time.Second
	MaxSnapshotSize        = 1 * 1024 * 1024 * 1024 // 1GB
	MaxChunkSize           = 2 * 1024 * 1024        // 2MB
	MaxSnapshotsInList     = 100
)

// SyncStatus represents the status of a state sync operation.
type SyncStatus int

const (
	SyncStatusIdle SyncStatus = iota
	SyncStatusDiscovering
	SyncStatusDownloading
	SyncStatusVerifying
	SyncStatusApplying
	SyncStatusComplete
	SyncStatusFailed
)

// String returns a string representation of the sync status.
func (s SyncStatus) String() string {
	switch s {
	case SyncStatusIdle:
		return "idle"
	case SyncStatusDiscovering:
		return "discovering"
	case SyncStatusDownloading:
		return "downloading"
	case SyncStatusVerifying:
		return "verifying"
	case SyncStatusApplying:
		return "applying"
	case SyncStatusComplete:
		return "complete"
	case SyncStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// SnapshotInfo contains metadata about an available snapshot.
type SnapshotInfo struct {
	Height    uint64
	Round     types.Round
	StateHash [32]byte
	Size      uint64
	ChunkCount int
	Digest    SnapshotDigest
}

// Marshal serializes snapshot info.
func (i *SnapshotInfo) Marshal() []byte {
	data := make([]byte, 8+8+32+8+4+32)
	offset := 0

	binary.BigEndian.PutUint64(data[offset:], i.Height)
	offset += 8

	binary.BigEndian.PutUint64(data[offset:], uint64(i.Round))
	offset += 8

	copy(data[offset:], i.StateHash[:])
	offset += 32

	binary.BigEndian.PutUint64(data[offset:], i.Size)
	offset += 8

	binary.BigEndian.PutUint32(data[offset:], uint32(i.ChunkCount))
	offset += 4

	copy(data[offset:], i.Digest[:])

	return data
}

// UnmarshalSnapshotInfo deserializes snapshot info.
func UnmarshalSnapshotInfo(data []byte) (*SnapshotInfo, error) {
	if len(data) < 92 {
		return nil, errors.New("snapshot info too short")
	}

	info := &SnapshotInfo{}
	offset := 0

	info.Height = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	info.Round = types.Round(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	copy(info.StateHash[:], data[offset:offset+32])
	offset += 32

	info.Size = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	info.ChunkCount = int(binary.BigEndian.Uint32(data[offset:]))
	offset += 4

	copy(info.Digest[:], data[offset:])

	return info, nil
}

// SnapshotChunk represents a chunk of snapshot data.
type SnapshotChunk struct {
	Index uint32
	Total uint32
	Data  []byte
}

// Marshal serializes a snapshot chunk.
func (c *SnapshotChunk) Marshal() []byte {
	data := make([]byte, 4+4+4+len(c.Data))
	offset := 0

	binary.BigEndian.PutUint32(data[offset:], c.Index)
	offset += 4

	binary.BigEndian.PutUint32(data[offset:], c.Total)
	offset += 4

	binary.BigEndian.PutUint32(data[offset:], uint32(len(c.Data)))
	offset += 4

	copy(data[offset:], c.Data)

	return data
}

// UnmarshalSnapshotChunk deserializes a snapshot chunk.
func UnmarshalSnapshotChunk(data []byte) (*SnapshotChunk, error) {
	if len(data) < 12 {
		return nil, errors.New("chunk data too short")
	}

	chunk := &SnapshotChunk{}
	offset := 0

	chunk.Index = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	chunk.Total = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	dataLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(dataLen) > len(data) {
		return nil, errors.New("chunk data truncated")
	}

	chunk.Data = make([]byte, dataLen)
	copy(chunk.Data, data[offset:])

	return chunk, nil
}

// SyncProgress tracks the progress of a state sync operation.
type SyncProgress struct {
	Status       SyncStatus
	Height       uint64
	TotalChunks  int
	ReceivedChunks int
	BytesReceived  uint64
	TotalBytes     uint64
	StartTime    time.Time
	Error        error
}

// PercentComplete returns the completion percentage.
func (p *SyncProgress) PercentComplete() float64 {
	if p.TotalChunks == 0 {
		return 0
	}
	return float64(p.ReceivedChunks) / float64(p.TotalChunks) * 100
}

// StateSync handles state synchronization from peers.
type StateSync struct {
	host     host.Host
	store    *SnapshotStore
	restorer *Restorer
	config   *SnapshotConfig

	mu       sync.RWMutex
	progress *SyncProgress
	cancel   context.CancelFunc
}

// NewStateSync creates a new StateSync.
func NewStateSync(h host.Host, store *SnapshotStore, restorer *Restorer, config *SnapshotConfig) *StateSync {
	if config == nil {
		config = DefaultSnapshotConfig()
	}
	return &StateSync{
		host:     h,
		store:    store,
		restorer: restorer,
		config:   config,
		progress: &SyncProgress{Status: SyncStatusIdle},
	}
}

// RegisterHandlers registers protocol handlers for serving snapshots.
func (s *StateSync) RegisterHandlers() {
	s.host.SetStreamHandler(protocol.ID(ProtocolSnapshotList), s.handleListSnapshots)
	s.host.SetStreamHandler(protocol.ID(ProtocolSnapshotFetch), s.handleFetchSnapshot)
	s.host.SetStreamHandler(protocol.ID(ProtocolSnapshotChunk), s.handleFetchChunk)
}

// UnregisterHandlers removes protocol handlers.
func (s *StateSync) UnregisterHandlers() {
	s.host.RemoveStreamHandler(protocol.ID(ProtocolSnapshotList))
	s.host.RemoveStreamHandler(protocol.ID(ProtocolSnapshotFetch))
	s.host.RemoveStreamHandler(protocol.ID(ProtocolSnapshotChunk))
}

// Progress returns the current sync progress.
func (s *StateSync) Progress() *SyncProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.progress
}

// Cancel cancels an ongoing sync operation.
func (s *StateSync) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// Sync starts a state sync operation from available peers.
func (s *StateSync) Sync(ctx context.Context, peers []peer.ID) (*RestoreResult, error) {
	s.mu.Lock()
	if s.progress.Status != SyncStatusIdle && s.progress.Status != SyncStatusComplete && s.progress.Status != SyncStatusFailed {
		s.mu.Unlock()
		return nil, errors.New("sync already in progress")
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultSyncTimeout)
	s.cancel = cancel
	s.progress = &SyncProgress{
		Status:    SyncStatusDiscovering,
		StartTime: time.Now(),
	}
	s.mu.Unlock()

	defer cancel()

	// Discover available snapshots from peers
	slog.Info("Discovering snapshots from peers", "peer_count", len(peers))
	snapshots, err := s.discoverSnapshots(ctx, peers)
	if err != nil {
		s.setStatus(SyncStatusFailed, err)
		return nil, err
	}

	if len(snapshots) == 0 {
		err := errors.New("no snapshots available from peers")
		s.setStatus(SyncStatusFailed, err)
		return nil, err
	}

	// Select best snapshot
	best := s.selectBestSnapshot(snapshots)
	if best == nil {
		err := errors.New("no valid snapshot found")
		s.setStatus(SyncStatusFailed, err)
		return nil, err
	}

	slog.Info("Selected snapshot for sync",
		"height", best.info.Height,
		"round", best.info.Round,
		"peer", best.peer)

	// Download snapshot
	s.setStatus(SyncStatusDownloading, nil)
	s.mu.Lock()
	s.progress.Height = best.info.Height
	s.progress.TotalChunks = best.info.ChunkCount
	s.progress.TotalBytes = best.info.Size
	s.mu.Unlock()

	snapshot, err := s.downloadSnapshot(ctx, best.peer, best.info)
	if err != nil {
		s.setStatus(SyncStatusFailed, err)
		return nil, err
	}

	// Verify snapshot
	s.setStatus(SyncStatusVerifying, nil)
	if err := VerifySnapshot(snapshot, nil); err != nil {
		s.setStatus(SyncStatusFailed, err)
		return nil, err
	}

	// Apply snapshot
	s.setStatus(SyncStatusApplying, nil)
	result, err := s.restorer.Restore(ctx, snapshot)
	if err != nil {
		s.setStatus(SyncStatusFailed, err)
		return nil, err
	}

	s.setStatus(SyncStatusComplete, nil)
	slog.Info("State sync complete",
		"height", result.Height,
		"round", result.Round,
		"certificates", result.RestoredCertificates)

	return result, nil
}

// setStatus updates the sync status.
func (s *StateSync) setStatus(status SyncStatus, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress.Status = status
	s.progress.Error = err
}

type peerSnapshot struct {
	peer peer.ID
	info *SnapshotInfo
}

// discoverSnapshots queries peers for available snapshots.
func (s *StateSync) discoverSnapshots(ctx context.Context, peers []peer.ID) ([]peerSnapshot, error) {
	var mu sync.Mutex
	var snapshots []peerSnapshot
	var wg sync.WaitGroup

	for _, p := range peers {
		wg.Add(1)
		go func(peerID peer.ID) {
			defer wg.Done()

			infos, err := s.listSnapshotsFromPeer(ctx, peerID)
			if err != nil {
				slog.Debug("Failed to list snapshots from peer",
					"peer", peerID,
					"error", err)
				return
			}

			mu.Lock()
			for _, info := range infos {
				snapshots = append(snapshots, peerSnapshot{
					peer: peerID,
					info: info,
				})
			}
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	return snapshots, nil
}

// listSnapshotsFromPeer queries a single peer for available snapshots.
func (s *StateSync) listSnapshotsFromPeer(ctx context.Context, peerID peer.ID) ([]*SnapshotInfo, error) {
	stream, err := s.host.NewStream(ctx, peerID, protocol.ID(ProtocolSnapshotList))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	// Send empty request
	if err := writeMessage(stream, nil, DefaultChunkTimeout); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read response
	data, err := readMessage(stream, MaxSnapshotsInList*100, DefaultChunkTimeout)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return unmarshalSnapshotInfoList(data)
}

// selectBestSnapshot selects the best snapshot from available options.
func (s *StateSync) selectBestSnapshot(snapshots []peerSnapshot) *peerSnapshot {
	if len(snapshots) == 0 {
		return nil
	}

	// Select the snapshot with the highest height
	var best *peerSnapshot
	for i := range snapshots {
		ps := &snapshots[i]
		if best == nil || ps.info.Height > best.info.Height {
			best = ps
		}
	}

	return best
}

// downloadSnapshot downloads a complete snapshot from a peer.
func (s *StateSync) downloadSnapshot(ctx context.Context, peerID peer.ID, info *SnapshotInfo) (*Snapshot, error) {
	stream, err := s.host.NewStream(ctx, peerID, protocol.ID(ProtocolSnapshotFetch))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	// Send request with snapshot height
	req := make([]byte, 8)
	binary.BigEndian.PutUint64(req, info.Height)
	if err := writeMessage(stream, req, DefaultChunkTimeout); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read response
	data, err := readMessage(stream, MaxSnapshotSize, DefaultSyncTimeout)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if len(data) == 0 {
		return nil, errors.New("snapshot not found")
	}

	s.mu.Lock()
	s.progress.BytesReceived = uint64(len(data))
	s.progress.ReceivedChunks = info.ChunkCount
	s.mu.Unlock()

	return UnmarshalSnapshot(data)
}

// handleListSnapshots handles incoming snapshot list requests.
func (s *StateSync) handleListSnapshots(stream network.Stream) {
	defer stream.Close()

	// Read empty request
	_, err := readMessage(stream, 1, DefaultChunkTimeout)
	if err != nil && err != io.EOF {
		slog.Debug("Failed to read list request", "error", err)
		return
	}

	// Get available snapshots
	heights := s.store.List()
	infos := make([]*SnapshotInfo, 0, len(heights))

	for _, h := range heights {
		snap := s.store.Get(h)
		if snap == nil {
			continue
		}

		data, err := snap.Marshal()
		if err != nil {
			continue
		}

		infos = append(infos, &SnapshotInfo{
			Height:     snap.Height,
			Round:      snap.Round,
			StateHash:  snap.StateHash,
			Size:       uint64(len(data)),
			ChunkCount: (len(data) + s.config.ChunkSize - 1) / s.config.ChunkSize,
			Digest:     snap.Digest(),
		})
	}

	// Send response
	data := marshalSnapshotInfoList(infos)
	if err := writeMessage(stream, data, DefaultChunkTimeout); err != nil {
		slog.Debug("Failed to send snapshot list", "error", err)
	}
}

// handleFetchSnapshot handles incoming snapshot fetch requests.
func (s *StateSync) handleFetchSnapshot(stream network.Stream) {
	defer stream.Close()

	// Read request (height)
	data, err := readMessage(stream, 8, DefaultChunkTimeout)
	if err != nil {
		slog.Debug("Failed to read fetch request", "error", err)
		return
	}

	if len(data) < 8 {
		slog.Debug("Invalid fetch request")
		return
	}

	height := binary.BigEndian.Uint64(data)

	// Get snapshot
	snap := s.store.Get(height)
	if snap == nil {
		// Send empty response
		if err := writeMessage(stream, nil, DefaultChunkTimeout); err != nil {
			slog.Debug("Failed to send empty response", "error", err)
		}
		return
	}

	// Marshal and send snapshot
	snapData, err := snap.Marshal()
	if err != nil {
		slog.Debug("Failed to marshal snapshot", "error", err)
		return
	}

	if err := writeMessage(stream, snapData, DefaultSyncTimeout); err != nil {
		slog.Debug("Failed to send snapshot", "error", err)
	}
}

// handleFetchChunk handles incoming chunk fetch requests.
func (s *StateSync) handleFetchChunk(stream network.Stream) {
	defer stream.Close()

	// Read request (height + chunk index)
	data, err := readMessage(stream, 12, DefaultChunkTimeout)
	if err != nil {
		slog.Debug("Failed to read chunk request", "error", err)
		return
	}

	if len(data) < 12 {
		slog.Debug("Invalid chunk request")
		return
	}

	height := binary.BigEndian.Uint64(data[0:8])
	chunkIndex := binary.BigEndian.Uint32(data[8:12])

	// Get snapshot
	snap := s.store.Get(height)
	if snap == nil {
		if err := writeMessage(stream, nil, DefaultChunkTimeout); err != nil {
			slog.Debug("Failed to send empty response", "error", err)
		}
		return
	}

	// Marshal snapshot
	snapData, err := snap.Marshal()
	if err != nil {
		slog.Debug("Failed to marshal snapshot", "error", err)
		return
	}

	// Extract chunk
	totalChunks := uint32((len(snapData) + s.config.ChunkSize - 1) / s.config.ChunkSize)
	if chunkIndex >= totalChunks {
		if err := writeMessage(stream, nil, DefaultChunkTimeout); err != nil {
			slog.Debug("Failed to send empty response", "error", err)
		}
		return
	}

	start := int(chunkIndex) * s.config.ChunkSize
	end := start + s.config.ChunkSize
	if end > len(snapData) {
		end = len(snapData)
	}

	chunk := &SnapshotChunk{
		Index: chunkIndex,
		Total: totalChunks,
		Data:  snapData[start:end],
	}

	if err := writeMessage(stream, chunk.Marshal(), DefaultChunkTimeout); err != nil {
		slog.Debug("Failed to send chunk", "error", err)
	}
}

// Helper functions for message I/O

func writeMessage(stream network.Stream, data []byte, timeout time.Duration) error {
	if err := stream.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}

	// Write length prefix
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := stream.Write(lenBuf); err != nil {
		return err
	}

	// Write data
	if len(data) > 0 {
		if _, err := stream.Write(data); err != nil {
			return err
		}
	}

	return nil
}

func readMessage(stream network.Stream, maxSize int, timeout time.Duration) ([]byte, error) {
	if err := stream.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}

	// Read length prefix
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lenBuf)
	if int(length) > maxSize {
		return nil, fmt.Errorf("message too large: %d > %d", length, maxSize)
	}

	if length == 0 {
		return nil, nil
	}

	// Read data
	data := make([]byte, length)
	if _, err := io.ReadFull(stream, data); err != nil {
		return nil, err
	}

	return data, nil
}

func marshalSnapshotInfoList(infos []*SnapshotInfo) []byte {
	if len(infos) == 0 {
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, 0)
		return data
	}

	// count(4) + infos
	size := 4
	for range infos {
		size += 92 // Fixed size of SnapshotInfo
	}

	data := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint32(data[offset:], uint32(len(infos)))
	offset += 4

	for _, info := range infos {
		infoData := info.Marshal()
		copy(data[offset:], infoData)
		offset += len(infoData)
	}

	return data
}

func unmarshalSnapshotInfoList(data []byte) ([]*SnapshotInfo, error) {
	if len(data) < 4 {
		return nil, errors.New("snapshot info list too short")
	}

	count := binary.BigEndian.Uint32(data[0:4])
	if count == 0 {
		return nil, nil
	}

	if count > MaxSnapshotsInList {
		return nil, errors.New("too many snapshots in list")
	}

	infos := make([]*SnapshotInfo, 0, count)
	offset := 4

	for i := uint32(0); i < count; i++ {
		if offset+92 > len(data) {
			return nil, errors.New("snapshot info list truncated")
		}

		info, err := UnmarshalSnapshotInfo(data[offset : offset+92])
		if err != nil {
			return nil, err
		}

		infos = append(infos, info)
		offset += 92
	}

	return infos, nil
}
