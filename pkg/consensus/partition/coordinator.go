// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package partition provides coordination between multiple DAG-BFT partitions.
// In the Accumulate architecture, each partition (DN and BVNs) runs its own
// DAG-BFT consensus, but they coordinate via anchoring.
package partition

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// PartitionType indicates the type of partition.
type PartitionType int

const (
	// PartitionTypeDN is the Directory Network partition.
	PartitionTypeDN PartitionType = iota
	// PartitionTypeBVN is a Block Validator Network partition.
	PartitionTypeBVN
)

// String returns a string representation of the partition type.
func (t PartitionType) String() string {
	switch t {
	case PartitionTypeDN:
		return "DN"
	case PartitionTypeBVN:
		return "BVN"
	default:
		return fmt.Sprintf("PartitionType(%d)", t)
	}
}

// Block represents a committed block with its hash.
type Block struct {
	// Index is the block height.
	Index uint64
	// Hash is the block hash (state root).
	Hash [32]byte
	// Round is the consensus round that committed this block.
	Round types.Round
	// Time is the block timestamp.
	Time time.Time
	// Certificate is the committed certificate.
	Certificate *types.Certificate
}

// CoordinatorConfig holds configuration for the PartitionCoordinator.
type CoordinatorConfig struct {
	// Partition is the name of this partition (e.g., "dn", "bvn0", "bvn1").
	Partition string
	// PartitionType indicates whether this is DN or BVN.
	PartitionType PartitionType
	// KeyPair is the validator's private key for signing anchors.
	KeyPair ed25519.PrivateKey
	// AnchorInterval is how often to send anchors (in blocks).
	// Defaults to 1 (anchor every block).
	AnchorInterval uint64
	// DNAddress is the address of the DN for BVNs to send anchors to.
	// Only required for BVN partitions.
	DNAddress string
}

// Dispatcher handles routing of synthetic transactions and anchors
// between partitions.
type Dispatcher interface {
	// SubmitAnchor submits an anchor to the target partition.
	SubmitAnchor(ctx context.Context, anchor *Anchor) error
	// SubmitSyntheticTx submits a synthetic transaction to the target partition.
	SubmitSyntheticTx(ctx context.Context, partition string, tx []byte) error
}

// Anchorer handles anchor creation and verification.
type Anchorer interface {
	// CreateAnchor creates an anchor for the given block.
	CreateAnchor(block *Block) (*Anchor, error)
	// VerifyAnchor verifies an anchor from another partition.
	VerifyAnchor(anchor *Anchor) error
}

// Coordinator manages coordination between DAG-BFT partitions.
// For DN: collects and orders anchors from BVNs.
// For BVN: sends anchors to DN and receives anchor receipts.
type Coordinator struct {
	config     CoordinatorConfig
	dispatcher Dispatcher
	anchorer   Anchorer

	mu              sync.RWMutex
	lastAnchoredIdx uint64
	pendingAnchors  map[string]*Anchor // partition -> anchor
	anchorReceipts  []*AnchorReceipt

	// Callbacks
	onAnchorReceived func(anchor *Anchor)
	onReceiptReceived func(receipt *AnchorReceipt)
}

// NewCoordinator creates a new partition coordinator.
func NewCoordinator(config CoordinatorConfig, dispatcher Dispatcher, anchorer Anchorer) (*Coordinator, error) {
	if config.Partition == "" {
		return nil, errors.New("partition name is required")
	}
	if len(config.KeyPair) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid key pair")
	}
	if config.AnchorInterval == 0 {
		config.AnchorInterval = 1
	}

	return &Coordinator{
		config:         config,
		dispatcher:     dispatcher,
		anchorer:       anchorer,
		pendingAnchors: make(map[string]*Anchor),
	}, nil
}

