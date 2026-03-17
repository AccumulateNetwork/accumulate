// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package dagbft provides a node service wrapper for DAG-BFT consensus.
// It integrates the DAG-based consensus with the accumulated binary.
package dagbft

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ServiceConfig holds the configuration for a DAG-BFT service.
type ServiceConfig struct {
	// Partition is the network partition info.
	Partition *protocol.PartitionInfo

	// NodeConfig is the consensus node configuration.
	NodeConfig consensus.NodeConfig

	// Adapter bridges consensus to the executor.
	Adapter adapter.ConsensusAdapter

	// EventBus is the event bus for publishing events.
	EventBus *events.Bus

	// Logger is the logger to use.
	Logger log.Logger

	// Genesis is the path to the genesis file/snapshot.
	Genesis string
}

// Service wraps the DAG-BFT consensus node for integration with accumulated.
type Service struct {
	config    ServiceConfig
	node      *consensus.Node
	adapter   adapter.ConsensusAdapter
	eventBus  *events.Bus
	logger    logging.OptionalLogger
	committee *types.Committee

	// Lifecycle management
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.RWMutex
	running  bool
	stopping bool

	// Block production state
	lastBlockIndex uint64
	lastBlockTime  time.Time

	// Validator synchronization
	validatorUpdateHeight uint64 // Height at which validator update was detected

	// State hash verification
	stateHashTracker *types.StateHashTracker
	halted           bool
	haltReason       error
}

// NewService creates a new DAG-BFT service.
func NewService(config ServiceConfig) (*Service, error) {
	if config.Partition == nil {
		return nil, errors.BadRequest.With("partition is required")
	}
	if config.Adapter == nil {
		return nil, errors.BadRequest.With("adapter is required")
	}
	if config.EventBus == nil {
		return nil, errors.BadRequest.With("event bus is required")
	}

	s := &Service{
		config:           config,
		adapter:          config.Adapter,
		eventBus:         config.EventBus,
		stateHashTracker: types.NewStateHashTracker(100), // Track last 100 rounds
	}
	s.logger.L = config.Logger

	// Register divergence callback
	s.stateHashTracker.OnDivergence(s.onStateDivergence)

	return s, nil
}

// Start initializes and starts the DAG-BFT service.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.BadRequest.With("service already running")
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true
	s.mu.Unlock()

	// Initialize committee from genesis
	committee, err := s.initializeCommittee()
	if err != nil {
		return errors.UnknownError.WithFormat("initialize committee: %w", err)
	}
	s.committee = committee

	// Create consensus node (without libp2p for now - can be added later)
	nodeConfig := s.config.NodeConfig
	// Set up pre-batch transaction validation using the adapter
	// This is equivalent to CometBFT's CheckTx
	nodeConfig.WorkerConfig.Validator = s.adapter
	s.node, err = consensus.NewNode(nodeConfig, committee, nil, nil)
	if err != nil {
		return errors.UnknownError.WithFormat("create consensus node: %w", err)
	}

	// Initialize genesis if needed
	if err := s.initializeGenesis(); err != nil {
		return errors.UnknownError.WithFormat("initialize genesis: %w", err)
	}

	// Start consensus node
	if err := s.node.Start(s.ctx); err != nil {
		return errors.UnknownError.WithFormat("start consensus node: %w", err)
	}

	// Register for validator set changes from the adapter
	s.adapter.OnValidatorSetChange(s.onValidatorSetChange)

	// Start block production loop
	s.wg.Add(1)
	go s.blockProductionLoop()

	s.logger.Info("DAG-BFT service started",
		"partition", s.config.Partition.ID,
		"validators", len(committee.Validators))

	return nil
}

// Stop gracefully stops the DAG-BFT service.
func (s *Service) Stop() error {
	s.mu.Lock()
	if !s.running || s.stopping {
		s.mu.Unlock()
		return nil
	}
	s.stopping = true
	s.mu.Unlock()

	s.logger.Info("Stopping DAG-BFT service", "partition", s.config.Partition.ID)

	// Cancel context
	if s.cancel != nil {
		s.cancel()
	}

	// Stop consensus node
	if s.node != nil {
		s.node.Stop()
	}

	// Wait for goroutines
	s.wg.Wait()

	s.mu.Lock()
	s.running = false
	s.stopping = false
	s.mu.Unlock()

	s.logger.Info("DAG-BFT service stopped", "partition", s.config.Partition.ID)
	return nil
}

// IsRunning returns whether the service is running.
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running && !s.stopping
}

// Node returns the underlying consensus node.
func (s *Service) Node() *consensus.Node {
	return s.node
}

// Committee returns the current committee.
func (s *Service) Committee() *types.Committee {
	return s.committee
}

// CurrentRound returns the current consensus round.
func (s *Service) CurrentRound() types.Round {
	if s.node == nil {
		return 0
	}
	return s.node.CurrentRound()
}

