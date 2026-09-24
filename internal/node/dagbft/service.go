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
	stderrors "errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/metrics"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
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
	Logger logging.Logger

	// DataDir is where the service keeps its consensus checkpoint, the
	// position it resumes from after a restart (#4238). Empty keeps none.
	DataDir string

	// Genesis is the path to the genesis file/snapshot.
	Genesis string

	// Database is the store the executor commits to, where a join puts the
	// state it pulls. The handoff reads the partition's system ledger from
	// it: the block the pulled state is and the leader round that committed
	// that block (#4362). Required.
	Database *database.Database

	// Host is the libp2p host for networking (optional, enables multi-node).
	Host host.Host

	// PubSub is the GossipSub instance for certificate/batch dissemination (optional).
	PubSub *pubsub.PubSub

	// InitialValidators provides the initial validator set for committee initialization.
	// This is needed because the WillChangeGlobals event that populates the adapter's
	// validators fires before the adapter is created.
	InitialValidators []adapter.ValidatorInfo

	// InitialNetworkVersion is the network definition version the initial
	// validators were read from. It becomes the initial committee epoch —
	// the epoch tracks the version so that every node, including one
	// restoring from a snapshot, derives the same epoch from state.
	InitialNetworkVersion uint64
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

	// Block production state. lastLeaderRound is the leader round of the
	// group lastBlockIndex was produced from: what a join's handoff measures
	// the pulled state's round against (#4362).
	lastBlockIndex  uint64
	lastBlockTime   time.Time
	lastLeaderRound types.Round

	// Consensus checkpoints: the position for the block about to be produced
	// and the one before it, so whichever block the executor actually holds
	// after a crash has a matching position (#4238).
	checkpoint     *persist.Store
	prevCheckpoint *persist.Store
	lastSaved      *persist.Checkpoint

	// Liveness watchdog. lastBlockAt is the local wall clock; lastBlockTime
	// carries the certificate author's timestamp, which is executed state and
	// is clamped to be strictly increasing, so it cannot be compared against
	// the local clock to decide whether this node is still making progress.
	// startedAt lets the watchdog fire for a partition that has never produced
	// a block at all — the case the previous check silently skipped.
	startedAt    time.Time
	lastBlockAt  time.Time
	stallSince   time.Time
	lastStallLog time.Time

	// Joining (#4292): while collecting, every committed group is kept in
	// the buffer instead of executed, and nothing here
	// advances the block index. See collect.go.
	collecting    bool
	buffer        []*CollectedGroup
	bufferBytes   int
	bufferOverrun bool
	// collectedThrough is the highest leader round of any group that reached
	// the buffer since collecting started, kept or refused. lostThrough is
	// set from it when the buffer starts again after an overrun: the groups
	// at or below it are in no buffer, so a state below it cannot be handed
	// off at (#4407).
	collectedThrough types.Round
	lostThrough      types.Round
	// handoff and stageThrough carry the join's requests; the block
	// production loop serves both, because it is the only thing that
	// produces blocks and the only thing that writes the buffer (#4294).
	handoff      chan handoffRequest
	stageThrough chan stageRequest
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
	if config.Database == nil {
		// Without it a join could never hand off, and a node that restarts
		// joins (#4205): required here rather than discovered at the handoff.
		return nil, errors.BadRequest.With("database is required")
	}

	s := &Service{
		config:           config,
		adapter:          config.Adapter,
		eventBus:         config.EventBus,
		stateHashTracker: types.NewStateHashTracker(100), // Track last 100 rounds
		handoff:          make(chan handoffRequest, 1),
		stageThrough:     make(chan stageRequest, 1),
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
	s.startedAt = time.Now()
	s.mu.Unlock()

	// Initialize committee from genesis
	committee, err := s.initializeCommittee()
	if err != nil {
		return errors.UnknownError.WithFormat("initialize committee: %w", err)
	}

	// The committee epoch is the network definition version, which is part
	// of executed state — a rejoining node derives it from its restored
	// globals (InitialNetworkVersion).
	s.committee = committee

	// Create consensus node with optional libp2p networking
	nodeConfig := s.config.NodeConfig
	// Set up pre-batch transaction validation using the adapter
	// This is equivalent to CometBFT's CheckTx
	nodeConfig.WorkerConfig.Validator = s.adapter
	s.node, err = consensus.NewNode(nodeConfig, committee, s.config.Host, s.config.PubSub)
	if err != nil {
		return errors.UnknownError.WithFormat("create consensus node: %w", err)
	}

	// Initialize genesis if needed
	if err := s.initializeGenesis(); err != nil {
		return errors.UnknownError.WithFormat("initialize genesis: %w", err)
	}

	// Resume the consensus position the executor's state was produced at
	s.seedFromCheckpoint()

	// Start consensus node
	if err := s.node.Start(s.ctx); err != nil {
		return errors.UnknownError.WithFormat("start consensus node: %w", err)
	}

	// Register for validator set changes from the adapter
	s.adapter.OnValidatorSetChange(s.onValidatorSetChange)

	// Start block production loop
	s.wg.Add(1)
	go s.blockProductionLoop()

	// The watchdog runs on its own goroutine, NOT as another case in the block
	// production select. Producing a block blocks until every batch named by
	// the certificate has been collected, so a partition that wedges waiting
	// for a batch wedges that whole loop — and a watchdog sharing it would go
	// silent at exactly the moment it is supposed to speak up.
	s.wg.Add(1)
	go s.livenessLoop()

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
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.node
}

