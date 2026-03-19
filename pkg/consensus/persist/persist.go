// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package persist provides state persistence for DAG-BFT consensus nodes.
// It handles checkpointing and restoring consensus state for graceful
// shutdown and restart scenarios.
package persist

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// CheckpointFileName is the default filename for consensus checkpoints.
const CheckpointFileName = "consensus_checkpoint.json"

// ErrNoCheckpoint is returned when no checkpoint file exists.
var ErrNoCheckpoint = errors.New("no checkpoint found")

// Checkpoint represents a snapshot of consensus state for persistence.
type Checkpoint struct {
	// Version identifies the checkpoint format version.
	Version int `json:"version"`

	// Timestamp is when the checkpoint was created.
	Timestamp time.Time `json:"timestamp"`

	// Partition is the network partition this checkpoint is for.
	Partition string `json:"partition"`

	// CurrentRound is the consensus round at checkpoint time.
	CurrentRound types.Round `json:"current_round"`

	// CurrentEpoch is the consensus epoch at checkpoint time.
	CurrentEpoch uint64 `json:"current_epoch"`

	// LastCommitRound is the most recently committed leader round.
	LastCommitRound types.Round `json:"last_commit_round"`

	// LastCommitted tracks the last committed round for each author.
	// Key is the hex-encoded author public key.
	LastCommitted map[string]types.Round `json:"last_committed"`

	// Certificates holds serialized certificates that need to be restored.
	// This includes pending certificates that weren't yet committed.
	Certificates []CertificateData `json:"certificates,omitempty"`

	// PendingBatches holds batch digests for pending transactions.
	PendingBatches []string `json:"pending_batches,omitempty"`
}

// CertificateData holds serialized certificate information for persistence.
type CertificateData struct {
	// Digest is the certificate digest in hex.
	Digest string `json:"digest"`

	// Round is the certificate round.
	Round types.Round `json:"round"`

	// Author is the hex-encoded author public key.
	Author string `json:"author"`

	// Data is the full certificate serialized as JSON.
	Data json.RawMessage `json:"data"`
}

// NewCheckpoint creates a new checkpoint with the given state.
func NewCheckpoint(partition string, currentRound types.Round, currentEpoch uint64, lastCommitRound types.Round, lastCommitted map[string]types.Round) *Checkpoint {
	// Make a copy of lastCommitted to avoid external modification
	committed := make(map[string]types.Round, len(lastCommitted))
	for k, v := range lastCommitted {
		committed[k] = v
	}

	return &Checkpoint{
		Version:         1,
		Timestamp:       time.Now().UTC(),
		Partition:       partition,
		CurrentRound:    currentRound,
		CurrentEpoch:    currentEpoch,
		LastCommitRound: lastCommitRound,
		LastCommitted:   committed,
	}
}

// Store provides thread-safe checkpoint storage.
type Store struct {
	mu   sync.RWMutex
	dir  string
	file string
}

// NewStore creates a new checkpoint store with the given directory.
func NewStore(dir string) *Store {
	return &Store{
		dir:  dir,
		file: CheckpointFileName,
	}
}

// SetFilename sets a custom checkpoint filename.
func (s *Store) SetFilename(filename string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file = filename
}

// checkpointPath returns the full path to the checkpoint file.
func (s *Store) checkpointPath() string {
	return filepath.Join(s.dir, s.file)
}

// Save writes a checkpoint to disk atomically.
func (s *Store) Save(checkpoint *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if checkpoint == nil {
		return errors.New("checkpoint is nil")
	}

	// Ensure directory exists
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("create checkpoint directory: %w", err)
	}

	// Serialize checkpoint
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	// Write atomically using temp file + rename
	tmpPath := s.checkpointPath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write checkpoint temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.checkpointPath()); err != nil {
		// Clean up temp file on failure
		os.Remove(tmpPath)
		return fmt.Errorf("rename checkpoint file: %w", err)
	}

	return nil
}

// Load reads a checkpoint from disk.
func (s *Store) Load() (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.checkpointPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoCheckpoint
		}
		return nil, fmt.Errorf("read checkpoint file: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// Exists returns true if a checkpoint file exists.
func (s *Store) Exists() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, err := os.Stat(s.checkpointPath())
	return err == nil
}

// Delete removes the checkpoint file.
func (s *Store) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := os.Remove(s.checkpointPath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete checkpoint: %w", err)
	}
	return nil
}

// Dir returns the checkpoint directory.
func (s *Store) Dir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dir
}

// StateSnapshot captures current node state for checkpointing.
type StateSnapshot struct {
	// Partition is the network partition.
	Partition string

	// CurrentRound is the current consensus round.
	CurrentRound types.Round

	// CurrentEpoch is the current consensus epoch.
	CurrentEpoch uint64

	// LastCommitRound is the most recently committed leader round.
	LastCommitRound types.Round

	// LastCommitted tracks the last committed round for each author.
	LastCommitted map[string]types.Round

	// PendingCertificates holds certificates waiting for parents.
	PendingCertificates []*types.Certificate
}

// ToCheckpoint converts a state snapshot to a persistable checkpoint.
func (s *StateSnapshot) ToCheckpoint() *Checkpoint {
	cp := NewCheckpoint(
		s.Partition,
		s.CurrentRound,
		s.CurrentEpoch,
		s.LastCommitRound,
		s.LastCommitted,
	)

	// Add pending certificates if any
	if len(s.PendingCertificates) > 0 {
		cp.Certificates = make([]CertificateData, 0, len(s.PendingCertificates))
		for _, cert := range s.PendingCertificates {
			if cert == nil {
				continue
			}
			// Serialize certificate using binary marshaling, then base64 encode
			// for JSON storage. We use binary marshaling because Certificate
			// contains Header.Payload which has a non-string map key type
			// (BatchDigest) that json.Marshal doesn't support.
			data, err := cert.Marshal()
			if err != nil {
				continue // Skip on error
			}
			encoded := base64.StdEncoding.EncodeToString(data)
			cp.Certificates = append(cp.Certificates, CertificateData{
				Digest: cert.Digest().String(),
				Round:  cert.Round(),
				Author: fmt.Sprintf("%x", cert.Author()),
				Data:   json.RawMessage(`"` + encoded + `"`),
			})
		}
	}

	return cp
}

// Validate checks if the checkpoint data is consistent.
func (c *Checkpoint) Validate() error {
	if c.Version < 1 {
		return errors.New("invalid checkpoint version")
	}
	if c.Partition == "" {
		return errors.New("partition is empty")
	}
	if c.LastCommitRound > c.CurrentRound {
		return errors.New("last commit round exceeds current round")
	}
	return nil
}