// LastCommitRound returns the last committed leader round.
func (s *Service) LastCommitRound() types.Round {
	if s.node == nil {
		return 0
	}
	return s.node.LastCommitRound()
}

// SubmitTransaction submits a transaction to the consensus node.
func (s *Service) SubmitTransaction(tx []byte) error {
	if s.node == nil {
		return errors.BadRequest.With("node not started")
	}
	return s.node.SubmitTransaction(tx)
}

// initializeCommittee creates the initial committee from genesis.
func (s *Service) initializeCommittee() (*types.Committee, error) {
	// For now, create a single-validator committee using our own key
	pubKey := s.config.NodeConfig.KeyPair.Public().(ed25519.PublicKey)

	validators := []types.ValidatorInfo{
		{
			PublicKey: pubKey,
			Stake:     1,
		},
	}

	// Check for validator info from adapter
	adapterValidators := s.adapter.Validators()
	if len(adapterValidators) > 0 {
		validators = make([]types.ValidatorInfo, len(adapterValidators))
		for i, v := range adapterValidators {
			validators[i] = types.ValidatorInfo{
				PublicKey: v.PublicKey[:],
				Stake:     v.Stake,
			}
		}
	}

	committee := types.NewCommittee(validators, 0)
	return committee, nil
}

// initializeGenesis initializes the DAG with genesis certificates.
func (s *Service) initializeGenesis() error {
	// Check if we have genesis state already
	lastIndex, _, err := s.adapter.LastBlock()
	if err != nil {
		return err
	}

	if lastIndex > 0 {
		// Already have state, skip genesis initialization
		s.lastBlockIndex = lastIndex
		return nil
	}

	// Load genesis doc if provided
	if s.config.Genesis != "" {
		if _, err := os.Stat(s.config.Genesis); err == nil {
			s.logger.Info("Loading genesis", "path", s.config.Genesis)
			// Genesis loading would happen here - for now we just note it
			// The actual genesis initialization happens via the executor
		}
	}

	// For a single-validator setup, insert genesis certificates
	keys := []ed25519.PrivateKey{s.config.NodeConfig.KeyPair}
	if err := s.node.InsertGenesisForAll(keys); err != nil {
		return fmt.Errorf("insert genesis certificates: %w", err)
	}

	return nil
}

// blockProductionLoop processes committed certificates and produces blocks.
func (s *Service) blockProductionLoop() {
	defer s.wg.Done()

	committed := s.node.Committed()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return

		case cert, ok := <-committed:
			if !ok {
				return
			}
			if cert == nil {
				continue
			}

			if err := s.processCommittedCertificate(cert); err != nil {
				slog.Error("Failed to process committed certificate",
					"error", err,
					"round", cert.Header.Round)
			}

		case <-ticker.C:
			// Periodic check - could be used for timeouts or health checks
		}
	}
}

