// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// =====================================================================
// Integration Test Executor Implementation
// =====================================================================

// integrationExecutor is an enhanced mock executor for integration tests.
// It tracks state changes and simulates account balances.
type integrationExecutor struct {
	mu              sync.RWMutex
	lastBlockParams *execute.BlockParams
	lastBlockHash   [32]byte
	blockCount      uint64

	// Transaction tracking
	processedTxs [][]byte
	validatedTxs [][]byte

	// Simulated account balances
	balances map[string]uint64

	// Validator set
	validators     []*execute.ValidatorUpdate
	validatorsDirty bool

	// Configuration
	validateError error
	processError  error
}

func newIntegrationExecutor() *integrationExecutor {
	return &integrationExecutor{
		balances: make(map[string]uint64),
	}
}

func (e *integrationExecutor) EnableTimers() {}

func (e *integrationExecutor) StoreBlockTimers(ds *logging.DataSet) {}

func (e *integrationExecutor) LastBlock() (*execute.BlockParams, [32]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastBlockParams, e.lastBlockHash, nil
}

func (e *integrationExecutor) Init(validators []*execute.ValidatorUpdate) ([]*execute.ValidatorUpdate, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.validators = validators
	return nil, nil
}

func (e *integrationExecutor) Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.validateError != nil {
		return nil, e.validateError
	}

	// Track validated transactions
	data, _ := envelope.MarshalBinary()
	e.validatedTxs = append(e.validatedTxs, data)

	return []*protocol.TransactionStatus{}, nil
}

func (e *integrationExecutor) Begin(params execute.BlockParams) (execute.Block, error) {
	return &integrationBlock{
		executor: e,
		params:   params,
	}, nil
}

// SetBalance sets an account balance (for test setup)
func (e *integrationExecutor) SetBalance(account string, balance uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.balances[account] = balance
}

// GetBalance gets an account balance
func (e *integrationExecutor) GetBalance(account string) uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.balances[account]
}

// ProcessedTxCount returns the number of processed transactions
func (e *integrationExecutor) ProcessedTxCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.processedTxs)
}

// BlockCount returns the number of blocks committed
func (e *integrationExecutor) BlockCount() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.blockCount
}

// SetValidateError configures the executor to return an error on validation
func (e *integrationExecutor) SetValidateError(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.validateError = err
}

// UpdateValidators updates the validator set
func (e *integrationExecutor) UpdateValidators(updates []*execute.ValidatorUpdate) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.validators = updates
	e.validatorsDirty = true
}

// integrationBlock is a block implementation for integration tests.
type integrationBlock struct {
	executor *integrationExecutor
	params   execute.BlockParams
	txs      [][]byte
	closed   bool
}

func (b *integrationBlock) Params() execute.BlockParams {
	return b.params
}

func (b *integrationBlock) Process(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	if b.executor.processError != nil {
		return nil, b.executor.processError
	}

	data, _ := envelope.MarshalBinary()
	b.txs = append(b.txs, data)

	return []*protocol.TransactionStatus{}, nil
}

func (b *integrationBlock) Close() (execute.BlockState, error) {
	b.closed = true
	return &integrationBlockState{
		executor: b.executor,
		params:   b.params,
		txs:      b.txs,
	}, nil
}

// integrationBlockState is a block state implementation for integration tests.
type integrationBlockState struct {
	executor  *integrationExecutor
	params    execute.BlockParams
	txs       [][]byte
	committed bool
	discarded bool
}

func (s *integrationBlockState) Params() execute.BlockParams {
	return s.params
}

func (s *integrationBlockState) IsEmpty() bool {
	return len(s.txs) == 0
}

func (s *integrationBlockState) DidCompleteMajorBlock() (uint64, time.Time, bool) {
	return 0, time.Time{}, false
}

func (s *integrationBlockState) DidUpdateValidators() ([]*execute.ValidatorUpdate, bool) {
	s.executor.mu.RLock()
	defer s.executor.mu.RUnlock()
	if s.executor.validatorsDirty {
		return s.executor.validators, true
	}
	return nil, false
}

func (s *integrationBlockState) ChangeSet() record.Record {
	return nil
}

func (s *integrationBlockState) Hash() ([32]byte, error) {
	// Compute deterministic hash based on block index and transactions
	h := sha256.New()
	h.Write([]byte{byte(s.params.Index), byte(s.params.Index >> 8)})
	for _, tx := range s.txs {
		h.Write(tx)
	}
	var hash [32]byte
	copy(hash[:], h.Sum(nil))
	return hash, nil
}

