// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package proof

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Service provides cryptographic proof creation and validation
type Service struct {
	db     database.Beginner
	logger logging.Logger
	mu     sync.RWMutex
}

// NewService creates a new proof service
func NewService(db database.Beginner, logger logging.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// CreateProof generates a cryptographic proof for a transaction
func (s *Service) CreateProof(ctx context.Context, req *ProofRequest) (*ProofResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("proof request is required")
	}

	if req.Account == nil {
		return nil, fmt.Errorf("account URL is required")
	}

	if req.Anchor == nil {
		return nil, fmt.Errorf("anchor URL is required")
	}

	start := time.Now()

	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()

	// Open a read transaction to access the database
	batch := db.Begin(false)
	defer batch.Discard()

	// Retrieve the account from the database
	record := batch.Account(req.Account).Main()

	// Read the account record
	account, err := record.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve account: %w", err)
	}

	// Generate the proof - for now, create a simple merkle proof
	// In production, this would use the full merkle tree infrastructure
	proof := s.generateMerkleProof(req, account)
	generationTime := time.Since(start).Milliseconds()

	// Record metrics if configured
	_ = generationTime

	return &ProofResponse{
		Proof:     proof,
		Type:      ProofTypeTransaction,
		Root:      req.Root,
		Anchor:    req.Anchor,
		Sequence:  req.Sequence,
		Timestamp: req.Timestamp,
	}, nil
}

// ValidateProof validates a cryptographic proof
func (s *Service) ValidateProof(ctx context.Context, proof []byte, root [32]byte, anchor *url.URL) (*ValidationResult, error) {
	if len(proof) == 0 {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: "proof is empty",
		}, nil
	}

	if anchor == nil {
		return &ValidationResult{
			Valid:        false,
			ErrorMessage: "anchor URL is required",
		}, nil
	}

	// Verify the proof against the root hash
	proofHash := sha256.Sum256(proof)
	isValid := bytes.Equal(proofHash[:], root[:])

	return &ValidationResult{
		Valid:     isValid,
		ProofType: ProofTypeTransaction,
		Timestamp: time.Now(),
	}, nil
}

// generateMerkleProof creates a simple merkle proof
// This is a placeholder implementation that should be replaced with
// the full merkle tree infrastructure in production
func (s *Service) generateMerkleProof(req *ProofRequest, account protocol.Account) []byte {
	// Create a deterministic proof based on the account and sequence
	h := sha256.New()
	h.Write([]byte(account.GetUrl().String()))
	h.Write(req.Root[:])

	// Add sequence number to make it unique per transaction
	seqBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		seqBytes[i] = byte(req.Sequence >> (8 * (7 - i)))
	}
	h.Write(seqBytes)

	// Add timestamp for temporal ordering
	h.Write(req.Timestamp.AppendFormat(nil, time.RFC3339Nano))

	return h.Sum(nil)
}

// Close closes the service and releases resources
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil
}
