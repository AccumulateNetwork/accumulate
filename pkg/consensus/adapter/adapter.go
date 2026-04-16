// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package adapter bridges DAG-BFT consensus with the Accumulate execution layer.
// It converts committed certificates and batches into block production calls.
package adapter

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// BlockProducer manages block lifecycle: begin, process certificates, commit.
type BlockProducer interface {
	// BeginBlock opens a new block at the given index and time.
	BeginBlock(ctx context.Context, index uint64, time time.Time) error

	// ProcessCertificate processes a committed certificate's transactions into
	// the currently open block without committing.
	ProcessCertificate(ctx context.Context, params CertificateParams) error

	// CommitBlock closes and commits the currently open block.
	// Returns the block hash.
	CommitBlock(ctx context.Context) ([32]byte, error)

	// ProduceBlock is the legacy single-shot method that opens, processes, and
	// commits a block in one call. Kept for backward compatibility with tests.
	ProduceBlock(ctx context.Context, params BlockParams) (hash [32]byte, err error)
}

// BlockParams contains the parameters for single-shot block production (legacy).
type BlockParams struct {
	Index       uint64
	Time        time.Time
	IsLeader    bool
	LeaderRound types.Round
	Certificate *types.Certificate
	Batches     map[types.BatchDigest]*types.Batch
}

// CertificateParams contains the parameters for processing a certificate
// into an already-open block.
type CertificateParams struct {
	IsLeader    bool
	LeaderRound types.Round
	Certificate *types.Certificate
	Batches     map[types.BatchDigest]*types.Batch
}

// TransactionValidator validates transactions before they are added to batches.
type TransactionValidator interface {
	// ValidateTransaction validates a transaction before batching.
	// Returns nil if valid, or an error describing why it's invalid.
	ValidateTransaction(tx []byte) error
}

// StateProvider provides state information to the consensus layer.
type StateProvider interface {
	// LastBlock returns the last committed block index and hash.
	LastBlock() (index uint64, hash [32]byte, err error)

	// StateHash returns the current state hash.
	StateHash() [32]byte
}

// ValidatorSetProvider provides validator set information.
type ValidatorSetProvider interface {
	// Validators returns the current validator set.
	Validators() []ValidatorInfo

	// OnValidatorSetChange is called when the validator set changes.
	// The callback should update the consensus committee.
	OnValidatorSetChange(callback func(validators []ValidatorInfo))
}

// ValidatorInfo contains information about a validator.
type ValidatorInfo struct {
	// PublicKey is the validator's ed25519 public key.
	PublicKey [32]byte

	// Stake is the validator's stake weight.
	Stake uint64

	// Active indicates if the validator is currently active.
	Active bool
}

// ConsensusAdapter combines all adapter interfaces.
// This is the main interface that the DAG-BFT consensus node uses
// to interact with the execution layer.
type ConsensusAdapter interface {
	BlockProducer
	TransactionValidator
	StateProvider
	ValidatorSetProvider
}