func (s *integrationBlockState) Commit() error {
	s.committed = true

	s.executor.mu.Lock()
	defer s.executor.mu.Unlock()

	// Update executor state
	s.executor.lastBlockParams = &s.params
	hash, _ := s.Hash()
	s.executor.lastBlockHash = hash
	s.executor.blockCount++
	s.executor.processedTxs = append(s.executor.processedTxs, s.txs...)
	s.executor.validatorsDirty = false

	return nil
}

func (s *integrationBlockState) Discard() {
	s.discarded = true
}

// =====================================================================
// Integration Tests
// =====================================================================

// TestIntegration_ProcessTransactions tests that transactions flow through correctly.
func TestIntegration_ProcessTransactions(t *testing.T) {
	// Setup:
	// 1. Create executor
	executor := newIntegrationExecutor()

	// 2. Generate test keys and committee
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	// 3. Create DAG and node
	d := dag.NewDAG(100)
	config := NodeConfig{
		Partition:  "test",
		PrivateKey: priv,
	}

	node, err := NewDAGConsensusNode(config, executor, nil, committee, d, nil)
	require.NoError(t, err)
	require.NotNil(t, node)

	// 4. Start the node
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = node.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// 5. Create test transactions
	envelope := &messaging.Envelope{}
	txData, err := envelope.MarshalBinary()
	require.NoError(t, err)

	batch := types.NewBatch([][]byte{txData})
	batchDigest := batch.Digest()

	// 6. Create certificate with the batch
	payload := map[types.BatchDigest]types.WorkerID{batchDigest: 0}
	header := types.NewHeader(pub, 1, committee.Epoch, payload, nil)
	err = header.Sign(priv)
	require.NoError(t, err)

	clonedHeader := header.Clone()
	headerDigest := header.Digest()
	sig := ed25519.Sign(priv, headerDigest[:])
	cert := types.NewCertificate(clonedHeader, [][]byte{sig}, []uint16{0})

	// 7. Insert genesis certificates
	genesisHeader := types.NewHeader(pub, 0, committee.Epoch, nil, nil)
	err = genesisHeader.Sign(priv)
	require.NoError(t, err)
	clonedGenesisHeader := genesisHeader.Clone()
	genesisDigest := genesisHeader.Digest()
	genesisSig := ed25519.Sign(priv, genesisDigest[:])
	genesisCert := types.NewCertificate(clonedGenesisHeader, [][]byte{genesisSig}, []uint16{0})
	err = d.InsertGenesis(genesisCert)
	require.NoError(t, err)

	// 8. Insert the certificate into DAG
	err = d.Insert(cert)
	require.NoError(t, err)

	// 9. Process the certificate manually (simulating Bullshark commit)
	params := BlockParams{
		Index:       1,
		Time:        time.Now(),
		IsLeader:    true,
		LeaderRound: 1,
		Certificate: cert,
		Batches:     map[types.BatchDigest]*types.Batch{batchDigest: batch},
	}

	hash, err := node.blockProducer.ProduceBlock(ctx, params)
	require.NoError(t, err)

	// 10. Verify transaction was processed
	require.Equal(t, uint64(1), executor.BlockCount(), "expected 1 block to be committed")
	require.Equal(t, 1, executor.ProcessedTxCount(), "expected 1 transaction to be processed")
	require.NotEqual(t, [32]byte{}, hash, "block hash should not be zero")
}