// processCommittedCertificate processes a committed certificate and produces a block.
func (s *Service) processCommittedCertificate(cert *types.Certificate) error {
	s.mu.Lock()

	// Check if halted due to state divergence
	if s.halted {
		s.mu.Unlock()
		return fmt.Errorf("consensus halted due to state divergence: %w", s.haltReason)
	}

	// Get batches from workers - the Payload map contains batch digests
	batches := make(map[types.BatchDigest]*types.Batch)
	committedDigests := make([]types.BatchDigest, 0)
	for digest := range cert.Header.Payload {
		for _, w := range s.node.Workers() {
			batch, err := w.GetBatch(digest)
			if err == nil && batch != nil {
				batches[digest] = batch
				committedDigests = append(committedDigests, digest)
				break
			}
		}
	}

	// Check if this validator is the leader
	pubKey := s.config.NodeConfig.KeyPair.Public().(ed25519.PublicKey)
	isLeader := types.ValidatorsEqual(cert.Header.Author, pubKey)

	// Produce block
	blockIndex := s.lastBlockIndex + 1
	blockTime := time.Now()

	params := adapter.BlockParams{
		Index:       blockIndex,
		Time:        blockTime,
		IsLeader:    isLeader,
		LeaderRound: cert.Header.Round,
		Certificate: cert,
		Batches:     batches,
	}

	s.mu.Unlock()

	hash, err := s.adapter.ProduceBlock(s.ctx, params)
	if err != nil {
		return fmt.Errorf("produce block: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update state
	s.lastBlockIndex = blockIndex
	s.lastBlockTime = blockTime

	// Record state hash for consistency verification
	stateHash := s.adapter.StateHash()
	cert.SetStateHash(types.StateHash(stateHash))
	s.RecordStateHash(cert.Header.Round, blockIndex, types.StateHash(stateHash))

	// Prune batches from workers now that they've been processed
	for _, w := range s.node.Workers() {
		w.PruneBatches(committedDigests)
	}

	// Emit block event
	event := events.DidCommitBlock{
		Index: blockIndex,
		Time:  blockTime,
	}
	if err := s.eventBus.Publish(event); err != nil {
		s.logger.Error("Failed to publish block event", "error", err)
	}

	s.logger.Debug("Produced block",
		"index", blockIndex,
		"round", cert.Header.Round,
		"hash", fmt.Sprintf("%x", hash[:8]),
		"stateHash", fmt.Sprintf("%x", stateHash[:8]),
		"batches", len(batches))

	return nil
}

// Status returns the current status of the DAG-BFT service.
type Status struct {
	Running          bool
	Partition        string
	CurrentRound     types.Round
	LastCommitRound  types.Round
	LastBlockIndex   uint64
	LastBlockTime    time.Time
	ValidatorCount   int
	TxSubmitted      uint64
	CertsCommitted   uint64
	// State verification status
	StateHalted      bool
	StateHaltReason  string
}

// Status returns the current status of the service.
func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := Status{
		Running:         s.running && !s.stopping,
		Partition:       s.config.Partition.ID,
		LastBlockIndex:  s.lastBlockIndex,
		LastBlockTime:   s.lastBlockTime,
		StateHalted:     s.halted,
	}

	if s.haltReason != nil {
		status.StateHaltReason = s.haltReason.Error()
	}

	if s.node != nil {
		status.CurrentRound = s.node.CurrentRound()
		status.LastCommitRound = s.node.LastCommitRound()
		status.TxSubmitted, status.CertsCommitted = s.node.Metrics()
	}

	if s.committee != nil {
		status.ValidatorCount = s.committee.Len()
	}

	return status
}

// For genesis loading - use existing infrastructure
var _ = genesis.DocProvider

// onValidatorSetChange is called when the adapter detects a validator set change.
// It updates the consensus node's committee to reflect the new validator set.
func (s *Service) onValidatorSetChange(validators []adapter.ValidatorInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.node == nil {
		slog.Warn("Validator set change received but node not started")
		return
	}

	// Convert adapter validators to committee validators
	committeeValidators := make([]types.ValidatorInfo, len(validators))
	for i, v := range validators {
		committeeValidators[i] = types.ValidatorInfo{
			PublicKey: v.PublicKey[:],
			Stake:     v.Stake,
		}
	}

	// Create new committee with incremented epoch
	newEpoch := s.committee.Epoch + 1
	newCommittee := types.NewCommittee(committeeValidators, newEpoch)

	slog.Info("Validator set changed, updating committee",
		"partition", s.config.Partition.ID,
		"oldEpoch", s.committee.Epoch,
		"newEpoch", newEpoch,
		"oldValidators", s.committee.Len(),
		"newValidators", len(validators),
		"atHeight", s.lastBlockIndex)

	// Track when the update was detected
	s.validatorUpdateHeight = s.lastBlockIndex

	// Update the local committee reference
	s.committee = newCommittee

	// Propagate to consensus node (updates Primary and Bullshark)
	s.node.UpdateCommittee(newCommittee)
}

// UpdateCommittee allows external callers to update the committee.
// This can be used during testing or for manual committee updates.
func (s *Service) UpdateCommittee(committee *types.Committee) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.node == nil {
		slog.Warn("UpdateCommittee called but node not started")
		return
	}

	slog.Info("Updating committee",
		"partition", s.config.Partition.ID,
		"oldEpoch", s.committee.Epoch,
		"newEpoch", committee.Epoch,
		"validators", committee.Len())

	s.committee = committee
	s.node.UpdateCommittee(committee)
}

// ValidatorUpdateHeight returns the block height at which the last validator update was detected.
func (s *Service) ValidatorUpdateHeight() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validatorUpdateHeight
}

// onStateDivergence is called when state divergence is detected.
// It halts the service to prevent further state corruption.
func (s *Service) onStateDivergence(err *types.StateDivergenceError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.halted {
		return // Already halted
	}

	s.halted = true
	s.haltReason = err

	slog.Error("STATE DIVERGENCE DETECTED - HALTING CONSENSUS",
		"round", err.Round,
		"blockIndex", err.BlockIndex,
		"expectedHash", err.ExpectedHash.String(),
		"actualHash", err.ActualHash.String(),
		"conflictAuthor", fmt.Sprintf("%x", err.ConflictAuthor[:8]))

	// Emit divergence event
	if s.eventBus != nil {
		event := events.StateDivergenceDetected{
			Round:        uint64(err.Round),
			BlockIndex:   err.BlockIndex,
			ExpectedHash: err.ExpectedHash,
			ActualHash:   err.ActualHash,
		}
		if pubErr := s.eventBus.Publish(event); pubErr != nil {
			s.logger.Error("Failed to publish state divergence event", "error", pubErr)
		}
	}
}