// Committee returns the current committee.
func (s *Service) Committee() *types.Committee {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.committee
}

// CurrentRound returns the current consensus round.
func (s *Service) CurrentRound() types.Round {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.node == nil {
		return 0
	}
	return s.node.CurrentRound()
}

// LastCommitRound returns the last committed leader round.
func (s *Service) LastCommitRound() types.Round {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.node == nil {
		return 0
	}
	return s.node.LastCommitRound()
}

// SubmitTransaction submits a transaction to the consensus node, spreading
// keyless traffic across workers.
//
// This used to call SubmitTransactionFor(""), which routes by hashing the key
// — and the hash of an empty key is a constant. Since this is the ONLY path
// the submitter API uses, every transaction on every node landed in the same
// worker (worker 1 for the 2 and 4 worker counts we ship). That worker held
// all of the node's own uncommitted batches inside its 1/N share of the
// partition's byte budget — 8 MB of 32 MB at NumWorkers=4 — while the other
// workers held budget they could never fill. Own batches cannot be evicted, so
// the moment commit lagged, worker 1 evicted PEERS' batches instead, which are
// what certificates need; they were refetched, commit lagged further, and the
// network collapsed 15 minutes into a 500 tx/s soak (#4179).
func (s *Service) SubmitTransaction(tx []byte) error {
	s.mu.RLock()
	node := s.node
	s.mu.RUnlock()
	if node == nil {
		return errors.BadRequest.With("node not started")
	}
	return node.SubmitTransaction(tx)
}

// SubmitTransactionFor submits on behalf of a named sender, so everything from
// one signer is handled by one worker and keeps its execution order. Without
// it, replay protection rejects all but an increasing subsequence of a
// signer's transactions — 96 of 100, silently (#4132).
// SubmitUserTransaction is SubmitTransaction for a user's transaction: it may
// be refused with worker.ErrStoreFull while the store is full.
func (s *Service) SubmitUserTransaction(tx []byte) error {
	s.mu.RLock()
	node := s.node
	s.mu.RUnlock()
	if node == nil {
		return errors.BadRequest.With("node not started")
	}
	return node.SubmitUserTransaction(tx)
}

func (s *Service) SubmitTransactionFor(key string, tx []byte) error {
	s.mu.RLock()
	node := s.node
	s.mu.RUnlock()
	if node == nil {
		return errors.BadRequest.With("node not started")
	}
	return node.SubmitTransactionFor(key, tx)
}