// OnBlockCommit is called when a block is committed by the local consensus.
// It triggers the appropriate anchoring behavior based on partition type.
func (c *Coordinator) OnBlockCommit(ctx context.Context, block *Block) error {
	if block == nil {
		return errors.New("block is nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we should anchor this block
	if block.Index%c.config.AnchorInterval != 0 {
		return nil
	}

	switch c.config.PartitionType {
	case PartitionTypeDN:
		// DN doesn't send anchors, it only collects them
		return nil
	case PartitionTypeBVN:
		// BVN sends anchors to DN
		return c.sendAnchorLocked(ctx, block)
	default:
		return fmt.Errorf("unknown partition type: %v", c.config.PartitionType)
	}
}

// sendAnchorLocked sends an anchor for the given block to the DN.
// Must be called with c.mu held.
func (c *Coordinator) sendAnchorLocked(ctx context.Context, block *Block) error {
	if c.dispatcher == nil {
		return errors.New("dispatcher is nil")
	}
	if c.anchorer == nil {
		return errors.New("anchorer is nil")
	}

	// Create the anchor
	anchor, err := c.anchorer.CreateAnchor(block)
	if err != nil {
		return fmt.Errorf("create anchor: %w", err)
	}

	// Sign the anchor
	pubKey := c.config.KeyPair.Public().(ed25519.PublicKey)
	anchor.Validator = pubKey
	anchor.Sign(c.config.KeyPair)

	slog.Debug("Sending anchor to DN",
		"partition", c.config.Partition,
		"blockIndex", block.Index,
		"blockHash", fmt.Sprintf("%x", block.Hash[:8]))

	// Submit to DN
	if err := c.dispatcher.SubmitAnchor(ctx, anchor); err != nil {
		return fmt.Errorf("submit anchor: %w", err)
	}

	c.lastAnchoredIdx = block.Index
	return nil
}

// ReceiveAnchor handles an incoming anchor from another partition.
// Only valid for DN partitions.
func (c *Coordinator) ReceiveAnchor(anchor *Anchor) error {
	if anchor == nil {
		return errors.New("anchor is nil")
	}

	if c.config.PartitionType != PartitionTypeDN {
		return errors.New("only DN can receive anchors from BVNs")
	}

	// Verify the anchor
	if c.anchorer != nil {
		if err := c.anchorer.VerifyAnchor(anchor); err != nil {
			return fmt.Errorf("verify anchor: %w", err)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Store the anchor
	c.pendingAnchors[anchor.SourcePartition] = anchor

	slog.Debug("Received anchor from BVN",
		"source", anchor.SourcePartition,
		"blockIndex", anchor.BlockIndex,
		"blockHash", fmt.Sprintf("%x", anchor.BlockHash[:8]))

	// Call callback if registered
	if c.onAnchorReceived != nil {
		c.onAnchorReceived(anchor)
	}

	return nil
}

// ProcessPendingAnchors processes all pending anchors and creates receipts.
// Called by DN when committing a block that includes anchors.
func (c *Coordinator) ProcessPendingAnchors(dnBlockIndex uint64, dnBlockHash [32]byte) ([]*AnchorReceipt, error) {
	if c.config.PartitionType != PartitionTypeDN {
		return nil, errors.New("only DN can process anchors")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.pendingAnchors) == 0 {
		return nil, nil
	}

	receipts := make([]*AnchorReceipt, 0, len(c.pendingAnchors))
	for partition, anchor := range c.pendingAnchors {
		receipt := &AnchorReceipt{
			SourcePartition: partition,
			AnchorBlockIdx:  anchor.BlockIndex,
			AnchorBlockHash: anchor.BlockHash,
			DNBlockIndex:    dnBlockIndex,
			DNBlockHash:     dnBlockHash,
			Timestamp:       time.Now(),
		}
		receipts = append(receipts, receipt)
	}

	// Clear pending anchors
	c.pendingAnchors = make(map[string]*Anchor)
	c.anchorReceipts = append(c.anchorReceipts, receipts...)

	return receipts, nil
}

// ReceiveReceipt handles an incoming anchor receipt from the DN.
// Only valid for BVN partitions.
func (c *Coordinator) ReceiveReceipt(receipt *AnchorReceipt) error {
	if receipt == nil {
		return errors.New("receipt is nil")
	}

	if c.config.PartitionType != PartitionTypeBVN {
		return errors.New("only BVN can receive receipts from DN")
	}

	if receipt.SourcePartition != c.config.Partition {
		return fmt.Errorf("receipt is for partition %s, not %s", receipt.SourcePartition, c.config.Partition)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	slog.Debug("Received anchor receipt from DN",
		"partition", c.config.Partition,
		"anchorBlockIdx", receipt.AnchorBlockIdx,
		"dnBlockIdx", receipt.DNBlockIndex)

	// Store receipt
	c.anchorReceipts = append(c.anchorReceipts, receipt)

	// Call callback if registered
	if c.onReceiptReceived != nil {
		c.onReceiptReceived(receipt)
	}

	return nil
}

// SubmitSyntheticTx routes a synthetic transaction to the appropriate partition.
func (c *Coordinator) SubmitSyntheticTx(ctx context.Context, targetPartition string, tx []byte) error {
	if c.dispatcher == nil {
		return errors.New("dispatcher is nil")
	}
	return c.dispatcher.SubmitSyntheticTx(ctx, targetPartition, tx)
}

// OnAnchorReceived registers a callback for when anchors are received (DN only).
func (c *Coordinator) OnAnchorReceived(callback func(anchor *Anchor)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onAnchorReceived = callback
}

// OnReceiptReceived registers a callback for when receipts are received (BVN only).
func (c *Coordinator) OnReceiptReceived(callback func(receipt *AnchorReceipt)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onReceiptReceived = callback
}

// PendingAnchors returns the current pending anchors (DN only).
func (c *Coordinator) PendingAnchors() map[string]*Anchor {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*Anchor, len(c.pendingAnchors))
	for k, v := range c.pendingAnchors {
		result[k] = v
	}
	return result
}

// AnchorReceipts returns all received anchor receipts.
func (c *Coordinator) AnchorReceipts() []*AnchorReceipt {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*AnchorReceipt, len(c.anchorReceipts))
	copy(result, c.anchorReceipts)
	return result
}

// LastAnchoredIndex returns the last block index that was anchored.
func (c *Coordinator) LastAnchoredIndex() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastAnchoredIdx
}

// Partition returns the partition name.
func (c *Coordinator) Partition() string {
	return c.config.Partition
}

// PartitionType returns the partition type.
func (c *Coordinator) Type() PartitionType {
	return c.config.PartitionType
}