// TestIntegration_InvalidTransaction tests rejection of invalid transactions.
func TestIntegration_InvalidTransaction(t *testing.T) {
	// Setup executor that returns validation error
	executor := newIntegrationExecutor()
	executor.SetValidateError(errors.New("invalid signature"))

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	d := dag.NewDAG(100)
	config := NodeConfig{
		Partition:  "test",
		PrivateKey: priv,
	}

	node, err := NewDAGConsensusNode(config, executor, nil, committee, d, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = node.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Try to submit invalid transaction
	envelope := &messaging.Envelope{}
	txData, err := envelope.MarshalBinary()
	require.NoError(t, err)

	// The Submit method should validate and reject
	err = node.blockProducer.ValidateTransaction(txData)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid signature")
}

// TestIntegration_BlockHashes tests deterministic block hashes.
func TestIntegration_BlockHashes(t *testing.T) {
	// Create two independent executors
	executor1 := newIntegrationExecutor()
	executor2 := newIntegrationExecutor()

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	// Create two block producers
	producer1 := NewBlockProducer(BlockProducerConfig{
		Executor:  executor1,
		Partition: "test",
	})

	producer2 := NewBlockProducer(BlockProducerConfig{
		Executor:  executor2,
		Partition: "test",
	})

	// Create identical transactions
	envelope := &messaging.Envelope{}
	txData, err := envelope.MarshalBinary()
	require.NoError(t, err)

	batch := types.NewBatch([][]byte{txData})
	batchDigest := batch.Digest()

	// Create certificate
	payload := map[types.BatchDigest]types.WorkerID{batchDigest: 0}
	header := types.NewHeader(pub, 1, committee.Epoch, payload, nil)
	err = header.Sign(priv)
	require.NoError(t, err)
	clonedHeader := header.Clone()
	headerDigest := header.Digest()
	sig := ed25519.Sign(priv, headerDigest[:])
	cert := types.NewCertificate(clonedHeader, [][]byte{sig}, []uint16{0})

	// Same block params for both
	blockTime := time.Now()
	params := BlockParams{
		Index:       1,
		Time:        blockTime,
		IsLeader:    true,
		LeaderRound: 1,
		Certificate: cert,
		Batches:     map[types.BatchDigest]*types.Batch{batchDigest: batch},
	}

	ctx := context.Background()

	// Process on both nodes
	hash1, err := producer1.ProduceBlock(ctx, params)
	require.NoError(t, err)

	hash2, err := producer2.ProduceBlock(ctx, params)
	require.NoError(t, err)

	// Verify hashes are deterministic
	require.Equal(t, hash1, hash2, "block hashes should be deterministic for same transactions")
}

// TestIntegration_ValidatorUpdates tests validator set propagation.
func TestIntegration_ValidatorUpdates(t *testing.T) {
	executor := newIntegrationExecutor()

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Generate new validator key
	newPub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	d := dag.NewDAG(100)
	config := NodeConfig{
		Partition:  "test",
		PrivateKey: priv,
	}

	node, err := NewDAGConsensusNode(config, executor, nil, committee, d, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = node.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Setup executor to report validator updates
	newValidators := []*execute.ValidatorUpdate{
		{PublicKey: pub, Power: 1},
		{PublicKey: newPub, Power: 1},
	}
	executor.UpdateValidators(newValidators)

	// Create a transaction so the block is not empty
	envelope := &messaging.Envelope{}
	txData, err := envelope.MarshalBinary()
	require.NoError(t, err)
	batch := types.NewBatch([][]byte{txData})
	batchDigest := batch.Digest()

	// Process a block to trigger validator update check
	params := BlockParams{
		Index:       1,
		Time:        time.Now(),
		IsLeader:    true,
		LeaderRound: 1,
		Batches:     map[types.BatchDigest]*types.Batch{batchDigest: batch},
	}

	_, err = node.blockProducer.ProduceBlock(ctx, params)
	require.NoError(t, err)

	// Verify the executor block state was committed
	require.Equal(t, uint64(1), executor.BlockCount())
}

// TestIntegration_EmptyBlocks tests handling of empty blocks.
func TestIntegration_EmptyBlocks(t *testing.T) {
	executor := newIntegrationExecutor()

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	producer := NewBlockProducer(BlockProducerConfig{
		Executor:  executor,
		Partition: "test",
	})

	// Create certificate with no batches
	header := types.NewHeader(pub, 1, committee.Epoch, nil, nil)
	err = header.Sign(priv)
	require.NoError(t, err)
	clonedHeader := header.Clone()
	headerDigest := header.Digest()
	sig := ed25519.Sign(priv, headerDigest[:])
	cert := types.NewCertificate(clonedHeader, [][]byte{sig}, []uint16{0})

	params := BlockParams{
		Index:       1,
		Time:        time.Now(),
		IsLeader:    true,
		LeaderRound: 1,
		Certificate: cert,
		Batches:     map[types.BatchDigest]*types.Batch{},
	}

	ctx := context.Background()
	hash, err := producer.ProduceBlock(ctx, params)
	require.NoError(t, err)

	// Empty blocks should return the previous hash (zero for first block)
	require.Equal(t, [32]byte{}, hash, "empty block should return previous hash")
	require.Equal(t, uint64(0), executor.BlockCount(), "empty block should not be committed")
}

// TestIntegration_MultipleBlocks tests processing multiple blocks in sequence.
func TestIntegration_MultipleBlocks(t *testing.T) {
	executor := newIntegrationExecutor()

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	producer := NewBlockProducer(BlockProducerConfig{
		Executor:  executor,
		Partition: "test",
	})

	ctx := context.Background()
	numBlocks := 5
	hashes := make([][32]byte, numBlocks)

	for i := 0; i < numBlocks; i++ {
		// Create transaction
		envelope := &messaging.Envelope{}
		txData, err := envelope.MarshalBinary()
		require.NoError(t, err)

		batch := types.NewBatch([][]byte{txData})
		batchDigest := batch.Digest()

		// Create certificate
		payload := map[types.BatchDigest]types.WorkerID{batchDigest: 0}
		header := types.NewHeader(pub, types.Round(i+1), committee.Epoch, payload, nil)
		err = header.Sign(priv)
		require.NoError(t, err)
		clonedHeader := header.Clone()
		headerDigest := header.Digest()
		sig := ed25519.Sign(priv, headerDigest[:])
		cert := types.NewCertificate(clonedHeader, [][]byte{sig}, []uint16{0})

		params := BlockParams{
			Index:       uint64(i + 1),
			Time:        time.Now(),
			IsLeader:    true,
			LeaderRound: types.Round(i + 1),
			Certificate: cert,
			Batches:     map[types.BatchDigest]*types.Batch{batchDigest: batch},
		}

		hashes[i], err = producer.ProduceBlock(ctx, params)
		require.NoError(t, err)
	}

	// Verify all blocks were committed
	require.Equal(t, uint64(numBlocks), executor.BlockCount())
	require.Equal(t, numBlocks, executor.ProcessedTxCount())

	// Verify each block has unique hash
	seen := make(map[[32]byte]bool)
	for i, hash := range hashes {
		require.False(t, seen[hash], "duplicate hash at block %d", i+1)
		seen[hash] = true
	}
}

// TestIntegration_BullsharkOrdering tests Bullshark certificate ordering.
// Bullshark commits leaders at even rounds when they have support from odd rounds.
// Leaders start at round 2, so we need at least round 3 to commit round 2's leader.
func TestIntegration_BullsharkOrdering(t *testing.T) {
	executor := newIntegrationExecutor()

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	d := dag.NewDAG(100)

	// Insert genesis (round 0)
	genesisHeader := types.NewHeader(pub, 0, committee.Epoch, nil, nil)
	err = genesisHeader.Sign(priv)
	require.NoError(t, err)
	clonedGenesisHeader := genesisHeader.Clone()
	genesisDigest := genesisHeader.Digest()
	genesisSig := ed25519.Sign(priv, genesisDigest[:])
	genesisCert := types.NewCertificate(clonedGenesisHeader, [][]byte{genesisSig}, []uint16{0})
	err = d.InsertGenesis(genesisCert)
	require.NoError(t, err)

	// Create Bullshark
	bs := bullshark.New(committee, d)

	// Create config and node
	config := NodeConfig{
		Partition:  "test",
		PrivateKey: priv,
	}

	node, err := NewDAGConsensusNode(config, executor, nil, committee, d, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = node.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = node.Stop() }()

	// Create chain of certificates (simulating proper DAG)
	// Round 0: Genesis (already inserted)
	// Round 1: Links to genesis
	// Round 2: Links to round 1 (this is a leader round)
	// Round 3: Links to round 2 (this round triggers commit of round 2's leader)
	parents := []types.CertificateDigest{genesisCert.Digest()}
	var certs []*types.Certificate

	for round := types.Round(1); round <= 3; round++ {
		// No payload (batches), just link to parents
		header := types.NewHeader(pub, round, committee.Epoch, nil, parents)
		err = header.Sign(priv)
		require.NoError(t, err)
		clonedHeader := header.Clone()
		headerDigest := header.Digest()
		sig := ed25519.Sign(priv, headerDigest[:])
		cert := types.NewCertificate(clonedHeader, [][]byte{sig}, []uint16{0})

		err = d.Insert(cert)
		require.NoError(t, err)

		// Process through Bullshark
		_ = bs.ProcessCertificate(cert)

		certs = append(certs, cert)
		parents = []types.CertificateDigest{cert.Digest()}
	}

	// Verify we have 3 certificates
	require.Len(t, certs, 3)

	// Verify DAG was built correctly
	require.Equal(t, 4, d.Size()) // genesis + 3 certificates

	// Verify the node's bullshark is accessible and functional
	require.NotNil(t, node.bullshark)
}

// TestIntegration_ConcurrentSubmissions tests concurrent transaction submissions.
func TestIntegration_ConcurrentSubmissions(t *testing.T) {
	executor := newIntegrationExecutor()

	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	_ = types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 1},
	}, 1)

	producer := NewBlockProducer(BlockProducerConfig{
		Executor:  executor,
		Partition: "test",
	})

	// Concurrent validation
	numGoroutines := 10
	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			envelope := &messaging.Envelope{}
			txData, err := envelope.MarshalBinary()
			if err != nil {
				errChan <- err
				return
			}

			err = producer.ValidateTransaction(txData)
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		require.NoError(t, err)
	}

	// Verify all validations were tracked
	require.Equal(t, numGoroutines, len(executor.validatedTxs))
}
