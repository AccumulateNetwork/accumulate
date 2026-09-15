// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package snapshot provides snapshot creation and state sync for DAG-BFT consensus.
// Snapshots capture application state, DAG state, and committee information
// to enable new nodes to sync from a known checkpoint.
package snapshot

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Version is the snapshot format version.
const Version = 1

// Default configuration values for snapshots.
const (
	DefaultSnapshotInterval   = 1000            // Create snapshot every N blocks
	DefaultRetainedSnapshots  = 3               // Number of snapshots to retain
	DefaultChunkSize          = 1 * 1024 * 1024 // 1MB chunks
	DefaultCertificateHistory = 100             // Number of recent certificate rounds to include
)

// SnapshotConfig contains configuration for snapshot creation.
type SnapshotConfig struct {
	// Interval is the number of blocks between snapshots.
	Interval uint64

	// RetainedSnapshots is the number of snapshots to retain.
	RetainedSnapshots int

	// ChunkSize is the size of snapshot chunks for transfer.
	ChunkSize int

	// CertificateHistory is the number of recent certificate rounds to include.
	CertificateHistory int
}

// DefaultSnapshotConfig returns a SnapshotConfig with default values.
func DefaultSnapshotConfig() *SnapshotConfig {
	return &SnapshotConfig{
		Interval:           DefaultSnapshotInterval,
		RetainedSnapshots:  DefaultRetainedSnapshots,
		ChunkSize:          DefaultChunkSize,
		CertificateHistory: DefaultCertificateHistory,
	}
}

// Snapshot represents a point-in-time capture of consensus state.
// It contains enough information to bootstrap a new node.
type Snapshot struct {
	// Version is the snapshot format version.
	Version uint32

	// Height is the block height at which the snapshot was taken.
	Height uint64

	// Round is the consensus round at the snapshot.
	Round types.Round

	// StateHash is the hash of the application state.
	StateHash [32]byte

	// Committee is the validator committee at this height.
	Committee *types.Committee

	// Certificates contains recent certificates for catch-up.
	// These allow a node to resume consensus participation.
	Certificates []*types.Certificate

	// Timestamp is when the snapshot was created.
	Timestamp time.Time

	// Metadata contains optional snapshot metadata.
	Metadata *SnapshotMetadata
}

// SnapshotMetadata contains optional metadata about a snapshot.
type SnapshotMetadata struct {
	// ChainID identifies the blockchain network.
	ChainID string

	// LastCommitRound is the last committed consensus round.
	LastCommitRound types.Round

	// TotalCertificates is the total number of certificates included.
	TotalCertificates int

	// CreatedBy is the node ID that created this snapshot.
	CreatedBy string
}

// SnapshotDigest is a 32-byte hash uniquely identifying a snapshot.
type SnapshotDigest [32]byte

// IsZero returns true if the digest is all zeros.
func (d SnapshotDigest) IsZero() bool {
	return d == SnapshotDigest{}
}

// Digest computes the SHA-256 hash of the snapshot.
func (s *Snapshot) Digest() SnapshotDigest {
	data, _ := s.marshalForDigest()
	return sha256.Sum256(data)
}

// marshalForDigest serializes the snapshot for digest computation.
func (s *Snapshot) marshalForDigest() ([]byte, error) {
	// Calculate size: version(4) + height(8) + round(8) + stateHash(32) + timestamp(8)
	size := 4 + 8 + 8 + 32 + 8

	data := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint32(data[offset:], s.Version)
	offset += 4

	binary.BigEndian.PutUint64(data[offset:], s.Height)
	offset += 8

	binary.BigEndian.PutUint64(data[offset:], uint64(s.Round))
	offset += 8

	copy(data[offset:], s.StateHash[:])
	offset += 32

	binary.BigEndian.PutUint64(data[offset:], uint64(s.Timestamp.UnixNano()))

	return data, nil
}

// Validate checks that the snapshot is well-formed.
func (s *Snapshot) Validate() error {
	if s.Version == 0 {
		return errors.New("snapshot version is zero")
	}
	if s.Height == 0 {
		return errors.New("snapshot height is zero")
	}
	if s.Committee == nil {
		return errors.New("snapshot committee is nil")
	}
	if err := s.Committee.Validate(); err != nil {
		return err
	}
	return nil
}