// initializeCommittee creates the initial committee from genesis.
func (s *Service) initializeCommittee() (*types.Committee, error) {
	// Try to get validators from multiple sources in order of preference:
	// 1. InitialValidators from config (set at startup from globals)
	// 2. Adapter validators (populated via WillChangeGlobals event)
	// 3. Fallback to single-validator with our own key

	var validators []types.ValidatorInfo

	// First, try initial validators from config
	if len(s.config.InitialValidators) > 0 {
		validators = make([]types.ValidatorInfo, len(s.config.InitialValidators))
		for i, v := range s.config.InitialValidators {
			validators[i] = types.ValidatorInfo{
				PublicKey: v.PublicKey[:],
				Stake:     v.Stake,
			}
		}
		s.logger.Info("Using initial validators from config",
			"count", len(validators))
	}

	// Second, check adapter validators
	if len(validators) == 0 {
		adapterValidators := s.adapter.Validators()
		if len(adapterValidators) > 0 {
			validators = make([]types.ValidatorInfo, len(adapterValidators))
			for i, v := range adapterValidators {
				validators[i] = types.ValidatorInfo{
					PublicKey: v.PublicKey[:],
					Stake:     v.Stake,
				}
			}
			s.logger.Info("Using validators from adapter",
				"count", len(validators))
		}
	}

	// Fallback: create single-validator committee with our own key
	if len(validators) == 0 {
		pubKey := s.config.NodeConfig.KeyPair.Public().(ed25519.PublicKey)
		validators = []types.ValidatorInfo{
			{
				PublicKey: pubKey,
				Stake:     1,
			},
		}
		slog.Warn("No validators found, using single-validator fallback",
			"partition", s.config.Partition.ID)
	}

	committee := types.NewCommittee(validators, s.config.InitialNetworkVersion)
	return committee, nil
}

// The checkpoint files under DataDir.
const (
	checkpointFile     = "consensus-checkpoint.json"
	prevCheckpointFile = "consensus-checkpoint.prev.json"
)

// saveCheckpoint writes the node's consensus position for blockIndex, keeping
// the previous block's position as well. A failure to write is reported, not
// fatal: the next block writes again.
func (s *Service) saveCheckpoint(blockIndex uint64) {
	if s.config.DataDir == "" {
		return
	}
	if s.checkpoint == nil {
		s.checkpoint = persist.NewStore(s.config.DataDir)
		s.checkpoint.SetFilename(checkpointFile)
		s.prevCheckpoint = persist.NewStore(s.config.DataDir)
		s.prevCheckpoint.SetFilename(prevCheckpointFile)
	}
	cp := s.node.Checkpoint()
	cp.BlockIndex = blockIndex
	if s.lastSaved != nil {
		if err := s.prevCheckpoint.Save(s.lastSaved); err != nil {
			s.logger.Error("Saving previous consensus checkpoint", "error", err, "block", s.lastSaved.BlockIndex)
		}
	}
	if err := s.checkpoint.Save(cp); err != nil {
		s.logger.Error("Saving consensus checkpoint", "error", err, "block", blockIndex)
		return
	}
	s.lastSaved = cp
}

// seedFromCheckpoint restores the consensus position that produced the
// executor's last block, so a restarted validator rejoins at the live round
// instead of round zero (#4238). Of the two checkpoints kept, the one whose
// block is the executor's last block is the position to resume; a node with
// state but no matching checkpoint starts at round zero and cannot catch a
// live network (DIFFERENCES E11).
func (s *Service) seedFromCheckpoint() {
	if s.config.DataDir == "" || s.lastBlockIndex == 0 {
		return
	}
	for _, file := range []string{checkpointFile, prevCheckpointFile} {
		store := persist.NewStore(s.config.DataDir)
		store.SetFilename(file)
		cp, err := store.Load()
		if err != nil {
			continue
		}
		if cp.BlockIndex != s.lastBlockIndex || cp.Partition != s.config.Partition.ID {
			continue
		}
		s.node.Restore(cp)
		s.lastSaved = cp
		// Consensus resumes after this round, so the groups it commits from
		// here are the ones after it; a join's handoff at a state below it
		// would need groups this node will not be delivered (#4362).
		s.lastLeaderRound = cp.LastCommitRound
		return
	}
	slog.Warn("No consensus checkpoint matches the executor's last block; starting at round zero",
		"partition", s.config.Partition.ID, "lastBlock", s.lastBlockIndex)
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

	// Only insert genesis certificates in single-validator mode.
	// In multi-validator mode, genesis certificates need to be created with
	// signatures from all validators, which requires either:
	// 1. Pre-computed genesis certs embedded in the genesis snapshot
	// 2. Gossip-based genesis cert synchronization
	// For now, we skip genesis insertion in multi-validator mode and rely on
	// the Primary to create round 1 certificates that reference empty parents.
	// The Primary's getParentCertsForRound() has been updated to allow this.
	if s.committee.Len() == 1 {
		keys := []ed25519.PrivateKey{s.config.NodeConfig.KeyPair}
		if err := s.node.InsertGenesisForAll(keys); err != nil {
			return fmt.Errorf("insert genesis certificates: %w", err)
		}
		s.logger.Info("Inserted genesis certificates for single-validator mode")
	} else {
		s.logger.Info("Multi-validator mode: skipping local genesis insertion",
			"validators", s.committee.Len())
	}

	return nil
}

