// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package crosschain contains interfaces for CrossChain Conductor integration.
package crosschain

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MessageSender is the interface that CCC uses to send list proofs to other partitions.
// This will be implemented by the protocol for deployment and by a simulator for testing.
type MessageSender interface {
	// SendListProof sends a list proof containing a batch of transactions to the destination.
	// The proof contains all transactions from the "top of chain" index to current chain top.
	SendListProof(ctx context.Context, destination string, proof *protocol.AnnotatedReceipt) error
}

// MessageReceiver is the interface that CCC uses to receive messages from other partitions.
// This will be implemented by the protocol for deployment and by a simulator for testing.
type MessageReceiver interface {
	// ReceiveMessage is called when a message arrives from another partition.
	// Returns the last received sequence number for gap healing.
	ReceiveMessage(ctx context.Context, source string, message *messaging.Envelope) (lastReceivedSeq uint64, err error)
	
	// RequestMissingMessages is called to request missing transactions from source partition.
	// The source should respond by sending a list proof starting from the given sequence number.
	RequestMissingMessages(ctx context.Context, source string, fromSequence uint64) error
}

// ChainProvider is the interface that CCC uses to access anchor and synthetic transaction chains.
// This will be implemented by the protocol for deployment and by a simulator for testing.
type ChainProvider interface {
	// GetAnchorChain returns the anchor transaction chain for building list proofs.
	GetAnchorChain(ctx context.Context) (Chain, error)
	
	// GetSyntheticChain returns the synthetic transaction chain for building list proofs.
	GetSyntheticChain(ctx context.Context) (Chain, error)
	
	// GetTopOfChainIndex returns the current "top of chain" index for the specified chain type.
	GetTopOfChainIndex(ctx context.Context, chainType ChainType) (uint64, error)
	
	// SetTopOfChainIndex updates the "top of chain" index after successful transmission.
	SetTopOfChainIndex(ctx context.Context, chainType ChainType, index uint64) error
}

// Chain represents a transaction chain that CCC can collect lists from.
type Chain interface {
	// CollectTransactionsFrom collects transactions starting from the given sequence number.
	CollectTransactionsFrom(startSeq uint64) ([]Transaction, error)
	
	// GetCurrentTop returns the current top sequence number of the chain.
	GetCurrentTop() (uint64, error)
}

// ChainType identifies the type of transaction chain.
type ChainType int

const (
	ChainTypeAnchor ChainType = iota
	ChainTypeSynthetic
)

// Transaction represents a transaction in a chain.
type Transaction interface {
	GetSequenceNumber() uint64
	GetHash() []byte
	GetData() []byte
}