// Marshal serializes the snapshot to bytes.
func (s *Snapshot) Marshal() ([]byte, error) {
	// Marshal committee
	committeeData, err := marshalCommittee(s.Committee)
	if err != nil {
		return nil, err
	}

	// Marshal certificates
	certsData, err := marshalCertificates(s.Certificates)
	if err != nil {
		return nil, err
	}

	// Marshal metadata
	var metadataData []byte
	if s.Metadata != nil {
		metadataData, err = marshalMetadata(s.Metadata)
		if err != nil {
			return nil, err
		}
	}

	// Calculate total size
	// version(4) + height(8) + round(8) + stateHash(32) + timestamp(8)
	// + committeeLen(4) + committee + certsLen(4) + certs
	// + metadataLen(4) + metadata
	size := 4 + 8 + 8 + 32 + 8 + 4 + len(committeeData) + 4 + len(certsData) + 4 + len(metadataData)

	data := make([]byte, size)
	offset := 0

	// Version
	binary.BigEndian.PutUint32(data[offset:], s.Version)
	offset += 4

	// Height
	binary.BigEndian.PutUint64(data[offset:], s.Height)
	offset += 8

	// Round
	binary.BigEndian.PutUint64(data[offset:], uint64(s.Round))
	offset += 8

	// StateHash
	copy(data[offset:], s.StateHash[:])
	offset += 32

	// Timestamp
	binary.BigEndian.PutUint64(data[offset:], uint64(s.Timestamp.UnixNano()))
	offset += 8

	// Committee
	binary.BigEndian.PutUint32(data[offset:], uint32(len(committeeData)))
	offset += 4
	copy(data[offset:], committeeData)
	offset += len(committeeData)

	// Certificates
	binary.BigEndian.PutUint32(data[offset:], uint32(len(certsData)))
	offset += 4
	copy(data[offset:], certsData)
	offset += len(certsData)

	// Metadata
	binary.BigEndian.PutUint32(data[offset:], uint32(len(metadataData)))
	offset += 4
	if len(metadataData) > 0 {
		copy(data[offset:], metadataData)
	}

	return data, nil
}

// UnmarshalSnapshot deserializes a snapshot from bytes.
func UnmarshalSnapshot(data []byte) (*Snapshot, error) {
	if len(data) < 64 { // Minimum size check
		return nil, errors.New("snapshot data too short")
	}

	s := &Snapshot{}
	offset := 0

	// Version
	s.Version = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// Height
	s.Height = binary.BigEndian.Uint64(data[offset:])
	offset += 8

	// Round
	s.Round = types.Round(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	// StateHash
	copy(s.StateHash[:], data[offset:offset+32])
	offset += 32

	// Timestamp
	s.Timestamp = time.Unix(0, int64(binary.BigEndian.Uint64(data[offset:])))
	offset += 8

	// Committee
	if offset+4 > len(data) {
		return nil, errors.New("snapshot truncated: missing committee length")
	}
	committeeLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(committeeLen) > len(data) {
		return nil, errors.New("snapshot truncated: missing committee data")
	}
	committee, err := unmarshalCommittee(data[offset : offset+int(committeeLen)])
	if err != nil {
		return nil, err
	}
	s.Committee = committee
	offset += int(committeeLen)

	// Certificates
	if offset+4 > len(data) {
		return nil, errors.New("snapshot truncated: missing certificates length")
	}
	certsLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(certsLen) > len(data) {
		return nil, errors.New("snapshot truncated: missing certificates data")
	}
	certs, err := unmarshalCertificates(data[offset : offset+int(certsLen)])
	if err != nil {
		return nil, err
	}
	s.Certificates = certs
	offset += int(certsLen)

	// Metadata (optional)
	if offset+4 > len(data) {
		return nil, errors.New("snapshot truncated: missing metadata length")
	}
	metadataLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if metadataLen > 0 {
		if offset+int(metadataLen) > len(data) {
			return nil, errors.New("snapshot truncated: missing metadata")
		}
		metadata, err := unmarshalMetadata(data[offset : offset+int(metadataLen)])
		if err != nil {
			return nil, err
		}
		s.Metadata = metadata
	}

	return s, nil
}

// SnapshotStore manages snapshot storage and retrieval.
type SnapshotStore struct {
	mu        sync.RWMutex
	snapshots map[uint64]*Snapshot // keyed by height
	config    *SnapshotConfig
}

// NewSnapshotStore creates a new SnapshotStore.
func NewSnapshotStore(config *SnapshotConfig) *SnapshotStore {
	if config == nil {
		config = DefaultSnapshotConfig()
	}
	return &SnapshotStore{
		snapshots: make(map[uint64]*Snapshot),
		config:    config,
	}
}

// Store stores a snapshot.
func (s *SnapshotStore) Store(snapshot *Snapshot) error {
	if snapshot == nil {
		return errors.New("snapshot is nil")
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshots[snapshot.Height] = snapshot

	// Prune old snapshots
	s.pruneUnlocked()

	return nil
}

// Get retrieves a snapshot by height.
func (s *SnapshotStore) Get(height uint64) *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshots[height]
}

// Latest returns the most recent snapshot.
func (s *SnapshotStore) Latest() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *Snapshot
	for _, snap := range s.snapshots {
		if latest == nil || snap.Height > latest.Height {
			latest = snap
		}
	}
	return latest
}