// A partition is reported stalled once no block has been produced for
// blockStallThreshold, and re-reported every blockStallRepeat while it stays
// stalled. The report is an error, not an info line: a partition that stops
// producing blocks is the failure operators are watching for, and the previous
// check logged it at info level with a "WARNING:" string glued to the message,
// which no level filter and no `grep -w ERROR` would ever surface. It also ran
// on the loop's 100ms ticker with no rate limit, so a real stall emitted ten
// identical lines per second and drowned the log it was supposed to flag.
const (
	blockStallThreshold = 10 * time.Second
	blockStallRepeat    = 10 * time.Second
)

// blockProductionLoop processes committed certificates and produces blocks.
func (s *Service) blockProductionLoop() {
	defer s.wg.Done()

	committed := s.node.Committed()

	for {
		select {
		case <-s.ctx.Done():
			return

		case req := <-s.stageThrough:
			// The join has matched the root: take the buffer into staging
			// through the block after the state, here, where the buffer is
			// written (#4294, #4398).
			req.done <- s.stageThroughNow(req.block)

		case req := <-s.handoff:
			// The join has matched the root and settled staging: leave
			// collecting mode and produce what was buffered, here, where
			// nothing else is producing blocks (#4294).
			err := s.performHandoff(req.q)
			if err != nil {
				s.logger.Error("Handoff failed; the join syncs again",
					"partition", s.config.Partition.ID, "block", req.q, "error", err)
			}
			req.done <- err

		case group, ok := <-committed:
			if !ok {
				return
			}
			if len(group) == 0 {
				continue
			}

			cert, err := s.processCommittedGroup(group)
			if err == nil && !s.Collecting() {
				// A joining node has executed nothing, and must read as
				// lagging: its primary proposes no batches it could not
				// execute until the handoff (consensus spec, invariant 9;
				// #4292, #4294).
				s.node.ReportExecuted()
			}
			if err != nil {
				// A committed certificate whose batches are gone from the whole
				// network (#4159) cannot be executed and MUST NOT be skipped —
				// skipping diverges this node's state. Halt cleanly so the node
				// stops here and can recover by state-sync, rather than the loop
				// logging an error and moving to the next group (which would
				// execute it out of order against missing predecessor state).
				if stderrors.Is(err, consensus.ErrBatchesUnrecoverable) {
					s.haltForUnrecoverableBatches(cert, err)
					return
				}
				slog.Error("Failed to process committed group",
					"error", err,
					"leaderRound", group[len(group)-1].Header.Round,
					"certs", len(group))
			}
		}
	}
}

// livenessLoop reports a stalled partition, independently of block production.
func (s *Service) livenessLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkBlockLiveness()
		}
	}
}

// noteBlockProduced records that a block landed and clears the stall watchdog.
// Callers must hold s.mu.
//
// A stall that ended is worth one line. Without it the log shows a partition
// going down and never coming back, and a reader cannot tell a blip that
// recovered from an outage that is still open.
func (s *Service) noteBlockProduced(blockIndex uint64, round types.Round) {
	s.lastBlockAt = time.Now()
	if s.stallSince.IsZero() {
		return
	}
	s.logger.Error("Partition resumed producing blocks",
		"partition", s.config.Partition.ID,
		"stalledFor", time.Since(s.stallSince).Round(time.Millisecond),
		"block", blockIndex,
		"round", round)
	s.stallSince = time.Time{}
	s.lastStallLog = time.Time{}
}