// IsHalted returns whether the service has been halted due to state divergence.
func (s *Service) IsHalted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.halted
}

// HaltReason returns the reason for halting, if halted.
func (s *Service) HaltReason() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.haltReason
}

// RecordStateHash records the local state hash for a round after block execution.
// This is called by the block production loop after successfully producing a block.
func (s *Service) RecordStateHash(round types.Round, blockIndex uint64, stateHash types.StateHash) {
	if s.stateHashTracker == nil {
		return
	}

	s.stateHashTracker.RecordLocalHash(round, stateHash)

	// Periodically prune old state hashes
	s.stateHashTracker.Prune(round)
}

// VerifyRemoteStateHash verifies a state hash received from another validator.
// Returns an error if divergence is detected.
func (s *Service) VerifyRemoteStateHash(msg *types.StateHashMessage) error {
	if s.stateHashTracker == nil {
		return nil
	}

	// Verify signature
	if err := msg.Verify(); err != nil {
		return fmt.Errorf("invalid state hash message: %w", err)
	}

	// Record and check for divergence
	if divergence := s.stateHashTracker.RecordRemoteHash(msg.Round, msg.Author, msg.StateHash); divergence != nil {
		return divergence
	}

	return nil
}

// StateConsistencyStatus returns the current state consistency status.
type StateConsistencyStatus struct {
	// Halted indicates whether the service has been halted due to divergence.
	Halted bool
	// HaltReason contains the divergence error if halted.
	HaltReason error
	// LastVerifiedRound is the last round where state was verified consistent.
	LastVerifiedRound types.Round
	// TrackedRounds is the number of rounds being tracked.
	TrackedRounds int
}

// StateConsistency returns the current state consistency status.
func (s *Service) StateConsistency() StateConsistencyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := StateConsistencyStatus{
		Halted:     s.halted,
		HaltReason: s.haltReason,
	}

	// Find last verified round
	if s.stateHashTracker != nil {
		// This is a simplified check; a production implementation would track more
		currentRound := s.CurrentRound()
		for r := currentRound; r > 0 && r > currentRound-10; r-- {
			if s.stateHashTracker.CheckConsistency(r) {
				status.LastVerifiedRound = r
				break
			}
		}
	}

	return status
}

// RequestStateSync initiates a state sync to recover from state divergence.
// This should be called after the service has been halted due to divergence.
// The caller should provide peers to sync from.
func (s *Service) RequestStateSync(ctx context.Context, targetHeight uint64) error {
	s.mu.Lock()
	if !s.halted {
		s.mu.Unlock()
		return fmt.Errorf("cannot request state sync when not halted")
	}
	s.mu.Unlock()

	s.logger.Info("Initiating state sync for recovery",
		"targetHeight", targetHeight)

	// State sync would be performed via the snapshot.StateSync component
	// which handles discovering snapshots from peers, downloading, and restoring.
	// The actual sync is coordinated externally by the node operator or automation.

	return nil
}

// ResumeAfterSync resumes consensus after successful state sync.
// This resets the halted state and allows block production to continue.
func (s *Service) ResumeAfterSync(newHeight uint64, newStateHash types.StateHash) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.halted {
		return fmt.Errorf("service is not halted")
	}

	s.logger.Info("Resuming after state sync",
		"newHeight", newHeight,
		"newStateHash", newStateHash.String())

	// Reset halt state
	s.halted = false
	s.haltReason = nil

	// Update block index
	s.lastBlockIndex = newHeight

	// Record the new state hash
	if s.stateHashTracker != nil {
		s.stateHashTracker.RecordLocalHash(s.CurrentRound(), newStateHash)
	}

	return nil
}

// GetStateHashForRound returns the recorded state hash for a given round.
func (s *Service) GetStateHashForRound(round types.Round) (types.StateHash, bool) {
	if s.stateHashTracker == nil {
		return types.StateHash{}, false
	}
	return s.stateHashTracker.GetLocalHash(round)
}

// BroadcastStateHash creates and would broadcast a state hash message.
// This is called periodically to enable cross-validator state verification.
func (s *Service) BroadcastStateHash(round types.Round, blockIndex uint64, stateHash types.StateHash) (*types.StateHashMessage, error) {
	pubKey := s.config.NodeConfig.KeyPair.Public().(ed25519.PublicKey)

	msg := types.NewStateHashMessage(round, s.committee.Epoch, blockIndex, stateHash, pubKey)
	if err := msg.Sign(s.config.NodeConfig.KeyPair); err != nil {
		return nil, fmt.Errorf("failed to sign state hash message: %w", err)
	}

	// In a production implementation, this message would be gossiped to peers
	// via the gossip layer. For now, we return the message for the caller to handle.
	return msg, nil
}