// List returns all available snapshot heights in descending order.
func (s *SnapshotStore) List() []uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	heights := make([]uint64, 0, len(s.snapshots))
	for h := range s.snapshots {
		heights = append(heights, h)
	}

	// Sort descending
	for i := 0; i < len(heights)-1; i++ {
		for j := i + 1; j < len(heights); j++ {
			if heights[j] > heights[i] {
				heights[i], heights[j] = heights[j], heights[i]
			}
		}
	}

	return heights
}

// pruneUnlocked removes old snapshots beyond the retention limit.
// Must be called with the lock held.
func (s *SnapshotStore) pruneUnlocked() {
	if len(s.snapshots) <= s.config.RetainedSnapshots {
		return
	}

	// Find heights to remove
	heights := make([]uint64, 0, len(s.snapshots))
	for h := range s.snapshots {
		heights = append(heights, h)
	}

	// Sort ascending
	for i := 0; i < len(heights)-1; i++ {
		for j := i + 1; j < len(heights); j++ {
			if heights[j] < heights[i] {
				heights[i], heights[j] = heights[j], heights[i]
			}
		}
	}

	// Remove oldest snapshots
	toRemove := len(heights) - s.config.RetainedSnapshots
	for i := 0; i < toRemove; i++ {
		delete(s.snapshots, heights[i])
	}
}

// SnapshotCreator creates snapshots from consensus state.
type SnapshotCreator struct {
	config     *SnapshotConfig
	store      *SnapshotStore
	lastHeight uint64
	mu         sync.Mutex
}

// NewSnapshotCreator creates a new SnapshotCreator.
func NewSnapshotCreator(config *SnapshotConfig, store *SnapshotStore) *SnapshotCreator {
	if config == nil {
		config = DefaultSnapshotConfig()
	}
	return &SnapshotCreator{
		config: config,
		store:  store,
	}
}

// ShouldSnapshot returns true if a snapshot should be created at the given height.
func (c *SnapshotCreator) ShouldSnapshot(height uint64) bool {
	if c.config.Interval == 0 {
		return false
	}
	return height > 0 && height%c.config.Interval == 0
}

// CreateSnapshotParams contains parameters for creating a snapshot.
type CreateSnapshotParams struct {
	Height    uint64
	Round     types.Round
	StateHash [32]byte
	Committee *types.Committee
	GetCerts  func(fromRound, toRound types.Round) []*types.Certificate
	ChainID   string
	NodeID    string
}

// CreateSnapshot creates a new snapshot from the given parameters.
func (c *SnapshotCreator) CreateSnapshot(params CreateSnapshotParams) (*Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if params.Height <= c.lastHeight {
		return nil, errors.New("snapshot height must be greater than last snapshot")
	}

	if params.Committee == nil {
		return nil, errors.New("committee is required")
	}

	// Collect recent certificates
	var certs []*types.Certificate
	if params.GetCerts != nil && params.Round > 0 {
		fromRound := params.Round
		if fromRound > types.Round(c.config.CertificateHistory) {
			fromRound = params.Round - types.Round(c.config.CertificateHistory)
		} else {
			fromRound = 0
		}
		certs = params.GetCerts(fromRound, params.Round)
	}

	snapshot := &Snapshot{
		Version:      Version,
		Height:       params.Height,
		Round:        params.Round,
		StateHash:    params.StateHash,
		Committee:    params.Committee.Clone(),
		Certificates: certs,
		Timestamp:    time.Now(),
		Metadata: &SnapshotMetadata{
			ChainID:           params.ChainID,
			LastCommitRound:   params.Round,
			TotalCertificates: len(certs),
			CreatedBy:         params.NodeID,
		},
	}

	if c.store != nil {
		if err := c.store.Store(snapshot); err != nil {
			return nil, err
		}
	}

	c.lastHeight = params.Height
	return snapshot, nil
}

// Helper functions for committee serialization

func marshalCommittee(c *types.Committee) ([]byte, error) {
	if c == nil {
		return nil, errors.New("committee is nil")
	}

	// epoch(8) + numValidators(4) + validators
	size := 8 + 4
	for range c.Validators {
		size += 32 + 8 // pubkey + stake
	}

	data := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint64(data[offset:], c.Epoch)
	offset += 8

	binary.BigEndian.PutUint32(data[offset:], uint32(len(c.Validators)))
	offset += 4

	for _, v := range c.Validators {
		copy(data[offset:], v.PublicKey)
		offset += 32
		binary.BigEndian.PutUint64(data[offset:], v.Stake)
		offset += 8
	}

	return data, nil
}