// checkBlockLiveness reports this partition as stalled when no block has been
// produced for blockStallThreshold.
//
// The reference point is the last block this node actually produced, or the
// time the service started if it has produced none. Measuring only from the
// last block means a partition that comes up and never commits anything looks
// identical to one that is healthy, because the "last block" timestamp stays
// zero and the check never arms — which is exactly how a Directory frozen at
// its startup height ran unremarked while its BVNs advanced past block 63000.
func (s *Service) checkBlockLiveness() {
	now := time.Now()

	s.mu.Lock()
	since := s.lastBlockAt
	produced := true
	if since.IsZero() {
		since, produced = s.startedAt, false
	}
	if since.IsZero() {
		s.mu.Unlock()
		return // not started yet
	}

	elapsed := now.Sub(since)
	if elapsed < blockStallThreshold {
		s.mu.Unlock()
		return
	}

	// Rate-limit: one line when the stall opens, then one per blockStallRepeat.
	if s.stallSince.IsZero() {
		s.stallSince = since
	} else if now.Sub(s.lastStallLog) < blockStallRepeat {
		s.mu.Unlock()
		return
	}
	s.lastStallLog = now
	lastIndex := s.lastBlockIndex
	s.mu.Unlock()

	args := []interface{}{
		"partition", s.config.Partition.ID,
		"stalledFor", elapsed.Round(time.Millisecond),
		"threshold", blockStallThreshold,
		"lastBlock", lastIndex,
		"round", s.CurrentRound(),
	}

	// A joining node produces no blocks on purpose: it is collecting them
	// while it pulls the state (#4292). That is not a stall, and reporting it
	// as one would put an error in the log for every join.
	if s.Collecting() {
		s.logger.Info("Joining: collecting committed blocks, executing none",
			append(args, "buffered", s.BufferedCount())...)
		return
	}
	if !produced {
		// Distinguish "stopped" from "never started": the round is the tell —
		// a partition whose consensus rounds climb while its block height
		// stays put is executing nothing, not partitioned away.
		args = append(args, "everProducedBlock", false)
	}
	s.logger.Error("Partition stalled: no block produced", args...)
}

// processCommittedGroup executes one committed LEADER's sub-DAG as ONE block.
//
// One block per certificate multiplied end-of-block cost — BPT recompute,
// leveldb commit, anchor — by the committee size: ~12 executor blocks per
// consensus round, 7.4 blocks/s at 10 tps, with leveldb compaction and anchor
// processing eating half the profile (#4164). The leader is the deterministic
// unit of commitment, so it is the block boundary; the group arrives in
// canonical order (round ascending, author ascending, leader last) and every
// validator sees identical groups.
//
// Returns the certificate to blame when the error is ErrBatchesUnrecoverable.
func (s *Service) processCommittedGroup(group []*types.Certificate) (*types.Certificate, error) {
	s.mu.Lock()

	// Check if halted due to state divergence
	if s.halted {
		s.mu.Unlock()
		return nil, fmt.Errorf("consensus halted due to state divergence: %w", s.haltReason)
	}

	s.mu.Unlock()

	// The leader is last in canonical order: it is the unique maximum round
	// of its own ancestor set.
	leader := group[len(group)-1]

	// Collect every certificate's batches, outside the lock. Each payload
	// slice is in canonical order and batches are executed in payload order —
	// identical on every validator (#4054). This BLOCKS until every batch is
	// available (fetching from peers when needed): executing a certificate
	// without some of its batches silently diverges this node's state from
	// every node that had them — six nodes at the same block index produced
	// six different state hashes before this waited (#4116/#4119). The
	// certificate is proof the data exists; waiting costs liveness only.
	//
	// A certificate delivered twice is not a failure: this node executed it
	// already and its batches were retired on purpose (#4125). Skip it and
	// execute the rest of the group.
	var executedCerts []*types.Certificate
	var batches []*types.Batch
	payloadEntries := 0
	for _, cert := range group {
		certBatches, err := s.node.CollectBatches(s.ctx, cert)
		if err != nil {
			if stderrors.Is(err, consensus.ErrAlreadyExecuted) {
				slog.Debug("Ignoring re-delivered certificate",
					"round", cert.Header.Round,
					"partition", s.config.Partition.ID)
				continue
			}
			return cert, fmt.Errorf("collect batches: %w", err)
		}
		batches = append(batches, certBatches...)
		payloadEntries += len(cert.Header.Payload)
		executedCerts = append(executedCerts, cert)
	}
	if len(executedCerts) == 0 {
		// The whole group was executed before (a redelivery after restart);
		// its block already exists.
		return nil, nil
	}

	// Check if this validator is the leader of this commit
	pubKey := s.config.NodeConfig.KeyPair.Public().(ed25519.PublicKey)
	isLeader := types.ValidatorsEqual(leader.Header.Author, pubKey)

	// A joining node collects this group into staging and keeps it; it
	// executes nothing until the join says which block its state is
	// (executor spec, "Sync", steps 1 and 4; #4292).
	if s.Collecting() {
		err := s.collectGroup(executedCerts, batches, leader, isLeader)
		if err != nil {
			return leader, err
		}
		// The batches are kept in the buffer, so the workers may retire
		// theirs: a joining node that held its workers' copies as well would
		// pay for every block twice.
		s.pruneCommitted(executedCerts, 0)
		return nil, nil
	}

	err := s.produceGroup(executedCerts, batches, leader, isLeader, payloadEntries)
	if err != nil {
		return leader, err
	}
	return nil, nil
}

