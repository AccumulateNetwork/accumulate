// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"context"
	"time"
)

// Executor is the interface DAG-BFT uses to execute blocks.
// This provides a clean abstraction between consensus and execution layers.
type Executor interface {
	// BeginBlock starts a new block.
	BeginBlock(ctx context.Context, params BeginBlockParams) error

	// ExecuteTransaction executes a single transaction.
	ExecuteTransaction(ctx context.Context, tx []byte) (*TxResult, error)

	// EndBlock finalizes the block and returns state hash.
	EndBlock(ctx context.Context) (stateHash [32]byte, err error)

	// Commit persists the block to storage.
	Commit(ctx context.Context) error

	// StateHash returns the current state hash.
	StateHash() [32]byte
}

// BeginBlockParams contains parameters for starting a new block.
type BeginBlockParams struct {
	// Index is the block height/index.
	Index uint64

	// Time is the block timestamp.
	Time time.Time

	// IsLeader indicates if this node was the leader for this block.
	IsLeader bool
}

// TxResult contains the result of executing a single transaction.
type TxResult struct {
	// Success indicates if the transaction executed successfully.
	Success bool

	// Error contains any error that occurred during execution.
	Error error

	// Events contains events emitted during execution.
	Events []Event
}

// Event represents an event emitted during transaction execution.
type Event struct {
	// Type is the event type.
	Type string

	// Attributes contains event key-value pairs.
	Attributes map[string]string
}