func unmarshalCommittee(data []byte) (*types.Committee, error) {
	if len(data) < 12 {
		return nil, errors.New("committee data too short")
	}

	offset := 0

	epoch := binary.BigEndian.Uint64(data[offset:])
	offset += 8

	numValidators := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if numValidators > 10000 {
		return nil, errors.New("too many validators")
	}

	validators := make([]types.ValidatorInfo, numValidators)
	for i := uint32(0); i < numValidators; i++ {
		if offset+40 > len(data) {
			return nil, errors.New("committee data truncated")
		}

		validators[i].PublicKey = make([]byte, 32)
		copy(validators[i].PublicKey, data[offset:offset+32])
		offset += 32

		validators[i].Stake = binary.BigEndian.Uint64(data[offset:])
		offset += 8
	}

	return types.NewCommittee(validators, epoch), nil
}

func marshalCertificates(certs []*types.Certificate) ([]byte, error) {
	if len(certs) == 0 {
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, 0)
		return data, nil
	}

	// Serialize each certificate
	certData := make([][]byte, len(certs))
	totalSize := 4 // count
	for i, cert := range certs {
		d, err := cert.Marshal()
		if err != nil {
			return nil, err
		}
		certData[i] = d
		totalSize += 4 + len(d) // length prefix + data
	}

	data := make([]byte, totalSize)
	offset := 0

	binary.BigEndian.PutUint32(data[offset:], uint32(len(certs)))
	offset += 4

	for _, cd := range certData {
		binary.BigEndian.PutUint32(data[offset:], uint32(len(cd)))
		offset += 4
		copy(data[offset:], cd)
		offset += len(cd)
	}

	return data, nil
}

func unmarshalCertificates(data []byte) ([]*types.Certificate, error) {
	if len(data) < 4 {
		return nil, errors.New("certificates data too short")
	}

	count := binary.BigEndian.Uint32(data[0:4])
	if count == 0 {
		return nil, nil
	}

	if count > 100000 {
		return nil, errors.New("too many certificates")
	}

	certs := make([]*types.Certificate, 0, count)
	offset := 4

	for i := uint32(0); i < count; i++ {
		if offset+4 > len(data) {
			return nil, errors.New("certificates data truncated")
		}

		certLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4

		if offset+int(certLen) > len(data) {
			return nil, errors.New("certificate data truncated")
		}

		cert, err := types.UnmarshalCertificate(data[offset : offset+int(certLen)])
		if err != nil {
			return nil, err
		}

		certs = append(certs, cert)
		offset += int(certLen)
	}

	return certs, nil
}

func marshalMetadata(m *SnapshotMetadata) ([]byte, error) {
	if m == nil {
		return nil, nil
	}

	// chainIDLen(4) + chainID + lastCommitRound(8) + totalCerts(4) + createdByLen(4) + createdBy
	size := 4 + len(m.ChainID) + 8 + 4 + 4 + len(m.CreatedBy)

	data := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint32(data[offset:], uint32(len(m.ChainID)))
	offset += 4
	copy(data[offset:], m.ChainID)
	offset += len(m.ChainID)

	binary.BigEndian.PutUint64(data[offset:], uint64(m.LastCommitRound))
	offset += 8

	binary.BigEndian.PutUint32(data[offset:], uint32(m.TotalCertificates))
	offset += 4

	binary.BigEndian.PutUint32(data[offset:], uint32(len(m.CreatedBy)))
	offset += 4
	copy(data[offset:], m.CreatedBy)

	return data, nil
}

func unmarshalMetadata(data []byte) (*SnapshotMetadata, error) {
	if len(data) < 20 {
		return nil, errors.New("metadata too short")
	}

	m := &SnapshotMetadata{}
	offset := 0

	chainIDLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(chainIDLen) > len(data) {
		return nil, errors.New("metadata truncated: chainID")
	}
	m.ChainID = string(data[offset : offset+int(chainIDLen)])
	offset += int(chainIDLen)

	if offset+8 > len(data) {
		return nil, errors.New("metadata truncated: lastCommitRound")
	}
	m.LastCommitRound = types.Round(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	if offset+4 > len(data) {
		return nil, errors.New("metadata truncated: totalCertificates")
	}
	m.TotalCertificates = int(binary.BigEndian.Uint32(data[offset:]))
	offset += 4

	if offset+4 > len(data) {
		return nil, errors.New("metadata truncated: createdBy length")
	}
	createdByLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(createdByLen) > len(data) {
		return nil, errors.New("metadata truncated: createdBy")
	}
	m.CreatedBy = string(data[offset : offset+int(createdByLen)])

	return m, nil
}