// produceGroup produces one block from a committed group whose batches are in
// hand: the block production half of processCommittedGroup, called there and
// again by the handoff, which produces the groups a joining node buffered
// (#4294). Every node must produce a group the same way whichever path it
// arrived by.
func (s *Service) produceGroup(certs []*types.Certificate, batches []*types.Batch, leader *types.Certificate, isLeader bool, payloadEntries int) error {
	return s.produce(certs, batches, leader, isLeader, payloadEntries, true)
}

// produce is produceGroup with a say over the checkpoint. A buffered group is
// produced with the consensus position as it is NOW, which is ahead of the
// block being produced — the node collected while consensus ran on — so
// writing a checkpoint for it would pair a block with a position that commits
// certificates the executor has not executed. The handoff writes none; the
// first live block after it writes one that is true (#4238, #4294).
func (s *Service) produce(certs []*types.Certificate, batches []*types.Batch, leader *types.Certificate, isLeader bool, payloadEntries int, checkpoint bool) error {
	s.mu.Lock()
	if s.halted {
		s.mu.Unlock()
		return fmt.Errorf("consensus halted due to state divergence: %w", s.haltReason)
	}
	blockIndex := s.lastBlockIndex + 1
	lastTime := s.lastBlockTime
	s.mu.Unlock()

	// The block time MUST be derived from the certificate, not the local
	// clock: block time is part of executed state, so if each validator
	// stamps its own wall clock the state trees diverge on the very first
	// block and cross-partition anchors never gather a signature quorum —
	// each validator signs a different version of the "same" anchor (#4054).
	// The LEADER's header timestamp is the same on every validator, covered
	// by the header signature; clamp it to be strictly increasing so a bad
	// clock cannot move time backwards.
	blockTime := time.Unix(0, leader.Header.Timestamp).UTC()
	if !blockTime.After(lastTime) {
		blockTime = lastTime.Add(time.Millisecond)
	}

	params := adapter.BlockParams{
		Index:       blockIndex,
		Time:        blockTime,
		IsLeader:    isLeader,
		LeaderRound: leader.Header.Round,
		Certificate: leader,
		Batches:     batches,
	}

	// Record the consensus position this block is produced at, before it is
	// produced: a crash on either side of ProduceBlock leaves a checkpoint
	// that matches the executor's last block (#4238).
	if checkpoint {
		s.saveCheckpoint(blockIndex)
	}

	hash, err := s.adapter.ProduceBlock(s.ctx, params)
	if err != nil {
		return fmt.Errorf("produce block: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Update state
	s.lastBlockIndex = blockIndex
	s.lastBlockTime = blockTime
	s.lastLeaderRound = leader.Header.Round

	s.noteBlockProduced(blockIndex, leader.Header.Round)
	// Separate "producing blocks" from "producing blocks with something in
	// them": an idle network commits empty rounds forever, which reads as a
	// stall to anything watching the ledger index and as health to anything
	// watching block production.
	metrics.BlocksProducedTotal.WithLabelValues(s.config.Partition.ID).Inc()
	if payloadEntries == 0 {
		metrics.BlocksEmptyTotal.WithLabelValues(s.config.Partition.ID).Inc()
	}

	// Record state hash for consistency verification. Every certificate in
	// the group carries the block's resulting hash — the group IS the block.
	stateHash := s.adapter.StateHash()
	for _, cert := range certs {
		cert.SetStateHash(types.StateHash(stateHash))
	}
	s.RecordStateHash(leader.Header.Round, blockIndex, types.StateHash(stateHash))

	s.pruneCommitted(certs, blockIndex)

	// Emit block event.
	//
	// Major must be carried: subscribers key on it to distinguish a major
	// block from an ordinary one. HaltController.OnDidCommitBlock returns
	// early unless Major is non-zero, so while this was omitted a halt
	// requested through the API was recorded, reported as pending, and never
	// acted on (#4097).
	event := events.DidCommitBlock{
		Index: blockIndex,
		Time:  blockTime,
		Round: uint64(leader.Header.Round),
		Epoch: s.committee.Epoch,
	}
	if major, _, ok := s.adapter.LastMajorBlock(); ok {
		event.Major = major
	}
	if err := s.eventBus.Publish(event); err != nil {
		s.logger.Error("Failed to publish block event", "error", err)
	}

	s.logger.Debug("Produced block",
		"index", blockIndex,
		"leaderRound", leader.Header.Round,
		"certs", len(certs),
		"hash", fmt.Sprintf("%x", hash[:8]),
		"stateHash", fmt.Sprintf("%x", stateHash[:8]),
		"batches", len(batches))

	return nil
}

// Status returns the current status of the DAG-BFT service.
type Status struct {
	Running         bool
	Partition       string
	CurrentRound    types.Round
	LastCommitRound types.Round
	LastBlockIndex  uint64
	LastBlockTime   time.Time
	ValidatorCount  int
	TxSubmitted     uint64
	CertsCommitted  uint64
	// State verification status
	StateHalted     bool
	StateHaltReason string
}

// Status returns the current status of the service.
func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := Status{
		Running:        s.running && !s.stopping,
		Partition:      s.config.Partition.ID,
		LastBlockIndex: s.lastBlockIndex,
		LastBlockTime:  s.lastBlockTime,
		StateHalted:    s.halted,
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

// onValidatorSetChange is called when the adapter detects a validator set
// change (or a network definition version bump). It updates the consensus
// node's committee to reflect the new validator set.
//
// The committee epoch IS the network definition version. The version is part
// of executed state, so a node that replays these blocks later — or restores
// state from a snapshot — derives exactly the same epoch at exactly the same
// block as every node that executed them live. A locally incremented counter
// (the previous scheme) only exists in the memory of nodes that were running
// at the time, which is precisely what a joining node is not.
func (s *Service) onValidatorSetChange(validators []adapter.ValidatorInfo, version uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.node == nil {
		slog.Warn("Validator set change received but node not started")
		return
	}

	if version <= s.committee.Epoch {
		// Replayed or out-of-order notification — the committee is already at
		// or past this version
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

	newEpoch := version
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

// haltForUnrecoverableBatches halts the partition when a committed
// certificate's batches are gone from the whole network (#4159). Like state
// divergence, this is a safety stop: the certificate cannot be executed and
// must not be skipped, so the node stops cleanly here and recovers by
// state-sync rather than spinning forever inside batch collection.
func (s *Service) haltForUnrecoverableBatches(cert *types.Certificate, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.halted {
		return
	}
	s.halted = true
	s.haltReason = err

	slog.Error("UNRECOVERABLE BATCHES - HALTING PARTITION (state-sync to recover)",
		"partition", s.config.Partition.ID,
		"round", cert.Header.Round,
		"cert", cert.Digest().String()[:16],
		"error", err)
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
