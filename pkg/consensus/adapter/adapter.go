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

// BlockProducer is called by the consensus layer when certificates are committed.
// It processes batches from committed certificates and produces blocks.
type BlockProducer interface {
	// ProduceBlock processes a committed certificate and its batches.
	// Returns the block hash and any error.
	ProduceBlock(ctx context.Context, params BlockParams) (hash [32]byte, err error)
}

// BlockParams contains the parameters for block production.
type BlockParams struct {
	// Index is the block height/index.
	Index uint64

	// Time is the block timestamp.
	Time time.Time

	// IsLeader indicates if this node was the leader for this block.
	IsLeader bool

	// LeaderRound is the consensus round that committed this block.
	LeaderRound types.Round

	// Certificate is the committed certificate that triggered this block.
	Certificate *types.Certificate

	// Batches contains the transaction batches referenced by the
	// certificate, in the certificate's canonical payload order. Batches
	// are executed in this order, so it must be identical on every
	// validator (#4054).
	Batches []*types.Batch
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

	// OnValidatorSetChange is called when the validator set or the network
	// definition version changes. The callback should update the consensus
	// committee, using the version as the committee epoch: the version is
	// part of executed state, so every node — including one that replays or
	// restores from a snapshot — derives the same epoch for the same
	// committee, which a locally incremented counter cannot guarantee.
	OnValidatorSetChange(callback func(validators []ValidatorInfo, version uint64))
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
